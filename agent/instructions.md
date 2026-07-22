# Инструкции для агента

Основной язык проекта: русский.

## Контекст

Проект исследует authenticated port knocking для MikroTik RouterOS.

Главная линия: ROS-only реализация через staged UDP knock и PSK-derived short-lived token на базе `:convert transform=sha512`.

Важно не переименовывать это в полноценный HMAC, пока HMAC действительно не реализован. Текущий компромиссный термин:

```text
PSK time-token gated port opening for RouterOS
```

## Принципы

- Не включать и не выключать NAT rules динамически; использовать статический `dst-nat` с `src-address-list`.
- Успешный knock добавляет только observed source IP.
- Любой allow должен иметь timeout.
- Уведомление о новом allowed IP - обязательная часть дизайна.
- Ошибка уведомления не должна ломать открытие доступа, но должна логироваться.
- Replay caveat нужно документировать честно.
- SSH/Ed25519 рассматривать как отдельный более строгий режим, а не смешивать с UDP-token без verifier.

## Перед имплементацией

Сначала проверить реальные возможности RouterOS:

- `:convert` синтаксис и output для `sha512`;
- получение time bucket;
- firewall `content` matching по UDP payload;
- обновление firewall rules scheduler-ом;
- производительность matcher-ов;
- notification channels.

## Документация

При изменениях обновлять:

- `docs/context.md` для решений и новых фактов;
- `docs/design.md` для архитектуры;
- `docs/threat-model.md` для security caveats;
- `docs/roadmap.md` для статуса работ.

