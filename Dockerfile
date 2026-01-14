FROM golang:1.24-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOFLAGS=-mod=mod go build -trimpath -ldflags="-s -w" -o /out/scs-web ./cmd/scs-web

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/scs-web /scs-web
ENV SCS_WEB_ADDR=:8090
EXPOSE 8090
ENTRYPOINT ["/scs-web"]
