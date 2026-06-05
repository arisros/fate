# Multi-stage build for the scs-web statechart studio server.
#
#   docker build -t fate-scs-web .
#   docker run --rm -p 8090:8090 fate-scs-web
#
# The engine is dependency-free, so the build stage needs no module downloads
# for the root module; the resulting image is a single static binary on
# distroless (no shell, non-root).
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOFLAGS=-mod=mod go build -trimpath -ldflags="-s -w" -o /out/scs-web ./cmd/scs-web

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/scs-web /scs-web
ENV SCS_WEB_ADDR=:8090
EXPOSE 8090
ENTRYPOINT ["/scs-web"]
