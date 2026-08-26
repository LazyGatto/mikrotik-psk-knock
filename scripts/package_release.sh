#!/usr/bin/env bash
#
# Собрать и опубликовать macOS-артефакты релиза vX.Y.Z:
#
#   1. нативный клиент  client-macos/ → sign → notarize .app → DMG → notarize DMG
#   2. Wails-приложение mkpk-provision-desktop → тот же конвейер
#   3. Sparkle: подпись клиентского DMG (EdDSA) → appcast.xml
#   4. CI: DMG-и → GitLab package registry + asset-links к релизу
#   5. CI: GitHub-релиз на публичном зеркале — оба DMG, Go-zips, appcast.xml
#
# Локальный запуск (fallback, машина maintainer'а): шаги 1–3 + подсказка по
# публикации; публикует только CI (нужны MKPK_RELEASE_TOKEN / GITHUB_TOKEN).
#
# Использование:
#   bash scripts/package_release.sh vX.Y.Z
#
# План и провизия раннера: docs/plans/2026-08-26-macos-release-ci-plan.md.
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
cd "$ROOT"

TAG="${1:-$(git describe --tags --abbrev=0 2>/dev/null || true)}"
if ! [[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "✗ нет релизного тега — передайте явно: $0 vX.Y.Z" >&2
  exit 64
fi

# Артефакты обязаны соответствовать релизу: собираем только с tagged-коммита.
head_tag="$(git describe --tags --exact-match HEAD 2>/dev/null || true)"
if [[ "$head_tag" != "$TAG" ]]; then
  echo "✗ HEAD — не $TAG (найдено: ${head_tag:-нет точного тега}); сначала checkout тега" >&2
  exit 1
fi
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "✗ рабочее дерево не чистое — commit/stash" >&2
  exit 1
fi

SIGN_ID="${MKPK_SIGN_ID:-Developer ID Application: EDINY GOROD, OOO (R2M77TY8U9)}"
NOTARY_PROFILE="${MKPK_NOTARY_PROFILE:-mkpk-notary}"
GITHUB_REPO="${MKPK_GITHUB_REPO:-LazyGatto/mikrotik-psk-knock}"
# Аккаунт EdDSA-ключа в keychain. НЕ дефолтный "ed25519": CI-keychain на
# раннере общий с другим проектом, и его ключ живёт под дефолтным именем.
SPARKLE_ACCOUNT="${MKPK_SPARKLE_ACCOUNT:-ru.eg23.mkpk.client}"
SPARKLE_TOOLS_VERSION="${SPARKLE_TOOLS_VERSION:-2.9.6}"
SPARKLE_SIGN_UPDATE="${SPARKLE_SIGN_UPDATE:-$ROOT/tmp/sparkle-tools/bin/sign_update}"

# CI: подписные активы живут в выделенном не-блокирующемся keychain на
# раннере (login запирается вместе с экраном и вешает headless codesign).
if [[ -n "${CI:-}" ]]; then
  KC="${MKPK_CI_KEYCHAIN:-$HOME/Library/Keychains/mkpk-ci.keychain-db}"
  KC_PASS_FILE="${MKPK_CI_KEYCHAIN_PASS_FILE:-$HOME/.mkpk-ci-keychain-pass}"
  security unlock-keychain -p "$(cat "$KC_PASS_FILE")" "$KC"
  export MKPK_KEYCHAIN="$KC"
fi
export MKPK_SIGN_ID="$SIGN_ID"

DIST="$ROOT/dist"
mkdir -p "$DIST" "$ROOT/tmp"

CLIENT_DMG="$DIST/mkpk-client_${TAG}_darwin_arm64.dmg"
DESKTOP_DMG="$DIST/mkpk-provision-desktop_${TAG}_darwin_arm64.dmg"
APPCAST="$DIST/appcast.xml"

set -e

# --- 1) Нативный клиент ------------------------------------------------------
echo "▸ 1/5 mkpk client: build + sign + notarize"
( cd client-macos
  # MKPK_SPARKLE_ED_PUBLIC_KEY приходит из окружения (CI-переменная) и включает
  # SUFeedURL/SUPublicEDKey в Info.plist; без него — сборка без автоапдейта.
  MKPK_VERSION="$TAG" bash script/build_app.sh
  bash script/sign.sh .build/mkpk.app
  bash script/notarize.sh .build/mkpk.app "$NOTARY_PROFILE"
  bash script/make_dmg.sh .build/mkpk.app "$CLIENT_DMG" mkpk
  bash script/notarize.sh "$CLIENT_DMG" "$NOTARY_PROFILE"
)

# --- 2) Wails mkpk-provision-desktop ----------------------------------------
echo "▸ 2/5 mkpk-provision-desktop: build + sign + notarize"
command -v wails >/dev/null || { echo "✗ wails не в PATH (go install github.com/wailsapp/wails/v2/cmd/wails@latest)"; exit 1; }
( cd client && make desktop )   # VERSION сам резолвится из тега (git describe)
WAILS_APP="$ROOT/client/cmd/mkpk-provision-desktop/build/bin/mkpk-provision-desktop.app"
[[ -d "$WAILS_APP" ]] || { echo "✗ не найден $WAILS_APP"; exit 1; }
# MKPK_PROVISION_PROFILE="" — иначе sign.sh встроит провижининг-профиль
# клиента (чужой bundle id) и amfi откажется запускать приложение.
MKPK_PROVISION_PROFILE="" bash client-macos/script/sign.sh "$WAILS_APP"
bash client-macos/script/notarize.sh "$WAILS_APP" "$NOTARY_PROFILE"
bash client-macos/script/make_dmg.sh "$WAILS_APP" "$DESKTOP_DMG" mkpk-provision
bash client-macos/script/notarize.sh "$DESKTOP_DMG" "$NOTARY_PROFILE"

# --- 3) Sparkle: подпись + appcast ------------------------------------------
echo "▸ 3/5 Sparkle appcast"
if [[ -n "${MKPK_SPARKLE_ED_PUBLIC_KEY:-}" ]]; then
  if [[ ! -x "$SPARKLE_SIGN_UPDATE" ]]; then
    echo "  fetching Sparkle tools $SPARKLE_TOOLS_VERSION"
    mkdir -p "$ROOT/tmp/sparkle-tools"
    curl -sSL --retry 3 "https://github.com/sparkle-project/Sparkle/releases/download/$SPARKLE_TOOLS_VERSION/Sparkle-$SPARKLE_TOOLS_VERSION.tar.xz" \
      | tar -xJ -C "$ROOT/tmp/sparkle-tools" bin/
    SPARKLE_SIGN_UPDATE="$ROOT/tmp/sparkle-tools/bin/sign_update"
  fi
  if [[ -n "${CI:-}" ]]; then
    # Приватный ключ — из CI-keychain, через stdin: ничего на диске и в argv.
    SPARKLE_KEY="$(security find-generic-password -s "https://sparkle-project.org" -a "$SPARKLE_ACCOUNT" -w "$MKPK_KEYCHAIN")"
    sig_out="$(printf '%s' "$SPARKLE_KEY" | "$SPARKLE_SIGN_UPDATE" --ed-key-file - "$CLIENT_DMG")"
  else
    sig_out="$("$SPARKLE_SIGN_UPDATE" --account "$SPARKLE_ACCOUNT" "$CLIENT_DMG")"
  fi
  ed_sig="$(echo "$sig_out" | sed -E 's/.*sparkle:edSignature="([^"]+)".*/\1/')"
  ed_len="$(echo "$sig_out" | sed -E 's/.*length="([^"]+)".*/\1/')"
  build_num="$(git rev-list --count HEAD)"   # требует полной истории (GIT_DEPTH=0)
  VERSION="${TAG#v}"
  cat > "$APPCAST" <<APPCAST
<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
  <channel>
    <title>mkpk</title>
    <language>en</language>
    <item>
      <title>$VERSION</title>
      <pubDate>$(date -R)</pubDate>
      <sparkle:version>$build_num</sparkle:version>
      <sparkle:shortVersionString>$VERSION</sparkle:shortVersionString>
      <enclosure url="https://github.com/$GITHUB_REPO/releases/download/$TAG/$(basename "$CLIENT_DMG")"
                 sparkle:edSignature="$ed_sig"
                 length="$ed_len"
                 type="application/octet-stream" />
    </item>
  </channel>
</rss>
APPCAST
  echo "  ✓ appcast → $APPCAST (build $build_num)"
else
  echo "  ℹ MKPK_SPARKLE_ED_PUBLIC_KEY не задан — сборка и релиз без автоапдейта"
fi

if [[ -z "${CI:-}" ]]; then
  echo
  echo "✓ Артефакты готовы (локальный режим — публикация вручную):"
  ls -lh "$DIST" | sed 's/^/  /'
  echo "  glab release upload $TAG $CLIENT_DMG $DESKTOP_DMG"
  echo "  gh release upload $TAG $CLIENT_DMG $DESKTOP_DMG ${MKPK_SPARKLE_ED_PUBLIC_KEY:+$APPCAST} -R $GITHUB_REPO"
  exit 0
fi

# --- 4) GitLab: registry + asset-links ---------------------------------------
echo "▸ 4/5 GitLab: publish DMGs"
: "${MKPK_RELEASE_TOKEN:?нет protected CI-переменной MKPK_RELEASE_TOKEN}"
PKG="${CI_API_V4_URL}/projects/${CI_PROJECT_ID}/packages/generic/mkpk/${TAG}"
for f in "$CLIENT_DMG" "$DESKTOP_DMG"; do
  name="$(basename "$f")"
  curl -fsS --retry 3 --header "JOB-TOKEN: ${CI_JOB_TOKEN}" \
    --upload-file "$f" "$PKG/$name" >/dev/null
  # Releases API отвергает CI_JOB_TOKEN — asset-links только по project token.
  # Одноимённую ссылку с прошлого прогона убираем (GitLab даёт 400 на дубликат).
  links="$(curl -fsS --header "PRIVATE-TOKEN: ${MKPK_RELEASE_TOKEN}" \
    "${CI_API_V4_URL}/projects/${CI_PROJECT_ID}/releases/${TAG}/assets/links" 2>/dev/null || echo '[]')"
  old_id="$(python3 -c 'import json,sys; print(next((str(l["id"]) for l in json.load(sys.stdin) if l.get("name")==sys.argv[1]), ""))' "$name" <<<"$links")"
  [[ -n "$old_id" ]] && curl -fsS --request DELETE --header "PRIVATE-TOKEN: ${MKPK_RELEASE_TOKEN}" \
    "${CI_API_V4_URL}/projects/${CI_PROJECT_ID}/releases/${TAG}/assets/links/$old_id" >/dev/null
  curl -fsS --request POST --header "PRIVATE-TOKEN: ${MKPK_RELEASE_TOKEN}" \
    --form "name=$name" --form "url=$PKG/$name" \
    "${CI_API_V4_URL}/projects/${CI_PROJECT_ID}/releases/${TAG}/assets/links" >/dev/null
  echo "  ✓ $name"
done

# --- 5) GitHub: зеркало + релиз + ассеты -------------------------------------
# Отдельный set +e: GitLab-релиз уже полон; провал GitHub-шага докладываем и
# падаем В КОНЦЕ (перезапуск job безопасен — все шаги идемпотентны).
echo "▸ 5/5 GitHub: mirror release"
set +e
gh_fail=0
: "${GITHUB_TOKEN:?нет protected CI-переменной GITHUB_TOKEN}"
GH_API="https://api.github.com/repos/$GITHUB_REPO"
gh_curl() { curl -fsS --retry 3 -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/vnd.github+json" "$@"; }

# Тег (и main, best-effort) должны существовать на зеркале до релиза.
git push "https://x-access-token:${GITHUB_TOKEN}@github.com/$GITHUB_REPO.git" \
  "refs/tags/$TAG" "HEAD:refs/heads/main" 2>/dev/null \
  || git push "https://x-access-token:${GITHUB_TOKEN}@github.com/$GITHUB_REPO.git" "refs/tags/$TAG" \
  || { echo "  ✗ не удалось запушить тег на GitHub"; gh_fail=1; }

rel_json="$(gh_curl "$GH_API/releases/tags/$TAG" 2>/dev/null)"
if [[ -z "$rel_json" ]]; then
  rel_json="$(gh_curl -X POST "$GH_API/releases" \
    -d "{\"tag_name\":\"$TAG\",\"name\":\"mkpk $TAG\",\"body\":\"See CHANGELOG.md. Auto-published from CI.\"}")" \
    || { echo "  ✗ не удалось создать GitHub-релиз"; gh_fail=1; }
fi
rel_id="$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("id",""))' <<<"$rel_json" 2>/dev/null)"

if [[ -n "$rel_id" ]]; then
  # Go-zips подтягиваем из package registry (публичный проект → публичное чтение),
  # чтобы зеркало получало полный набор ассетов без ручной синхронизации.
  for os in linux darwin windows; do for arch in amd64 arm64; do for bin in mkpk mkpk-provision; do
    z="${bin}_${TAG}_${os}_${arch}.zip"
    [[ -f "$DIST/$z" ]] || curl -fsS --retry 3 -o "$DIST/$z" "$PKG/$z" || echo "  ⚠︎ пропуск $z (нет в registry)"
  done; done; done
  assets_json="$(gh_curl "$GH_API/releases/$rel_id/assets")"
  for f in "$CLIENT_DMG" "$DESKTOP_DMG" "$APPCAST" "$DIST"/*.zip; do
    [[ -f "$f" ]] || continue
    name="$(basename "$f")"
    old_id="$(python3 -c 'import json,sys; print(next((str(a["id"]) for a in json.load(sys.stdin) if a.get("name")==sys.argv[1]), ""))' "$name" <<<"$assets_json" 2>/dev/null)"
    [[ -n "$old_id" ]] && gh_curl -X DELETE "$GH_API/releases/assets/$old_id" >/dev/null
    if gh_curl -X POST -H "Content-Type: application/octet-stream" \
        --data-binary "@$f" \
        "https://uploads.github.com/repos/$GITHUB_REPO/releases/$rel_id/assets?name=$name" >/dev/null; then
      echo "  ✓ $name"
    else
      echo "  ✗ upload $name"; gh_fail=1
    fi
  done
else
  gh_fail=1
fi
set -e

if [[ "$gh_fail" -ne 0 ]]; then
  echo "✗ GitHub-шаг не завершился полностью — GitLab-релиз опубликован; перезапустите job"
  exit 1
fi
echo "✓ $TAG: GitLab release + GitHub mirror + appcast опубликованы"
