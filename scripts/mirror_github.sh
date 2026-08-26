#!/usr/bin/env bash
#
# Публикация релиза на публичном GitHub-зеркале: тег, релиз и Go-архивы.
#
# Живёт отдельно от scripts/package_release.sh намеренно. Раньше зеркалирование
# делала macOS-джоба вместе с DMG — и когда мак-раннер был недоступен, зеркало
# останавливалось целиком: на GitHub не уезжали ни релиз, ни Go-бинарники, ни
# appcast, хотя вся linux-часть отработала. Теперь зеркало наполняет linux-джоба,
# а macOS-джоба лишь докладывает свои DMG в уже созданный релиз.
#
# Использование (в CI): bash scripts/mirror_github.sh vX.Y.Z
set -uo pipefail

TAG="${1:-${CI_COMMIT_TAG:-}}"
[[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "✗ нужен тег vX.Y.Z" >&2; exit 64; }

GITHUB_REPO="${MKPK_GITHUB_REPO:-LazyGatto/mikrotik-psk-knock}"
: "${GITHUB_TOKEN:?нет protected CI-переменной GITHUB_TOKEN}"
GH_API="https://api.github.com/repos/$GITHUB_REPO"
gh_curl() { curl -fsS --retry 3 -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/vnd.github+json" "$@"; }

fail=0
DIST="${CI_PROJECT_DIR:-$PWD}/dist"
mkdir -p "$DIST"

echo "▸ 1/3 тег на зеркало"
git push "https://x-access-token:${GITHUB_TOKEN}@github.com/$GITHUB_REPO.git" \
  "refs/tags/$TAG" "HEAD:refs/heads/main" 2>/dev/null \
  || git push "https://x-access-token:${GITHUB_TOKEN}@github.com/$GITHUB_REPO.git" "refs/tags/$TAG" \
  || { echo "  ✗ не удалось запушить тег"; fail=1; }

echo "▸ 2/3 релиз"
# Заметки берём из CHANGELOG — это публичное лицо релиза.
notes_file="$(mktemp -t relnotes)"
python3 - "${TAG#v}" "${CI_PROJECT_DIR:-$PWD}/CHANGELOG.md" "$notes_file" <<'PY'
import re, sys
version, changelog, out = sys.argv[1], sys.argv[2], sys.argv[3]
try:
    text = open(changelog).read()
except OSError:
    open(out, 'w').write('See CHANGELOG.md.\n'); raise SystemExit
m = re.search(r'## \[' + re.escape(version) + r'\][^\n]*\n(.*?)(?=\n## \[)', text, re.S)
open(out, 'w').write((m.group(1).strip() if m else 'See CHANGELOG.md.') + '\n')
PY
body_json="$(python3 -c 'import json,sys; print(json.dumps({"tag_name":sys.argv[1],"name":"mkpk "+sys.argv[1],"body":open(sys.argv[2]).read()}))' "$TAG" "$notes_file")"

rel_json="$(gh_curl "$GH_API/releases/tags/$TAG" 2>/dev/null)"
if [[ -z "$rel_json" ]]; then
  rel_json="$(gh_curl -X POST "$GH_API/releases" -d "$body_json")" \
    || { echo "  ✗ не удалось создать релиз"; fail=1; }
else
  echo "  релиз уже есть — обновляю заметки"
  rel_id_existing="$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("id",""))' <<<"$rel_json")"
  [[ -n "$rel_id_existing" ]] && gh_curl -X PATCH "$GH_API/releases/$rel_id_existing" \
    -d "$(python3 -c 'import json,sys; print(json.dumps({"body":open(sys.argv[1]).read()}))' "$notes_file")" >/dev/null
fi
rm -f "$notes_file"
rel_id="$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("id",""))' <<<"$rel_json" 2>/dev/null)"
[[ -n "$rel_id" ]] || { echo "  ✗ нет id релиза"; exit 1; }

echo "▸ 3/3 Go-архивы"
PKG="${CI_API_V4_URL}/projects/${CI_PROJECT_ID}/packages/generic/mkpk/${TAG}"
for os in linux darwin windows; do for arch in amd64 arm64; do for bin in mkpk mkpk-provision; do
  z="${bin}_${TAG}_${os}_${arch}.zip"
  [[ -f "$DIST/$z" ]] || curl -fsS --retry 3 -o "$DIST/$z" "$PKG/$z" || echo "  ⚠︎ пропуск $z (нет в registry)"
done; done; done
for arch in amd64 arm64; do
  z="mkpk-client_${TAG}_windows_${arch}.zip"
  [[ -f "$DIST/$z" ]] || curl -fsS --retry 3 -o "$DIST/$z" "$PKG/$z" || echo "  ⚠︎ пропуск $z (нет в registry)"
done

assets_json="$(gh_curl "$GH_API/releases/$rel_id/assets")"
for f in "$DIST"/*.zip; do
  [[ -f "$f" ]] || continue
  name="$(basename "$f")"
  old_id="$(python3 -c 'import json,sys; print(next((str(a["id"]) for a in json.load(sys.stdin) if a.get("name")==sys.argv[1]), ""))' "$name" <<<"$assets_json" 2>/dev/null)"
  [[ -n "$old_id" ]] && gh_curl -X DELETE "$GH_API/releases/assets/$old_id" >/dev/null
  if gh_curl -X POST -H "Content-Type: application/octet-stream" --data-binary "@$f" \
      "https://uploads.github.com/repos/$GITHUB_REPO/releases/$rel_id/assets?name=$name" >/dev/null; then
    echo "  ✓ $name"
  else
    echo "  ✗ upload $name"; fail=1
  fi
done

[[ "$fail" -eq 0 ]] || { echo "✗ зеркалирование завершилось с ошибками — перезапустите job (шаги идемпотентны)"; exit 1; }
echo "✓ $TAG на GitHub: релиз и Go-архивы"
echo "  (macOS DMG и appcast докладывает джоба release:macos)"
