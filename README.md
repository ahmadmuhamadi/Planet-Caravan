# Planet-Caravan — SPHEREx Sky Explorer

Enter an object name or coordinates and watch that patch of sky change over time in SPHEREx data.
Requirements and specs live in [`docs/`](docs/); start with [`docs/CONTRACT.md`](docs/CONTRACT.md).

## Layout

| Directory              | Role                                                        | Stack                  |
| ---------------------- | ----------------------------------------------------------- | ---------------------- |
| [`client/`](client/)   | One-page web viewer. Calls the gateway only.                | SvelteKit (static SPA) |
| [`gateway/`](gateway/) | Public API: validation, jobs, cache, rate limits, serving.  | Go, Gin                |
| [`compute/`](compute/) | Internal, stateless astronomy pipeline. Never public.       | Python, FastAPI        |

Flow: `client → gateway (public) → compute (private)`.

## Prerequisites

| Tool | Version                                            | Used by |
| ---- | -------------------------------------------------- | ------- |
| Go   | from `gateway/go.mod` (auto-downloaded by `go`)    | gateway |
| uv   | 0.11+ (installs Python 3.14 from `.python-version`) | compute |
| Node | 24 (`client/.nvmrc`; ≥ 22.12 works)                | client  |

## Run locally

```bash
# gateway  → http://localhost:8080/health
cd gateway && go run .

# compute  → http://localhost:8000/health
cd compute && uv sync && uv run uvicorn main:app --reload

# client   → http://localhost:5173
cd client && cp .env.example .env && npm ci && npm run dev
```

## Checks (same commands CI runs)

```bash
# gateway
cd gateway && golangci-lint run && go test -race ./... \
  && go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

# compute
cd compute && uv run ruff check . && uv run ruff format --check . \
  && uv run mypy && uv run pytest && uv run pip-audit --strict

# client (component tests need: npx playwright install chromium)
cd client && npm run lint && npm run check && npm test && npm run build
```

## CI/CD

- **[CI](.github/workflows/ci.yml)** runs on every pull request and push to `main`. Only the
  components whose files changed are checked: lint, format, type check, tests, vulnerability scan,
  and a Docker image build. Require the **`CI OK`** check in branch protection.
- **[Release](.github/workflows/release.yml)** runs on a `vX.Y.Z` tag. It runs the full CI, then
  publishes `ghcr.io/<owner>/planet-caravan-{gateway,compute,client}` images with SBOM and
  provenance. Set the repository variable `PUBLIC_GATEWAY_URL` first; the client build needs it.
- **Deploy** is not wired yet. The hosting target (AWS or GCP) gets recorded in `decisions.md` (H8)
  first.
- **[Dependabot](.github/dependabot.yml)** opens grouped weekly updates for actions, Go, Python,
  npm, and base images.

Secrets come from the environment only (H10). Commit `*.example` files, never `.env`.
