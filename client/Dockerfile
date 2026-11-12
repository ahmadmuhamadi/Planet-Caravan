# syntax=docker/dockerfile:1

# ---- build: static SPA into /app/build ----
FROM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json .npmrc ./
RUN --mount=type=cache,target=/root/.npm npm ci --ignore-scripts
COPY . .
# Public, build-time only (H10: never put secrets here; they would ship to browsers).
ARG PUBLIC_GATEWAY_URL=http://localhost:8080
ENV PUBLIC_GATEWAY_URL=${PUBLIC_GATEWAY_URL}
RUN npm run build

# ---- runtime: unprivileged nginx serving static files ----
FROM nginxinc/nginx-unprivileged:1.29-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/build /usr/share/nginx/html
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/"]
