# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Multi-stage build: web client -> Go binary -> tiny runtime with FFmpeg.

# Build stages run on the build machine's own architecture and cross-compile,
# so multi-arch images (amd64, arm64, armv7) don't build under emulation.
FROM --platform=$BUILDPLATFORM node:24-alpine@sha256:8a32c8cf649a49915a66e81e7a609223c708d0adabf288ee3f4077d765542f85 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci --ignore-scripts --no-audit --no-fund
COPY web/ ./
# The legal notices' wording, shared with the server.
COPY internal/legal/ /src/internal/legal/
# The guides become the in-app help.
COPY docs/ /src/docs/
COPY CHANGELOG.md SECURITY.md /src/
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build
ARG VERSION=dev
ARG TARGETOS TARGETARCH TARGETVARIANT
WORKDIR /src
ENV GOWORK=off
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN VERSION="${VERSION#v}"  && GOARM="${TARGETVARIANT#v}" CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH"     go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/wittymediaserver ./cmd/wittymediaserver

# Debian, as game cores are prebuilt for glibc: on Alpine the game runner can't even
# start, and a compatibility layer leaves several cores unable to load. libgl1 is
# linked by a few software-rendered cores that never use it.
FROM debian:trixie-slim@sha256:f6b08b09a05725f9431502d3b5714f5c4576e82c1a84fa1debe0c24a7e0c3ec4
ARG TARGETARCH VERSION=dev
LABEL org.opencontainers.image.title="WittyMediaServer" \
      org.opencontainers.image.description="Your own media server: films, TV, music, photos, books, DVD and Blu-ray menus, and games" \
      org.opencontainers.image.source="https://github.com/wittywilla/WittyMediaServer" \
      org.opencontainers.image.documentation="https://github.com/wittywilla/WittyMediaServer/blob/master/docs/DOCKER.md" \
      org.opencontainers.image.licenses="AGPL-3.0-or-later" \
      org.opencontainers.image.version="${VERSION}"
# Manuals, translations of programs' messages and changelogs are no use here. Each
# package's copyright file stays, as its licence asks.
RUN printf '%s\n' 'path-exclude /usr/share/doc/*' 'path-include /usr/share/doc/*/copyright' \
      'path-exclude /usr/share/man/*' 'path-exclude /usr/share/info/*' 'path-exclude /usr/share/lintian/*' \
      'path-exclude /usr/share/locale/*' 'path-include /usr/share/locale/locale.alias' \
      > /etc/dpkg/dpkg.cfg.d/wms-slim
# Debian's FFmpeg has VA-API, Quick Sync and NVENC built in, but not the drivers they
# load, which are only recommended packages. On amd64 (Intel and AMD graphics):
# mesa-va-drivers for AMD, Intel's media driver for 5th generation (Broadwell) and newer
# with its encoding kernels from non-free, i965 with its shaders for older chips, and
# the oneVPL runtime for Quick Sync. vainfo is for checking by hand. NVIDIA's libraries
# come from the host through the NVIDIA Container Toolkit, when the container starts.
RUN set -eux; \
    gpu=""; \
    if [ "$TARGETARCH" = amd64 ]; then \
      sed -i 's/^Components: main$/Components: main non-free/' /etc/apt/sources.list.d/debian.sources; \
      gpu="mesa-va-drivers intel-media-va-driver-non-free i965-va-driver-shaders libmfx-gen1.2 vainfo"; \
    fi; \
    apt-get update; \
    apt-get install -y --no-install-recommends ffmpeg ca-certificates tzdata tini libgl1 $gpu; \
    rm -rf /var/lib/apt/lists/* /var/cache/debconf/*-old /var/log/apt /var/log/dpkg.log; \
    groupadd -g 1000 wms && useradd -u 1000 -g wms -m wms; \
    mkdir -p /config /media && chown wms:wms /config && chmod 0777 /config
COPY --from=build /out/wittymediaserver /usr/local/bin/
# It runs as 1000:1000, and any other user works (user: in compose): nothing needs a
# name in /etc/passwd, and /config is open to whoever runs it, so a new named volume
# works for any user too. The server makes its own folders inside it private.
# The graphics drivers keep a cache under XDG_CACHE_HOME, which has to be writable by
# that user, with a read-only root too.
ENV WMS_DOCKER=1 \
    WMS_DATA_DIR=/config \
    WMS_LISTEN=:9797 \
    WMS_TRANSCODE_DIR=/config/transcode \
    XDG_CACHE_HOME=/config/cache/xdg \
    NVIDIA_DRIVER_CAPABILITIES=compute,video,utility
EXPOSE 9797
VOLUME ["/config"]
USER wms
# The server checks itself, on the port in WMS_LISTEN, so the image needs no HTTP client.
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["wittymediaserver", "health"]
ENTRYPOINT ["/usr/bin/tini", "--", "wittymediaserver"]
CMD ["serve"]
