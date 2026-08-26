#!/usr/bin/env bash
#
# Единый гейт качества для mikrotik-psk-knock.
#
# Гейт — это код возврата этого скрипта, а не grep по его выводу.
# Стадия, которую невозможно выполнить на этой машине, помечается SKIP и
# НЕ делает гейт зелёным по случайности: `--strict` превращает любой SKIP в отказ.
#
# Использование:
#   bash scripts/verify.sh            # go vet/build/test + swift build + selfcheck
#   bash scripts/verify.sh --strict   # SKIP считается провалом
#   bash scripts/verify.sh --quick    # только сборка (без тестов и selfcheck)
#   bash scripts/verify.sh --docs     # дополнительно markdownlint (нужен npx)
#
# См. docs/agents/README_FOR_AGENTS.md (раздел «Гейт — одна команда»)
# и .agents/skills/verification/SKILL.md.

set -uo pipefail

cd "$(dirname "$0")/.." || exit 2

STRICT=0
QUICK=0
DOCS=0
for arg in "$@"; do
    case "$arg" in
        --strict) STRICT=1 ;;
        --quick)  QUICK=1 ;;
        --docs)   DOCS=1 ;;
        -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
        *) echo "unknown option: $arg" >&2; exit 2 ;;
    esac
done

FAILED=()
SKIPPED=()

log_dir="$(mktemp -d)"
trap 'rm -rf "$log_dir"' EXIT

run_stage() {
    local name="$1"
    shift
    local log="$log_dir/${name//:/_}.log"
    printf '── %-22s ' "$name"
    if "$@" >"$log" 2>&1; then
        echo "PASS"
        return 0
    fi
    echo "FAIL"
    FAILED+=("$name")
    echo "   ↓ последние 40 строк $name"
    tail -40 "$log" | sed 's/^/   /'
    return 1
}

skip_stage() {
    local name="$1" reason="$2"
    printf '── %-22s SKIP (%s)\n' "$name" "$reason"
    SKIPPED+=("$name: $reason")
}

# --- Go: всё, кроме десктопной обёртки (ей нужен платформенный webview) -------
go_pkgs() {
    (cd client && go list ./... 2>/dev/null | grep -v '/cmd/mkpk-provision-desktop')
}

go_stage() {  # go_stage <subcommand> [flags...]
    local sub="$1"; shift
    local pkgs
    pkgs="$(go_pkgs)" || return 1
    [ -n "$pkgs" ] || return 1
    # shellcheck disable=SC2086
    (cd client && go "$sub" "$@" $pkgs)
}

echo "mkpk — verify"
echo

if command -v go >/dev/null 2>&1; then
    run_stage "go:vet"   go_stage vet
    run_stage "go:build" go_stage build
    if [ "$QUICK" -eq 0 ]; then
        run_stage "go:test" go_stage test -cover
    else
        skip_stage "go:test" "--quick"
    fi
else
    skip_stage "go:vet"   "go не в PATH"
    skip_stage "go:build" "go не в PATH"
    skip_stage "go:test"  "go не в PATH"
fi

# --- Swift-клиент: только macOS ------------------------------------------------
# `swift test` здесь нет намеренно: CLT не несёт тестового рантайма, поэтому
# корректность рантайма живёт в mkpk-selfcheck (золотые векторы токена).
if [ "$(uname -s)" = "Darwin" ] && command -v swift >/dev/null 2>&1; then
    run_stage "swift:build" env -C client-macos swift build
    if [ "$QUICK" -eq 0 ]; then
        run_stage "swift:selfcheck" env -C client-macos swift run mkpk-selfcheck
    else
        skip_stage "swift:selfcheck" "--quick"
    fi
else
    skip_stage "swift:build"     "не macOS или swift не в PATH"
    skip_stage "swift:selfcheck" "не macOS или swift не в PATH"
fi

# --- Документация: по запросу ---------------------------------------------------
if [ "$DOCS" -eq 1 ]; then
    if command -v npx >/dev/null 2>&1; then
        run_stage "docs:lint" npx --yes markdownlint-cli2 \
            "docs/agents/*.md" ".agents/**/*.md" ".claude/**/*.md" "AGENTS.md"
    else
        skip_stage "docs:lint" "npx не в PATH"
    fi
fi

echo
if [ "${#SKIPPED[@]}" -gt 0 ]; then
    echo "Пропущено:"
    printf '  - %s\n' "${SKIPPED[@]}"
fi

if [ "${#FAILED[@]}" -gt 0 ]; then
    echo "VERIFY FAILED: ${FAILED[*]}"
    exit 1
fi

if [ "$STRICT" -eq 1 ] && [ "${#SKIPPED[@]}" -gt 0 ]; then
    echo "VERIFY FAILED: --strict и пропущено стадий: ${#SKIPPED[@]}"
    exit 1
fi

echo "VERIFY OK"
