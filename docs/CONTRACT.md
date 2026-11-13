# CONTRACT — shared by all 3 repos (copy into each)

Source of truth for anything two repos must agree on. Change here first, then bump the version.
Version: 0.2

## Flow
client → gateway (public, Go/Gin) → compute (private, Python/FastAPI)
- Client calls gateway only. Compute is never public.
- Gateway owns state (Postgres, Redis). Compute is stateless.
- Images never pass through the gateway: compute writes frame PNGs to object storage,
  the gateway returns each frame's public `url`, and the client loads it directly
  (from storage or a CDN in front of it).

## Terms
- target: ICRS RA/Dec in degrees, optional proper motion + reference epoch
- fov: cutout size, arcmin. min 1, default 3, max 30. Above max → reject
- epoch: one SPHEREx pass over the target. Images grouped by t_min; new epoch when gap > 30 days (config)
- frame: one rendered image for (epoch, detector, mode)
- blink: the time-ordered frame sequence. Never call it "video"
- candidate: source with no catalogue match. Never call it "discovery"

## Public API — gateway, prefix /api/v1
Phase 1
- GET /resolve?name= → ra, dec, pm, parallax, type, links
- GET /autocomplete?q= → featured target suggestions
- GET /objects/{slug} → featured target: slug, name, type, ra, dec
- GET /coverage?ra=&dec= → epochs, releases. No coverage = 200 with covered:false
- POST /views → 200 + result if cached, else 202 {job_id, status_url}
- GET /views?ra=&dec=&fov=&mode= → same cache key as POST /views
- GET /jobs/{id} → status (queued|running|done|failed), progress 0-100, result or error
- GET /views/{id} → epochs, frames, stretch, release_set, newer_data_available, warnings
  (release_set: the releases the view was built from, [{name, collection}];
  newer_data_available: a release it was not built from now covers the target)
- GET /healthz, /readyz, /metrics
Phase 2
- GET /views/{id}/blink.gif (Should)
Phase 3
- GET /views/{id}/sources (filters: type, unknown_only, quality; sort; limit/offset)
- GET /views/{id}/sources/{source_id}
- GET /lightcurve?ra=&dec=&detector=
- GET /track?name= or ?ra=&dec=&pmra=&pmdec=

## POST /views body
- target: {name} or {ra, dec}
- fov_arcmin: 1-30, default 3
- mode: blink | difference
- detectors: subset of [1..6]
- epochs: {from, to} optional dates
- difference: {a, b} epoch indices (mode=difference)
- tracking: bool
- stretch: {type: asinh|log, pmin, pmax}
- analysis: {sources, crossmatch, sso_check} bools

## Shared shapes
Error (every non-2xx): {error: {code, message, details}}

Frame: id, epoch_index, date, mjd, detector, wavelength_min_um, wavelength_max_um,
center_ra, center_dec, pixel_scale_arcsec, width, height, orientation_deg,
alignment_residual_arcsec, warnings[], url (public PNG URL)

Source: id, ra, dec, type, catalog_ids, links[], is_candidate,
pixel_positions[{frame_id, x, y}], motion{value, direction, uncertainty, detected}, warnings[]

Light curve point: mjd, detector, wavelength_um, flux, error, is_upper_limit, comparable_group

Warnings (fixed vocabulary): low_alignment_quality, not_comparable,
catalog_check_incomplete, saturated, upper_limit

## Internal API — compute, called only by gateway
- Header X-Internal-Secret required on every call. Header X-Request-ID propagated and logged
- POST /internal/resolve, /internal/coverage, /internal/process, /internal/lightcurve, /internal/track
- GET /internal/collections
- GET /openapi.json (published with every tagged release)
- /internal/process reports progress (polling or callback URL)

## Cache key
Hash of: coords rounded to 0.1 arcsec, fov, mode, options, data release set.

## Versioning
- Every repo tags releases
- gateway pins a compute schema version; client pins a gateway schema version
- Each repo's build runs a contract test against its pinned schema
