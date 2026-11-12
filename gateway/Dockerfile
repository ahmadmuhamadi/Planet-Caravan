# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.27.4-alpine AS build
WORKDIR /src

# Download modules in their own layer so code changes don't refetch them.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags="-s -w" \
      -o /out/gateway .

# ---- runtime ----
# Static, shell-less, non-root image: nothing to exploit beyond the binary.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/gateway /gateway
ENV GIN_MODE=release
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/gateway"]
