# Первичный дизайн ROS-only решения

## Назначение

ROS-only режим предназначен для окружений, где нельзя или нежелательно запускать внешний verifier/daemon, но нужно получить защиту лучше, чем обычный port knocking.

Это не полноценный HMAC-протокол с nonce cache. Это staged UDP knock плюс короткоживущий PSK-derived token, который MikroTik может предварительно вычислять через `:convert transform=sha512`.

Основной сценарий - dynamic/roaming клиенты. Заранее известные static source IP не являются целевой веткой: если IP известен заранее, его проще и честнее обслуживать через static allow-list или отдельную firewall policy.

## User flow и trust zones

Проект разделяет настройку и runtime-доступ на разные trust zones.

```text
safe/admin network:
  generate profile/client secrets
  render RouterOS .rsc
  import/apply config on MikroTik
  verify NAT, scripts, schedulers, reboot behavior

unsafe roaming network:
  no RouterOS SSH/API dependency
  no management plane requirement
  send staged UDP knock + PSK time-token only
  open only observed source IP for a short timeout

break-glass/admin mode:
  optional explicit tooling for environments where management path is already acceptable
  may add observed/explicit src IP directly through RouterOS SSH/API
```

Основной production runtime - второй сценарий. После provisioning MikroTik management plane не должен
быть доступен roaming-клиенту и не должен быть условием успешного knock. Целевой сервис остается
невидимым до успешного allow observed source IP через `allowed` address-list.

Текущий Go runtime transport для UDP knock использует IPv4 (`udp4`). TCP endpoint check использует
обычный `tcp` и может быть dual-stack в зависимости от resolver/OS. IPv6 knock нужно проектировать и
проверять отдельно.

## Основные сущности

- `profile` - профиль доступа к конкретному сервису.
- `client_id` - идентификатор клиента, участвующий в вычислении токена.
- `psk` - высокоэнтропийный секрет профиля или клиента.
- `bucket` - временное окно, например `floor(unix_time / 30)`.
- `token` - hex-encoded SHA512 от фиксированной строки.
- `stage address-list` - временные списки прохождения дешевых knock-стадий.
- `token-hit address-list` - временный список source IP, которые предъявили валидный token.
- `allowed address-list` - список IP, которым временно разрешен `dst-nat`.
- `used bucket/token state` - состояние, запрещающее повторное использование token в текущем bucket.

## Предлагаемый flow

```text
1. Клиент считает текущий token.
2. Клиент отправляет UDP knock на stage port A.
3. RouterOS добавляет source IP в stage1 на 5 секунд.
4. Клиент отправляет UDP knock на stage port B.
5. RouterOS добавляет source IP из stage1 в stage2 на 5 секунд.
6. Клиент отправляет UDP пакет на token port C с payload/token.
7. Firewall rule матчить source IP из stage2 и payload/content == current token.
8. Firewall добавляет source IP в token-hit address-list на короткий timeout, например 2 секунды.
9. Scheduler раз в 1 секунду проверяет token-hit list.
10. Если token/bucket еще не used и hit ровно один, scheduler добавляет observed source IP в allowed address-list на 3-5 минут и помечает token/bucket used.
11. Scheduler отключает или меняет token firewall rule до следующего bucket.
12. RouterOS отправляет уведомление.
13. `dst-nat` работает только для source IP из allowed address-list.
14. Клиент опционально проверяет целевой TCP endpoint через обычный connect-check, без RouterOS SSH/API.
```

TCP check подтверждает сквозную доступность target endpoint после knock. Он не доказывает отдельно, что
RouterOS уже добавил `allowed` entry: если внутренний сервис не отвечает, check останется closed даже при
успешном allow.

## Формат токена

Базовая идея:

```text
message = "v1|" + service + "|" + client_id + "|" + bucket
token = sha512(psk + "|" + message + "|" + psk)
```

Результат лучше хранить и передавать в hex.

Причины:

- формат фиксированный и версионированный;
- `service` не дает использовать токен одного сервиса для другого;
- `client_id` позволяет разделять клиентов;
- `bucket` ограничивает срок жизни;
- PSK с двух сторон сообщения избегает самого слабого варианта `sha512(psk|message)`.

Это не стандартный HMAC. Если получится реализовать настоящий HMAC-SHA512 в RouterOS script без чрезмерной сложности, его стоит предпочесть.

Проверенный прототип [prototype-time-token.rsc](../routeros/prototype-time-token.rsc) использует формулу:

```text
token = sha512(psk + "|v1|" + service + "|" + client_id + "|" + bucket + "|" + psk)
```

Scheduler держит два firewall `content` rules:

- `token now` для текущего bucket;
- `token prev` для предыдущего bucket.

Это подтверждает, что RouterOS может сам считать PSK-derived token через `:convert ... transform=sha512`
и обновлять `content` matcher без внешнего verifier.

## Time bucket

Начальный вариант:

```text
bucket_size = 30 секунд
accepted_buckets = now, now - 1
scheduler_interval = 1 секунда
token_hit_timeout = 2 секунды
used_timeout >= 2 * bucket_size
```

`now + 1` стоит добавлять только если есть проблемы с рассинхронизацией времени.

Чем короче окно, тем меньше replay window, но тем выше требования к синхронизации времени клиента и роутера.

`used_timeout` обязан перекрывать полный интервал приема токена. Так как RouterOS принимает `now` и
`prev`, marker для уже использованного bucket должен жить не меньше `2 * bucket_size`. При bucket 30
секунд безопасный дефолт - 65 секунд: 60 секунд полного окна плюс небольшой запас.

## Profile storage

Текущий проверенный прототип хранит profile/client параметры в отдельном persistent RouterOS script,
например `mkpk-tt-profile-demo`. Основной poller запускает этот profile script и получает:

- `service`;
- `client_id`;
- `psk`;
- `token_port`;
- `allowed_list`;
- `allowed_timeout`;
- `used_timeout`.

Такой формат переживает reboot и отделяет профиль от основной poller-логики. Подробности описаны в
[profile-format.md](profile-format.md).

Ограничение: PSK остается в RouterOS script source. Production-конфигурация должна ограничивать права
пользователей на чтение scripts и учитывать секреты в export/backup.

## Single-use bucket через polling

RouterOS firewall и scheduler/script нужно рассматривать как два разных runtime:

```text
firewall:
  видит packet-path
  матчить stage/address-list/content
  добавляет source IP в token-hit list

scheduler/script:
  считает tokens
  обновляет firewall rules
  читает token-hit list
  помечает bucket/token used
  добавляет один source IP в allowed list
  отправляет notification
```

Идеальная атомарная операция `check token -> mark used -> allow src` в ROS-only режиме, вероятно, недоступна. Поэтому используется polling:

```text
replay window ~= scheduler_interval + processing time
```

При scheduler interval 1 секунда replay window можно практически сузить примерно до 1 секунды, вместо полного `bucket_size`.
Это верно только при условии, что used-marker живет дольше полного окна приема `now+prev`; иначе старый
token может снова пройти после истечения marker timeout, пока он еще принимается как `prev`.

Если за один polling interval в `token-hit` попало больше одного source IP, безопасное поведение должно быть консервативным.

Текущая политика прототипа:

- если hit ровно один, открыть observed source IP;
- если hits больше одного, сжечь token/bucket и не разрешать никого;
- отправить/log warning о collision/replay suspicion.

Нельзя разрешать все адреса из `token-hit` для одного token/bucket.

Остаточный availability-риск: on-path атакующий, увидевший валидный token, может специально отправлять
его с другого IP в тот же polling interval. Консервативная collision-политика fail-closed сожжет bucket и
не откроет доступ легитимному клиенту. Это лучше, чем открыть неверный адрес, но остается DoS-вектором.

Для used-state прототип использует временный address-list marker в list `mkpk-proto-used-<bucket>`. Первый
вариант через script global на CHR не остановил повторный hit в том же bucket, поэтому global не стоит
считать достаточным hot-path state без дополнительной проверки.

## Reboot-survival

Механика внутри MikroTik должна переживать reboot устройства в fail-closed режиме.

Должны быть persistent RouterOS config objects:

- firewall rules для stage1/stage2/token;
- scheduler;
- scripts;
- static `dst-nat` rules с `src-address-list`;
- profile/client metadata, из которых можно пересчитать текущие tokens.

Нужно считать потерянными после reboot:

- dynamic `stage1`, `stage2`, `token-hit`, `allowed` address-list entries;
- временные used-bucket markers;
- script globals и другой in-memory state.

Следствие для production-варианта: после reboot scheduler должен быстро пересчитать current/previous
bucket tokens и обновить token firewall rules. До успешного пересчета token-stage должен быть disabled
или содержать заведомо невалидный content. Потеря `allowed` entries при reboot является приемлемым
fail-closed поведением: клиент должен выполнить knock заново.

После import без reboot применяется тот же порядок через one-shot scheduler `mkpk-tt-install`: он
запускает `mkpk-tt-startup`, который применяет service NAT, сбрасывает token-hit runtime state и запускает
poller для расчета token rules. Затем `mkpk-tt-install` удаляет сам себя.

Практическая проверка generator output показала важный RouterOS nuance: self-remove должен идти первым
действием в `mkpk-tt-install` on-event. Если сначала запускать `mkpk-tt-startup`, `:return 0` из
startup/poller прерывает остаток on-event, и install scheduler остается циклически запускаться.

Практический caveat: если scheduler уже включил token rules, RouterOS сохраняет измененный `content` как
config state. После reboot rules могут кратко содержать старый token до первого startup tick scheduler-а.
Это окно нужно отдельно измерить на CHR. Текущий production direction: scheduler с `start-time=startup`
должен обновлять rules как можно раньше, а stale token считается допустимым только в очень коротком
startup window и только после прохождения staged UDP.

Reboot-тест на CHR RouterOS 7.23.2 подтвердил:

- firewall rules, script и scheduler сохранились как persistent config objects;
- dynamic `allowed` и used markers не сохранились;
- scheduler стартовал после reboot и пересчитал `token now`/`token prev`;
- post-reboot knock снова открыл observed source IP.

Дополнительный startup guard прототипа `mkpk-tt-startup` проверен на CHR:

- startup scheduler выполнился после reboot;
- первым шагом поставил token rules в `disabled=yes` и `content=mkpk-tt-token-not-initialized`;
- затем запустил poller, который пересчитал и включил актуальные `token now`/`token prev`;
- post-reboot knock после этого сработал.

Остался теоретический production-hardening вопрос: полностью исключить окно до самого первого startup
script tick средствами persistent filter rules сложно. Текущий guard сужает его до раннего startup script
и делает порядок действий явным в логах.

## Address-list и NAT

NAT-правило не включается динамически. Оно всегда существует, но содержит matcher:

```text
src-address-list=knock-allowed-<service>
```

В текущем прототипе это проверяется через `mkpk-tt-apply-service`: script читает profile fields
`nat_enabled`, `nat_dst_port`, `nat_to_address`, `nat_to_port` и создает/обновляет persistent NAT rule.
Demo defaults дают disabled rule:

```routeros
/ip firewall nat
add chain=dstnat action=dst-nat protocol=tcp dst-port=2222 \
    src-address-list=mkpk-tt-allowed to-addresses=192.0.2.10 to-ports=22 \
    disabled=yes comment="mkpk-tt dst-nat demo ssh"
```

Production setup должен заменить demo NAT target в profile script, поставить `nat_enabled=true` и
запустить `mkpk-tt-apply-service`. Knock не должен динамически включать/выключать NAT rule.

CHR reboot-тест подтвердил, что disabled NAT rule сохраняется как persistent config object, а dynamic
`allowed` state после reboot сбрасывается. Startup guard дополнительно запускает `mkpk-tt-apply-service`,
чтобы NAT rule был пересверен с persistent profile после reboot.

Отдельно проверено: если изменить profile на `nat_enabled=true`, `nat_dst_port=2022`,
`nat_to_address=192.0.2.20`, `nat_to_port=2222`, apply script обновляет existing NAT rule. Эти значения
пережили reboot, startup снова применил rule, и post-reboot knock открыл observed source IP.

Успешный knock добавляет:

```text
address=<observed packet source ip>
list=knock-allowed-<service>
timeout=3m..5m
comment="client_id=<id>; mode=udp-token; service=<service>"
```

Payload не должен иметь права просить открыть произвольный IP. Открывается только фактический source IP принятого пакета.

Token не привязывается к `source_ip` в основном ROS-only режиме, потому что source IP заранее неизвестен. Это оставляет короткое replay window для атакующего, который успел повторить валидный token с другого IP до polling-прохода scheduler.

## Уведомления

После успешного добавления address-list entry должен запускаться неблокирующий notification path.

Минимальный payload:

```text
router=<identity>
service=<service>
client_id=<client_id>
src=<observed source ip>
list=<allowed address-list>
ttl=<timeout>
mode=udp-token
time=<router timestamp>
```

Первичные каналы для исследования:

- `/tool e-mail`;
- `/tool fetch` на webhook;
- Telegram bot через HTTPS API;
- remote syslog.

Ошибки уведомлений не должны отменять уже успешное открытие доступа.

Текущий прототип добавляет script `mkpk-tt-notify`. Poller передает ему данные через globals, удаляет
`token-hit`, затем вызывает hook после успешного allow. По умолчанию `mkpkTtNotifyEnabled=false`, поэтому
прототип не выполняет внешние HTTP-запросы без явного изменения profile script. Ошибка `/tool fetch`
логируется warning и не откатывает добавление observed source IP в `allowed`.

На CHR проверен HTTPS webhook path через `/tool fetch`: direct POST вернул HTTP 200, а успешный knock с
включенным `mkpkTtNotifyEnabled=true` вызвал `mkpk-tt-notify` без локального `notify failed`.

Notify payload формируется как корректный JSON через RouterOS `[:serialize {...} to=json]` и
отправляется с `Content-Type: application/json`. Это снимает прежнее ограничение сырого
`key=value&...` без экранирования: спецсимволы (`&`, `=`, `"`) в router identity / service / client_id
экранируются сериализатором. Ключи JSON в array-литерале должны быть в кавычках (`"client_id"=...`),
иначе RouterOS ломает разбор литерала. Проверено на CHR: POST на `postman-echo.com/post` вернул
валидный распарсенный JSON, а успешный knock вызвал `mkpk-tt-notify` без `notify failed`.

## Вопросы реализации

Актуальный реестр открыт в [open-questions.md](open-questions.md).

На уровне дизайна сейчас зафиксировано:

- основной режим рассчитан на dynamic/roaming clients;
- token/PSK предпочтительно делать per-client;
- открывать только observed source IP;
- использовать `token-hit` как bridge между firewall и scheduler;
- при нескольких hits одного token/bucket не открывать все адреса;
- remaining implementation details проверять на живом RouterOS.
