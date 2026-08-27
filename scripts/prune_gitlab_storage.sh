#!/usr/bin/env bash
#
# Уборка на self-hosted GitLab: держим только последние MKPK_KEEP_RELEASES
# версий — релизы, generic-пакеты и теги образа. Диск инстанса наш, и хранить
# на нём всю историю бинарников смысла нет: полная история живёт на публичном
# зеркале GitHub, где место не наше.
#
# Git-теги НЕ трогаем: это история версий и возможность пересобрать любую.
# Тег образа `latest` НЕ трогаем: на него смотрят уже развёрнутые инсталляции.
#
# В CI запускается джобой cleanup:gitlab после release. Локально:
#   CI_API_V4_URL=https://<host>/api/v4 CI_PROJECT_ID=<id> \
#   MKPK_RELEASE_TOKEN=<pat> bash scripts/prune_gitlab_storage.sh
set -uo pipefail

KEEP="${MKPK_KEEP_RELEASES:-2}"
API="${CI_API_V4_URL:?нет CI_API_V4_URL}"
PROJ="${CI_PROJECT_ID:?нет CI_PROJECT_ID}"
DRY="${MKPK_PRUNE_DRY_RUN:-0}"   # 1 — только показать, что было бы удалено
[[ "$DRY" == 1 ]] || : "${MKPK_RELEASE_TOKEN:?нет protected CI-переменной MKPK_RELEASE_TOKEN}"

api() { curl -fsS --retry 3 --header "PRIVATE-TOKEN: ${MKPK_RELEASE_TOKEN:-}" "$@"; }
del() {
  [[ "$DRY" == 1 ]] && return 0
  curl -fsS --retry 2 --request DELETE --header "PRIVATE-TOKEN: $MKPK_RELEASE_TOKEN" "$1" >/dev/null 2>&1
}
mark() { [[ "$DRY" == 1 ]] && echo "  ~ (dry-run) $1" || echo "  ✓ $1"; }

# Список версий, которые оставляем: N самых свежих релизов. Порядок берём у
# API (releases отдаются от новых к старым), а не парсим semver сами.
keep_list="$(api "$API/projects/$PROJ/releases?per_page=100" \
  | python3 -c 'import json,sys; d=json.load(sys.stdin); print("\n".join(r["tag_name"] for r in d[:int(sys.argv[1])]))' "$KEEP")"
if [[ -z "$keep_list" ]]; then
  echo "✗ не удалось получить список релизов — ничего не удаляю" >&2
  exit 1
fi
echo "▸ оставляем: $(echo "$keep_list" | tr '\n' ' ')"
keeps() { grep -qxF "$1" <<<"$keep_list"; }

echo "▸ релизы"
api "$API/projects/$PROJ/releases?per_page=100" \
  | python3 -c 'import json,sys;[print(r["tag_name"]) for r in json.load(sys.stdin)]' \
  | while read -r tag; do
      keeps "$tag" && continue
      del "$API/projects/$PROJ/releases/$tag" && mark "релиз $tag" || echo "  ✗ релиз $tag"
    done

echo "▸ generic-пакеты"
api "$API/projects/$PROJ/packages?per_page=100" \
  | python3 -c 'import json,sys;[print(p["id"],p["version"]) for p in json.load(sys.stdin)]' \
  | while read -r id ver; do
      keeps "$ver" && continue
      del "$API/projects/$PROJ/packages/$id" && mark "пакет $ver" || echo "  ✗ пакет $ver"
    done

echo "▸ теги образов"
api "$API/projects/$PROJ/registry/repositories?per_page=100" \
  | python3 -c 'import json,sys;[print(r["id"]) for r in json.load(sys.stdin)]' \
  | while read -r repo; do
      api "$API/projects/$PROJ/registry/repositories/$repo/tags?per_page=100" \
        | python3 -c 'import json,sys;[print(t["name"]) for t in json.load(sys.stdin)]' \
        | while read -r tag; do
            [[ "$tag" == latest ]] && continue
            keeps "$tag" && continue
            del "$API/projects/$PROJ/registry/repositories/$repo/tags/$tag" \
              && mark "образ $tag" || echo "  ✗ образ $tag"
          done
    done

# Артефакты джоб в норме чистит `expire_in` в .gitlab-ci.yml — этот шаг нужен,
# чтобы разгрести хвост, накопленный при прежнем сроке хранения. Поэтому он
# выключен по умолчанию. GitLab сам решает, что удалять можно: артефакты
# последнего успешного пайплайна на каждую ветку/тег он сохраняет.
if [[ "${MKPK_PRUNE_ARTIFACTS:-0}" == 1 ]]; then
  echo "▸ артефакты джоб"
  if [[ "$DRY" == 1 ]]; then
    echo "  ~ (dry-run) DELETE projects/$PROJ/artifacts"
  elif curl -fsS --request DELETE --header "PRIVATE-TOKEN: $MKPK_RELEASE_TOKEN" \
      "$API/projects/$PROJ/artifacts" >/dev/null 2>&1; then
    echo "  ✓ удаление поставлено в очередь (статистика обновится не сразу)"
  else
    echo "  ✗ не удалось запросить удаление артефактов"
  fi
fi

echo "✓ уборка закончена (git-теги и latest не тронуты)"
