# Контекст проекта

Дата первичного обсуждения: 2026-07-22.

## Проблема

Есть роутеры MikroTik, где прямое публичное выставление `dst-nat` небезопасно, а VPN может быть избыточен, неудобен или организационно сложен.

Нужно открывать port forward временно и только для конкретного source IP после успешной авторизованной процедуры.

Желаемое направление:

- не выставлять сервисные порты напрямую в Интернет;
- по возможности не требовать Docker или отдельный внешний сервис;
- сначала исследовать MikroTik-only вариант;
- в дальнейшем сделать клиентское приложение с CLI и GUI;
- рассмотреть использование уже существующего клиентского Ed25519 ключа/сертификата и публичного ключа на MikroTik.

## Базовый безопасный NAT-паттерн

`dst-nat` правила должны оставаться статическими, но ограничиваться source address-list:

```text
dst-nat rule:
  src-address-list=knock-allowed-service-x
```

Knock-процесс не должен включать и выключать NAT-правила напрямую. Он должен только добавлять фактический source IP в address-list с timeout.

Плюсы:

- меньше race condition;
- простая автоматическая очистка через timeout;
- более безопасный failure mode при ошибках скрипта;
- проще аудит и сопровождение.

## Уведомления как обратная связь

Важная идея: при каждом добавлении нового source IP в разрешенный address-list MikroTik должен отправлять внешнее уведомление.

Возможные каналы:

- email;
- Telegram или другой messenger bot;
- webhook в мониторинг или логирование;
- syslog/SIEM;
- push-уведомление через будущий клиент или backend.

Payload уведомления:

```text
router identity
service/profile name
source IP that was allowed
address-list name
allowed TTL
timestamp
knock mode, for example ssh/token/manual
client_id if available
```

Плюсы:

- владелец сразу видит, что port forward был открыт;
- проще заметить неожиданные или подозрительные открытия;
- появляется audit trail вне динамического firewall state;
- система становится операционно понятнее и довереннее.

Ограничение:

- доставка уведомления не должна блокировать основной knock-flow;
- если email/messenger/webhook недоступен, address-list update все равно должен завершиться;
- ошибки уведомлений нужно логировать локально.

## Классический MikroTik-only port knocking

RouterOS firewall может реализовать staged knocking:

```text
UDP port A -> add src to stage1 for 5s
UDP port B from stage1 -> add src to stage2 for 5s
UDP port C/token from stage2 -> add src to allowed list for 5m
```

Это можно сделать через firewall rules и dynamic address lists.

Свойства:

- снижает шум и случайное сканирование;
- не дает сильной криптографической аутентификации само по себе;
- если последовательность подсмотрена, ее можно повторить.

## Желаемый криптографический knock

Идеальный пакет:

```text
version | timestamp | nonce | client_id | service | requested_ttl | hmac
```

Идеальная логика на MikroTik:

```text
read UDP payload
verify HMAC/PSK
check timestamp window
check nonce replay cache
add packet source IP to allowed address-list
```

Цель безопасности:

- атакующий без PSK не может создать валидный knock;
- timestamp делает токен короткоживущим;
- nonce плюс replay cache запрещают повторное использование уже принятого пакета.

## Возможности RouterOS `:convert`

Согласно актуальной официальной документации RouterOS scripting, `:convert` поддерживает:

```text
from:
  base32, base64, byte-array, hex, num, raw, url

to:
  base32, base64, bit-array-lsb, bit-array-msb, byte-array, hex, num, raw, url

transform:
  lc, uc, lcfirst, ucfirst, crlf,
  ed25519-private-to-x25519-private,
  ed25519-private-to-ed25519-public,
  ed25519-public-to-x25519-public,
  x25519-private-to-x25519-public,
  md5, reverse, rot13, sha512, none
```

Это подтверждает, что RouterOS умеет считать как минимум `md5` и `sha512`, а также выполнять некоторые операции конвертации Ed25519/X25519 ключей.

Важное различие:

- `sha512` transform есть;
- Ed25519/X25519 key conversion есть;
- встроенный HMAC не указан;
- Ed25519 signature verification для произвольных сообщений не указан.

## Ключевое ограничение MikroTik-only дизайна

RouterOS scripting может считать хеши через `:convert ... transform=sha512`, но firewall rule не является нормальным UDP application handler.

Открытый ограничивающий момент:

- script может посчитать hash;
- firewall может матчить свойства пакета и иногда payload через `content`/`layer7-protocol`;
- но не очевидно, что script может получить и разобрать тело конкретного входящего UDP-пакета из firewall path.

Из-за этого полноценный verifier вида "прочитать payload, проверить HMAC, проверить nonce" на чистом RouterOS выглядит трудным или непрактичным.

## Снижение CPU-нагрузки

Можно использовать staged UDP knocks перед дорогой проверкой payload/hash:

```text
stage 1: cheap UDP port knock
stage 2: cheap UDP port knock
stage 3: only then check cryptographic token/payload
```

Это снижает нагрузку от интернет-шума, потому что дорогая проверка применяется только к IP, прошедшим первые стадии.

Но это не решает replay само по себе. Это rate/noise gate, а не replay protection.

## Replay protection

HMAC или PSK-derived hash защищает от подделки, но не автоматически защищает от replay.

Если атакующий перехватил валидный пакет:

- он не может создать новый валидный пакет без PSK;
- он может повторить тот же пакет, если роутер не отвергает использованные или просроченные токены.

Replay protection требует хотя бы одного из вариантов:

- короткое timestamp/time-bucket окно;
- server-side nonce cache, который помнит использованные nonce.

Без nonce cache replay внутри окна валидности возможен.

## Практический MikroTik-only компромисс

Вместо разбора каждого входящего UDP payload в скрипте MikroTik scheduler/script может заранее вычислять текущие валидные токены и обновлять firewall match rules.

Пример:

```text
token = sha512(psk | time_bucket | service | client_id)
time_bucket = floor(unix_time / 30)
```

Firewall затем матчить UDP payload/content на один из текущих допустимых токенов.

Допустимые buckets:

```text
now
now - 1
now + 1
```

`now + 1` стоит включать только если есть проблемы с синхронизацией времени, потому что это расширяет replay window.

Свойства:

- подделать токен без PSK практически невозможно при сильном PSK;
- перехваченный токен можно replay-нуть в пределах его time bucket;
- nonce не помогает без возможности хранить и проверять использованные nonce per accepted packet;
- это ближе к time-based signed knock token, чем к полноценному HMAC verifier.

## HMAC против простого SHA

Настоящий HMAC-SHA256/SHA512 предпочтительнее.

RouterOS exposes SHA transforms through scripting, but not a built-in HMAC primitive.

Простая конструкция:

```text
sha512(psk | message)
```

не является стандартным HMAC.

Немного более аккуратная closed-format конструкция:

```text
sha512(psk | message | psk)
```

тоже не HMAC, но может быть приемлема для constrained knock token, если:

- PSK высокоэнтропийный;
- формат сообщения фиксированный и однозначный;
- токены короткоживущие;
- система документирована как компромисс.

Реализовать полный HMAC в RouterOS script, возможно, получится через byte operations, ipad/opad, xor и SHA512, но это выглядит хрупко и тяжело для сопровождения.

## Ed25519

Использование существующего клиентского Ed25519 ключа привлекательно, но RouterOS scripting, судя по текущему списку `:convert`, не предоставляет Ed25519 signature verification для произвольного payload.

Возможные применения:

1. Использовать Ed25519 через SSH authentication.
2. Использовать Ed25519 signatures в custom UDP protocol только если verifier находится где-то еще.

MikroTik-only UDP с Ed25519 signature verification выглядит непрактичным, если только в RouterOS нет отдельного подходящего примитива.

## SSH-based альтернатива

Использовать SSH public-key authentication с существующим Ed25519 ключом.

Flow:

```text
client -> ssh knock-user@router "/system script run open-forward-service-x"
router script -> add remote/source IP to address-list with timeout
```

Плюсы:

- используется устоявшаяся асимметричная криптография;
- не надо реализовывать HMAC или Ed25519 verify в RouterOS script;
- хорошо ложится на модель уже развернутых public keys;
- хороший кандидат для CLI/GUI wrapper.

Риски и требования:

- SSH service должен быть доступен снаружи или иным образом достижим;
- SSH нужно жестко ограничить firewall rules, правами пользователя, allowed-address где возможно, rate limits и логированием;
- RouterOS не дает такой же удобной модели forced-command, как OpenSSH, поэтому user/group/script permissions нужно проверять отдельно.

## External agent альтернатива

Запустить небольшой UDP knock daemon, который умеет:

- принимать UDP payload;
- проверять HMAC или Ed25519 signature;
- проверять timestamp и nonce replay cache;
- вызывать RouterOS API или SSH для добавления address-list entry.

Где может жить агент:

- маленький внутренний host;
- VPS;
- router container, если модель MikroTik и версия RouterOS поддерживают контейнеры;
- маленькое отдельное устройство в LAN.

Это самая чистая криптографическая архитектура, но она не удовлетворяет требованию "MikroTik-only везде".

## Текущая рекомендация

Вести два трека.

### Track A: MikroTik-only компромисс

```text
staged UDP knock
-> short-lived PSK-derived time token
-> firewall payload/content match
-> add src IP to address-list with timeout
-> notification
```

Документировать как:

- защиту от сканеров и неавторизованного открытия;
- заметно лучше обычного port knocking;
- ограниченную replay resistance через короткое временное окно;
- не полноценную replay protection.

### Track B: более строгая криптография

```text
client CLI/GUI
-> SSH Ed25519 or external UDP verifier
-> RouterOS address-list update
-> notification
```

Этот вариант предпочтительнее, если важны строгая replay-защита и более чистая криптографическая модель.

## Открытые вопросы

- Может ли RouterOS firewall/script надежно передавать UDP payload в script для parsing? Текущая гипотеза: нет.
- Какие точные `:convert` transforms доступны на целевых RouterOS versions?
- Можно ли реализовать настоящий HMAC-SHA512 в RouterOS script достаточно чисто для поддержки?
- Насколько дороги `content` или `layer7-protocol` matches на целевом железе под интернет-шумом?
- Достаточно ли `content` для exact token matching, или нужен `layer7-protocol`?
- Как client identity должен мапиться на services и TTL?
- Source IP должен всегда браться только из observed packet или payload может просить открыть другой IP? Текущая позиция: только observed packet source.
- Какое replay window приемлемо: 10s, 30s, 60s?
- Должен ли будущий клиент поддерживать оба режима: SSH mode и UDP token mode?

