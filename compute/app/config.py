"""Settings. Every data fact Phase 0 could only assume lives here, not in code (D2).

Each field can be overridden by an environment variable of the same name in upper case,
e.g. PIXEL_SCALE_ARCSEC=6.0. docs/decisions.md records which values are Confirmed.
"""

from pathlib import Path

from pydantic_settings import BaseSettings

# FLAGS bit names (from the MSKNnnnn header keys) whose pixels are masked out (S1).
DEFAULT_MASK_FLAGS = frozenset(
    {
        "TRANSIENT",
        "OVERFLOW",
        "SUR_ERROR",
        "NONFUNC",
        "MISSING_DATA",
        "HOT",
        "COLD",
        "NONLINEAR",
        "PERSIST",
        "OUTLIER",
    }
)


class Settings(BaseSettings):
    simbad_server: str = "simbad.harvard.edu"
    cutout_cache: Path = Path(".cache/cutouts")

    # Downloads (D4)
    download_timeout_s: float = 120.0
    download_attempts: int = 5
    max_parallel_downloads: int = 8

    # Pipeline
    epoch_gap_days: float = 30.0  # a longer gap between images starts a new epoch
    pixel_scale_arcsec: float = 6.2  # output grid (S2)
    align_threshold_arcsec: float = 1.2  # 0.2 pixel (S3)
    detect_snr: float = 5.0
    mask_flags: frozenset[str] = DEFAULT_MASK_FLAGS
    saturation_flags: frozenset[str] = frozenset({"OVERFLOW", "NONLINEAR"})


settings = Settings()
