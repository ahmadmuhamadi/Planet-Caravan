# client

Planet Caravan web viewer. SvelteKit, built as a static SPA (`200.html` fallback).

```bash
npm ci
npm run dev     # dev server
npm run lint    # prettier + eslint
npm run check   # types
npm test        # vitest (first time: npx playwright install chromium)
npm run build   # → build/
```

Config: `PUBLIC_GATEWAY_URL` (see `.env.example`). For `npm run dev`, put it in `client/.env`:
set only in the shell, it was lost after a hot reload and the client fell back to `localhost:8080`.

## Routes

- `/` search, with featured targets
- `/object/{slug}?fov=&mode=&det=&frame=&id=` a featured target
- `/search?ra=&dec=&name=&fov=&mode=&det=&frame=&id=` any position (degrees); `name` is what a search resolved

The address bar always holds the view on screen, so copying it shares that view.
`det` is the detector, `frame` the 1-based frame (set once paused), and `id` the
view, which reopens the same frames after a newer data release (F8).

## Gateway API types

Types are generated from the pinned gateway schema, `schema/gateway.openapi.json` (H2).
After changing it, run `npm run gen:api` and commit both files. `src/lib/contract.spec.ts`
fails when they differ (H3).
