# Builds the documentation site published at fate.arisjirat.com.
#
# Stage 1 builds the VitePress site, stage 2 builds the static server, and the
# final image is distroless and runs as nonroot with a read-only root
# filesystem, matching the fate-site Deployment in homelab-platform.
#
# Build from the repository root, because the site config reads the version
# from .release-please-manifest.json:
#
#   docker build -f docs/Dockerfile -t fate-site:<version> .

# ---- 1. build the site -------------------------------------------------------
FROM node:22-alpine AS site

WORKDIR /src

# Install dependencies first so the layer caches across content edits.
COPY docs/package.json docs/package-lock.json* ./docs/
RUN cd docs && (npm ci 2>/dev/null || npm install)

COPY .release-please-manifest.json ./
COPY docs ./docs
RUN cd docs && npm run build

# ---- 2. build the server -----------------------------------------------------
FROM golang:1.24-alpine AS server

WORKDIR /src
COPY go.mod ./
COPY docs/server ./docs/server
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/fate-site ./docs/server

# ---- 3. runtime --------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=server /out/fate-site /fate-site
COPY --from=site /src/docs/.vitepress/dist /public

ENV FATE_SITE_ADDR=":8090" \
    FATE_SITE_ROOT="/public"

EXPOSE 8090
USER 65532:65532
ENTRYPOINT ["/fate-site"]
