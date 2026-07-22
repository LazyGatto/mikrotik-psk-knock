# Roadmap

## Этап 1: исследование RouterOS

- Проверить на актуальной RouterOS синтаксис `:convert` для `sha512`.
- Проверить, можно ли удобно получить Unix time или стабильный time bucket в RouterOS script.
- Проверить поведение firewall `content` для UDP payload.
- Проверить, можно ли обновлять `content` matcher scheduler-ом без побочных эффектов.
- Проверить стоимость `content` и `layer7-protocol` на тестовом железе.
- Проверить каналы уведомлений: email, Telegram/webhook через `/tool fetch`, syslog.

## Этап 2: прототип RouterOS-only

- Описать profile format.
- Написать RouterOS scripts для генерации token.
- Написать firewall rules для stage1/stage2/token.
- Настроить `dst-nat` через `src-address-list`.
- Добавить notification script.
- Добавить logging и comments для address-list entries.

## Этап 3: клиент CLI

- Выбрать язык и упаковку.
- Реализовать расчет token.
- Реализовать UDP staged knock.
- Реализовать profiles/config.
- Добавить dry-run/debug mode.
- Добавить проверку времени и предупреждение о рассинхронизации.

## Этап 4: SSH/Ed25519 режим

- Проверить права RouterOS user/group для безопасного script run.
- Настроить SSH public-key auth.
- Реализовать client command для SSH-open.
- Сравнить UX и threat model с UDP-token режимом.

## Этап 5: GUI

- Обернуть profiles и actions в GUI.
- Показывать статус последних knock attempts.
- Добавить локальный лог.
- Возможно добавить получение уведомлений/истории из внешнего канала.

