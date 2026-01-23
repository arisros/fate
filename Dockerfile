FROM golang:1.24-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOFLAGS=-mod=mod go build -trimpath -ldflags="-s -w" -o /out/fate-studio ./cmd/fate-studio

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/fate-studio /fate-studio
ENV FATE_STUDIO_ADDR=:8090
EXPOSE 8090
ENTRYPOINT ["/fate-studio"]
