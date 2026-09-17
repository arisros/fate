FROM node:22-alpine AS site

WORKDIR /src

COPY docs/package.json docs/package-lock.json* ./docs/
RUN cd docs && (npm ci 2>/dev/null || npm install)

COPY .release-please-manifest.json ./
COPY docs ./docs
RUN cd docs && npm run build

FROM golang:1.24-alpine AS server

WORKDIR /src
COPY go.mod ./
COPY docs/server ./docs/server
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/fate-site ./docs/server

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=server /out/fate-site /fate-site
COPY --from=site /src/docs/.vitepress/dist /public

ENV FATE_SITE_ADDR=":8090" \
    FATE_SITE_ROOT="/public"

EXPOSE 8090
USER 65532:65532
ENTRYPOINT ["/fate-site"]
