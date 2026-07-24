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
- принято, что основной runtime-клиент работает в UDP-token mode без RouterOS SSH/API; SSH/API остается
  только optional admin/break-glass tooling для safe сети;
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

Multi-profile RouterOS render реализован и проверен end-to-end на живом CHR (ROUTER-A, 7.23.2):
`mkpk-provision routeros render` без `--client` рендерит все services/clients в per-profile объекты с
per-service изоляцией allowed-list. Подтверждено: per-client pollers поднимают token rules после import
без reboot, end-to-end knock открывает observed source IP, per-service изоляция allowed-list работает
(`ca`->svca не открывает svcb), per-client used-marker/replay-путь работает. Детали в
[multi-profile-render.md](multi-profile-render.md).

Следующий полезный шаг: нагрузочный тест `content`/N планировщиков 1с на большем числе клиентов; либо
переход к оставшимся трекам (notify URL-encoding/JSON, каналы уведомлений, admin/SSH режим).

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
- Реализован multi-profile RouterOS render: все services/clients в per-profile объекты, per-service
  allowed-list изоляция, per-client poller/scheduler; валидация безопасных имён services/clients.
- Проверено на CHR (2 services / 2 clients): pollers поднимают token rules после import без reboot,
  end-to-end knock открывает observed source IP, per-service изоляция allowed-list и per-client
  used-marker/replay работают (см. [multi-profile-render.md](multi-profile-render.md)).
- Добавлено: `mkpk-provision profile init` для создания стартового YAML с generated PSK.
- Добавлено: `mkpk-provision service add` для добавления service/NAT target без ручного редактирования YAML.
- Добавлено: `mkpk-provision client add` для добавления roaming clients без ручного редактирования YAML.
- Добавлено: `mkpk-provision config validate` для safe/admin проверки YAML перед render/import.
- Добавлен debug mode для `token` и `knock`.
- Зафиксирован user flow: provisioning/render/import выполняются из safe/admin сети, runtime `knock` работает из unsafe roaming сети без SSH/API зависимости.
- Разделены CLI binaries: `mkpk` содержит runtime команды `check`/`knock`, `mkpk-provision` содержит
  admin/provisioning команды `secret generate`, `config validate`, `profile init`, `service add`,
  `client add`, `token` и `routeros render`.
- Проверено: `go test ./...` проходит.
- Проверено: `token` совпадает с shell `shasum -a 512`.
- Проверено: generated `.rsc` успешно импортируется на CHR и one-shot post-import init активирует token rules.
- Исторически проверено: после временного отключения LuLu single-delay режим `mkpk knock --delay 750ms` проходил до CHR end-to-end и открывал observed source IP.
- Исторически проверено: `--delay 100ms` был слишком быстрым для staged address-lists в текущем VPN route; `750ms` и `1500ms` сработали.
- Реализовано: fixed delay заменен на retry windows для stage1/stage2/token.
- Добавлено: optional UDP noise через `mkpk knock --noise N`, выключено по умолчанию.
- Добавлено: optional post-knock TCP check целевого endpoint через `mkpk knock --check`, без RouterOS SSH/API.
- Добавлено: standalone runtime `mkpk check` для before/after TCP endpoint status, пригодно для будущего UI.
- Добавлено: machine-readable `mkpk check --json` для будущего UI.
- Добавлено: client-side bucket-age guard `--min-bucket-age`, чтобы не отправлять token сразу после локального rollover bucket.
- Исправлено: `defaults.bucket_seconds` пробрасывается в RouterOS render, без hardcoded `30s` в generated poller.
- Добавлен hardening config validation: safe PSK alphabet, distinct stage/token ports, timeout parsing.
- Исправлено: `used_timeout` теперь должен перекрывать полный интервал приема `now+prev` token buckets; дефолт поднят до 65s.
- Решено: remote clock check через SSH/API не входит в основной stealth UDP-token flow; если management channel доступен, это отдельный admin-mode, а не runtime-зависимость knock.
- Добавить локальные diagnostics/presets для clock-skew сценариев без обратного канала.

## Этап 4: Admin/SSH режим

- Рассматривать SSH/API только как optional admin tooling для окружений, где management path уже допустим.
- Не делать SSH/API зависимостью основного UDP-token режима: если SSH доступен снаружи, сам port-knocking теряет значительную часть смысла.
- Сделано: CLI verbs разделены на provisioning/admin команды и runtime `check`/`knock`, чтобы mobile flow
  не тянул management assumptions.
- Проверить права RouterOS user/group для безопасного script run.
- Реализовать явную admin-команду для прямого добавления observed/explicit src IP в address-list только при осознанном выборе этого режима.
- Сравнить UX и threat model с основным stealth UDP-token режимом.

## Этап 5: GUI

- Обернуть profiles и actions в GUI.
- Показывать статус последних knock attempts.
- Добавить локальный лог.
- Возможно добавить получение уведомлений/истории из внешнего канала.
