"""Image processing on synthetic data. No network."""

import astropy.units as u
import numpy as np
import pytest
from astropy.coordinates import SkyCoord
from astropy.io import fits
from astropy.table import MaskedColumn, Table
from astropy.wcs import WCS

from app import pipeline
from app.pipeline import Calibrated
from app.sources import Exposure


def star(shape=(31, 31), x=15.0, y=15.0, flux=1000.0, sigma=1.2, sky=10.0):
    yy, xx = np.mgrid[: shape[0], : shape[1]]
    psf = np.exp(-((xx - x) ** 2 + (yy - y) ** 2) / (2 * sigma**2))
    return sky + flux * psf / psf.sum()


def exposure(t_min: float) -> Exposure:
    return Exposure("c", f"obs{t_min}", 1, t_min, "2026-01-01", "https://x")


def calibrated(image, variance=None, flags=None) -> Calibrated:
    return Calibrated(
        image=image,
        variance=np.ones_like(image) if variance is None else variance,
        flags=np.zeros(image.shape, np.int32) if flags is None else flags,
        wcs=WCS(naxis=2),
    )


def test_group_epochs_splits_on_gaps_longer_than_the_limit():
    exposures = [exposure(t) for t in (100, 101, 129, 200, 230.5, 400)]
    epochs = pipeline.group_epochs(reversed(exposures), gap_days=30)  # order does not matter
    assert [[e.t_min for e in ep] for ep in epochs] == [[100, 101, 129], [200], [230.5], [400]]


def test_flag_bits_reads_names_from_the_header():
    h = fits.Header({"MSKN0000": "TRANSIENT", "MSKN0001": "OVERFLOW", "MSKN0015": "NONLINEAR"})
    assert pipeline.flag_bits(h, {"OVERFLOW", "NONLINEAR"}) == (1 << 1) | (1 << 15)
    assert pipeline.flag_bits(h, {"HOT"}) == 0


def test_calibrate_subtracts_zodi_and_blanks_masked_pixels():
    flags_header = fits.Header({"MSKN0000": "HOT", "MSKN0001": "SOURCE"})
    flags = np.array([[0, 1], [2, 3]], np.int32)
    hdul = fits.HDUList(
        [
            fits.PrimaryHDU(),
            fits.ImageHDU(np.full((2, 2), 5.0), name="IMAGE"),
            fits.ImageHDU(flags, flags_header, name="FLAGS"),
            fits.ImageHDU(np.full((2, 2), 0.5), name="VARIANCE"),
            fits.ImageHDU(np.full((2, 2), 1.0), name="ZODI"),
        ]
    )
    cal = pipeline.calibrate(hdul, {"HOT"})
    expected = np.array([[4.0, np.nan], [4.0, np.nan]])  # bit 0 (HOT) masked, bit 1 kept
    np.testing.assert_array_equal(cal.image, expected)
    np.testing.assert_array_equal(np.isnan(cal.variance), np.isnan(expected))


def test_photometry_detects_a_star_and_reports_its_core_flags():
    flags = np.zeros((31, 31), np.int32)
    m = pipeline.photometry(calibrated(star(), flags=flags), 15, 15, 0b10, 0b11)
    assert m is not None and m.snr > 50 and not m.saturated and not m.flagged

    flags[15, 15] = 0b10  # saturation bit in the core
    m = pipeline.photometry(calibrated(star(), flags=flags), 15, 15, 0b10, 0b11)
    assert m is not None and m.saturated and m.flagged


def test_photometry_on_empty_sky_is_not_a_detection():
    sky = np.random.default_rng(1).normal(10, 1, (31, 31))
    m = pipeline.photometry(calibrated(sky), 15, 15, 0, 0)
    assert m is not None and m.snr < 5


@pytest.mark.parametrize(("x", "y"), [(-1, 5), (5, 31), (31, 5)])
def test_photometry_off_the_image_is_none(x, y):
    assert pipeline.photometry(calibrated(star()), x, y, 0, 0) is None


def test_centroid_recovers_a_sub_pixel_position():
    c = pipeline.centroid(star(x=14.3, y=16.6), 14, 17)
    assert c == pytest.approx((14.3, 16.6), abs=0.05)


def test_centroid_rejects_edges_and_masked_pixels():
    img = star()
    assert pipeline.centroid(img, 1, 15) is None
    img[15, 15] = np.nan
    assert pipeline.centroid(img, 15, 15) is None


def test_tan_grid_puts_the_centre_on_the_middle_pixel_with_east_left():
    c = SkyCoord(133.8 * u.deg, -7.2 * u.deg)
    w, shape = pipeline.tan_grid(c, 3, 6.2)
    assert shape == (31, 31)  # 3' / 6.2" = 29.03 -> 30 -> odd 31
    x, y = w.world_to_pixel(c)
    assert (float(x), float(y)) == pytest.approx((15, 15))
    east = SkyCoord(c.ra + 6.2 * u.arcsec / np.cos(c.dec), c.dec)
    assert float(w.world_to_pixel(east)[0]) == pytest.approx(14, abs=1e-3)


def test_coadd_is_inverse_variance_weighted_and_skips_masked_pixels():
    a = (np.array([[1.0, 1.0]]), np.array([[1.0, 1.0]]))
    b = (np.array([[4.0, np.nan]]), np.array([[3.0, 3.0]]))
    co = pipeline.coadd([a, b])
    assert co[0, 0] == pytest.approx((1 * 1 + 4 / 3) / (1 + 1 / 3))
    assert co[0, 1] == 1.0
    assert np.isnan(pipeline.coadd([(np.array([[np.nan]]), np.array([[1.0]]))])[0, 0])


def test_gaia_positions_move_by_proper_motion_and_masked_motion_stays_put():
    stars = Table(
        {
            "ra": [10.0, 20.0],
            "dec": [0.0, 0.0],
            "pmra": MaskedColumn([1000.0, 1.0], mask=[False, True]),  # 1 arcsec/yr east
            "pmdec": MaskedColumn([0.0, 1.0], mask=[False, True]),
        }
    )
    ten_years_later = 57388.5 + 10 * 365.25  # MJD of 2016.0 is 57388.5
    moved = pipeline.gaia_positions_at(stars, ten_years_later)
    assert moved[0].ra.deg == pytest.approx(10 + 10 / 3600, abs=1e-6)
    assert (moved[1].ra.deg, moved[1].dec.deg) == pytest.approx((20.0, 0.0))


def test_alignment_measures_a_known_shift_and_ignores_the_target():
    centre = SkyCoord(100 * u.deg, 0 * u.deg)
    grid, shape = pipeline.tan_grid(centre, 6, 6.2)
    xy = [(10, 10), (40, 12), (15, 45), (45, 44)]
    image = np.zeros(shape) + np.random.default_rng(2).normal(0, 0.1, shape)
    for x, y in xy:
        image += star(shape, x + 0.3, y, sky=0)  # every star measured 0.3 px east of Gaia
    image += star(shape, 29.5, 29, sky=0)  # the target, a little off its catalogue pixel
    stars = SkyCoord(grid.pixel_to_world([x for x, _ in xy] + [29], [y for _, y in xy] + [29]))

    a = pipeline.alignment(image, grid, stars, 6.2, target_xy=(29, 29))
    assert a.stars == 4
    assert a.dx_arcsec == pytest.approx(0.3 * 6.2, abs=0.2)
    assert a.dy_arcsec == pytest.approx(0, abs=0.2)


def test_alignment_with_too_few_stars_has_no_residual():
    grid, shape = pipeline.tan_grid(SkyCoord(100 * u.deg, 0 * u.deg), 6, 6.2)
    a = pipeline.alignment(np.zeros(shape), grid, SkyCoord([] * u.deg, [] * u.deg), 6.2, (29, 29))
    assert a.stars == 0 and a.residual_arcsec is None
