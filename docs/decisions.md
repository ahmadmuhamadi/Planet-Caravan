# Decisions

Phase 0 gate record (G1–G4) and later design decisions. Each external fact is
**Confirmed** (checked against a live service or the docs) or **Assumed** (our
choice, kept in compute config). All checks below were made on 2026-11-13.

Script: `compute/gates/gates.py` (`g1` to `g4`). Reproduce with
`cd compute && uv run python -m gates.gates <gate>`. G2–G4 also write their full
result to `compute/gates/results/<gate>.json`.

## G1 — data facts

| Fact | Status | Finding | Source |
|---|---|---|---|
| SIA v2 endpoint | Confirmed | `https://irsa.ipac.caltech.edu/SIA?COLLECTION=…&POS=CIRCLE ra dec r` returns a VOTable | [data access](https://caltech-ipac.github.io/spherex-archive-documentation/spherex-data-access), live query |
| Collections | **Changed** | Spectral-image collections are now `spherex_dr1`, `spherex_dr1_deep`, `spherex_qr3`, `spherex_qr3_deep`. `spherex_qr2*` are gone. `spherex_dr1_cube` (all-sky cube) and `spherex_dr1_cal` (calibration) also exist and are not exposures | IRSA TAP, live |
| Collection discovery | Confirmed | IRSA TAP: `SELECT DISTINCT o.collection FROM caom.observation o JOIN caom.plane p ON o.obsid = p.obsid WHERE p.dataproducttype = 'image' AND o.collection LIKE 'spherex%'` (same query astroquery's `list_collections` runs). It takes **~50 s**, so compute caches the list in memory (TTL in config) | `astroquery.ipac.irsa` source, live |
| Result columns | Confirmed | `access_url, t_min, t_max, t_exptime, energy_bandpassname, obs_id, s_region` all present, plus `em_min, em_max` (m), `obs_release_date`, `obs_publisher_did`, `s_pixel_scale`. One row per (obs_id, detector); `energy_bandpassname` = `SPHEREx-D<n>` | live query |
| Cutout parameters | Confirmed | Append `?center=<ra>,<dec>&size=<s>` to `access_url`. Bare numbers are **degrees** (`size=0.05` and `size=3arcmin` both give 29×29 px). Unit suffixes `deg`, `arcmin`, `pix` are accepted; compute always sends explicit units | [IBE cutouts](https://irsa.ipac.caltech.edu/ibe/cutouts.html), [cutout tool](https://irsa.ipac.caltech.edu/data/SPHEREx/docs/cutout_tool.html), live |
| Cutout cost | Confirmed | Every cutout carries the full EPSF table: **~4 MB and 6–16 s each**, whatever the size. Not gzipped. See risk R-1 below | live |
| Cutout orientation | Confirmed | Cutout axes follow the detector, not north-up. Reprojection is required | IBE cutouts doc |
| HDUs | **Changed** | `IMAGE` (MJy/sr, zodi not subtracted), `FLAGS` (int32), `VARIANCE` ((MJy/sr)²), `ZODI` (MJy/sr), **`EPSF`** (DR1/QR3; was `PSF` in QR2), `WCS-WAVE` | [data products](https://caltech-ipac.github.io/spherex-archive-documentation/spherex-data-products), live |
| Celestial WCS | Confirmed | `IMAGE` header is `RA---TAN-SIP`; `astropy.wcs.WCS(header)` applies SIP | live |
| Per-pixel wavelength | Confirmed | `WCS(header=IMAGE.header, fobj=hdul, key="W")`, `.sip = None`, `pixel_to_world(x, y)` → (wavelength, bandwidth). D3 cutout centre: 2.307 µm | [spherex-intro](https://caltech-ipac.github.io/irsa-tutorials/spherex-intro), live |
| Observation time | Confirmed | `t_min`/`t_max` are MJD. Headers carry `MJD-OBS`, `MJD-AVG` | live |
| Pixel scale | Confirmed | `s_pixel_scale` = 6.15″. Header `PSF_FWHM` ≈ 7.4″ (D3), so stars are undersampled. Output grid default stays 6.2″ (config) | live |
| Detector → band | Confirmed | D1 0.75–1.12, D2 1.10–1.65, D3 1.63–2.44, D4 2.40–3.85, D5 3.81–4.43, D6 4.41–5.01 µm (`em_min`/`em_max`) | live, data products doc |
| Flag bits | Confirmed | Self-described in the `FLAGS` header (`MSKNnnnn` = name, `MSKDnnnn` = description): 0 TRANSIENT, 1 OVERFLOW, 2 SUR_ERROR, 6 NONFUNC, 7 DICHROIC, 9 MISSING_DATA, 10 HOT, 11 COLD, 12 FULLSAMPLE, 14 PHANMISS, 15 NONLINEAR, 17 PERSIST, 19 OUTLIER, 20 CROSSTALK, 21 SOURCE, 22 GHOST, 23 GHOST_FPA, 24 GHOST_EXT, 25 STREAK, 26 BLOOM, 27 SNOWBALL, 28 HALO. Compute reads bit numbers from the header and configures masks **by name** | live, spherex-intro |
| Default mask | Assumed | No official recommendation found. Masked: TRANSIENT, OVERFLOW, SUR_ERROR, NONFUNC, MISSING_DATA, HOT, COLD, NONLINEAR, PERSIST, OUTLIER. Not masked: SOURCE (marks known sources, i.e. the stars we want), FULLSAMPLE (informational), DICHROIC, PHANMISS, CROSSTALK, GHOST*, STREAK, BLOOM, SNOWBALL, HALO. Saturation = OVERFLOW or NONLINEAR on the core | our choice |
| SIMBAD | **Changed** | `simbad.cds.unistra.fr` returned 404 "page currently unavailable" for TAP and sim-id. Mirror `simbad.harvard.edu/simbad/sim-tap/sync` works. SIMBAD host is a config value. SIMBAD positions are ICRS at epoch J2000 | live |
| Gaia | Confirmed | `https://gea.esac.esa.int/tap-server/tap/sync`, table `gaiadr3.gaia_source`, `ref_epoch` 2016.0 | live |

### Risk R-1: cutout download time

Barnard's Star has 386 images (3 epochs × 6 detectors × ~20 exposures). At 8
parallel downloads and 6–16 s per cutout, a cold view takes about 5–13 min, over
the 5-minute job limit (B3). Mitigations already in the spec: D5 cutout cache
and B11 pre-computed featured targets. Requests can also be narrowed with the
`detectors` and `epochs` options. To revisit after Phase 1 timing.

## G2 — targets checked

First exposure per (epoch, detector), 2′ cutout at the target's position on that
date. Clean = detected at SNR ≥ 5 with no masked flag in the core (r ≤ 1.5 px).
Pass = at least 2 epochs, with a clean detection in at least 2 epochs.

| Target | Images | Epochs | Clean epochs | Result | Why |
|---|---|---|---|---|---|
| Barnard's Star | 364 | 3 | 0 | **Fail** | Core saturated in every detector |
| WISE 0855 | 258 | 3 | 3 | **Pass** | Clean in D1–D5; D6 not detected |
| Epsilon Indi B | 466 | 4 | 4 | **Pass** | Clean in all detectors (D4 core flagged in 2 epochs) |
| RW Aurigae | 336 | 3 | 1 | **Fail** | Saturated in most detectors |
| R Draconis | 2510 | 2 | 0 | **Fail** | Core saturated in every detector |

Findings:
- SPHEREx saturates on bright near-infrared stars. All three stars brighter than
  about K = 5 fail; both faint brown dwarfs pass. Featured targets must be faint
  in the near infrared.
- IRSA refuses some cutouts with HTTP 422 (one Epsilon Indi B image). Compute
  must skip such an image, not fail the view.
- Fast movers are far from their J2000 position by SPHEREx's dates: Barnard's Star
  by about 4.5′, more than a 3′ field. Featured positions are propagated to the
  mean image date (`seen_ra`, `seen_dec` in g2.json).

## G3 — science proven: **awaiting a ruling**

WISE 0855, 6′ field, 263 images, 3 epochs × 6 detectors, co-added per (epoch,
detector) on a 6.2″ TAN grid, one stretch per detector. Residual = median offset
of isolated Gaia DR3 stars (G < 16, propagated to the frame date) from their
centroids. Threshold 1.2″ (0.2 px).

| Epoch | D1 | D2 | D3 | D4 | D5 | D6 |
|---|---|---|---|---|---|---|
| 0 (2025-05) | too few stars | 0.83″ | 0.67″ | 1.04″ | 0.78″ | **1.44″** |
| 1 (2025-11) | 0.48″ | 0.66″ | 0.76″ | 0.76″ | 0.95″ | 0.59″ |
| 2 (2026-05) | 0.65″ | 0.73″ | 1.07″ | 0.89″ | 0.94″ | 0.65″ |

- 16 of 17 measured frames pass; median residual 0.76″ (0.12 px).
- Epoch 0 D6 fails at 1.44″. It is the weakest frame: 3 exposures, 3 stars.
- Every frame is offset north (dy +0.5″ to +1.0″, all positive): a systematic
  effect, not noise. Not yet explained.
- Only 3–5 usable Gaia stars per frame (11 with G < 16 in the field), so each
  residual rests on few stars.
- Visual check: field stars hold position across epochs and detectors.

Open question: does G3 require every frame under the threshold, or is it met when
alignment works and frames above it are flagged `low_alignment_quality` (S3)?

## G4 — demo set frozen: **not met**

Needs 3–5 targets passing G2; 2 pass (WISE 0855, Epsilon Indi B). The
requirements say a failing target is replaced. Replacements not chosen yet.

## Frame images are served directly from storage (2026-11-13)

The gateway used to stream every frame PNG from object storage to the client.
Now each frame in a view carries a public `url` (`FRAMES_BASE_URL` + object key)
and the client loads it straight from storage or a CDN. The gateway no longer
reads object storage or holds its credentials; only compute writes to it.
`GET /views/{id}/frames/{frame_id}.png` is removed from the contract.
Object keys never change what they point to, so these URLs can be cached forever.
Deploy needs: public read on the `views/` prefix (or a CDN with access to it).
