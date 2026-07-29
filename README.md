<p align="center">
  <img src="docs/logo.png" width="140" alt="mkpk">
</p>

<h1 align="center">mkpk — MikroTik PSK Knock</h1>

<p align="center">
  Временное открытие сервисов на MikroTik по authenticated port-knock.<br>
  Порты не висят в интернете постоянно — они открываются на короткое время только
  для конкретного адреса, после успешного «стука» с секретным ключом.
</p>

<p align="center">
  <a href="https://gitlab.eg23.ru/lazygatto/mikrotik-psk-knock/-/releases">Releases</a> ·
  <a href="docs/roadmap.md">Roadmap</a> ·
  <a href="docs/man/">Man pages</a>
</p>

---

## Что это

`dst-nat`-проброс, открытый из интернета круглосуточно, — это постоянная мишень. **mkpk**
держит сервис закрытым и открывает его точечно: клиент отправляет staged UDP-«стук» и
короткоживущий PSK-токен, роутер проверяет их и добавляет **именно этот** source-адрес в
allowed-list с таймаутом. Всё остальное время порт закрыт, а на роутере нет ни постоянных
дырок, ни внешнего сервиса — только нативные средства RouterOS.

Проект — это не только протокол, а готовый набор инструментов вокруг него:

- **Рантайм-клиент** `mkpk` — «стучится» и проверяет доступность (CLI).
- **Админка** `mkpk-provision` — конфиг, рендер RouterOS-скрипта, деплой по SSH, выдача
  клиентам. Доступна как **CLI**, как **локальный веб-UI** (`serve`) и как **десктоп-приложение**
  (`mkpk-provision-desktop`, нативное окно на Wails) — всё поверх одного ядра.
- **Клиент для macOS** (`client-macos/`) — нативное **меню-бар приложение** для получателей
  инвайта: импортирует `.mkpk`, стучится/проверяет, показывает обратный отсчёт открытого доступа
  и умеет «держать открытым» (авто-перестук перед истечением). Тот же крипто-рантайм, что и в CLI
  (переписан на Swift и сверен с Go golden-векторами).

CLI и веб-UI (`serve`, открывается в браузере) работают на **любой ОС**; нативный десктоп — это
просто удобная обёртка того же UI и собирается только под macOS. Криптографический рантайм
полностью на стороне клиента; SSH — только канал развёртывания.

## Возможности

- **Port-knock по PSK-time-token** — staged UDP как дешёвый фильтр + `sha512`-токен с привязкой
  к времени (bucket), poller-модель `token-hit → poller → allowed` сужает replay-окно.
- **Мульти-роутер, юзер × сервис** — один конфиг на много роутеров; матрица доступа; отдельный
  PSK на пару (юзер, роутер); токен per-service.
- **Раздача клиентам** — компактный invite-blob на юзера (только его адрес роутера, PSK, сервисы),
  без общего админ-конфига.
- **Три фронтенда, одно ядро** — CLI (скриптуемо, для Ansible), локальный веб-UI (loopback +
  per-session токен) и десктоп-обёртка.
- **Уведомления** — webhook / Telegram / email на каждый успешный стук, с graceful degradation.
- **SSH-провижининг** — установка/обновление/снятие слоя по SSH, идемпотентно (detect по config-hash),
  с dry-run.
- **Безопасность по умолчанию** — конфиг со всеми секретами пишется 0600 атомарно и не покидает
  машину; веб — только loopback; invite несёт лишь публичный адрес роутера.

## Как это работает

```text
client
  -> UDP knock stage 1
  -> UDP knock stage 2
  -> UDP token stage с короткоживущим PSK-токеном

MikroTik
  -> добавляет src-address в token-hit address-list
  -> poller выбирает допустимый hit и помечает bucket/token как used
  -> добавляет этот src-address в allowed address-list с таймаутом
  -> шлёт уведомление владельцу
  -> dst-nat начинает работать только для этого src-address
```

## Требования к роутеру

- RouterOS 7.x (проверено на 7.23.2).
- **Точные часы (включённый NTP).** Токен привязан к 30-секундному time-bucket, и роутер принимает
  только текущий + предыдущий bucket. Если часы роутера уплывут больше чем на ~пол-бакета, токены
  перестают совпадать и **стук молча не работает** (в firewall видно: stage1/stage2 матчатся, а
  token-правило — 0 пакетов). Включить: `/system ntp client set enabled=yes` + добавить сервер.
  Провижн-приложение (веб/десктоп) при опросе роутера сверяет время и статус NTP и показывает
  предупреждение, если стук работать не будет.

## Установка

Готовые бинари — во вкладке [Releases](https://gitlab.eg23.ru/lazygatto/mikrotik-psk-knock/-/releases)
(собираются CI на теге). Каждый CLI лежит в per-платформенном `.zip` — внутри бинарь с обычным
именем, бит исполняемости сохранён (`chmod +x` не нужен).

Для **macOS** дополнительно есть два нативных приложения — **DMG** (drag-to-Applications):
`mkpk-provision-desktop` (админка) и `mkpk-client` (клиент-получатель, меню-бар).

> **macOS — карантин Gatekeeper.** Пока сборки **не нотаризованы** ([#12](https://gitlab.eg23.ru/lazygatto/mikrotik-psk-knock/-/issues/12)),
> скачанные бинари/приложения помечаются карантином. Снять вручную:
> - CLI: `xattr -d com.apple.quarantine ./mkpk`
> - приложение: `xattr -cr /Applications/mkpk.app` (аналогично для `mkpk-provision-desktop.app`)

Сборка из исходников (каталог `client/`):

```text
make build       # CLI: bin/mkpk и bin/mkpk-provision (версия из git-тега)
make desktop     # десктоп-админка .app (macOS; нужны wails CLI + Xcode CLT)
make install     # бинари + man-страницы под PREFIX (по умолчанию /usr/local)
make test        # go test ./...
```

Клиент для macOS — отдельный SwiftPM-проект: `cd client-macos && script/build_app.sh`
(и `script/make_dmg.sh` для DMG). Подробности — в [client-macos/AGENTS.md](client-macos/AGENTS.md).

## CLI и автоматизация

Все три фронтенда — тонкие обёртки над ядром `internal/admin`, и **CLI самодостаточен**: веб и
десктоп ничего не умеют сверх него. Типовой headless-поток:

```text
mkpk-provision profile init --out mkpk.yaml --router-name r1 --router-address r1.example.com
mkpk-provision service add --config mkpk.yaml --name ssh \
  --stage1-port 41011 --stage2-port 41012 --token-port 41013 \
  --target-type forward --target-port 22 --target-to-address 192.0.2.10 --target-to-port 22
mkpk-provision user add --config mkpk.yaml --name laptop --services ssh
mkpk-provision deploy --config mkpk.yaml               # ставит слой по SSH
mkpk-provision export --config mkpk.yaml --user laptop --out laptop.mkpk
mkpk knock --invite @laptop.mkpk --service ssh --check # на стороне клиента
```

`deploy` и `config validate` поддерживают `--json` для скриптов/Ansible; `check --json` даёт
machine-readable результат доступности. Полный справочник — в man-страницах (`mkpk(1)`,
`mkpk-provision(1)`) или `mkpk-provision help`.

## Статус

Рабочая ROS-only реализация с CLI, локальным веб-UI, десктоп-админкой и **нативным macOS-клиентом**
для получателей инвайта, плюс SSH-провижининг и стриминг прогресса деплоя; всё проверено end-to-end
на живых роутерах (RouterOS 7.x). Версионирование — semver, пре-1.0; актуальная версия и бинари — во вкладке [Releases](https://gitlab.eg23.ru/lazygatto/mikrotik-psk-knock/-/releases). Есть также
end-to-end **тест стука** из провижн-приложения (стучит и по SSH сверяет счётчики/лог/порт роутера)
и «держать открытым» в клиенте. Дальше по плану: нотаризация macOS-сборок (Developer ID), ICMP-вариант
транспорта. Подробности — в [docs/roadmap.md](docs/roadmap.md).

## Документы

- [docs/context.md](docs/context.md) — консолидированный контекст и технические заметки.
- [docs/design.md](docs/design.md) — первичный дизайн ROS-only решения.
- [docs/threat-model.md](docs/threat-model.md) — модель угроз и ограничения.
- [docs/admin-app.md](docs/admin-app.md) — модель админ-приложения, мульти-роутер, раздача (invite-blob).
- [docs/multi-profile-render.md](docs/multi-profile-render.md) — схема render и data-driven poller.
- [docs/profile-format.md](docs/profile-format.md) — справочник полей конфига.
- [docs/open-questions.md](docs/open-questions.md) — открытые вопросы и принятые решения.
- [docs/roadmap.md](docs/roadmap.md) — план дальнейшей работы.
- [docs/man/](docs/man/) — man-страницы CLI.
- [client/README.md](client/README.md) — детали CLI, provisioning и deploy по SSH.
