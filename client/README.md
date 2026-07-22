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
go run ./cmd/mkpk knock --config testdata/mkpk.yaml --client demo-client --debug
go run ./cmd/mkpk knock --config testdata/mkpk.yaml --client demo-client --check --debug
go run ./cmd/mkpk knock --config testdata/mkpk.yaml --client demo-client --min-bucket-age 2s --debug
go run ./cmd/mkpk knock --config testdata/mkpk.yaml --client demo-client --noise 2 --debug
go run ./cmd/mkpk-provision secret generate
go run ./cmd/mkpk-provision token --config testdata/mkpk.yaml --client demo-client --debug
go run ./cmd/mkpk-provision routeros render --config testdata/mkpk.yaml --client demo-client --out ../routeros/generated-demo.rsc
```

Текущий `mkpk-provision routeros render` намеренно рендерит один выбранный client/service в уже проверенную
single-profile RouterOS схему. Config format допускает несколько services/clients, но multi-profile
RouterOS runtime еще не реализован.

PSK в `testdata/mkpk.yaml` демонстрационный. Production-конфигурация не должна хранить реальные секреты
в открытом репозитории. `psk` должен использовать base64url-safe ASCII alphabet: `A-Z`, `a-z`, `0-9`,
`-` и `_`; `mkpk-provision secret generate` уже выдает такой формат.

## Текущий статус проверки

- `token` совпадает с shell `shasum -a 512` для RouterOS prototype formula.
- `mkpk-provision routeros render` генерирует `.rsc`, который успешно импортируется на CHR.
- `mkpk-provision routeros render` использует configured `defaults.bucket_seconds` в RouterOS poller, чтобы клиент и
  RouterOS считали один и тот же time bucket.
- Сгенерированный `.rsc` создает one-shot `mkpk-tt-install`; после import token rules активируются без reboot.
- `knock --debug` проверен на CHR: retry windows проходят stage1/stage2/token, `mkpk-tt-allowed`
  получает observed source IP.
- `knock --debug` показывает router, bucket, stage ports, local UDP address, remote UDP address и bytes sent.
- `check` выполняет тот же TCP connect-check целевого endpoint без отправки knock. Это runtime primitive
  для before/after status в будущем UI.
- `knock --check` после отправки knock выполняет TCP connect-check целевого endpoint. По умолчанию
  проверяется `router:service.nat.dst_port`; можно переопределить через `--check-host` и `--check-port`.
- `knock` по умолчанию ждет, пока текущий bucket станет хотя бы на 2 секунды старше
  (`--min-bucket-age 2s`). Это снижает риск, что клиент с чуть спешащими часами отправит token для
  bucket, который RouterOS еще не принимает.
- `knock --noise N` отправляет N random UDP payloads на token port вокруг фаз. По умолчанию `0`, потому
  что noise увеличивает traffic/counters и должен быть осознанным режимом.

Примечание по локальному окружению: при включенном LuLu Go UDP попадал под проверку локального firewall.
После временного отключения LuLu `mkpk knock` начал доходить до CHR. Старый single-delay режим был
заменен на retry windows: по умолчанию stage1 и stage2 отправляются 2 секунды с interval 250ms, token -
1 секунду с interval 250ms.
