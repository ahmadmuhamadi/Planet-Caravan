"""Phase 0 gates (G1-G4). Standalone script, not service code.

Usage:  uv run python gates/gates.py g1
        uv run python gates/gates.py g2 [--out DIR]
        uv run python gates/gates.py g3 "Barnard's star" [--fov 6] [--out DIR]

Results are printed as Markdown and copied into docs/decisions.md by hand.
"""

from __future__ import annotations

import argparse
import hashlib
import io
import os
import random
import sys
import time
import warnings
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

import astropy.units as u
import httpx
import numpy as np
from astropy.coordinates import Distance, SkyCoord
from astropy.io import fits
from astropy.time import Time
from astropy.wcs import WCS, FITSFixedWarning
from astroquery.gaia import Gaia
from astroquery.ipac.irsa import Irsa
from astroquery.simbad import Simbad
from astroquery.simbad import conf as simbad_conf
from PIL import Image
from reproject import reproject_interp

warnings.simplefilter("ignore", FITSFixedWarning)
warnings.filterwarnings("ignore", message=".*SIP.*")

SIMBAD_SERVER = os.environ.get("SIMBAD_SERVER", "simbad.harvard.edu")
CACHE = Path(os.environ.get("CUTOUT_CACHE", Path(__file__).parent.parent / ".cache/cutouts"))
TARGETS = ["Barnard's star", "WISE J085510.83-071442.5", "eps Ind B", "RW Aur", "R Dra"]
EPOCH_GAP_DAYS = 30
PIXEL_SCALE = 6.2  # arcsec, output grid
ALIGN_THRESHOLD_ARCSEC = 1.2
DETECT_SNR = 5.0
MASK_FLAGS = {
    "TRANSIENT", "OVERFLOW", "SUR_ERROR", "NONFUNC", "MISSING_DATA", "HOT", "COLD",
    "NONLINEAR", "PERSIST", "OUTLIER",
}  # fmt: skip
SATURATION_FLAGS = {"OVERFLOW", "NONLINEAR"}
MAX_PARALLEL = 8

simbad_conf.server = SIMBAD_SERVER


# ---------- data access ----------


def collections() -> list[str]:
    t = Irsa.list_collections(servicetype="SIA", filter="spherex")
    # Spectral images only: *_cube is the all-sky cube product, not exposures.
    return [c for c in t["collection"] if not c.endswith("_cube")]


def resolve(name: str) -> SkyCoord:
    s = Simbad()
    s.add_votable_fields("pmra", "pmdec", "plx_value")
    r = s.query_object(name)
    if r is None or len(r) == 0 or np.ma.is_masked(r["ra"][0]):
        raise SystemExit(f"SIMBAD could not resolve {name!r}")
    row = r[0]

    def val(k: str) -> float:
        return 0.0 if np.ma.is_masked(row[k]) else float(row[k])

    plx = val("plx_value")
    return SkyCoord(
        ra=row["ra"] * u.deg,
        dec=row["dec"] * u.deg,
        pm_ra_cosdec=val("pmra") * u.mas / u.yr,
        pm_dec=val("pmdec") * u.mas / u.yr,
        distance=Distance(parallax=plx * u.mas) if plx > 0 else 10 * u.kpc,
        obstime=Time("J2000"),
    )


def at(c: SkyCoord, mjd: float) -> SkyCoord:
    p = c.apply_space_motion(new_obstime=Time(mjd, format="mjd"))
    return SkyCoord(p.ra, p.dec)


def query_images(target: SkyCoord, radius: u.Quantity) -> list[dict]:
    """All SPHEREx spectral images covering target, one row per (obs_id, detector)."""
    centre = at(target, Time("2026-01-01").mjd)
    best: dict[tuple[str, int], dict] = {}
    for coll in collections():
        t = Irsa.query_sia(pos=(centre, radius), collection=coll)
        for r in t:
            det = int(str(r["energy_bandpassname"]).rsplit("D", 1)[1])
            row = {
                "collection": coll,
                "obs_id": str(r["obs_id"]),
                "detector": det,
                "t_min": float(r["t_min"]),
                "release_date": str(r["obs_release_date"]),
                "access_url": str(r["access_url"]),
            }
            key = (row["obs_id"], det)
            if key not in best or row["release_date"] > best[key]["release_date"]:
                best[key] = row
    return sorted(best.values(), key=lambda r: (r["t_min"], r["detector"]))


def group_epochs(rows: list[dict]) -> list[list[dict]]:
    epochs: list[list[dict]] = []
    last = None
    for r in sorted(rows, key=lambda r: r["t_min"]):
        if last is None or r["t_min"] - last > EPOCH_GAP_DAYS:
            epochs.append([])
        epochs[-1].append(r)
        last = r["t_min"]
    return epochs


def cutout(url: str, c: SkyCoord, size_arcmin: float) -> fits.HDUList:
    q = f"center={c.ra.deg:.6f},{c.dec.deg:.6f}deg&size={size_arcmin}arcmin"
    path = CACHE / (hashlib.sha256(f"{url}?{q}".encode()).hexdigest() + ".fits")
    if not path.exists():
        for attempt in range(5):
            try:
                r = httpx.get(f"{url}?{q}", timeout=120, follow_redirects=True)
                r.raise_for_status()
                break
            except httpx.HTTPError:
                if attempt == 4:
                    raise
                time.sleep(2**attempt + random.random())
        path.parent.mkdir(parents=True, exist_ok=True)
        tmp = path.with_suffix(".part")
        tmp.write_bytes(r.content)
        tmp.rename(path)
    return fits.open(io.BytesIO(path.read_bytes()))


def bits(flags_header: fits.Header, names: set[str]) -> int:
    mask = 0
    for k in flags_header["MSKN*"]:
        if flags_header[k] in names:
            mask |= 1 << int(k[4:])
    return mask


def calibrated(h: fits.HDUList) -> tuple[np.ndarray, np.ndarray, np.ndarray, WCS]:
    """Zodi-subtracted image, variance, flags, celestial WCS."""
    img = h["IMAGE"].data.astype(float) - h["ZODI"].data
    var = h["VARIANCE"].data.astype(float)
    flags = h["FLAGS"].data
    bad = (flags & bits(h["FLAGS"].header, MASK_FLAGS)) != 0
    img[bad] = np.nan
    var[bad] = np.nan
    return img, var, flags, WCS(h["IMAGE"].header)


def wavelength_at(h: fits.HDUList, x: float, y: float) -> float:
    w = WCS(header=h["IMAGE"].header, fobj=h, key="W")
    w.sip = None
    wl, _bw = w.pixel_to_world(x, y)
    return float(wl.to_value(u.um))


# ---------- G1 ----------


def g1() -> None:
    print("collections:", collections())
    c = resolve("Barnard's star")
    rows = query_images(c, 0.5 * u.arcmin)
    t = Irsa.query_sia(pos=(at(c, rows[0]["t_min"]), 0.5 * u.arcmin), collection=rows[0]["collection"])
    print("columns:", t.colnames)
    by_det: dict[int, tuple[float, float]] = {}
    for r in t:
        by_det[int(str(r["energy_bandpassname"]).rsplit("D", 1)[1])] = (
            r["em_min"] * 1e6, r["em_max"] * 1e6)  # fmt: skip
    for d in sorted(by_det):
        print(f"D{d}: {by_det[d][0]:.3f}-{by_det[d][1]:.3f} um")
    r0 = rows[0]
    h = cutout(r0["access_url"], at(c, r0["t_min"]), 3)
    print("HDUs:", [(x.name, getattr(x.data, "shape", None)) for x in h])
    print("3 arcmin cutout shape:", h["IMAGE"].data.shape, "BUNIT:", h["IMAGE"].header["BUNIT"])
    fh = h["FLAGS"].header
    for k in fh["MSKN*"]:
        print(f"  flag bit {int(k[4:]):2d} {fh[k]:<13} {fh['MSKD' + k[4:]]}")
    ny, nx = h["IMAGE"].data.shape
    print("wavelength at centre:", wavelength_at(h, nx / 2, ny / 2), "um")


# ---------- G2 ----------


def aperture(img, var, flags, sat_mask, flag_mask, x, y, r=2.0):
    ny, nx = img.shape
    yy, xx = np.mgrid[:ny, :nx]
    d = np.hypot(xx - x, yy - y)
    ap = d <= r
    ann = (d >= 4) & (d <= 7)
    core = d <= 1.5
    if not ap.any() or x < 0 or y < 0 or x >= nx or y >= ny:
        return None
    sky = np.nanmedian(img[ann])
    good = ap & np.isfinite(img)
    flux = np.nansum(img[good] - sky)
    err = np.sqrt(np.nansum(var[good]))
    return {
        "snr": float(flux / err) if err > 0 else 0.0,
        "saturated": bool((flags[core] & sat_mask).any()),
        "flagged": bool((flags[core] & flag_mask).any()),
    }


def g2_target(name: str) -> dict:
    c = resolve(name)
    rows = query_images(c, 0.5 * u.arcmin)
    epochs = group_epochs(rows)
    picks = []  # first exposure per (epoch, detector)
    for i, ep in enumerate(epochs):
        seen = set()
        for r in ep:
            if r["detector"] not in seen:
                seen.add(r["detector"])
                picks.append((i, r))

    def measure(item):
        i, r = item
        pos = at(c, r["t_min"])
        h = cutout(r["access_url"], pos, 2)
        img, var, flags, w = calibrated(h)
        x, y = w.world_to_pixel(pos)
        fh = h["FLAGS"].header
        m = aperture(img, var, flags, bits(fh, SATURATION_FLAGS), bits(fh, MASK_FLAGS), x, y)
        return i, r, m

    with ThreadPoolExecutor(MAX_PARALLEL) as ex:
        results = list(ex.map(measure, picks))
    per_det = defaultdict(lambda: {"n": 0, "det": 0, "clean": 0, "sat": 0, "flag": 0})
    for _i, r, m in results:
        s = per_det[r["detector"]]
        s["n"] += 1
        if m is None:
            continue
        s["det"] += m["snr"] >= DETECT_SNR
        s["sat"] += m["saturated"]
        s["flag"] += m["flagged"]
        s["clean"] += m["snr"] >= DETECT_SNR and not m["flagged"]
    clean_epochs = {i for i, _r, m in results if m and m["snr"] >= DETECT_SNR and not m["flagged"]}
    return {
        "name": name,
        "ra": c.ra.deg,
        "dec": c.dec.deg,
        "images": len(rows),
        "collections": sorted({r["collection"] for r in rows}),
        "epochs": [
            (Time(ep[0]["t_min"], format="mjd").iso[:10], len(ep)) for ep in epochs
        ],
        "per_det": dict(sorted(per_det.items())),
        "clean_epochs": len(clean_epochs),
        "pass": len(epochs) >= 2 and len(clean_epochs) >= 2,
    }


def g2() -> None:
    for name in TARGETS:
        try:
            s = g2_target(name)
        except Exception as e:  # report and continue with the next target
            print(f"### {name}\nERROR: {e!r}\n")
            continue
        print(f"### {s['name']}  (J2000 {s['ra']:.5f} {s['dec']:.5f})")
        print(f"images: {s['images']} from {s['collections']}")
        print(f"epochs: {len(s['epochs'])} -> " + ", ".join(f"{d} ({n})" for d, n in s["epochs"]))
        print("| det | exposures checked | detected (SNR>=5) | clean | saturated core | flagged core |")
        print("|---|---|---|---|---|---|")
        for d, v in s["per_det"].items():
            print(f"| D{d} | {v['n']} | {v['det']} | {v['clean']} | {v['sat']} | {v['flag']} |")
        print(f"epochs with a clean detection: {s['clean_epochs']}  PASS: {s['pass']}\n")


# ---------- G3 ----------


def tan_grid(centre: SkyCoord, fov_arcmin: float) -> tuple[WCS, tuple[int, int]]:
    n = int(np.ceil(fov_arcmin * 60 / PIXEL_SCALE)) | 1  # odd, target on a pixel centre
    w = WCS(naxis=2)
    w.wcs.ctype = ["RA---TAN", "DEC--TAN"]
    w.wcs.crval = [centre.ra.deg, centre.dec.deg]
    w.wcs.crpix = [(n + 1) / 2, (n + 1) / 2]
    w.wcs.cdelt = [-PIXEL_SCALE / 3600, PIXEL_SCALE / 3600]
    return w, (n, n)


def gaia_field(centre: SkyCoord, radius_deg: float, gmax: float = 16.0):
    q = f"""SELECT ra, dec, pmra, pmdec, parallax, phot_g_mean_mag FROM gaiadr3.gaia_source
    WHERE 1=CONTAINS(POINT(ra,dec),CIRCLE({centre.ra.deg},{centre.dec.deg},{radius_deg}))
    AND phot_g_mean_mag < {gmax}"""
    return Gaia.launch_job(q).get_results()


def gaia_at(g, mjd: float) -> SkyCoord:
    pm_ok = ~(np.ma.getmaskarray(g["pmra"]) | np.ma.getmaskarray(g["pmdec"]))
    pmra = np.where(pm_ok, np.ma.filled(g["pmra"], 0), 0)
    pmdec = np.where(pm_ok, np.ma.filled(g["pmdec"], 0), 0)
    dt = (Time(mjd, format="mjd") - Time(2016.0, format="jyear")).to_value(u.yr)
    dec = np.asarray(g["dec"]) + pmdec * dt / 3.6e6
    ra = np.asarray(g["ra"]) + pmra * dt / 3.6e6 / np.cos(np.radians(dec))
    return SkyCoord(ra * u.deg, dec * u.deg)


def centroid(img: np.ndarray, x: float, y: float, half: int = 3):
    """Background-subtracted first-moment centroid in a small box; None if unusable."""
    xi, yi = int(round(x)), int(round(y))
    ny, nx = img.shape
    if xi - half < 0 or yi - half < 0 or xi + half >= nx or yi + half >= ny:
        return None
    box = img[yi - half : yi + half + 1, xi - half : xi + half + 1]
    if not np.isfinite(box).all():
        return None
    b = box - np.median(img[max(0, yi - 8) : yi + 9, max(0, xi - 8) : xi + 9])
    b = np.clip(b, 0, None)
    if b.sum() <= 0:
        return None
    yy, xx = np.mgrid[-half : half + 1, -half : half + 1]
    return xi + (xx * b).sum() / b.sum(), yi + (yy * b).sum() / b.sum()


def g3(name: str, fov: float, out: Path) -> None:
    c = resolve(name)
    rows = query_images(c, fov / 2 * u.arcmin)
    epochs = group_epochs(rows)
    ref_pos = at(c, epochs[0][0]["t_min"])
    grid, shape = tan_grid(ref_pos, fov)
    print(f"{name}: {len(rows)} images, {len(epochs)} epochs, grid {shape} @ {PIXEL_SCALE}\"")

    def load(r):
        h = cutout(r["access_url"], ref_pos, fov * 1.2)
        img, var, _flags, w = calibrated(h)
        ri, _ = reproject_interp((img, w), grid, shape_out=shape)
        rv, _ = reproject_interp((var, w), grid, shape_out=shape)
        return r, ri, rv

    with ThreadPoolExecutor(MAX_PARALLEL) as ex:
        loaded = list(ex.map(load, rows))

    frames = {}
    for i, ep in enumerate(epochs):
        ids = {(r["obs_id"], r["detector"]) for r in ep}
        for det in sorted({r["detector"] for r in ep}):
            stack = [(ri, rv) for r, ri, rv in loaded
                     if (r["obs_id"], r["detector"]) in ids and r["detector"] == det]  # fmt: skip
            imgs = np.array([s[0] for s in stack])
            wts = 1 / np.array([s[1] for s in stack])
            ok = np.isfinite(imgs) & np.isfinite(wts) & (wts > 0)
            wsum = np.where(ok, wts, 0).sum(0)
            with np.errstate(invalid="ignore", divide="ignore"):
                co = np.where(ok, imgs * wts, 0).sum(0) / wsum
            co[wsum == 0] = np.nan
            mjd = float(np.mean([r["t_min"] for r in ep if r["detector"] == det]))
            frames[(i, det)] = (co, mjd, len(stack))

    gaia = gaia_field(ref_pos, fov / 60 * 0.75)
    print(f"Gaia stars G<16 in field: {len(gaia)}")
    print("| epoch | det | n exp | n stars | dx\" | dy\" | residual\" | scatter\" | pass |")
    print("|---|---|---|---|---|---|---|---|---|")
    worst = 0.0
    for (i, det), (co, mjd, n) in sorted(frames.items()):
        gx, gy = grid.world_to_pixel(gaia_at(gaia, mjd))
        noise = 1.4826 * np.nanmedian(np.abs(co - np.nanmedian(co)))
        offs = []
        for x, y in zip(gx, gy, strict=True):
            near = np.hypot(gx - x, gy - y)
            if (near < 3).sum() > 1:  # blended with another Gaia star
                continue
            if np.hypot(x - grid.wcs.crpix[0] + 1, y - grid.wcs.crpix[1] + 1) < 3:
                continue  # the target itself
            m = centroid(co, x, y)
            if m is None:
                continue
            xi, yi = int(round(x)), int(round(y))
            if co[yi, xi] - np.nanmedian(co) < 20 * noise:
                continue
            dx, dy = m[0] - x, m[1] - y
            if np.hypot(dx, dy) < 1.5:
                offs.append((dx, dy))
        if len(offs) < 3:
            print(f"| {i} | D{det} | {n} | {len(offs)} | - | - | - | - | too few stars |")
            continue
        o = np.array(offs) * PIXEL_SCALE
        mdx, mdy = np.median(o[:, 0]), np.median(o[:, 1])
        res = float(np.hypot(mdx, mdy))
        scat = float(np.median(np.hypot(o[:, 0] - mdx, o[:, 1] - mdy)))
        worst = max(worst, res)
        ok = res <= ALIGN_THRESHOLD_ARCSEC
        print(f"| {i} | D{det} | {n} | {len(o)} | {mdx:+.2f} | {mdy:+.2f} | {res:.2f} | {scat:.2f} | {ok} |")
    print(f"\nworst frame residual: {worst:.2f}\" (threshold {ALIGN_THRESHOLD_ARCSEC}\")")

    # Shared stretch per detector from the reference epoch, applied to every epoch.
    out.mkdir(parents=True, exist_ok=True)
    for det in sorted({d for _, d in frames}):
        ref = next(frames[(i, det)][0] for i in range(len(epochs)) if (i, det) in frames)
        lo, hi = np.nanpercentile(ref, [1, 99.5])
        for (i, d), (co, mjd, _n) in sorted(frames.items()):
            if d != det:
                continue
            v = np.clip((co - lo) / (hi - lo), 0, 1)
            v = np.arcsinh(10 * v) / np.arcsinh(10)
            png = np.flipud(np.nan_to_num(v * 255)).astype(np.uint8)
            date = Time(mjd, format="mjd").iso[:10]
            Image.fromarray(png).resize((png.shape[1] * 4, png.shape[0] * 4), Image.NEAREST).save(
                out / f"D{det}_e{i}_{date}.png"
            )
    print(f"frames written to {out}")


def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument("gate", choices=["g1", "g2", "g3"])
    p.add_argument("target", nargs="?", default="Barnard's star")
    p.add_argument("--fov", type=float, default=6.0)
    p.add_argument("--out", type=Path, default=Path("gates/out"))
    a = p.parse_args()
    {"g1": g1, "g2": g2}.get(a.gate, lambda: g3(a.target, a.fov, a.out))()


if __name__ == "__main__":
    sys.exit(main())
