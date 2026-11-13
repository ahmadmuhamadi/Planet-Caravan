import io

import numpy as np
from PIL import Image

from app import render


def test_stretch_is_8_bit_and_north_up():
    out = render.stretch(np.array([[0.0, 0.5], [1.0, np.nan]]), 0, 1)
    assert out.dtype == np.uint8
    assert out[0, 0] == 255 and out[1, 0] == 0  # the last row (north) is drawn on top
    assert out[0, 1] == 0  # masked pixels are black


def test_stretch_limits_ignore_masked_pixels():
    lo, hi = render.stretch_limits(np.array([np.nan, *range(201)]))
    assert (lo, hi) == (2.0, 199.0)


def test_png_upscales_with_hard_pixel_edges():
    pixels = np.array([[0, 255]], np.uint8)
    img = Image.open(io.BytesIO(render.png(pixels, upscale=3)))
    assert img.size == (6, 3)
    assert img.getpixel((2, 1)) == 0 and img.getpixel((3, 1)) == 255
