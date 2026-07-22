# Инструкции для агента

Основной язык проекта: русский.

## Контекст

Проект исследует authenticated port knocking для MikroTik RouterOS.

Главная линия: ROS-only реализация через staged UDP knock и PSK-derived short-lived token на базе `:convert transform=sha512`.

Текущий важный дизайн-вывод: в RouterOS нужно учитывать два runtime - firewall packet path и scheduler/scripting. Их нельзя считать одной атомарной программой.

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
- Основной сценарий - dynamic/roaming clients; known static IP не является основной веткой.
- Не привязывать основной token к source IP, если это требует заранее известных адресов.
- Для ROS-only replay mitigation рассматривать token-hit address-list и scheduler polling около 1s.
- При нескольких hits одного token/bucket за polling interval не разрешать все адреса.
- SSH/Ed25519 рассматривать как отдельный более строгий режим, а не смешивать с UDP-token без verifier.

## Перед имплементацией

Сначала проверить реальные возможности RouterOS:

- `:convert` синтаксис и output для `sha512`;
- получение time bucket;
- firewall `content` matching по UDP payload;
- обновление firewall rules scheduler-ом;
- polling token-hit address-list с interval около 1s;
- возможность надежно определить первый dynamic address-list entry;
- производительность matcher-ов;
- notification channels.

## Документация

При изменениях обновлять:

- `docs/context.md` для решений и новых фактов;
- `docs/design.md` для архитектуры;
- `docs/threat-model.md` для security caveats;
- `docs/roadmap.md` для статуса работ.
