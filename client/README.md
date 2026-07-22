# Client

Здесь находится Go CLI для расчета token, отправки staged UDP knock и генерации RouterOS `.rsc`.

Клиент должен поддерживать profiles, dry-run/debug mode и проверку рассинхронизации времени.

## Команды

```bash
go run ./cmd/mkpk secret generate
go run ./cmd/mkpk token --config testdata/mkpk.yaml --client demo-client --debug
go run ./cmd/mkpk knock --config testdata/mkpk.yaml --client demo-client --debug
go run ./cmd/mkpk routeros render --config testdata/mkpk.yaml --client demo-client --out ../routeros/generated-demo.rsc
```

Текущий `routeros render` намеренно рендерит один выбранный client/service в уже проверенную
single-profile RouterOS схему. Config format допускает несколько services/clients, но multi-profile
RouterOS runtime еще не реализован.

PSK в `testdata/mkpk.yaml` демонстрационный. Production-конфигурация не должна хранить реальные секреты
в открытом репозитории.

## Текущий статус проверки

- `token` совпадает с shell `shasum -a 512` для RouterOS prototype formula.
- `routeros render` генерирует `.rsc`, который успешно импортируется на CHR.
- Сгенерированный `.rsc` создает one-shot `mkpk-tt-install`; после import token rules активируются без reboot.
- `knock --debug` показывает router, bucket, stage ports, local UDP address, remote UDP address и bytes sent.

Ограничение текущей локальной проверки: на macOS в этом окружении route до CHR идет через `utun18`, а Go
UDP получает local source `198.18.0.1`; такие пакеты до CHR не доходят, хотя `nc -u` из той же shell-сессии
доставляет UDP. Поэтому сам Go UDP transport требует повторной проверки вне этого VPN/proxy route или с
явно настроенным сетевым окружением.
