# mkpk-provision as a shared, networked instance (docs/deploy-docker.md).
#
# The image serves the admin UI over plain HTTP and expects a reverse proxy to
# terminate TLS in front of it — the container refuses to start on a non-loopback
# address without an admin password, and without MKPK_BEHIND_PROXY=1.

FROM golang:1.26-alpine AS build
WORKDIR /src
# Dependencies first so a source-only change reuses the module cache layer.
COPY client/go.mod client/go.sum ./
RUN go mod download
COPY client/ ./
ARG VERSION=dev
# CGO off: the provisioner is pure Go (x/crypto/ssh), so the result is a static
# binary that runs on any linux base. The Wails wrappers are not built here —
# they need a platform webview.
RUN CGO_ENABLED=0 go build -trimpath \
        -ldflags "-s -w -X mikrotik-psk-knock/client/internal/version.Version=${VERSION}" \
        -o /out/mkpk-provision ./cmd/mkpk-provision \
 && CGO_ENABLED=0 go build -trimpath \
        -ldflags "-s -w -X mikrotik-psk-knock/client/internal/version.Version=${VERSION}" \
        -o /out/mkpk ./cmd/mkpk

FROM alpine:3.20
# ca-certificates: outbound HTTPS for the update check and Telegram notifications.
# tzdata: readable timestamps in logs. wget (busybox) backs the healthcheck.
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 -h /data mkpk
COPY --from=build /out/mkpk-provision /out/mkpk /usr/local/bin/
COPY deploy/docker/entrypoint.sh /usr/local/bin/entrypoint.sh

# The config, the admin password record and SSH keys live here — mount a volume.
VOLUME /data
WORKDIR /data
USER mkpk
EXPOSE 8765

ENV MKPK_CONFIG=/data/mkpk.yaml \
    MKPK_ADDR=0.0.0.0:8765 \
    MKPK_BEHIND_PROXY=1

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8765/ping >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
