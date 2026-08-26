# План: автоматизация macOS-релизов на mac-ci-01 + автоапдейт клиента

Дата: 2026-08-26. Решения maintainer'а (зафиксированы в этой сессии):

- канал обновлений и публичные артефакты — **GitHub releases**
  (`github.com/LazyGatto/mikrotik-psk-knock`); GitLab-хост в публичные файлы
  не попадает;
- в автоматизированный job входят **оба** macOS-приложения: нативный клиент
  `client-macos/` и Wails `mkpk-provision-desktop`;
- зависимость **Sparkle** для автоапдейта клиента — одобрена (первая сторонняя
  зависимость `client-macos/`; обоснование — ADR-паттерн из gitlab-companion:
  у Apple нет фреймворка обновления вне App Store, самописный апдейтер
  реализует самые рискованные части — подпись EdDSA, карантин, атомарная
  замена работающего бандла).

Образец — рабочий конвейер `gitlab-companion` (job `release` на mac-ci-01,
`scripts/package_release.sh`, выстраданные уроки в его `docs/development/ci-status.md`).

## Архитектурная проверка

1. **Документы:** `AGENTS.md` (Release flow), `docs/agents/04_lessons.md`
   (новые уроки по мере набивания шишек). `docs/design.md` не трогаем —
   протокол не меняется.
2. **Инварианты:** формула токена, формат инвайта, `.rsc` — не затронуты.
   Sparkle живёт только в app-таргете `MkpkApp`; `MkpkKit` и selfcheck без
   зависимостей.
3. **Совместимость:** не ломается. Автоапдейт подхватят только сборки,
   начиная с первого релиза с вшитым фидом; старые обновляются вручную.
4. **Новые файлы:** `scripts/package_release.sh` (оркестратор). Новых
   каталогов нет.
5. **Зависимость:** Sparkle (MIT, SwiftPM, только `MkpkApp`) — одобрена, см. выше.
6. **Локализация:** одна новая строка — «Check for updates» / «Проверить
   обновления», через `L(...)`.
7. **Живой роутер:** не нужен. Нужен доступ к mac-ci-01 (только maintainer).

## Схема

```text
git push тег vX.Y.Z
├─ test            (linux)  bash scripts/verify.sh
├─ build:binaries  (linux)  Go-кроссы → zips → package registry
├─ release         (linux)  release-cli: GitLab-релиз + ссылки на zips
└─ release:macos   (mac-ci-01, GIT_DEPTH=0)  scripts/package_release.sh:
     1. клиент: build (MKPK_VERSION=тег, Sparkle-ключи в Info.plist)
        → sign.sh (CI-keychain) → notarize .app → DMG → notarize DMG
     2. wails:  make desktop → sign.sh (без profile) → notarize .app
        → DMG → notarize DMG
     3. sparkle: sign_update (EdDSA из CI-keychain) → appcast.xml
     4. GitLab: DMG-и → package registry, asset-links к релизу (MKPK_RELEASE_TOKEN)
     5. GitHub: пуш тега, релиз, ассеты: оба DMG + Go-zips + appcast.xml
        (GITHUB_TOKEN) — закрывает ручную синхронизацию зеркала
```

Фид Sparkle: `https://github.com/LazyGatto/mikrotik-psk-knock/releases/latest/download/appcast.xml`
— стабильный URL «последнего релиза», appcast лежит ассетом релиза, никаких
коммитов из CI в репозиторий.

## Шаги (файловые зоны)

| # | Что | Зона | Кто |
| --- | ----- | ------ | ----- |
| 1 | Sparkle в `Package.swift` + апдейтер в `App.swift` + кнопка в Settings | `client-macos/{Package.swift,Sources/MkpkApp/}` | Team Lead (сессия) |
| 2 | `build_app.sh`: embed фреймворков + Sparkle-ключи в Info.plist (по env) | `client-macos/script/build_app.sh` | Team Lead |
| 3 | `sign.sh`/`notarize.sh`/`make_dmg.sh`: `MKPK_KEYCHAIN`, подпись Sparkle-хелперов | `client-macos/script/` | Team Lead |
| 4 | `scripts/package_release.sh` — оркестратор | `scripts/` | Team Lead |
| 5 | `.gitlab-ci.yml`: job `release:macos` | `.gitlab-ci.yml` | Team Lead |
| 6 | `AGENTS.md` Release flow, CHANGELOG | корень | Team Lead |
| 7 | Провизия раннера (ниже) | mac-ci-01 | **только maintainer** |
| 8 | Первый тег-прогон, проверка автоапдейта с предыдущей версии | — | maintainer |

Готово, когда: `bash scripts/verify.sh` = 0; тег `vX.Y.Z` проходит
`release:macos` до конца; GitHub-релиз несёт оба DMG + appcast; клиент
предыдущей версии видит обновление и ставит его.

## Провизия mac-ci-01 (разово, только maintainer)

Identity уже в CI-keychain gitlab-companion — самый дешёвый путь **переиспользовать
его**, добавив mkpk-записи (либо создать отдельный `mkpk-ci.keychain-db` тем же
рецептом из `gitlab-companion/docs/development/ci-status.md`):

```bash
KC=~/Library/Keychains/gitlab-companion-ci.keychain-db   # или свой mkpk-ci
KPASS="$(cat ~/.gitlab-companion-ci-keychain-pass)"

# 1) notary-профиль mkpk в CI-keychain (login не годится — запирается)
xcrun notarytool store-credentials mkpk-notary \
  --apple-id "<apple-id>" --team-id R2M77TY8U9 \
  --password "<app-specific-password>" --keychain "$KC"

# 2) EdDSA-ключ Sparkle для mkpk (аккаунт НЕ ed25519 — он занят gitlab-companion)
.build/sparkle-tools/bin/generate_keys   # печатает public, кладёт private в login
# приватный перенести в CI-keychain:
security add-generic-password -U -s "https://sparkle-project.org" \
  -a "ru.eg23.mkpk.client" -w "<private-key-base64>" -A "$KC"

# 3) go + wails для десктоп-сборки
brew install go && go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

В GitLab (Settings → CI/CD):

- включить раннер mac-ci-01 для проекта mkpk;
- protected-тег правило `v*`;
- переменные (protected + masked): `MKPK_RELEASE_TOKEN` (project access token,
  scope api — asset-links: releases API отвергает `CI_JOB_TOKEN`),
  `GITHUB_TOKEN` (PAT c правом на релизы зеркала);
- переменные (protected, не masked): `MKPK_SPARKLE_ED_PUBLIC_KEY` (публичный
  ключ из шага 2 — он не секрет), при переиспользовании чужого keychain —
  `MKPK_CI_KEYCHAIN` и `MKPK_CI_KEYCHAIN_PASS_FILE`.

## Риски / уроки, заложенные заранее

- `GIT_DEPTH: "0"` обязателен: `CFBundleVersion = git rev-list --count HEAD`,
  на мелком клоне Sparkle увидит «даунгрейд».
- Sparkle-фреймворк: подписывать вложенные `Autoupdate`/`*.xpc`/`Updater.app`
  **до** внешней печати, иначе нотаризация отобьёт бандл.
- GitHub CDN с раннера нестабилен → все обращения к GitHub с ретраями;
  провал GitHub-шага не валит уже опубликованный GitLab-релиз (job
  докладывает и падает в конце — перезапуск джобы безопасен, шаги идемпотентны).
- Нотаризация: транзиентные таймауты Apple — просто перезапустить job.
- В `sign.sh` для Wails-приложения `MKPK_PROVISION_PROFILE=""` — иначе туда
  уедет provisioning profile клиента с чужим bundle id.
