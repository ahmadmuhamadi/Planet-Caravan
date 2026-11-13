"""Frames to 8-bit PNG (S7)."""

import io

import numpy as np
import numpy.typing as npt
from PIL import Image

ASINH_SOFTENING = 10.0


def stretch_limits(reference: npt.NDArray[np.float64]) -> tuple[float, float]:
    """Black and white levels from the reference epoch, shared by every frame (S7)."""
    lo, hi = np.nanpercentile(reference, [1, 99.5])
    return float(lo), float(hi)


def stretch(image: npt.NDArray[np.float64], lo: float, hi: float) -> npt.NDArray[np.uint8]:
    """8-bit asinh stretch between lo and hi, flipped so north is up. Masked pixels are black."""
    v = np.clip((image - lo) / (hi - lo), 0, 1)
    v = np.arcsinh(ASINH_SOFTENING * v) / np.arcsinh(ASINH_SOFTENING)
    pixels: npt.NDArray[np.uint8] = np.flipud(np.nan_to_num(v * 255)).astype(np.uint8)
    return pixels


def png(pixels: npt.NDArray[np.uint8], upscale: int = 1) -> bytes:
    """PNG bytes, each pixel drawn as an upscale x upscale block."""
    img = Image.fromarray(pixels)
    if upscale > 1:
        img = img.resize((img.width * upscale, img.height * upscale), Image.Resampling.NEAREST)
    buf = io.BytesIO()
    img.save(buf, format="PNG")
    return buf.getvalue()
