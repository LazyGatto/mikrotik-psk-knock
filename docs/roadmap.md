# Roadmap

## Текущий статус

Сделано:

- создана проектная структура и git-репозиторий;
- зафиксирован контекст обсуждения, ROS-only дизайн, модель угроз и инструкции для агента;
- добавлен отдельный реестр открытых вопросов и концептуальных решений;
- принято, что основной сценарий - dynamic/roaming clients, а static source IP не является основной веткой;
- принято, что успешный knock открывает только observed source IP;
- принято, что token/PSK предпочтительно делать per-client;
- принято, что при нескольких `token-hit` за polling interval нельзя открывать все адреса;
- принято, что клиент должен поддерживать UDP-token mode и SSH/Ed25519 mode.
- подтвержден доступ к тестовому CHR `admin@router.example.com` после обновления `known_hosts`;
- проверен CHR: `ROUTER-A`, RouterOS `7.23.2 stable`, CHR x86_64, 1 CPU, 1GB RAM;
- подтверждено, что `:convert ... transform=sha512` работает и совпадает с локальным `shasum -a 512` для `abc`;
- подтверждено, что `:timestamp` доступен, а выражение `[:timestamp] / 30s` дает числовой time bucket;
- подтверждено, что UDP `content` matcher работает в `input` chain по payload;
- подтверждено, что `content` matcher можно менять через SSH/script, и новое значение начинает применяться;
- подтверждено, что scheduler с `interval=1s` работает практически около 1 секунды, но с джиттером/стартовой задержкой;
- тестовые firewall rules, address-lists, scripts и schedulers после проверки удалены.

Промежуточный вывод: базовая ROS-only архитектура стала заметно реалистичнее. `sha512`, time bucket, UDP `content`, обновление rule content и scheduler 1s подтверждены на CHR.

Следующий полезный эксперимент: собрать маленький end-to-end prototype `stage1/stage2/token-hit` плюс scheduler, который переносит один IP в `allowed` и сжигает bucket.

## Этап 1: исследование RouterOS

- Проверено: SSH host key тестового CHR подтвержден, local `known_hosts` обновлен.
- Проверено: на RouterOS 7.23.2 `:convert` для `sha512` работает.
- Проверено: стабильный time bucket можно получить через `[:timestamp] / 30s`.
- Проверено: firewall `content` матчится по UDP payload.
- Проверено: `content` matcher можно обновлять через SSH/script.
- Проверить polling `token-hit` address-list с scheduler interval около 1s.
- Проверить порядок dynamic address-list entries при нескольких hits.
- Проверить варианты хранения `used bucket/token state`.
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
