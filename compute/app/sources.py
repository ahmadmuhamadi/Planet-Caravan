"""External data: SIMBAD (names), IRSA (SPHEREx images and cutouts) and Gaia (reference stars)."""

import hashlib
import io
import logging
import random
import time
from dataclasses import dataclass
from functools import lru_cache
from pathlib import Path

import astropy.units as u
import httpx
import numpy as np
from astropy.coordinates import Distance, SkyCoord
from astropy.io import fits
from astropy.table import Table
from astropy.time import Time
from astroquery.ipac.irsa import Irsa
from astroquery.simbad import Simbad

from app.config import settings

log = logging.getLogger(__name__)

# apply_space_motion needs a distance. Objects without a parallax get a far one, which
# leaves their motion to proper motion alone (a parallax of 0.1 mas is negligible here).
FAR_AWAY = 10 * u.kpc

_http = httpx.Client(timeout=settings.download_timeout_s, follow_redirects=True)


@dataclass(frozen=True)
class Exposure:
    """One SPHEREx spectral image covering the target, on one detector."""

    collection: str
    obs_id: str
    detector: int
    t_min: float  # MJD
    release_date: str  # ISO 8601, so string order is date order
    access_url: str


def detector_number(bandpass_name: str) -> int:
    """'SPHEREx-D3' -> 3."""
    return int(bandpass_name.rsplit("D", 1)[1])


def spectral_collections() -> list[str]:
    """SPHEREx collections with spectral images, discovered at runtime (D1)."""
    table = Irsa.list_collections(servicetype="SIA", filter="spherex")
    # *_cube is the all-sky spectral cube product, not exposures.
    return [c for c in table["collection"] if not c.endswith("_cube")]


def resolve(name: str) -> SkyCoord:
    """Position, proper motion and distance from SIMBAD, at epoch J2000."""
    simbad = Simbad()
    simbad.server = settings.simbad_server
    simbad.add_votable_fields("pmra", "pmdec", "plx_value")
    result = simbad.query_object(name)
    if result is None or len(result) == 0 or np.ma.is_masked(result["ra"][0]):
        raise LookupError(f"SIMBAD could not resolve {name!r}")
    row = result[0]

    def value(column: str) -> float:
        return 0.0 if np.ma.is_masked(row[column]) else float(row[column])

    parallax = value("plx_value")
    return SkyCoord(
        ra=row["ra"] * u.deg,
        dec=row["dec"] * u.deg,
        pm_ra_cosdec=value("pmra") * u.mas / u.yr,
        pm_dec=value("pmdec") * u.mas / u.yr,
        distance=Distance(parallax=parallax * u.mas) if parallax > 0 else FAR_AWAY,
        obstime=Time("J2000"),
    )


def position_at(target: SkyCoord, mjd: float) -> SkyCoord:
    """Where target is on date mjd, moved by its proper motion."""
    moved = target.apply_space_motion(new_obstime=Time(mjd, format="mjd"))
    return SkyCoord(moved.ra, moved.dec)


def query_exposures(target: SkyCoord, radius: u.Quantity) -> list[Exposure]:
    """All SPHEREx exposures covering target, oldest first. An image in several
    releases is kept once, from the newest release (D6)."""
    # SPHEREx observes from 2025 on; querying at 2026.0 keeps fast movers in the circle.
    centre = position_at(target, Time("2026-01-01").mjd)
    newest: dict[tuple[str, int], Exposure] = {}
    for collection in spectral_collections():
        for row in Irsa.query_sia(pos=(centre, radius), collection=collection):
            e = Exposure(
                collection=collection,
                obs_id=str(row["obs_id"]),
                detector=detector_number(str(row["energy_bandpassname"])),
                t_min=float(row["t_min"]),
                release_date=str(row["obs_release_date"]),
                access_url=str(row["access_url"]),
            )
            key = (e.obs_id, e.detector)
            if key not in newest or e.release_date > newest[key].release_date:
                newest[key] = e
    return sorted(newest.values(), key=lambda e: (e.t_min, e.detector))


def download(url: str, client: httpx.Client = _http) -> bytes:
    """GET with retries, exponential backoff and jitter (D4). A 4xx is not retried."""
    attempts = settings.download_attempts
    for attempt in range(1, attempts + 1):
        try:
            response = client.get(url)
            response.raise_for_status()
            return response.content
        except httpx.HTTPStatusError as e:
            if e.response.status_code < 500 or attempt == attempts:
                raise
        except httpx.TransportError:
            if attempt == attempts:
                raise
        time.sleep(2 ** (attempt - 1) + random.random())
    raise AssertionError("unreachable")


def cutout(url: str, centre: SkyCoord, size_arcmin: float) -> fits.HDUList:
    """A cutout around centre, never the full frame (D3), cached on disk (D5)."""
    query = f"center={centre.ra.deg:.6f},{centre.dec.deg:.6f}deg&size={size_arcmin}arcmin"
    full_url = f"{url}?{query}"
    path = Path(settings.cutout_cache) / (hashlib.sha256(full_url.encode()).hexdigest() + ".fits")
    if not path.exists():
        content = download(full_url)
        path.parent.mkdir(parents=True, exist_ok=True)
        partial = path.with_suffix(".part")
        partial.write_bytes(content)
        partial.rename(path)  # atomic, so a crash never leaves a broken cache entry
    return fits.open(io.BytesIO(path.read_bytes()))


def cutout_or_none(url: str, centre: SkyCoord, size_arcmin: float) -> fits.HDUList | None:
    """A cutout, or None when IRSA rejects this one image (4xx), so one bad image does
    not sink a whole target. Server and network errors still raise."""
    try:
        return cutout(url, centre, size_arcmin)
    except httpx.HTTPStatusError as e:
        if e.response.status_code >= 500:
            raise
        log.warning("skipped image (%d): %s", e.response.status_code, url)
        return None


@lru_cache(maxsize=32)
def gaia_stars(ra: float, dec: float, radius_deg: float, g_max: float = 16.0) -> Table:
    """Gaia DR3 stars brighter than g_max within radius_deg of (ra, dec), epoch 2016.0."""
    from astroquery.gaia import Gaia  # imported here: it contacts the archive on import

    query = f"""
        SELECT ra, dec, pmra, pmdec, parallax, phot_g_mean_mag FROM gaiadr3.gaia_source
        WHERE 1 = CONTAINS(POINT(ra, dec), CIRCLE({ra}, {dec}, {radius_deg}))
        AND phot_g_mean_mag < {g_max}"""
    return Gaia.launch_job(query).get_results()
