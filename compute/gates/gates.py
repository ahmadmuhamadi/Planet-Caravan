"""Phase 0 gates (G1-G4): checks run once against the live archives, not service code.

Run from compute/:
    uv run python -m gates.gates g1                # data facts, printed
    uv run python -m gates.gates g2                # -> gates/results/g2.json
    uv run python -m gates.gates g3 wise-0855      # -> gates/results/g3.json, PNGs in gates/out/
    uv run python -m gates.gates g4                # reads g2.json -> gates/results/g4.json

Each gate prints a Markdown summary for docs/decisions.md and saves its full result as
JSON, so later gates read earlier results instead of querying the archives again.
"""

import argparse
import enum
import json
import logging
import warnings
from concurrent.futures import ThreadPoolExecutor
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any

import astropy.units as u
import numpy as np
from astropy.time import Time
from astropy.wcs import FITSFixedWarning
from astroquery.ipac.irsa import Irsa
from reproject import reproject_interp

from app import pipeline, render, sources
from app.config import settings
from app.pipeline import Array
from app.sources import Exposure

log = logging.getLogger("gates")

HERE = Path(__file__).parent
RESULTS = HERE / "results"
MIN_CLEAN_EPOCHS = 2  # G2 pass
FEATURED_COUNT = (3, 5)  # G4 pass: how many featured targets
PNG_UPSCALE = 4  # gate PNGs are for people to look at


@dataclass(frozen=True)
class Target:
    """An acceptance target: how SIMBAD knows it, and how the app shows it (G4)."""

    simbad: str
    slug: str
    name: str
    kind: str  # a phrase for readers, not a SIMBAD object type


TARGETS = (
    Target("Barnard's star", "barnards-star", "Barnard's Star", "Fastest-moving star in the sky"),
    Target("WISE J085510.83-071442.5", "wise-0855", "WISE 0855", "Coldest known brown dwarf"),
    Target("eps Ind B", "epsilon-indi-b", "Epsilon Indi B", "Nearby pair of brown dwarfs"),
    Target("RW Aur", "rw-aurigae", "RW Aurigae", "Young variable star with a dusty disc"),
    Target("R Dra", "r-draconis", "R Draconis", "Mira variable star"),
)
BY_SLUG = {t.slug: t for t in TARGETS}


def save(name: str, data: Any) -> None:
    RESULTS.mkdir(exist_ok=True)
    path = RESULTS / f"{name}.json"
    path.write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n")
    log.info("wrote %s", path)


def load(name: str) -> Any:
    path = RESULTS / f"{name}.json"
    if not path.exists():
        raise SystemExit(f"{path} is missing: run gate {name} first")
    return json.loads(path.read_text())


def iso_date(mjd: float) -> str:
    return str(Time(mjd, format="mjd").iso)[:10]


# ---------- G1: data facts ----------


def g1() -> None:
    print("collections:", sources.spectral_collections())
    star = sources.resolve("Barnard's star")
    first = sources.query_exposures(star, 0.5 * u.arcmin)[0]
    pos = sources.position_at(star, first.t_min)

    table = Irsa.query_sia(pos=(pos, 0.5 * u.arcmin), collection=first.collection)
    print("columns:", table.colnames)
    bands = {
        sources.detector_number(str(row["energy_bandpassname"])): (
            row["em_min"] * 1e6,
            row["em_max"] * 1e6,
        )
        for row in table
    }
    for det, (lo, hi) in sorted(bands.items()):
        print(f"D{det}: {lo:.3f}-{hi:.3f} um")

    hdul = sources.cutout(first.access_url, pos, 3)
    image = hdul["IMAGE"]
    print("HDUs:", [(h.name, getattr(h.data, "shape", None)) for h in hdul])
    print("3 arcmin cutout shape:", image.data.shape, "BUNIT:", image.header["BUNIT"])
    flags = hdul["FLAGS"].header
    for key in flags["MSKN*"]:
        bit = key[4:]
        print(f"  flag bit {int(bit):2d} {flags[key]:<13} {flags['MSKD' + bit]}")
    ny, nx = image.data.shape
    print("wavelength at centre:", pipeline.wavelength_at(hdul, nx / 2, ny / 2), "um")


# ---------- G2: targets checked ----------


@dataclass
class DetectorCheck:
    exposures: int = 0
    detected: int = 0  # SNR >= detect_snr
    clean: int = 0  # detected, no flagged core pixel
    saturated: int = 0
    flagged: int = 0


@dataclass
class G2Result:
    slug: str
    error: str | None = None
    ra: float = 0.0  # J2000
    dec: float = 0.0
    seen_ra: float = 0.0  # at seen_mjd, the mean date of its SPHEREx images
    seen_dec: float = 0.0
    seen_mjd: float = 0.0
    images: int = 0
    skipped: int = 0  # images IRSA would not cut out
    collections: list[str] = field(default_factory=list)
    epochs: list[tuple[str, int]] = field(default_factory=list)  # (first date, images)
    detectors: dict[int, DetectorCheck] = field(default_factory=dict)
    clean_epochs: int = 0

    @property
    def passed(self) -> bool:
        return self.error is None and self.clean_epochs >= MIN_CLEAN_EPOCHS


class Skipped(enum.Enum):
    SKIPPED = enum.auto()


SKIPPED = Skipped.SKIPPED


def first_per_detector(epochs: list[list[Exposure]]) -> list[tuple[int, Exposure]]:
    """(epoch index, exposure) for the first exposure of each detector in each epoch."""
    picks = []
    for i, epoch in enumerate(epochs):
        seen: set[int] = set()
        for e in epoch:
            if e.detector not in seen:
                seen.add(e.detector)
                picks.append((i, e))
    return picks


def g2_target(target: Target) -> G2Result:
    """Measures the first exposure per (epoch, detector): detected, saturated, flagged."""
    star = sources.resolve(target.simbad)
    exposures = sources.query_exposures(star, 0.5 * u.arcmin)
    epochs = pipeline.group_epochs(exposures, settings.epoch_gap_days)

    def measure(e: Exposure) -> Skipped | pipeline.Photometry | None:
        """SKIPPED when IRSA rejects the image; None when the target is off the cutout."""
        pos = sources.position_at(star, e.t_min)
        hdul = sources.cutout_or_none(e.access_url, pos, 2)
        if hdul is None:
            return SKIPPED
        cal = pipeline.calibrate(hdul, settings.mask_flags)
        x, y = cal.wcs.world_to_pixel(pos)
        header = hdul["FLAGS"].header
        return pipeline.photometry(
            cal,
            float(x),
            float(y),
            saturation_mask=pipeline.flag_bits(header, settings.saturation_flags),
            flag_mask=pipeline.flag_bits(header, settings.mask_flags),
        )

    picks = first_per_detector(epochs)
    with ThreadPoolExecutor(settings.max_parallel_downloads) as pool:
        measured = list(pool.map(measure, [e for _, e in picks]))

    detectors: dict[int, DetectorCheck] = {}
    clean_epochs: set[int] = set()
    skipped = 0
    for (i, e), m in zip(picks, measured, strict=True):
        check = detectors.setdefault(e.detector, DetectorCheck())
        check.exposures += 1
        if m is SKIPPED:
            skipped += 1
            continue
        if m is None:
            continue
        detected = m.snr >= settings.detect_snr
        check.detected += detected
        check.saturated += m.saturated
        check.flagged += m.flagged
        if detected and not m.flagged:
            check.clean += 1
            clean_epochs.add(i)

    mean_mjd = float(np.mean([e.t_min for e in exposures]))
    seen = sources.position_at(star, mean_mjd)  # fast movers are far from J2000
    return G2Result(
        slug=target.slug,
        ra=float(star.ra.deg),
        dec=float(star.dec.deg),
        seen_ra=float(seen.ra.deg),
        seen_dec=float(seen.dec.deg),
        seen_mjd=mean_mjd,
        images=len(exposures),
        skipped=skipped,
        collections=sorted({e.collection for e in exposures}),
        epochs=[(iso_date(ep[0].t_min), len(ep)) for ep in epochs],
        detectors=dict(sorted(detectors.items())),
        clean_epochs=len(clean_epochs),
    )


def g2() -> None:
    results = []
    for target in TARGETS:
        try:
            results.append(g2_target(target))
        except Exception as e:  # one target's failure is part of the report, not its end
            log.exception("G2 failed for %s", target.name)
            results.append(G2Result(slug=target.slug, error=repr(e)))
    save("g2", [asdict(r) | {"passed": r.passed} for r in results])
    for r in results:
        print(g2_markdown(r))


def g2_markdown(r: G2Result) -> str:
    name = BY_SLUG[r.slug].name
    if r.error:
        return f"### {name}\nERROR: {r.error}\n"
    lines = [
        f"### {name}  (J2000 {r.ra:.5f} {r.dec:.5f})",
        f"images: {r.images} from {', '.join(r.collections)}; skipped: {r.skipped}",
        f"epochs: {len(r.epochs)} -> " + ", ".join(f"{d} ({n})" for d, n in r.epochs),
        "| det | exposures checked | detected (SNR>=5) | clean | saturated core | flagged core |",
        "|---|---|---|---|---|---|",
    ]
    for det, c in r.detectors.items():
        lines.append(
            f"| D{det} | {c.exposures} | {c.detected} | {c.clean} | {c.saturated} | {c.flagged} |"
        )
    lines.append(f"epochs with a clean detection: {r.clean_epochs}  PASS: {r.passed}\n")
    return "\n".join(lines)


# ---------- G3: science proven ----------


@dataclass
class Frame:
    epoch: int
    detector: int
    image: Array  # co-add on the shared grid
    mjd: float  # mean date of its exposures
    exposures: int


def g3(slug: str, fov_arcmin: float, out: Path) -> None:
    """Cutouts -> co-adds per (epoch, detector) on one grid with one stretch per
    detector; alignment residual of each against Gaia."""
    star = sources.resolve(BY_SLUG[slug].simbad)
    exposures = sources.query_exposures(star, fov_arcmin / 2 * u.arcmin)
    epochs = pipeline.group_epochs(exposures, settings.epoch_gap_days)
    centre = sources.position_at(star, epochs[0][0].t_min)
    grid, shape = pipeline.tan_grid(centre, fov_arcmin, settings.pixel_scale_arcsec)

    def reprojected(e: Exposure) -> tuple[Exposure, Array, Array] | None:
        # A larger cutout, so reprojection fills the grid's corners.
        hdul = sources.cutout_or_none(e.access_url, centre, fov_arcmin * 1.2)
        if hdul is None:
            return None
        cal = pipeline.calibrate(hdul, settings.mask_flags)
        image, _ = reproject_interp((cal.image, cal.wcs), grid, shape_out=shape)
        variance, _ = reproject_interp((cal.variance, cal.wcs), grid, shape_out=shape)
        return e, image, variance

    with ThreadPoolExecutor(settings.max_parallel_downloads) as pool:
        loaded = [r for r in pool.map(reprojected, exposures) if r is not None]

    frames = []
    for i, epoch in enumerate(epochs):
        in_epoch = set(epoch)
        for det in sorted({e.detector for e in epoch}):
            stack = [(img, var) for e, img, var in loaded if e in in_epoch and e.detector == det]
            if stack:
                mjd = float(np.mean([e.t_min for e in epoch if e.detector == det]))
                frames.append(Frame(i, det, pipeline.coadd(stack), mjd, len(stack)))

    gaia = sources.gaia_stars(centre.ra.deg, centre.dec.deg, fov_arcmin / 60 * 0.75)
    target_xy = (grid.wcs.crpix[0] - 1, grid.wcs.crpix[1] - 1)  # FITS pixels are 1-based
    limit = settings.align_threshold_arcsec
    rows = []
    for f in frames:
        stars = pipeline.gaia_positions_at(gaia, f.mjd)
        a = pipeline.alignment(f.image, grid, stars, settings.pixel_scale_arcsec, target_xy)
        passed = None if a.residual_arcsec is None else a.residual_arcsec <= limit
        rows.append(
            {
                "epoch": f.epoch,
                "detector": f.detector,
                "exposures": f.exposures,
                "stars": a.stars,
                "dx": a.dx_arcsec,
                "dy": a.dy_arcsec,
                "residual": a.residual_arcsec,
                "scatter": a.scatter_arcsec,
                "passed": passed,
            }
        )

    measured = [r["residual"] for r in rows if r["residual"] is not None]
    result = {
        "slug": slug,
        "fov_arcmin": fov_arcmin,
        "images": len(exposures),
        "skipped": len(exposures) - len(loaded),
        "threshold_arcsec": limit,
        "frames": rows,
        "worst_residual": max(measured, default=None),
        "passed": bool(measured) and max(measured) <= limit,
    }
    save("g3", result)
    write_pngs(frames, out)
    print(g3_markdown(result))


def write_pngs(frames: list[Frame], out: Path) -> None:
    """One stretch per detector from its first (reference) epoch, applied to every epoch."""
    out.mkdir(parents=True, exist_ok=True)
    limits: dict[int, tuple[float, float]] = {}
    for f in sorted(frames, key=lambda f: (f.detector, f.epoch)):
        lo, hi = limits.setdefault(f.detector, render.stretch_limits(f.image))
        pixels = render.stretch(f.image, lo, hi)
        path = out / f"D{f.detector}_e{f.epoch}_{iso_date(f.mjd)}.png"
        path.write_bytes(render.png(pixels, upscale=PNG_UPSCALE))
    log.info("frames written to %s", out)


def g3_markdown(r: dict[str, Any]) -> str:
    epochs = len({f["epoch"] for f in r["frames"]})
    lines = [
        f"{BY_SLUG[r['slug']].name}: {r['images']} images ({r['skipped']} skipped), "
        f"{epochs} epochs, fov {r['fov_arcmin']}′",
        '| epoch | det | n exp | n stars | dx" | dy" | residual" | scatter" | pass |',
        "|---|---|---|---|---|---|---|---|---|",
    ]
    for f in r["frames"]:
        head = f"| {f['epoch']} | D{f['detector']} | {f['exposures']} | {f['stars']} "
        if f["residual"] is None:
            lines.append(head + "| - | - | - | - | too few stars |")
        else:
            lines.append(
                head + f"| {f['dx']:+.2f} | {f['dy']:+.2f} | {f['residual']:.2f} "
                f"| {f['scatter']:.2f} | {f['passed']} |"
            )
    worst = "n/a" if r["worst_residual"] is None else f'{r["worst_residual"]:.2f}"'
    lines.append(f'\nworst frame residual: {worst} (threshold {r["threshold_arcsec"]}")')
    lines.append(f"PASS: {r['passed']}")
    return "\n".join(lines)


# ---------- G4: demo set frozen ----------


def featured(g2_results: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """The targets that passed G2, as gateway featured.json entries. Positions are where
    SPHEREx sees them (mean image date), since fast movers are far from J2000."""
    return [
        {
            "slug": r["slug"],
            "name": BY_SLUG[r["slug"]].name,
            "type": BY_SLUG[r["slug"]].kind,
            "ra": round(r["seen_ra"], 6),
            "dec": round(r["seen_dec"], 6),
        }
        for r in g2_results
        if r["passed"]
    ]


def g4() -> None:
    entries = featured(load("g2"))
    lo, hi = FEATURED_COUNT
    passed = lo <= len(entries) <= hi
    save("g4", {"featured": entries, "passed": passed})
    print(json.dumps(entries, indent=2, ensure_ascii=False))
    print(f"\n{len(entries)} targets passed G2 (need {lo} to {hi}). PASS: {passed}")


# ---------- command line ----------


def main() -> None:
    logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")
    logging.getLogger("httpx").setLevel(logging.WARNING)  # a line per request is noise
    warnings.simplefilter("ignore", FITSFixedWarning)
    warnings.filterwarnings("ignore", message=".*SIP.*")

    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawTextHelpFormatter
    )
    gates = parser.add_subparsers(dest="gate", required=True)
    gates.add_parser("g1", help="verify the data facts")
    gates.add_parser("g2", help="check every acceptance target")
    p3 = gates.add_parser("g3", help="align one target and measure the residual against Gaia")
    p3.add_argument("target", choices=sorted(BY_SLUG))
    p3.add_argument("--fov", type=float, default=6.0, help="arcmin")
    p3.add_argument("--out", type=Path, default=HERE / "out", help="PNG frames")
    gates.add_parser("g4", help="featured targets from the G2 result")
    args = parser.parse_args()

    match args.gate:
        case "g1":
            g1()
        case "g2":
            g2()
        case "g3":
            g3(args.target, args.fov, args.out)
        case "g4":
            g4()


if __name__ == "__main__":
    main()
