# SPEC — client repo (web viewer)

Read CONTRACT.md first. Starts after gateway Phase 1 is accepted.

## Role
One page. Search a target, watch its blink, share the link. Calls the gateway only.

## Stack
TBD — fill in before giving this file to an agent.
API types generated from the pinned gateway schema (H2). Gateway base URL from config.

## Phase 2 — viewer
Search
- F1: one search box; accepts object name or coordinates; suggests featured targets (/autocomplete)
- F2: call /coverage before creating a view; covered:false → show "No SPHEREx data here yet"
Loading
- F3: poll /jobs/{id}; show progress %; featured targets load from cache with no wait
- Failed job → show the error message from the API
Blink player
- F4: play, pause, slider; each frame shows its date and wavelength range; show total epoch count
- All frames displayed at the same size and position (alignment is done server-side; do not rescale per frame)
Links
- F7: page URL always reflects the current view; copying it gives a working link
- Routes: /object/{slug} and a view route carrying ra, dec, fov, mode, frame, release set
- F8 (Should): link on an older release opens the same frames + "newer data available" notice
Warnings
- F9: show API warnings next to the frame or value they apply to:
  low_alignment_quality, not_comparable, catalog_check_incomplete, saturated, upper_limit
Wording
- P6: say "blink", not "video"; say "candidate", not "discovery"

## Phase 3 — pins and analysis
- F5 (Should): pins placed per frame from pixel_positions, so they follow moving objects; candidates use a different pin colour
- F6 (Should): clicking a pin opens a panel: name, type, motion (with uncertainty), light curve, outside links
- Light curve: plot only comparable points together; upper limits drawn differently; not_comparable shown as text, not as a change
- Difference mode toggle (epoch A, epoch B)
- Navigation = jump: clicking a pin or nearby object opens that object's page (P5)

## Tests
- Contract test against the pinned gateway schema (H3)
- URL round trip: open a copied link → same view state
- No-coverage and failed-job states render

## Deploy
- H10: no secrets in the repo; only the public gateway URL in config
- H11: pipeline = test, build, deploy on tag
- The deployed domain must be added to the gateway's CORS list (H6)

## Do not
- Call compute directly
- Add accounts, free panning, or a whole-sky view (F10)
- Build watch list, voting, or candidate submission (Later)
