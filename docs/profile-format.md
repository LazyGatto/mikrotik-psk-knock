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
    :global mkpkTtUsedTimeout "35s"
    :global mkpkTtNotifyEnabled false
    :global mkpkTtNotifyUrl ""
}
```

`mkpk-tt-poller` запускает profile script перед расчетом token и читает значения из globals. Это
сохраняет профиль в RouterOS config и убирает hardcoded service/client/PSK из основной poller-логики.

## Поля

- `service` - имя сервиса, включается в token message.
- `client_id` - идентификатор клиента, включается в token message и audit comment.
- `psk` - per-client или per-profile секрет.
- `token_port` - UDP порт token stage.
- `allowed_list` - address-list, через который ограничивается `dst-nat`.
- `allowed_timeout` - время открытия observed source IP.
- `used_timeout` - время жизни used-bucket marker.
- `notify_enabled` - включает webhook notification path.
- `notify_url` - URL для `/tool fetch` POST после успешного allow.

## NAT и notification

`dst-nat` остается отдельным persistent firewall object. В прототипе есть disabled demo-rule:

```routeros
/ip firewall nat
add chain=dstnat action=dst-nat protocol=tcp dst-port=2222 \
    src-address-list=mkpk-tt-allowed to-addresses=192.0.2.10 to-ports=22 \
    disabled=yes comment="mkpk-tt dst-nat demo ssh"
```

Для production-профиля нужно заменить `dst-port`, `to-addresses` и `to-ports`, затем явно включить rule.
Успешный knock не меняет NAT rule, а только добавляет observed source IP в `allowed_list`.

Notification hook реализован отдельным script `mkpk-tt-notify`. Poller вызывает его после добавления
`allowed_list` entry и удаления `token-hit`. Если notification выключен или webhook падает, allow не
откатывается; ошибка логируется локально.

Текущий webhook payload является простым form-like `key=value&...` без полноценного URL-encoding. Это
достаточно для demo values, но production-вариант должен ограничить допустимые символы или добавить
корректное encoding/JSON-формирование.

## Ограничения

PSK в таком прототипе хранится в RouterOS script source. Это удобно для проверки, но требует строгого
ограничения прав RouterOS users/groups на чтение scripts. Production-вариант должен отдельно описать
секреты, ротацию и экспорт/backup hygiene.
