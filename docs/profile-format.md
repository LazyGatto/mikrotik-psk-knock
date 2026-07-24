# Profile format

Текущий RouterOS-only прототип использует persistent profile script как простой и проверенный формат
хранения параметров клиента/сервиса.

## RouterOS profile script

Пример:

```routeros
/system script
add name="mkpk-tt-profile-demo" policy=read,write,test source={
    :global mkpkTtService "demo-service"
    :global mkpkTtClientId "demo-client"
    :global mkpkTtPsk "mkpk-prototype-psk"
    :global mkpkTtTokenPort 41003
    :global mkpkTtAllowedList "mkpk-tt-allowed"
    :global mkpkTtAllowedTimeout "3m"
    :global mkpkTtUsedTimeout "65s"
    :global mkpkTtNotifyEnabled false
    :global mkpkTtNotifyUrl ""
    :global mkpkTtNatEnabled false
    :global mkpkTtNatComment "mkpk-tt dst-nat demo ssh"
    :global mkpkTtNatDstPort 2222
    :global mkpkTtNatToAddress "192.0.2.10"
    :global mkpkTtNatToPort 22
}
```

`mkpk-tt-poller` запускает profile script перед расчетом token и читает значения из globals. Это
сохраняет профиль в RouterOS config и убирает hardcoded service/client/PSK из основной poller-логики.

## Поля

- `service` - имя сервиса, включается в token message.
- `client_id` - идентификатор клиента, включается в token message и audit comment.
- `psk` - per-client или per-profile секрет. В Go config допускается только base64url-safe ASCII
  alphabet (`A-Z`, `a-z`, `0-9`, `-`, `_`), чтобы RouterOS string interpolation не меняла значение
  секрета при рендере profile script.
- `token_port` - UDP порт token stage.
- `allowed_list` - address-list, через который ограничивается `dst-nat`.
- `allowed_timeout` - время открытия observed source IP.
- `used_timeout` - время жизни used-bucket marker. Должен быть не меньше `2 * bucket_seconds`, чтобы
  marker перекрывал полный интервал приема `now` и `prev` token buckets.
- `notify_enabled` - включает webhook notification path.
- `notify_url` - URL для `/tool fetch` POST после успешного allow.
- `nat_enabled` - включает созданный service `dst-nat` rule.
- `nat_comment` - стабильный comment, по которому `mkpk-tt-apply-service` находит NAT rule.
- `nat_dst_port` - внешний TCP port для `dstnat`.
- `nat_to_address` - внутренний адрес сервиса.
- `nat_to_port` - внутренний TCP port сервиса.

## NAT и notification

`dst-nat` остается отдельным persistent firewall object. Прототип создает или обновляет его из profile
fields через `mkpk-tt-apply-service`:

```routeros
/system script run mkpk-tt-apply-service
```

Для production-профиля нужно заменить `nat_dst_port`, `nat_to_address` и `nat_to_port`, затем поставить
`nat_enabled=true` и запустить `mkpk-tt-apply-service`. Успешный knock не меняет NAT rule, а только
добавляет observed source IP в `allowed_list`. Startup guard также запускает `mkpk-tt-apply-service`
после reboot.

CHR-проверка подтвердила, что `mkpk-tt-apply-service` обновляет existing NAT rule из profile fields,
включая `disabled`, `dst-port`, `to-addresses` и `to-ports`. Modified profile values пережили reboot,
startup пере-применил NAT rule, и post-reboot knock снова открыл observed source IP.

Notification hook реализован отдельным script `mkpk-tt-notify`. Poller вызывает его после добавления
`allowed_list` entry и удаления `token-hit`. Если notification выключен или webhook падает, allow не
откатывается; ошибка логируется локально.

Webhook payload формируется как корректный JSON через `[:serialize {...} to=json]` и отправляется с
`Content-Type: application/json`. Спецсимволы в значениях экранируются сериализатором, поэтому прежнее
ограничение сырого `key=value&...` снято.

## Ограничения

PSK в таком прототипе хранится в RouterOS script source. Это удобно для проверки, но требует строгого
ограничения прав RouterOS users/groups на чтение scripts. Production-вариант должен отдельно описать
секреты, ротацию и экспорт/backup hygiene.
