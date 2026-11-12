# Decisions

Phase 0 gate record (G1–G4) and later design decisions. Each external fact is
**Confirmed** (checked against a live service or the docs) or **Assumed** (our
choice, kept in compute config). All checks below were made on 2026-11-13.

Script: `compute/gates/gates.py` (`g1`, `g2`, `g3`). Reproduce with
`cd compute && uv run python gates/gates.py g1`.

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
