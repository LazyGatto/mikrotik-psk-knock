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

## Ограничения

PSK в таком прототипе хранится в RouterOS script source. Это удобно для проверки, но требует строгого
ограничения прав RouterOS users/groups на чтение scripts. Production-вариант должен отдельно описать
секреты, ротацию и экспорт/backup hygiene.
