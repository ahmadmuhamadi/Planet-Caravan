# SPHEREx Sky Explorer: Requirements (risk-driven revision)

Nov 12, 2026 · @Ahmed Muhammadi

## 1. Purpose and scope

SPHEREx Sky Explorer lets anyone enter an object name or coordinates and watch that patch of sky change over time in SPHEREx data. This revision keeps the original architecture but turns each known risk into a requirement that can be checked.

**Finished means:** the gates in section 3 are passed, phases 1 to 3 in section 10 are built, and every acceptance criterion in section 9 holds for at least three confirmed targets.

**In scope**

- Search by object name or coordinates.
- Aligned, time-ordered frames (a "blink") for the patch around the target.
- Pins on known objects, with links to outside catalogues.
- A direct link for every object and every view.
- A small set of analysis: difference frames, light curve, motion track.
- A minimal viewer page, so the tool can be shown and used.

**Out of scope**

- Free panning across the whole sky, and any whole-sky tile layer.
- User accounts, email or push notifications.
- A survey-scale source catalogue of our own.
- Any claim of a confirmed discovery. The tool reports candidates only.

**Priority labels.** Each requirement is marked Must, Should, or Later. Must items define "finished". Later items are documented as design only.

## 2. Product concept

The tool works like a map search, not a map: the user looks up a place and gets a page for it, and every page has a shareable link.

| ID | Requirement | Priority |
| --- | --- | --- |
| P1 | The entry point is one search box that accepts an object name or coordinates. | Must |
| P2 | The result is a target page showing the patch around the target with a time player. | Must |
| P3 | Known objects in the patch appear as pins. Clicking a pin opens its details and outside links. | Should |
| P4 | Every object has a readable link (for example `/object/barnards-star`). Every view has a link carrying position, field size, mode, frame, and data release set. | Must |
| P5 | Moving around means jumping to another target: a new search, a clicked pin, or a changed field size. There is no continuous pan. | Must |
| P6 | The frame sequence is called a "blink", and unmatched sources are called "candidates", everywhere in the product and the docs. | Must |

## 3. Gates before building

No service code is written until all four gates pass. They remove the three risks that could sink the project: wrong data facts, unusable targets, and science that does not work.

| ID | Gate | Pass condition | Priority |
| --- | --- | --- | --- |
| G1 | Data facts verified | Each external fact is recorded in `decisions.md` as Confirmed or Assumed, with the source page and the date checked. This covers collection names, result column names, cutout parameter units, detector-to-band order, and flag bit meanings. | Must |
| G2 | Targets checked | For each acceptance target the record shows: number of epochs, detectors covering it, whether it is detected above the noise in a single exposure, and whether its core pixels are saturated or flagged. A target stays only with at least 2 epochs and a clean detection. Otherwise it is replaced. | Must |
| G3 | Science proven | One target is taken by hand from cutouts to aligned frames with a shared stretch. The alignment residual is measured against Gaia stars in the field and is at or below the threshold (proposed: 0.2 pixel, about 1.2″). | Must |
| G4 | Demo set frozen | Three to five featured targets that passed G2 are named. These are the targets used for acceptance, pre-computation, and the video. | Must |

If G3 fails, the project stops and the alignment method is rethought before anything else is built.

## 4. Data source requirements

The system must keep working when an outside service is slow, down, or different from what the original spec assumed.

| ID | Requirement | Priority |
| --- | --- | --- |
| D1 | SPHEREx collections are discovered at runtime. No release name is fixed in the code. | Must |
| D2 | Every external fact marked Assumed in G1 lives in configuration, not in code, and is listed in the README. | Must |
| D3 | Only cutouts are downloaded. Full detector frames are never fetched for a user request. | Must |
| D4 | Every outside call has its own timeout and retries with backoff and jitter. A job runs at most 8 downloads in parallel. | Must |
| D5 | Raw cutouts are cached by observation ID, centre, and size, so a repeated or failed job does not download them again. | Must |
| D6 | An image that appears in several releases is used once, from the newest release. | Must |
| D7 | If a catalogue service (SIMBAD, Gaia, SkyBoT, Horizons) fails, the view still returns, marked "catalogue check incomplete". No source is labelled unknown while a check is incomplete. | Must |
| D8 | Tests use recorded responses from the outside services, so the test suite runs offline. | Must |

## 5. Science pipeline requirements

The pipeline must never show a change that is really an artefact of wavelength, alignment, or display. Where it cannot tell, it says so.

| ID | Requirement | Priority |
| --- | --- | --- |
| S1 | The zodiacal model is subtracted from every image, and flagged pixels are masked. The default flag bits are listed in the README. | Must |
| S2 | All frames in a view are placed on one common grid centred on the target. | Must |
| S3 | Alignment is checked per frame against Gaia stars moved to the frame's date. The residual is stored in the view metadata. A frame above the threshold is marked "low alignment quality". | Must |
| S4 | The wavelength at the target pixel is recorded for every image. | Must |
| S5 | Brightness is compared only between measurements whose wavelengths fall in the same bin (bin width configurable). With no matching pair, the result is "not comparable", never a change. This applies to light curves, difference frames, and variability scores. | Must |
| S6 | Each displayed frame is labelled with the wavelength range of the images that went into it. | Must |
| S7 | One display stretch is computed from the reference epoch and applied to every frame in the view. Difference frames use a stretch symmetric around zero. | Must |
| S8 | Every flux has an error taken from the variance data. A brightness change is called significant only above a configurable threshold (proposed: 5 times its error). | Must |
| S9 | Motion is always reported with its uncertainty. It is called detected only when the shift exceeds a configurable multiple (proposed: 3) of the combined alignment residual and centroid error. | Must |
| S10 | A target with saturated or flagged core pixels is marked and left out of photometry and motion fits. | Must |
| S11 | When the target is not detected in an epoch, the result is an upper limit, not a data point. | Should |
| S12 | A source becomes a candidate only if it is seen in at least 2 epochs, sits on no flagged pixels, has no catalogue or solar-system match, and all catalogue checks completed. | Should |
| S13 | In tracking mode, each frame is centred on the position predicted for its date. | Should |

## 6. Backend and API requirements

The gateway (Go) handles jobs, caching, and serving. The compute service (Python) does the astronomy. The gateway stays thin until phase 3 is accepted.

| ID | Requirement | Priority |
| --- | --- | --- |
| B1 | In phases 1 to 3 the gateway only validates requests, runs jobs, caches, rate-limits, and serves results. Watch list, candidates, and voting tables are not built yet. | Must |
| B2 | Creating a view returns at once: the cached result, or a job ID with a status link. Job status reports queued, running, done, or failed, with a progress percentage. | Must |
| B3 | A job has a global time limit (default 5 minutes). A failed job returns an error code and a plain message. | Must |
| B4 | Identical requests that arrive while a job is running share that one job. | Must |
| B5 | The cache key is built from normalised parameters: coordinates rounded to 0.1″, field size, mode, options, and the data release set. | Must |
| B6 | The gateway's data types are generated from the compute service's OpenAPI schema, or a contract test fails the build when the two differ. | Must |
| B7 | Each frame's metadata gives its centre, pixel scale, size, and orientation. Each source gives its sky position and its pixel position in every frame. | Must |
| B8 | A fixed list maps readable object names to the featured targets. Other names are resolved through SIMBAD. | Must |
| B9 | The coverage endpoint returns an explicit "no coverage" result for an empty position. It is not an error. | Must |
| B10 | A view result names the release set it was built from and carries a "newer data available" flag. | Should |
| B11 | Featured targets are computed ahead of time and re-computed when a new release appears. | Must |
| B12 | A frame sequence can be exported as a GIF. | Should |
| B13 | All errors use one format: code, message, details. | Must |

## 7. Frontend requirements

The frontend is one page that makes the wait, the small number of frames, and the data limits clear to the user.

| ID | Requirement | Priority |
| --- | --- | --- |
| F1 | The search box accepts names and coordinates and suggests the featured targets. | Must |
| F2 | Coverage is checked before any processing starts. An empty position shows "No SPHEREx data here yet". | Must |
| F3 | While a job runs, the page shows its progress from the job status. Featured targets load from cache without a wait. | Must |
| F4 | The blink player has play, pause, and a slider. Each frame shows its date and wavelength range, and the player shows the total number of epochs. | Must |
| F5 | Pins are placed per frame from the per-frame positions, so they follow moving objects. Candidates use a different pin colour. | Should |
| F6 | Clicking a pin opens a panel with name, type, motion, light curve, and outside links (SIMBAD, NED, JPL, Gaia). | Should |
| F7 | The page address always reflects the current view, so copying it gives a working direct link. | Must |
| F8 | A link built on an older release opens the same frames and shows a "newer data available" notice. | Should |
| F9 | Quality warnings from the backend are shown next to the frame or value they apply to: low alignment quality, not comparable, catalogue check incomplete, saturated. | Must |
| F10 | The page has no accounts, no panning, and no whole-sky view. | Must |

## 8. Reliability, performance and abuse protection

A public tool with expensive jobs needs limits from the first release, and the demo must not depend on IRSA being reachable.

| ID | Requirement | Priority |
| --- | --- | --- |
| R1 | Field size is limited: minimum 1′, default 3′, maximum 30′. Requests above the maximum are rejected. | Must |
| R2 | Requests are rate-limited per IP address, with a stricter limit on endpoints that create jobs and a cap on jobs running at once per address. | Must |
| R3 | Inputs are validated: RA 0 to 360, Dec −90 to 90, field size within limits, name length capped. | Must |
| R4 | The compute service is not reachable from the public network and accepts only calls carrying the shared secret. | Must |
| R5 | Featured targets can be served entirely from cache and storage while IRSA is unreachable. | Must |
| R6 | Rendered files for non-featured views expire after a configurable time, and total storage has a size cap. | Should |
| R7 | Logs are structured and carry one request or job ID across both services. Metrics cover job duration, cache hit rate, and IRSA latency and errors. | Should |
| R8 | Voting and candidate submission are Later. If built, each client token gets one vote per candidate and submissions are rate-limited per IP address. | Later |

## 9. Acceptance criteria

The project is accepted when every item below holds for the featured targets frozen in G4.

- [ ] Gates G1 to G4 are passed and recorded in `decisions.md`.
- [ ] A fresh request for each featured target returns a blink view within the job time limit.
- [ ] A cached request for each featured target returns in under 2 seconds (proposed).
- [ ] Every frame reports its alignment residual, and featured targets are at or below the threshold.
- [ ] One fast-moving target (Barnard's Star, or its replacement from G2) shows a measured motion that agrees with Gaia within the stated uncertainty.
- [ ] One variable target (R Draconis or RW Aurigae, or a replacement) shows a light curve built only from wavelength-matched points. It shows a significant change, or the docs state that it is not measurable yet.
- [ ] Coordinates with no coverage return the "no data here yet" result.
- [ ] With IRSA unreachable, every featured target still loads.
- [ ] Opening a copied direct link reproduces the same frames.
- [ ] The test suite passes with no network access.

WISE 0855 and Epsilon Indi B stay on the list only if they pass G2. Neither has been checked yet.

## 10. Delivery phases and cut line

The work runs in five phases, and a phase starts only when the one before it is accepted. "Finished" is the end of phase 3.

1. **Phase 0: Gates.** G1 to G4. Output: `decisions.md` and the featured target list.
2. **Phase 1: Core backend.** Resolve, coverage, blink views, per-detector co-add, shared stretch, alignment residuals, jobs, cache, limits (D1 to D8, S1 to S7, and the Must items of sections 6 and 8).
3. **Phase 2: Viewer and links.** The one-page viewer, direct links, readable object names, pre-computed featured targets (section 7 Must items, plus B8 and B11).
4. **Phase 3: Selected analysis.** Difference frames, light curve, motion track, and pins with catalogue matches (S8 to S13, F5, F6).
5. **Phase 4: Later, design only.** Watch list, scheduler, change summaries, candidates, voting, stack and RGB modes, spectrum, older surveys.

If time runs short, cut from the bottom of phase 3 upward. Phases 0 to 2 are never cut.

## 11. Risk to requirement traceability

Every risk identified so far is covered by at least one requirement and one check.

| Risk | Requirements | How it is checked |
| --- | --- | --- |
| Brightness changes that are really wavelength differences | S4, S5, S6, F4 | Variable-target acceptance item; unit test with two wavelengths and no real change |
| Alignment errors that look like motion | G3, S2, S3, S9 | Residual reported per frame; fast-mover acceptance item |
| Acceptance targets too faint, saturated, or lacking epochs | G2, G4, S10, S11 | Target record in `decisions.md` |
| Wrong or changed external data facts | G1, D1, D2 | Confirmed or Assumed status with source and date |
| IRSA slow or unreachable | D4, D5, B3, B11, R5 | "IRSA unreachable" acceptance item |
| A catalogue service fails and sources look unknown | D7, S12, F9 | Test with a recorded catalogue failure |
| False discoveries | S12, P6, F9 | Candidate rules tested on synthetic data |
| Scope too large for one person | Section 1 priority labels, B1, section 10 cut line | Phase acceptance before the next phase starts |
| Gateway and compute types drift apart | B6 | Contract test in the build |
| Heavy or abusive public use | R1, R2, R3, R6, R8 | Limit tests |
| Long first load frustrates users | B2, B11, F3 | Cached-request acceptance item |
| Only a few frames per target | P6, F4 | Epoch count and dates shown in the player |
| Pins drift off moving objects | B7, F5, S13 | Fast-mover view checked by eye across frames |
| Shared links change after a new release | B5, B10, F7, F8 | Direct-link acceptance item |
| Position has no SPHEREx coverage | B9, F2 | No-coverage acceptance item |
| Nothing to show without a frontend | P1, P2, section 7, B12 | Viewer page present at end of phase 2 |

## 12. Open assumptions

These items are not yet confirmed. The first seven are settled by gates G1 and G2; the rest are proposed values or open questions.

- [ ] The SPHEREx collection names and the result column names on IRSA.
- [ ] The units of the cutout centre and size parameters.
- [ ] Which detector number maps to which wavelength band.
- [ ] Which flag bits should be masked by default.
- [ ] How many epochs each acceptance target has today.
- [ ] Whether WISE 0855 is bright enough to detect in a single exposure.
- [ ] Whether Barnard's Star, RW Aurigae, and R Draconis saturate.
- [ ] The alignment threshold (proposed 0.2 pixel).
- [ ] The significance thresholds (proposed 5 times the error for brightness, 3 times for motion).
- [ ] The wavelength bin width for brightness comparison.
- [ ] The cached-response time target (proposed 2 seconds).
- [ ] The challenge deadline and what the submission must contain.
