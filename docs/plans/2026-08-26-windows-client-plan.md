# План: Windows GUI-клиент mkpk-desktop (Wails) — issue #26

Дата: 2026-08-26. Решения maintainer'а: стек Wails v2; без подписи кода
(SmartScreen-предупреждение принято); имя `mkpk-desktop` (зарезервировано в
комментарии `cmd/mkpk-provision-desktop/main.go`).

## Архитектурная проверка

1. **Документы:** README (после выката фичи), CHANGELOG. Протокольные документы
   не трогаются.
2. **Инварианты:** формула токена, формат инвайта, `.rsc` — не затронуты вообще:
   клиент линкует эталонный рантайм `client/internal/{invite,token,knock,servicecheck}`
   напрямую. Нет ни новой реализации протокола, ни нового потребителя золотых
   векторов. SSH в клиенте нет.
3. **Совместимость:** не меняется — читаем те же `.mkpk` v2.
4. **Новые файлы:** `client/cmd/mkpk-desktop/` (main + wails.json),
   `client/internal/desktopui/` (HTTP-handler c embedded-frontend + файловое
   хранилище инвайтов). Новый каталог — решение Team Lead'а: да, по образцу
   существующей пары provision-desktop / internal/web.
5. **Зависимости:** новых нет — Wails v2 уже в go.mod (provision-desktop).
   Windows-таргет Wails собирается **без cgo** (go-webview2, чистые syscalls) →
   кросс-компиляция обычным `GOOS=windows go build -tags desktop,production`
   на любом хосте; Windows-раннер и wails CLI в CI не нужны.
6. **Локализация:** EN/RU словарём в frontend (аналог `L()` macOS-клиента),
   английский по умолчанию.
7. **Живой роутер:** для разработки не нужен (гейт — httptest + loopback);
   финальный smoke на реальной Windows-машине против роутера — maintainer.

## Архитектура (паттерн provision-desktop)

Wails-окно (~420×640) монтирует внутренний `http.Handler` как asset server:
per-session токен инжектится в страницу и требуется на каждом `/api/*`
(копия схемы `internal/web`). Никаких wails-bindings (`-skipbindings`),
frontend — один статический index.html без npm.

- Хранилище: `%APPDATA%\mkpk\` (`os.UserConfigDir()`), инвайты «как импортированы»
  (файл = blob), язык в `settings.json`. DPAPI-шифрование — фоллоу-ап (#26).
- Knock-поток на сервис = точный CLI-путь: ожидание возраста бакета ≥2s →
  `token.Compute` → `knock.Run` (staged UDP) → `servicecheck.Check`
  (host = адрес роутера, port = `check_port`, 10×500ms) → статус
  open / closed / error + attempts.

## Шаги (зоны)

| # | Что | Зона |
| --- | ----- | ------ |
| 1 | `internal/desktopui`: store + handler + `/api/{state,import,remove,knock,language}` + httptest-тесты | `client/internal/desktopui/` |
| 2 | `cmd/mkpk-desktop`: wails main + frontend (index.html, EN/RU) | `client/cmd/mkpk-desktop/` |
| 3 | Гейт: исключить пакет из дефолтного списка (как provision-desktop), добавить стадию кросс-сборки `GOOS=windows` | `scripts/verify.sh` |
| 4 | CI: windows amd64/arm64 exe в `build:binaries` → zip → registry; GitHub-синк подхватит из артефактов | `.gitlab-ci.yml`, `scripts/package_release.sh` |
| 5 | CHANGELOG | корень |

Готово, когда: `bash scripts/verify.sh` = 0 (включая новую windows-стадию и
httptest-пакет); zip с exe появляется в релизе следующего тега; maintainer
подтверждает live-smoke на Windows.

## Фоллоу-апы (не в этой волне)

Toast-уведомления, egress-IP-монитор, самообновление (проверка GitHub releases
API), DPAPI, иконка/манифест exe (winres), трей (Wails v3, когда стабилизируется).
