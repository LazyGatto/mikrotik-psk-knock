# Первичный дизайн ROS-only решения

## Назначение

ROS-only режим предназначен для окружений, где нельзя или нежелательно запускать внешний verifier/daemon, но нужно получить защиту лучше, чем обычный port knocking.

Это не полноценный HMAC-протокол с nonce cache. Это staged UDP knock плюс короткоживущий PSK-derived token, который MikroTik может предварительно вычислять через `:convert transform=sha512`.

Основной сценарий - dynamic/roaming клиенты. Заранее известные static source IP не являются целевой веткой: если IP известен заранее, его проще и честнее обслуживать через static allow-list или отдельную firewall policy.

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
```

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

## Time bucket

Начальный вариант:

```text
bucket_size = 30 секунд
accepted_buckets = now, now - 1
scheduler_interval = 1 секунда
token_hit_timeout = 2 секунды
```

`now + 1` стоит добавлять только если есть проблемы с рассинхронизацией времени.

Чем короче окно, тем меньше replay window, но тем выше требования к синхронизации времени клиента и роутера.

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

Если за один polling interval в `token-hit` попало больше одного source IP, безопасное поведение должно быть консервативным.

Текущая политика прототипа:

- если hit ровно один, открыть observed source IP;
- если hits больше одного, сжечь token/bucket и не разрешать никого;
- отправить/log warning о collision/replay suspicion.

Нельзя разрешать все адреса из `token-hit` для одного token/bucket.

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

## Address-list и NAT

NAT-правило не включается динамически. Оно всегда существует, но содержит matcher:

```text
src-address-list=knock-allowed-<service>
```

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

## Вопросы реализации

Актуальный реестр открыт в [open-questions.md](open-questions.md).

На уровне дизайна сейчас зафиксировано:

- основной режим рассчитан на dynamic/roaming clients;
- token/PSK предпочтительно делать per-client;
- открывать только observed source IP;
- использовать `token-hit` как bridge между firewall и scheduler;
- при нескольких hits одного token/bucket не открывать все адреса;
- remaining implementation details проверять на живом RouterOS.
