# MikroTik PSK Knock

Проект для проработки и дальнейшей реализации безопасного временного открытия `dst-nat` на MikroTik через authenticated knock.

Основная идея: не держать порт-форварды доступными из Интернета постоянно, а открывать их на короткое время только для конкретного `src-address` после успешного knock.

## Цели

- Реализовать максимально автономный RouterOS-only вариант без Docker и внешнего сервиса там, где это возможно.
- Использовать staged UDP knock как дешевый предварительный фильтр.
- Использовать PSK-derived time-token на базе возможностей RouterOS `:convert transform=sha512`.
- Добавлять успешный `src-address` в firewall address-list с timeout.
- Держать `dst-nat` правила статическими, но ограниченными через `src-address-list`.
- Отправлять уведомления при каждом добавлении нового адреса в разрешенный список.
- Сделать клиентское приложение с CLI (далее — локальный веб-UI и десктоп).
- Провижининг на роутер выполнять по SSH; runtime port-knocking держать полностью на стороне клиента.

## Базовый поток

```text
client
  -> UDP knock stage 1
  -> UDP knock stage 2
  -> UDP token stage with short-lived PSK token

MikroTik
  -> добавляет src-address в token-hit address-list
  -> scheduler выбирает допустимый hit и помечает bucket/token used
  -> добавляет selected src-address в allowed address-list с timeout
  -> отправляет уведомление владельцу
  -> dst-nat начинает работать только для этого src-address
```

## Статус

Рабочая ROS-only реализация с CLI, локальным веб-UI и десктопом (Wails), плюс SSH-провижининг; всё
проверено end-to-end на живом CHR (RouterOS 7.23.2). Текущая версия — `v0.1.0` (semver, пре-1.0).

Сделано:

- зафиксирован ROS-only дизайн через staged UDP и PSK-derived time-token; polling-модель
  `token-hit -> poller -> allowed` для сужения replay window;
- проверены на CHR ключевые RouterOS-примитивы: `sha512`, time bucket через `:timestamp`, UDP `content`,
  обновление rule content, scheduler 1s, reboot-survival, startup guard;
- **клиент** (Go): `mkpk` (runtime — `knock`, `check`) и `mkpk-provision` (admin — `secret`, `config`,
  `profile`, `router`, `service`, `user`, `token`, `routeros render`, `export`, `deploy`, `serve`);
- **multi-router / user×service**: один конфиг на много роутеров (сервисы и юзеры принадлежат роутеру),
  матрица доступа, per-(user,router) PSK, раздача клиенту через per-user invite-blob;
- **multi-profile render**: все services/clients в per-profile RouterOS-объекты с per-service изоляцией
  `allowed`-list; **data-driven poller** — один скрипт + один scheduler на все профили, с кэшем и hit-guard;
- **локальный веб-UI** `mkpk-provision serve` (loopback + per-session токен + Host-guard) поверх ядра
  `internal/admin`; **десктоп** `mkpk-desktop` — тот же UI в нативном окне (Wails v2, in-process, без
  открытого порта);
- **уведомления**: каналы `webhook`, `telegram`, `email` с graceful degradation;
- **SSH-провижининг**: `mkpk-provision deploy` ставит/обновляет/снимает mkpk-слой по SSH с detect по
  config-hash и verify;
- фиксация used-marker `used_timeout >= 2*bucket_seconds`, валидация конфига на входе и на бэке (адреса
  IP/hostname, имена, порты 1..65535, таймауты, PSK-alphabet).

Следующие шаги: релизная обвязка (CI + готовые бинари), стриминг прогресса деплоя и клиентский GUI для
получателей инвайта. План — в [docs/roadmap.md](docs/roadmap.md).

## Сборка и установка

Из каталога `client/`:

```text
make build       # CLI: bin/mkpk и bin/mkpk-provision (версия из git-тега)
make desktop     # десктоп .app (macOS; нужны wails CLI + Xcode CLT)
make test        # go test ./...
make install     # ставит бинари и man-страницы под PREFIX (по умолчанию /usr/local)
```

`make install PREFIX=~/.local` ставит `mkpk`/`mkpk-provision` в `bin/` и man-страницы
(`mkpk(1)`, `mkpk-provision(1)`, исходники в [docs/man/](docs/man)).

## CLI-first и автоматизация

Все три бинаря — тонкие фронтенды над одним ядром `internal/admin`, и **CLI полностью
самодостаточен**: веб-UI (`mkpk-provision serve`) и десктоп (`mkpk-desktop`) ничего не умеют
сверх CLI. Для Ansible/скриптов всё делается headless — `mkpk-provision router set …`,
`… service add …`, `… user add …`, `… deploy …`, `… export …`, а на клиенте `mkpk knock …`.
Полный список — в man-страницах или `mkpk-provision help`.

## Релизы

Готовые бинари — во вкладке Releases (собираются CI на теге `vX.Y.Z`). Каждый CLI лежит в
per-платформенном `.zip` (внутри — бинарь с обычным именем; zip сохраняет бит исполняемости,
так что `chmod +x` не нужен). Десктопный `.app` для macOS прикладывается отдельным архивом.

**macOS:** скачанные неподписанные бинари помечаются карантином Gatekeeper. Пока не настроена
нотаризация (нужен Apple Developer ID), снять карантин можно вручную:

```text
xattr -d com.apple.quarantine ./mkpk
```

Версия штампуется через `-ldflags` из `git describe --tags`; `mkpk version` / `mkpk-provision version`
её печатают. Веб-UI показывает версию в футере сайдбара.

## Документы

- [docs/context.md](docs/context.md) - консолидированный контекст обсуждения и технические заметки.
- [docs/design.md](docs/design.md) - первичный дизайн ROS-only решения.
- [docs/threat-model.md](docs/threat-model.md) - модель угроз и ограничения.
- [docs/open-questions.md](docs/open-questions.md) - открытые вопросы и принятые концептуальные решения.
- [docs/profile-format.md](docs/profile-format.md) - справочник полей конфига (service/client/notify/nat).
- [docs/multi-profile-render.md](docs/multi-profile-render.md) - схема multi-profile render и data-driven poller.
- [docs/admin-app.md](docs/admin-app.md) - модель админ-приложения, multi-router и раздача клиентам (invite-blob).
- [docs/roadmap.md](docs/roadmap.md) - план дальнейшей работы.
- [client/README.md](client/README.md) - CLI, provisioning и deploy по SSH.
- [agent/instructions.md](agent/instructions.md) - инструкции для будущего агента/разработчика.
