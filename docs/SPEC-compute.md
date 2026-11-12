# SPEC — compute repo (FastAPI, astronomy pipeline)

Read CONTRACT.md first. Work one phase at a time. Do not start a phase before the previous one passes.

## Role
Stateless internal service. Does all astronomy: resolve, query, cutouts, preprocess, reproject, co-add, render, analysis. Writes files to object storage, returns metadata + object keys to gateway.

## Stack
Python, FastAPI, Pydantic models on every request/response.
astropy, astroquery (ipac.irsa, simbad, gaia, imcce, jplhorizons), pyvo, reproject, photutils, numpy, scipy, Pillow or matplotlib, httpx.
CPU-heavy steps (reproject, co-add, detect) run in a process pool, not the event loop.

## Data facts — UNVERIFIED until Phase 0. Keep every one in config, not code
- SIA v2 endpoint: https://irsa.ipac.caltech.edu/SIA with COLLECTION parameter
- Collections seen so far: spherex_qr3, spherex_qr3_deep, spherex_qr2, spherex_qr2_deep
- Discover collections: Irsa.list_collections(servicetype='SIA', filter='spherex')
- Query: Irsa.query_sia(pos=(SkyCoord, radius), collection=...)
- Columns: access_url, t_min, t_max, t_exptime, energy_bandpassname, obs_id, s_region
- Cutout: append ?center=<ra>,<dec>&size=<size> to access_url (units unconfirmed)
- Level 2 FITS HDUs: IMAGE (MJy/sr, zodi not subtracted), FLAGS, VARIANCE, ZODI, PSF, WCS-WAVE
- Per-pixel wavelength: WCS(header=hdul["IMAGE"].header, fobj=hdul, key="W"), set .sip = None, pixel_to_world(x, y) → (wavelength, bandwidth)
- Pixel scale ~6.2 arcsec. 6 detectors, bands within 0.75-5.0 um (detector→band order unconfirmed)
Docs to verify against:
- https://caltech-ipac.github.io/spherex-archive-documentation/spherex-data-access
- https://caltech-ipac.github.io/spherex-archive-documentation/spherex-data-products
- https://irsa.ipac.caltech.edu/data/SPHEREx/docs/cutout_tool.html
- https://irsa.ipac.caltech.edu/ibe/cutouts.html
- https://caltech-ipac.github.io/irsa-tutorials/spherex-intro

## Phase 0 — gates (notebook or script, no service code)
- G1: verify each data fact above. Record in decisions.md as Confirmed/Assumed + source URL + date
- G2: for each target (Barnard's Star, WISE 0855, Epsilon Indi B, RW Aurigae, R Draconis) record: epoch count, detectors, detected in single exposure?, core saturated/flagged? Keep only targets with >=2 epochs and a clean detection
- G3: one target, cutouts → aligned frames with shared stretch. Measure alignment residual vs Gaia stars. Pass: <= 0.2 pixel (~1.2 arcsec, config)
- G4: output a list of 3-5 featured targets that passed G2
- If G3 fails: stop and report. Do not continue

## Phase 1 — core pipeline
Data access
- D1: discover collections at runtime; no hardcoded release
- D2: every Assumed fact is a config value, listed in README
- D3: cutouts only; never download full frames
- D4: per-call timeout + retries with backoff and jitter; max 8 parallel downloads per job
- D5: cache raw cutouts by obs_id + center + size
- D6: image in several releases → use once, newest release
- D7: catalogue service failure → still return result with warning catalog_check_incomplete; label nothing unknown while a check is incomplete
Pipeline
- Resolve: coordinates pass through; names via SIMBAD; propagate high-proper-motion targets to each observation date
- Query all SPHEREx collections, radius = fov/2; fetch cutouts slightly larger than fov
- S1: subtract ZODI from IMAGE; mask FLAGS bits (defaults in config + README)
- S2: reproject all frames to one TAN grid centred on target (pixel scale config, default 6.2 arcsec)
- S3: per-frame alignment residual vs Gaia stars propagated to frame date; store in metadata; above threshold → low_alignment_quality
- S4: record wavelength at target pixel for every image
- S6: each frame carries wavelength_min/max of its contributing images
- Co-add per epoch per detector: inverse-variance weighted mean, masked pixels excluded
- S7: one stretch from reference epoch applied to all frames; difference frames symmetric around 0
- Render 8-bit PNG; write to object storage; return keys + Frame metadata per CONTRACT
Endpoints: /internal/resolve, /coverage, /process, /collections, /openapi.json

## Phase 3 — analysis
- Difference mode (epoch B - epoch A)
- S5: compare brightness only within the same wavelength bin (width config). No matching pair → not_comparable, never a change
- S8: every flux has an error from VARIANCE; change is significant only above threshold (default 5 sigma, config)
- S9: motion always with uncertainty; detected only if shift > 3x (config) combined alignment residual + centroid error
- S10: saturated/flagged core → warning saturated; exclude from photometry and motion fits
- S11: non-detection → upper limit, not a data point
- S12: candidate only if seen in >=2 epochs, no flagged pixels, no catalogue or SkyBoT match, all checks completed
- S13: tracking mode centres each frame on the predicted position for its date
- Source detection per epoch co-add; cross-match Gaia DR3 (propagated) + SIMBAD within 6 arcsec (config); SkyBoT check per epoch
- Source output includes pixel_positions per frame (B7)
- Endpoints: /internal/lightcurve, /internal/track

## Tests
- Unit, synthetic data, no network: epoch grouping, stretch, motion fit, cross-match, wavelength-bin rule (two wavelengths, no real change → not_comparable)
- D8: integration tests use recorded IRSA/SIMBAD/Gaia responses; suite passes offline
- Recorded catalogue failure → catalog_check_incomplete, no unknown labels

## Deploy
- H7: no public address where the host allows; otherwise firewall + shared secret
- H8: record host memory, CPU, request time limit, sleep behaviour in decisions.md before choosing it
- H10: secrets from environment only
- Dockerfile, make targets, README, structured JSON logs with request ID, Prometheus metrics (IRSA latency/errors, step durations)

## Do not
- Invent endpoints, collection names, or column names
- Build stack/rgb modes, spectrum, or older surveys (Later)
- Keep state in this service
