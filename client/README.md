# Client

Здесь находятся два Go CLI:

- `mkpk` - runtime-клиент для everyday mobile/roaming use.
- `mkpk-provision` - provisioning/admin tool для safe сети.

## User flow

`mkpk-provision routeros render` и будущие provisioning/apply команды рассчитаны на safe/admin среду, где
есть полный management-доступ к MikroTik. После импорта конфигурации runtime-сценарий для mobile/roaming
клиента не должен требовать RouterOS SSH/API: `mkpk knock` отправляет только staged UDP packets и
PSK-derived time-token из внешней небезопасной сети.

Опциональный admin/break-glass режим через SSH/API может появиться отдельно, но он не является частью
основного stealth UDP-token flow.

## Команды

```bash
go run ./cmd/mkpk check --config testdata/mkpk.yaml --client demo-client --debug
go run ./cmd/mkpk check --config testdata/mkpk.yaml --client demo-client --json
go run ./cmd/mkpk knock --config testdata/mkpk.yaml --client demo-client --debug
go run ./cmd/mkpk knock --config testdata/mkpk.yaml --client demo-client --check --debug
go run ./cmd/mkpk knock --config testdata/mkpk.yaml --client demo-client --min-bucket-age 2s --debug
go run ./cmd/mkpk knock --config testdata/mkpk.yaml --client demo-client --noise 2 --debug
go run ./cmd/mkpk-provision secret generate
go run ./cmd/mkpk-provision profile init --out mkpk.yaml --router-address router.example
go run ./cmd/mkpk-provision service add --config mkpk.yaml --name ssh-home --stage1-port 41011 --stage2-port 41012 --token-port 41013 --nat-dst-port 2022 --nat-to-address 192.0.2.10 --nat-to-port 22
go run ./cmd/mkpk-provision client add --config mkpk.yaml --name phone --service demo-service
go run ./cmd/mkpk-provision config validate --config testdata/mkpk.yaml
go run ./cmd/mkpk-provision token --config testdata/mkpk.yaml --client demo-client --debug
go run ./cmd/mkpk-provision routeros render --config testdata/mkpk.yaml --client demo-client --out ../routeros/generated-demo.rsc
```

`mkpk-provision routeros render` без `--client` рендерит все services и clients из конфига в
per-profile RouterOS объекты (multi-profile). С `--client NAME` рендерится только один клиент и его
service. Каждый service получает свой `allowed` address-list (`mkpk-tt-allowed-<service>`), stage-правила
и NAT; каждый client — свои token-правила, hit-списки, poller и scheduler. Схема описана в
[../docs/multi-profile-render.md](../docs/multi-profile-render.md).

Multi-profile рендер проверен end-to-end на живом CHR (2 services / 2 clients): pollers поднимают token
rules после import без reboot, knock открывает observed source IP, per-service изоляция allowed-list
работает. Детали в [../docs/multi-profile-render.md](../docs/multi-profile-render.md).

PSK в `testdata/mkpk.yaml` демонстрационный. Production-конфигурация не должна хранить реальные секреты
в открытом репозитории. `psk` должен использовать base64url-safe ASCII alphabet: `A-Z`, `a-z`, `0-9`,
`-` и `_`; `mkpk-provision secret generate` уже выдает такой формат.

## Текущий статус проверки

- `token` совпадает с shell `shasum -a 512` для RouterOS prototype formula.
- `mkpk-provision routeros render` генерирует `.rsc`, который успешно импортируется на CHR
  (проверено для single-profile и multi-profile).
- `mkpk-provision routeros render` использует configured `defaults.bucket_seconds` в RouterOS poller, чтобы клиент и
  RouterOS считали один и тот же time bucket.
- `mkpk-provision profile init` создает стартовый YAML с generated PSK и безопасными defaults.
- `mkpk-provision service add` добавляет service/NAT target к существующему YAML. NAT rule остается
  disabled, если не передать `--nat-enabled`.
- `mkpk-provision client add` добавляет нового клиента к существующему service и генерирует PSK, если
  `--psk` не передан явно.
- `mkpk-provision config validate` загружает YAML, применяет defaults, проверяет инварианты и печатает
  summary без раскрытия PSK.
- `mkpk-provision service add` поддерживает выбор канала уведомлений через `--notify-channel webhook`
  (по умолчанию, с `--notify-url`) или `--notify-channel telegram`
  (`--notify-telegram-bot-token`, `--notify-telegram-chat-id`). Telegram-канал проверен на CHR:
  poller вызывает Bot API, ошибка доставки логируется и не откатывает `allowed` entry.
- Сгенерированный `.rsc` создает one-shot `mkpk-tt-install`; после import token rules активируются без reboot.
- `knock --debug` проверен на CHR: retry windows проходят stage1/stage2/token, `mkpk-tt-allowed`
  получает observed source IP.
- `knock --debug` показывает router, bucket, stage ports, local UDP address, remote UDP address и bytes sent.
- `check` выполняет тот же TCP connect-check целевого endpoint без отправки knock. Это runtime primitive
  для before/after status в будущем UI. Он проверяет сквозную TCP-доступность сервиса, а не факт
  появления `allowed` entry на RouterOS.
- `check --json` печатает machine-readable результат со статусом `open`, `closed` или `error`, host/port,
  количеством attempts, duration и error text.
- `knock --check` после отправки knock выполняет TCP connect-check целевого endpoint. По умолчанию
  проверяется `router:service.nat.dst_port`; можно переопределить через `--check-host` и `--check-port`.
- UDP knock transport сейчас IPv4-only (`udp4`); TCP `check` использует обычный `tcp`.
- `knock` по умолчанию ждет, пока текущий bucket станет хотя бы на 2 секунды старше
  (`--min-bucket-age 2s`). Это снижает риск, что клиент с чуть спешащими часами отправит token для
  bucket, который RouterOS еще не принимает.
- `knock --noise N` отправляет N random UDP payloads на token port вокруг фаз. По умолчанию `0`, потому
  что noise увеличивает traffic/counters и должен быть осознанным режимом.

Примечание по локальному окружению: при включенном LuLu Go UDP попадал под проверку локального firewall.
После временного отключения LuLu `mkpk knock` начал доходить до CHR. Старый single-delay режим был
заменен на retry windows: по умолчанию stage1 и stage2 отправляются 2 секунды с interval 250ms, token -
1 секунду с interval 250ms.
