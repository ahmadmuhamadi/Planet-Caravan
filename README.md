# Planet Caravan

| Directory  | What                        | Stack     |
| ---------- | --------------------------- | --------- |
| `client/`  | Web viewer                  | SvelteKit |
| `gateway/` | Public API                  | Go, Gin   |
| `compute/` | Internal service (private)  | FastAPI   |

Flow: `client → gateway → compute`. Specs are in `docs/`.

## Run

```bash
cd gateway && go run ./cmd/gateway                      # :8080/health (env: see gateway/cmd/gateway/config.go)
cd compute && uv sync && uv run uvicorn app.main:app --reload  # :8000/health
cd client && npm ci && npm run dev                      # :5173
```

## CI/CD

- **CI** (every PR and push to `main`): lint, test, Docker build for each component.
- **Release** (push a `v*` tag): runs CI, then pushes images to
  `ghcr.io/<owner>/planet-caravan-<component>:<tag>`. Set the repo variable
  `PUBLIC_GATEWAY_URL` for the client build.
