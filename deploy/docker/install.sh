#!/bin/sh
# mkpk-provision installer.
#
#   curl -fsSL <raw>/deploy/docker/install.sh | sudo sh -s -- --domain mkpk.example.com
#
# Pulls the public image from GitHub Packages by default; --registry points it at
# a private mirror of the same project instead.
#
# Downloads the compose files, generates an admin password, writes .env, starts
# the stack and prints how to get in. Re-running keeps the existing password and
# just refreshes the files.
#
# Two modes:
#   --domain <fqdn>   Caddy alongside, automatic Let's Encrypt (needs 80/443)
#   --behind-ingress  no TLS here; binds 127.0.0.1:PORT for your own proxy
set -eu

REPO_RAW="${MKPK_REPO_RAW:-https://raw.githubusercontent.com/LazyGatto/mikrotik-psk-knock/main/deploy/docker}"
DIR="${MKPK_DIR:-/opt/mkpk}"
# The image path is identical in every registry that carries this project, so a
# private mirror only changes the host: --registry gitlab.example.com:5050.
IMAGE_PATH="${MKPK_IMAGE_PATH:-lazygatto/mikrotik-psk-knock/provision}"
REGISTRY="${MKPK_REGISTRY:-ghcr.io}"
TAG="${MKPK_TAG:-latest}"
IMAGE="${MKPK_IMAGE:-}"
DOMAIN="${MKPK_DOMAIN:-}"
ACME_EMAIL="${MKPK_ACME_EMAIL:-}"
PORT="${MKPK_PORT:-8765}"
BIND="${MKPK_BIND:-127.0.0.1}"
MODE=""
INSTALL_DOCKER=0

# --- pretty output -----------------------------------------------------------
if [ -t 1 ]; then
    B=$(printf '\033[1m'); R=$(printf '\033[0m')
    GREEN=$(printf '\033[32m'); RED=$(printf '\033[31m'); YEL=$(printf '\033[33m')
else
    B=''; R=''; GREEN=''; RED=''; YEL=''
fi
say()  { printf '%s\n' "$*"; }
step() { printf '%s▸%s %s\n' "$B" "$R" "$*"; }
ok()   { printf '%s✓%s %s\n' "$GREEN" "$R" "$*"; }
warn() { printf '%s!%s %s\n' "$YEL" "$R" "$*"; }
die()  { printf '%s✗%s %s\n' "$RED" "$R" "$*" >&2; exit 1; }

usage() {
    cat <<USAGE
mkpk-provision installer

Usage:
  install.sh (--domain <fqdn> | --behind-ingress) [options]

Pick one:
  --domain <fqdn>     Run Caddy too and get a Let's Encrypt certificate for <fqdn>.
                      Needs ports 80 and 443 reachable from the internet.
  --behind-ingress    No TLS here; publish a port for your existing proxy.

Image (defaults to the public one — override only for a private mirror):
  --registry <host>   Registry host, e.g. gitlab.example.com:5050 (default: $REGISTRY)
  --tag <tag>         Version to run, e.g. v1.2.3 (default: $TAG)
  --image <ref>       Full image reference, overriding both of the above

Options:
  --acme-email <mail> Contact address for Let's Encrypt expiry notices
  --dir <path>        Install directory (default: $DIR)
  --bind <addr>       Address to publish on in --behind-ingress mode
                      (default: $BIND — loopback; use a LAN address such as
                      172.16.10.5 when the ingress runs on another host)
  --port <port>       Port to publish (default: $PORT)
  --install-docker    Install Docker via get.docker.com when it is missing
  -h, --help          This text
USAGE
}

while [ $# -gt 0 ]; do
    case "$1" in
        --image) IMAGE="${2:?--image needs a value}"; shift 2 ;;
        --registry) REGISTRY="${2:?--registry needs a value}"; shift 2 ;;
        --tag) TAG="${2:?--tag needs a value}"; shift 2 ;;
        --domain) DOMAIN="${2:?--domain needs a value}"; MODE=caddy; shift 2 ;;
        --behind-ingress) MODE=ingress; shift ;;
        --acme-email) ACME_EMAIL="${2:?--acme-email needs a value}"; shift 2 ;;
        --dir) DIR="${2:?--dir needs a value}"; shift 2 ;;
        --port) PORT="${2:?--port needs a value}"; shift 2 ;;
        --bind) BIND="${2:?--bind needs a value}"; shift 2 ;;
        --install-docker) INSTALL_DOCKER=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) usage >&2; die "unknown option: $1" ;;
    esac
done

# --image wins; otherwise compose it from the registry host, path and tag.
[ -n "$IMAGE" ] || IMAGE="${REGISTRY%/}/$IMAGE_PATH:$TAG"
[ -n "$DOMAIN" ] && MODE=caddy
[ -n "$MODE" ] || { usage >&2; die "pick --domain <fqdn> or --behind-ingress"; }

say ""
say "${B}mkpk-provision${R} — shared provisioning instance"
say ""

# --- prerequisites -----------------------------------------------------------
step "Checking prerequisites"
if ! command -v docker >/dev/null 2>&1; then
    if [ "$INSTALL_DOCKER" = 1 ]; then
        step "Installing Docker (get.docker.com)"
        curl -fsSL https://get.docker.com | sh
        command -v systemctl >/dev/null 2>&1 && systemctl enable --now docker || true
    else
        die "docker is not installed — re-run with --install-docker, or install it yourself"
    fi
fi
docker compose version >/dev/null 2>&1 || die "docker compose v2 is missing (this needs the 'docker compose' plugin, not docker-compose)"
[ "$(id -u)" -eq 0 ] || warn "not running as root — this may fail on $DIR and on docker"
ok "docker $(docker version --format '{{.Server.Version}}' 2>/dev/null || echo '?'), compose $(docker compose version --short 2>/dev/null || echo '?')"

# --- files -------------------------------------------------------------------
step "Installing into $DIR"
mkdir -p "$DIR"
cd "$DIR"

fetch() {  # fetch <name>
    curl -fsSL "$REPO_RAW/$1" -o "$1.new" || die "cannot download $1 from $REPO_RAW"
    mv "$1.new" "$1"
}
if [ "$MODE" = caddy ]; then
    COMPOSE=compose.caddy.yaml
    fetch compose.caddy.yaml
    # Keep a Caddyfile the operator already edited (allow-lists live there).
    if [ -f Caddyfile ]; then
        ok "Caddyfile kept (yours)"
    else
        fetch Caddyfile
    fi
else
    COMPOSE=compose.yaml
    fetch compose.yaml
fi
ok "compose files ready"

# --- .env --------------------------------------------------------------------
gen_password() {
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -base64 24 | tr -d '\n/+=' | cut -c1-24
    else
        LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 24
    fi
}

FRESH=1
if [ -f .env ]; then
    FRESH=0
    step "Existing .env found — keeping the current password"
    PASSWORD="$(grep -E '^MKPK_ADMIN_PASSWORD=' .env | head -1 | cut -d= -f2- || true)"
else
    step "Generating an admin password"
    PASSWORD="$(gen_password)"
fi

{
    echo "# Written by install.sh — keep this file at 0600."
    echo "MKPK_IMAGE=$IMAGE"
    [ -n "$PASSWORD" ] && echo "MKPK_ADMIN_PASSWORD=$PASSWORD"
    if [ "$MODE" = caddy ]; then
        echo "MKPK_DOMAIN=$DOMAIN"
        echo "MKPK_ACME_EMAIL=$ACME_EMAIL"
    else
        echo "MKPK_BIND=$BIND"
        echo "MKPK_PORT=$PORT"
    fi
} > .env.new
mv .env.new .env
chmod 600 .env
ok ".env written (0600)"

# --- start -------------------------------------------------------------------
step "Pulling the image"
docker compose -f "$COMPOSE" pull -q 2>/dev/null || docker compose -f "$COMPOSE" pull
step "Starting"
docker compose -f "$COMPOSE" up -d

# Wait for the container to report healthy rather than claiming success early.
step "Waiting for the service to become healthy"
CID=""
i=0
while [ "$i" -lt 60 ]; do
    CID="$(docker compose -f "$COMPOSE" ps -q provision 2>/dev/null || true)"
    if [ -n "$CID" ]; then
        state="$(docker inspect --format '{{.State.Health.Status}}' "$CID" 2>/dev/null || echo starting)"
        [ "$state" = healthy ] && break
        if [ "$(docker inspect --format '{{.State.Running}}' "$CID" 2>/dev/null || echo false)" != true ]; then
            say ""
            docker compose -f "$COMPOSE" logs --tail 20 provision || true
            die "the provision container stopped — see the log above"
        fi
    fi
    i=$((i + 1))
    sleep 2
done
[ "$(docker inspect --format '{{.State.Health.Status}}' "$CID" 2>/dev/null || echo unknown)" = healthy ] \
    || warn "not healthy yet — check: docker compose -f $COMPOSE logs -f provision"

# --- done --------------------------------------------------------------------
say ""
ok "mkpk-provision is up"
say ""
if [ "$MODE" = caddy ]; then
    say "   URL:      ${B}https://$DOMAIN${R}"
    say "   (the first certificate can take up to a minute)"
else
    say "   URL:      ${B}http://$BIND:$PORT${R} — point your proxy at this"
fi
if [ "$FRESH" = 1 ]; then
    say "   Login:    ${B}$PASSWORD${R}"
    say "             (one shared password; change it in the UI, then remove"
    say "              MKPK_ADMIN_PASSWORD from $DIR/.env)"
else
    say "   Login:    unchanged (existing installation)"
fi
say "   Image:    $IMAGE"
say "   Data:     docker volume, config + password + deploy key"
say "   Files:    $DIR"
say ""
say "${B}Next:${R}"
say "  1. Sign in and change the password (lock icon, bottom left)."
say "  2. Create the deploy key (key icon) and import its .pub on each router:"
say "     /user ssh-keys import user=<login> public-key-file=mkpk-provision.pub"
say "  3. Add a router, services and users, then deploy."
say ""
warn "This console holds SSH access to every router — restrict who can reach it"
if [ "$MODE" = ingress ] && [ "$BIND" != "127.0.0.1" ] && [ "$BIND" != "localhost" ]; then
    warn "published on $BIND:$PORT over plain HTTP — reachable by anything that can"
    warn "route there. Keep it on a trusted network and let the proxy do TLS."
fi
if [ "$MODE" = caddy ]; then
    warn "(remote_ip allow-list in $DIR/Caddyfile, or a firewall rule on 443)"
fi
say ""
