"""Gate bookkeeping. No network."""

from app.sources import Exposure
from gates import gates


def test_first_per_detector_picks_one_exposure_per_epoch_and_detector():
    def e(t: float, det: int) -> Exposure:
        return Exposure("c", f"o{t}", det, t, "2026-01-01", "u")

    epochs = [[e(1, 1), e(1, 2), e(2, 1)], [e(50, 1)]]
    picks = gates.first_per_detector(epochs)
    assert [(i, x.t_min, x.detector) for i, x in picks] == [(0, 1, 1), (0, 1, 2), (1, 50, 1)]


def test_g2_pass_needs_two_epochs_with_clean_detections():
    r = gates.G2Result(slug="wise-0855", clean_epochs=2)
    assert r.passed
    r.clean_epochs = 1
    assert not r.passed
    assert not gates.G2Result(slug="wise-0855", error="boom", clean_epochs=3).passed


def test_featured_lists_passing_targets_where_spherex_sees_them():
    results = [
        {"slug": "wise-0855", "passed": True, "seen_ra": 133.7864321, "seen_dec": -7.2445},
        {"slug": "barnards-star", "passed": False, "seen_ra": 269.4, "seen_dec": 4.7},
    ]
    assert gates.featured(results) == [
        {
            "slug": "wise-0855",
            "name": "WISE 0855",
            "type": "Coldest known brown dwarf",
            "ra": 133.786432,
            "dec": -7.2445,
        }
    ]


def test_targets_have_unique_slugs():
    assert len(gates.BY_SLUG) == len(gates.TARGETS)
