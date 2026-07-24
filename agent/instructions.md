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
- SSH — только канал провижининга (`mkpk-provision deploy`), не runtime-режим открытия доступа.
- Изменения в RouterOS-логике проверять на живом CHR перед фиксацией (RouterOS-семантика часто
  неочевидна: `:return`-цепочки, single-element arrays, `:serialize` ключи, scp vs sftp и т.п.).

## Возможности RouterOS (проверены на CHR)

Ключевые примитивы уже подтверждены: `:convert sha512`, time bucket (`:timestamp`), firewall `content`
по UDP payload, регулярное обновление rules poller-ом, scheduler `interval=1s` (в т.ч. под нагрузкой),
`do={}`-функции с `:return`, `:foreach` по массиву массивов, `:serialize ... to=json`, scp + `/import`,
persistent-маркер для detect. Для нового RouterOS-поведения — сначала разведка на живом CHR.

## Документация

При изменениях обновлять:

- `docs/context.md` для решений и новых фактов;
- `docs/design.md` для архитектуры;
- `docs/multi-profile-render.md` для render/poller-раскладки;
- `docs/threat-model.md` для security caveats;
- `docs/open-questions.md` для статусов вопросов;
- `docs/roadmap.md` для статуса работ.
