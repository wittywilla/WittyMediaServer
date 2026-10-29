# syntax=docker/dockerfile:1.7
# Multi-stage build: web client -> Go binary -> tiny runtime with FFmpeg.

FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci --no-audit --no-fund || npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/wms ./cmd/wms

FROM alpine:3.21
RUN apk add --no-cache ffmpeg ca-certificates tzdata tini \
 && addgroup -g 1000 wms && adduser -D -u 1000 -G wms wms \
 && mkdir -p /config /media && chown wms:wms /config
COPY --from=build /out/wms /usr/local/bin/wms
ENV WMS_DATA_DIR=/config \
    WMS_LISTEN=:9797 \
    WMS_TRANSCODE_DIR=/config/transcode
EXPOSE 9797
VOLUME ["/config"]
USER wms
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD wget -qO- http://127.0.0.1:9797/api/health || exit 1
ENTRYPOINT ["/sbin/tini", "--", "wms"]
CMD ["serve"]
