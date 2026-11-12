# client — SPHEREx Sky Explorer viewer

One page: search a target, watch its blink, share the link. Spec: [`docs/SPEC-client.md`](../docs/SPEC-client.md).

SvelteKit built as a static SPA (`@sveltejs/adapter-static` with `200.html` fallback), so deep links
such as `/object/barnards-star` work on any static host. It talks to the public gateway only, never to compute.

## Config

| Variable             | Where                      | Notes                                                    |
| -------------------- | -------------------------- | -------------------------------------------------------- |
| `PUBLIC_GATEWAY_URL` | `.env` (dev), build arg/CI | Baked into the bundle at build time. Public values only. |

## Scripts

```bash
npm ci                 # install exactly as locked
npm run dev            # dev server on :5173
npm run lint           # prettier --check + eslint
npm run check          # svelte-check (types)
npm test               # vitest; component tests run in headless Chromium
npm run build          # static site → build/
```

Component tests need a browser once per machine: `npx playwright install chromium`.

## Container

```bash
docker build --build-arg PUBLIC_GATEWAY_URL=http://localhost:8080 -t planet-caravan-client .
docker run -p 8081:8080 planet-caravan-client   # unprivileged nginx with SPA fallback
```
