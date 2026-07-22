# Первичный дизайн ROS-only решения

## Назначение

ROS-only режим предназначен для окружений, где нельзя или нежелательно запускать внешний verifier/daemon, но нужно получить защиту лучше, чем обычный port knocking.

Это не полноценный HMAC-протокол с nonce cache. Это staged UDP knock плюс короткоживущий PSK-derived token, который MikroTik может предварительно вычислять через `:convert transform=sha512`.

## Основные сущности

- `profile` - профиль доступа к конкретному сервису.
- `client_id` - идентификатор клиента, участвующий в вычислении токена.
- `psk` - высокоэнтропийный секрет профиля или клиента.
- `bucket` - временное окно, например `floor(unix_time / 30)`.
- `token` - hex-encoded SHA512 от фиксированной строки.
- `stage address-list` - временные списки прохождения дешевых knock-стадий.
- `allowed address-list` - список IP, которым временно разрешен `dst-nat`.

## Предлагаемый flow

```text
1. Клиент считает текущий token.
2. Клиент отправляет UDP knock на stage port A.
3. RouterOS добавляет source IP в stage1 на 5 секунд.
4. Клиент отправляет UDP knock на stage port B.
5. RouterOS добавляет source IP из stage1 в stage2 на 5 секунд.
6. Клиент отправляет UDP пакет на token port C с payload/token.
7. Firewall rule матчить source IP из stage2 и payload/content == current token.
8. RouterOS добавляет source IP в allowed address-list на 3-5 минут.
9. RouterOS отправляет уведомление.
10. `dst-nat` работает только для source IP из allowed address-list.
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
```

`now + 1` стоит добавлять только если есть проблемы с рассинхронизацией времени.

Чем короче окно, тем меньше replay window, но тем выше требования к синхронизации времени клиента и роутера.

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

- Как именно лучше обновлять firewall `content` match под текущие токены?
- Можно ли держать несколько правил для `now` и `now-1`, обновляя их scheduler-ом?
- Достаточно ли `content`, или потребуется `layer7-protocol`?
- Как извлечь или зафиксировать `client_id`, если payload матчится только как token?
- Нужен ли отдельный token на каждого клиента или на профиль?
- Как хранить PSK в RouterOS script так, чтобы не расширять поверхность утечки?

