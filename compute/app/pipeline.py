"""Image processing: calibrate, measure, align and co-add. Pure functions, no network."""

from collections.abc import Iterable, Sequence
from dataclasses import dataclass

import astropy.units as u
import numpy as np
import numpy.typing as npt
from astropy.coordinates import SkyCoord
from astropy.io import fits
from astropy.table import Table
from astropy.time import Time
from astropy.wcs import WCS

from app.sources import Exposure

Array = npt.NDArray[np.float64]
Flags = npt.NDArray[np.integer]

GAIA_EPOCH = Time(2016.0, format="jyear")
MAS_PER_DEG = 3.6e6


def group_epochs(exposures: Iterable[Exposure], gap_days: float) -> list[list[Exposure]]:
    """Split exposures by date: a gap of more than gap_days starts a new epoch."""
    epochs: list[list[Exposure]] = []
    last: float | None = None
    for e in sorted(exposures, key=lambda e: e.t_min):
        if last is None or e.t_min - last > gap_days:
            epochs.append([])
        epochs[-1].append(e)
        last = e.t_min
    return epochs


def flag_bits(flags_header: fits.Header, names: Iterable[str]) -> int:
    """Bit mask for flag names, read from the FLAGS header (MSKNnnnn = name)."""
    wanted = set(names)
    mask = 0
    for key in flags_header["MSKN*"]:
        if flags_header[key] in wanted:
            mask |= 1 << int(key[4:])
    return mask


@dataclass
class Calibrated:
    image: Array  # zodi subtracted, masked pixels NaN
    variance: Array  # masked pixels NaN
    flags: Flags
    wcs: WCS  # celestial


def calibrate(hdul: fits.HDUList, mask_flags: Iterable[str]) -> Calibrated:
    """Subtract ZODI from IMAGE and blank the pixels with a masked flag bit (S1)."""
    image = hdul["IMAGE"].data.astype(float) - hdul["ZODI"].data
    variance = hdul["VARIANCE"].data.astype(float)
    flags = hdul["FLAGS"].data
    bad = (flags & flag_bits(hdul["FLAGS"].header, mask_flags)) != 0
    image[bad] = np.nan
    variance[bad] = np.nan
    return Calibrated(image, variance, flags, WCS(hdul["IMAGE"].header))


def wavelength_at(hdul: fits.HDUList, x: float, y: float) -> float:
    """Wavelength in um at pixel (x, y), from the WCS-WAVE lookup table (S4)."""
    spectral = WCS(header=hdul["IMAGE"].header, fobj=hdul, key="W")
    spectral.sip = None  # the distortion terms belong to the celestial WCS only
    wavelength, _bandwidth = spectral.pixel_to_world(x, y)
    return float(wavelength.to_value(u.um))


# Aperture geometry, in pixels.
APERTURE_RADIUS = 2.0
CORE_RADIUS = 1.5
SKY_ANNULUS = (4.0, 7.0)


@dataclass
class Photometry:
    snr: float
    saturated: bool  # a saturation flag in the core
    flagged: bool  # any masked flag in the core


def photometry(
    cal: Calibrated, x: float, y: float, saturation_mask: int, flag_mask: int
) -> Photometry | None:
    """Aperture photometry at (x, y) against a local sky annulus; None off the image."""
    ny, nx = cal.image.shape
    if not (0 <= x < nx and 0 <= y < ny):
        return None
    yy, xx = np.mgrid[:ny, :nx]
    distance = np.hypot(xx - x, yy - y)
    in_aperture = (distance <= APERTURE_RADIUS) & np.isfinite(cal.image)
    in_sky = (distance >= SKY_ANNULUS[0]) & (distance <= SKY_ANNULUS[1])
    core = distance <= CORE_RADIUS

    sky = np.nanmedian(cal.image[in_sky])
    flux = np.nansum(cal.image[in_aperture] - sky)
    error = np.sqrt(np.nansum(cal.variance[in_aperture]))
    return Photometry(
        snr=float(flux / error) if error > 0 else 0.0,
        saturated=bool((cal.flags[core] & saturation_mask).any()),
        flagged=bool((cal.flags[core] & flag_mask).any()),
    )


def tan_grid(
    centre: SkyCoord, fov_arcmin: float, pixel_scale_arcsec: float
) -> tuple[WCS, tuple[int, int]]:
    """A north-up TAN grid with centre on its middle pixel (S2)."""
    n = int(np.ceil(fov_arcmin * 60 / pixel_scale_arcsec)) | 1  # odd: centre is a pixel centre
    wcs = WCS(naxis=2)
    wcs.wcs.ctype = ["RA---TAN", "DEC--TAN"]
    wcs.wcs.crval = [centre.ra.deg, centre.dec.deg]
    wcs.wcs.crpix = [(n + 1) / 2, (n + 1) / 2]
    wcs.wcs.cdelt = [-pixel_scale_arcsec / 3600, pixel_scale_arcsec / 3600]  # east left
    return wcs, (n, n)


def coadd(stack: Sequence[tuple[Array, Array]]) -> Array:
    """Inverse-variance weighted mean of (image, variance) pairs; masked pixels left out."""
    images = np.array([image for image, _ in stack])
    with np.errstate(divide="ignore"):
        weights = 1 / np.array([variance for _, variance in stack])
    ok = np.isfinite(images) & np.isfinite(weights) & (weights > 0)
    weight_sum = np.where(ok, weights, 0).sum(axis=0)
    with np.errstate(invalid="ignore", divide="ignore"):
        mean: Array = np.where(ok, images * weights, 0).sum(axis=0) / weight_sum
    mean[weight_sum == 0] = np.nan
    return mean


def centroid(image: Array, x: float, y: float, half: int = 3) -> tuple[float, float] | None:
    """Background-subtracted first-moment centroid in a (2 half + 1) box around (x, y).
    None near the edge or when the box has a masked pixel."""
    xi, yi = round(x), round(y)
    ny, nx = image.shape
    if xi - half < 0 or yi - half < 0 or xi + half >= nx or yi + half >= ny:
        return None
    box = image[yi - half : yi + half + 1, xi - half : xi + half + 1]
    if not np.isfinite(box).all():
        return None
    background = np.median(image[max(0, yi - 8) : yi + 9, max(0, xi - 8) : xi + 9])
    weights = np.clip(box - background, 0, None)
    total = weights.sum()
    if total <= 0:
        return None
    dy, dx = np.mgrid[-half : half + 1, -half : half + 1]
    return float(xi + (dx * weights).sum() / total), float(yi + (dy * weights).sum() / total)


def gaia_positions_at(stars: Table, mjd: float) -> SkyCoord:
    """Gaia DR3 positions moved from epoch 2016.0 to mjd by proper motion.
    Stars without a proper motion stay put."""
    has_pm = ~(np.ma.getmaskarray(stars["pmra"]) | np.ma.getmaskarray(stars["pmdec"]))
    pmra = np.where(has_pm, np.ma.filled(stars["pmra"], 0), 0)  # mas/yr, includes cos(dec)
    pmdec = np.where(has_pm, np.ma.filled(stars["pmdec"], 0), 0)
    years = (Time(mjd, format="mjd") - GAIA_EPOCH).to_value(u.yr)
    dec = np.asarray(stars["dec"])
    ra = np.asarray(stars["ra"]) + pmra * years / MAS_PER_DEG / np.cos(np.radians(dec))
    return SkyCoord(ra * u.deg, (dec + pmdec * years / MAS_PER_DEG) * u.deg)


# Reference star selection for the alignment check, in pixels.
MIN_REFERENCE_STARS = 3
BLEND_RADIUS = 3.0  # another star this close spoils the centroid
MAX_OFFSET = 1.5  # larger offsets are mismatches, not misalignment
MIN_PEAK_SIGMA = 20.0


@dataclass
class Alignment:
    stars: int
    dx_arcsec: float | None = None  # median offset, measured minus Gaia
    dy_arcsec: float | None = None
    residual_arcsec: float | None = None  # length of the median offset; None: too few stars
    scatter_arcsec: float | None = None


def alignment(
    image: Array,
    grid: WCS,
    stars: SkyCoord,
    pixel_scale_arcsec: float,
    target_xy: tuple[float, float],
) -> Alignment:
    """Median offset between the centroids of isolated bright stars and their Gaia
    positions on grid (S3). The target itself is left out."""
    sx, sy = (np.asarray(v) for v in grid.world_to_pixel(stars))
    level = np.nanmedian(image)
    noise = 1.4826 * np.nanmedian(np.abs(image - level))  # MAD as a robust sigma
    offsets = []
    for x, y in zip(sx, sy, strict=True):
        if (np.hypot(sx - x, sy - y) < BLEND_RADIUS).sum() > 1:
            continue
        if np.hypot(x - target_xy[0], y - target_xy[1]) < BLEND_RADIUS:
            continue
        measured = centroid(image, x, y)
        if measured is None or image[round(y), round(x)] - level < MIN_PEAK_SIGMA * noise:
            continue
        dx, dy = measured[0] - x, measured[1] - y
        if np.hypot(dx, dy) < MAX_OFFSET:
            offsets.append((dx, dy))

    if len(offsets) < MIN_REFERENCE_STARS:
        return Alignment(stars=len(offsets))
    arcsec = np.array(offsets) * pixel_scale_arcsec
    dx, dy = float(np.median(arcsec[:, 0])), float(np.median(arcsec[:, 1]))
    return Alignment(
        stars=len(offsets),
        dx_arcsec=dx,
        dy_arcsec=dy,
        residual_arcsec=float(np.hypot(dx, dy)),
        scatter_arcsec=float(np.median(np.hypot(arcsec[:, 0] - dx, arcsec[:, 1] - dy))),
    )
