# Client

Здесь находится Go CLI для расчета token, отправки staged UDP knock и генерации RouterOS `.rsc`.

Клиент должен поддерживать profiles, dry-run/debug mode и проверку рассинхронизации времени.

## Команды

```bash
go run ./cmd/mkpk secret generate
go run ./cmd/mkpk token --config testdata/mkpk.yaml --client demo-client --debug
go run ./cmd/mkpk knock --config testdata/mkpk.yaml --client demo-client --debug
go run ./cmd/mkpk knock --config testdata/mkpk.yaml --client demo-client --noise 2 --debug
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
- `knock --debug` проверен на CHR: retry windows проходят stage1/stage2/token, `mkpk-tt-allowed`
  получает observed source IP.
- `knock --debug` показывает router, bucket, stage ports, local UDP address, remote UDP address и bytes sent.
- `knock --noise N` отправляет N random UDP payloads на token port вокруг фаз. По умолчанию `0`, потому
  что noise увеличивает traffic/counters и должен быть осознанным режимом.

Примечание по локальному окружению: при включенном LuLu Go UDP попадал под проверку локального firewall.
После временного отключения LuLu `mkpk knock` начал доходить до CHR. Старый single-delay режим был
заменен на retry windows: по умолчанию stage1 и stage2 отправляются 2 секунды с interval 250ms, token -
1 секунду с interval 250ms.
