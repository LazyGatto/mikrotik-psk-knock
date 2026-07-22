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
- добавлен [routeros/prototype-stage-token.rsc](../routeros/prototype-stage-token.rsc) для минимального end-to-end прототипа;
- подтверждено на CHR, что `stage1 -> stage2 -> token-hit -> scheduler -> allowed` работает со статическим token payload;
- подтверждено, что `:return` в scheduler script нужно вызывать с явным значением, например `:return 0`;
- проверено, что script global не годится как единственный used-bucket state в этом прототипе: повторный проход не был остановлен и уперся в duplicate address-list entry;
- подтвержден рабочий used-bucket marker через временный address-list `mkpk-proto-used-<bucket>`;
- подтверждено replay-поведение: второй token-hit в том же bucket удаляется с warning `replay ignored`, без нового `allowed`;
- подтверждено collision-поведение ручной инъекцией двух разных `token-hit`: bucket сжигается, `allowed` не создается;
- зафиксировано требование reboot-survival: persistent config objects должны переживать reboot, dynamic runtime state считается потерянным и восстанавливается/fail-closed;
- добавлен [routeros/prototype-time-token.rsc](../routeros/prototype-time-token.rsc) с PSK-derived time-token;
- подтверждено, что RouterOS-side token formula совпадает с локальным `shasum -a 512`;
- подтверждено, что firewall `content` принимает 128-символьный SHA512 hex token;
- подтверждено, что scheduler обновляет `token now` и `token prev` rules для текущего/предыдущего bucket;
- подтвержден end-to-end flow с current bucket token;
- подтвержден end-to-end flow с previous bucket token;
- подтвержден negative case: неверный payload проходит stages, но не попадает в token-hit и не открывает `allowed`;
- подтверждены replay/collision политики для time-token прототипа;
- выполнен reboot-тест CHR: persistent `mkpk-tt` firewall rules/script/scheduler пережили reboot, dynamic state очистился, scheduler пересчитал token rules, post-reboot knock сработал;
- подтверждено, что `:timestamp` bucket после reboot совпал с клиентским epoch bucket даже при disabled NTP client на тестовом CHR;
- добавлен и проверен startup guard `mkpk-tt-startup`: после reboot он сбрасывает token rules в disabled/invalid, затем запускает poller для пересчета;
- profile/client параметры вынесены в отдельный persistent RouterOS script `mkpk-tt-profile-demo`;
- добавлен static disabled `dst-nat` demo-rule через `src-address-list=mkpk-tt-allowed`;
- добавлен notification hook `mkpk-tt-notify` с выключенным по умолчанию webhook;
- проверено на CHR: импорт прототипа создает disabled NAT rule, profile/notify/poller/startup scripts и schedulers;
- проверено на CHR: direct `/tool fetch` POST на `https://postman-echo.com/post` возвращает HTTP 200;
- проверено на CHR: при включенном webhook в demo profile успешный knock вызывает `mkpk-tt-notify` без `notify failed`;
- проверено на CHR: после reboot disabled NAT rule и scripts/schedulers сохраняются, dynamic state сбрасывается, startup guard срабатывает, post-reboot knock работает;
- NAT service target и notification webhook target вынесены в profile fields;
- startup guard теперь пере-применяет service NAT из profile перед пересчетом token rules;
- проверено на CHR: `mkpk-tt-apply-service` обновляет existing NAT rule из profile fields, включая `disabled`, `dst-port`, `to-addresses` и `to-ports`;
- проверено на CHR: modified NAT profile переживает reboot, startup пере-применяет NAT rule, post-reboot knock работает;
- тестовые firewall rules, nat rules, address-lists, scripts и schedulers после проверки удалены.

Промежуточный вывод: базовая ROS-only архитектура стала заметно реалистичнее. `sha512`, time bucket,
UDP `content`, обновление rule content, scheduler 1s и bridge `token-hit -> scheduler -> allowed`
подтверждены на CHR.

Следующий полезный эксперимент: перейти к клиентскому CLI и вынести ручной token helper в повторяемую
команду.

## Этап 1: исследование RouterOS

- Проверено: SSH host key тестового CHR подтвержден, local `known_hosts` обновлен.
- Проверено: на RouterOS 7.23.2 `:convert` для `sha512` работает.
- Проверено: стабильный time bucket можно получить через `[:timestamp] / 30s`.
- Проверено: firewall `content` матчится по UDP payload.
- Проверено: `content` matcher можно обновлять через SSH/script.
- Проверено: polling `token-hit` address-list с scheduler interval около 1s переносит один observed source IP в `allowed`.
- Проверено: при нескольких hits текущий прототип может fail-closed без опоры на порядок dynamic entries.
- Проверено: временный address-list marker подходит как used bucket/token state для прототипа.
- Проверить стоимость `content` и `layer7-protocol` на тестовом железе.
- Проверить каналы уведомлений: email, Telegram/webhook через `/tool fetch`, syslog.

## Этап 2: прототип RouterOS-only

- Описать profile format.
- Проверено прототипом: RouterOS scripts для генерации PSK-derived time-token.
- Проверено прототипом: firewall rules для stage1/stage2/token.
- Проверено: disabled `dst-nat` demo-rule через `src-address-list`.
- Проверено: notification script `mkpk-tt-notify`.
- Проверено: webhook notification через `/tool fetch` до внешнего HTTPS endpoint.
- Добавить logging и comments для address-list entries.
- Проверено: после reboot scheduler пересчитывает token rules, dynamic `allowed` сбрасывается, post-reboot knock работает.
- Проверено: persistent profile/client storage через отдельный RouterOS profile script.
- Проверено: startup guard против stale persisted token content.
- Добавлено: NAT service target и notification target в profile/service format.
- Проверено: apply/update сценарий для NAT service target из profile script.

## Этап 3: клиент CLI

- Выбрано: Go CLI.
- Добавлен Go module в `client/`.
- Реализован расчет token.
- Реализован UDP staged knock transport.
- Реализован YAML profiles/config.
- Реализован RouterOS `.rsc` render для текущей single-profile схемы.
- Добавлен debug mode для `token` и `knock`.
- Проверено: `go test ./...` проходит.
- Проверено: `token` совпадает с shell `shasum -a 512`.
- Проверено: generated `.rsc` успешно импортируется на CHR и one-shot post-import init активирует token rules.
- Исторически проверено: после временного отключения LuLu single-delay режим `mkpk knock --delay 750ms` проходил до CHR end-to-end и открывал observed source IP.
- Исторически проверено: `--delay 100ms` был слишком быстрым для staged address-lists в текущем VPN route; `750ms` и `1500ms` сработали.
- Реализовано: fixed delay заменен на retry windows для stage1/stage2/token.
- Добавлено: optional UDP noise через `mkpk knock --noise N`, выключено по умолчанию.
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
