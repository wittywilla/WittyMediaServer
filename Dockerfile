# syntax=docker/dockerfile:1.7
# Multi-stage build: web client -> Go binary -> tiny runtime with FFmpeg.

# Build stages run on the build machine's own architecture and cross-compile,
# so multi-arch images (amd64, arm64, armv7) don't build under emulation.
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci --ignore-scripts --no-audit --no-fund
COPY web/ ./
# The legal notices' wording, shared with the server.
COPY internal/legal/ /src/internal/legal/
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS build
ARG VERSION=dev
ARG TARGETOS TARGETARCH TARGETVARIANT
WORKDIR /src
ENV GOWORK=off
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN VERSION="${VERSION#v}"  && GOARM="${TARGETVARIANT#v}" CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH"     go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/wms ./cmd/wms

# Debian, as game cores are prebuilt for glibc: on Alpine the game runner can't even
# start, and a compatibility layer leaves several cores unable to load. libgl1 is
# linked by a few software-rendered cores that never use it.
FROM debian:trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg ca-certificates tzdata tini wget libgl1 \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd -g 1000 wms && useradd -u 1000 -g wms -m wms \
 && mkdir -p /config /media && chown wms:wms /config
COPY --from=build /out/wms /usr/local/bin/
ENV WMS_DOCKER=1 \
    WMS_DATA_DIR=/config \
    WMS_LISTEN=:9797 \
    WMS_TRANSCODE_DIR=/config/transcode
EXPOSE 9797
VOLUME ["/config"]
USER wms
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD wget -qO- http://127.0.0.1:9797/api/health || exit 1
ENTRYPOINT ["/usr/bin/tini", "--", "wms"]
CMD ["serve"]
