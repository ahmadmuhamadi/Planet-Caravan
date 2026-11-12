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

Config: `PUBLIC_GATEWAY_URL` (see `.env.example`).
