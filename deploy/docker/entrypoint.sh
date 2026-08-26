#!/bin/sh
# Default command: serve the admin UI. Any other argument set is passed to
# mkpk-provision as-is, so `docker run … mkpk-provision passwd` also works.
set -e

if [ "$#" -gt 0 ]; then
    exec mkpk-provision "$@"
fi

# The password gate is what makes the networked mode safe; say so plainly
# instead of letting the binary's refusal look like a crash loop.
if [ ! -f "$(dirname "$MKPK_CONFIG")/mkpk-admin.json" ] && [ -z "$MKPK_ADMIN_PASSWORD" ]; then
    echo "mkpk-provision: no admin password set." >&2
    echo "  Set MKPK_ADMIN_PASSWORD once (it is hashed into mkpk-admin.json on first start)," >&2
    echo "  or run: docker compose run --rm provision passwd --config $MKPK_CONFIG" >&2
    exit 1
fi

exec mkpk-provision serve --config "$MKPK_CONFIG" --addr "$MKPK_ADDR"
