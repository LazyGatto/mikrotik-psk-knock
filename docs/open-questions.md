# Открытые вопросы и решения

Документ фиксирует статус вопросов по проекту. Концептуальные решения считаются закрытыми до появления новых фактов с живого RouterOS.

## Закрытые концептуальные решения

### Static source IP не является основной веткой

Статус: решено.

Основной сценарий проекта - dynamic/roaming клиенты. Если source IP заранее известен, authenticated knock обычно не нужен как основной механизм: такой адрес проще добавить в static allow-list или обслуживать отдельной firewall policy.

Следствие: основной UDP token не привязывается к заранее известному `source_ip`.

### Открывается только observed source IP

Статус: решено.

Payload не должен иметь права просить открыть произвольный IP. Успешный knock может открыть только фактический source IP пакета, который увидел MikroTik.

Причина: иначе механизм превращается в удаленное "open arbitrary IP" действие, что расширяет blast radius при ошибках и replay.

### Поведение при нескольких token-hit за polling interval

Статус: проверено для prototype policy.

Нельзя разрешать все адреса, попавшие в `token-hit` для одного token/bucket.

Политика текущего прототипа fail-closed: если за polling interval есть больше одного `token-hit`,
scheduler сжигает bucket, никого не открывает и пишет warning `collision/replay suspicion`.

Это проверено на CHR ручной инъекцией двух разных `token-hit` адресов. Для рабочего packet path
открывается только observed source IP; тестовая инъекция нужна только потому, что с одного реального
source IP RouterOS не создаст две одинаковые address-list записи.

### Token должен быть per-client

Статус: решено.

Предпочтительно использовать token/PSK per client или per client profile.

Per-profile общий секрет проще, но хуже для аудита и ротации: утечка одного PSK компрометирует всех пользователей профиля.

### Runtime — только UDP-token; SSH — только провижининг

Статус: решено (уточнено).

Единственный runtime-механизм открытия доступа — client-side UDP-token. SSH используется исключительно
как канал развёртывания (`mkpk-provision deploy`): установка/обновление/снятие mkpk-слоя на роутере. Идея
«открыть доступ по SSH» как runtime-режим отклонена: если SSH доступен снаружи, сам knock теряет смысл.

## Требует проверки на RouterOS

### Доступ к тестовому CHR

Статус: проверено.

Доступ:

```text
ssh admin@router.example.com
```

Host key подтвержден, local `known_hosts` обновлен. CHR доступен.

Проверенная система:

```text
identity: ROUTER-A
RouterOS: 7.23.2 stable
platform: CHR x86_64
resources: 1 CPU, 1GB RAM
```

### Packet payload в script

Статус: гипотеза - скорее всего нельзя.

Вопрос: может ли RouterOS firewall/script надежно передавать UDP payload и metadata конкретного packet в script для parsing?

Почему важно: если да, можно построить более полноценный verifier с `check token -> mark used -> allow src` ближе к атомарной операции. Если нет, остается firewall/content плюс scheduler polling.

### `:convert sha512`

Статус: проверено на CHR RouterOS 7.23.2.

Результат:

- `:convert ... transform=sha512` работает;
- результат для `abc` совпал с локальным `shasum -a 512`;
- это подтверждает пригодность `sha512` как базового примитива для PSK-derived token.

### Time bucket в RouterOS script

Статус: проверено на CHR RouterOS 7.23.2.

Результат: `:timestamp` доступен, выражение ниже возвращает числовой bucket:

```text
[:timestamp] / 30s
```

Это хороший RouterOS-friendly кандидат для time bucket без ручного парсинга даты.

### Firewall `content` для UDP token

Статус: проверено базово на CHR RouterOS 7.23.2.

Результат:

- UDP packet с payload `mkpk-token-test` сработал в `input` chain и добавил source IP в address-list;
- UDP packet с неверным payload не сработал;
- `content` принимает 128-символьный SHA512 hex token (проверено end-to-end).

Открыто: полноценный flood-тест staged-портов под нагрузкой. `layer7-protocol` остаётся запасным вариантом.

### Обновление firewall rules scheduler-ом

Статус: проверено на CHR RouterOS 7.23.2.

Data-driven poller (`mkpk-tt-poller`, `interval=1s`) регулярно поддерживает token-правила `now`/`prev`:
пересчитывает и переписывает `content` на границе bucket (bucket-cache), старый token перестаёт
работать, новый начинает. Проверено end-to-end.

### Минимальный scheduler interval

Статус: проверено на CHR RouterOS 7.23.2, включая нагрузку.

`interval=1s` работает (~1с с джиттером). Нагрузочный тест на 20 клиентов: data-driven poller даёт
дельту CPU ~12% над baseline на 1-CPU CHR (детали и числа — в
[multi-profile-render.md](multi-profile-render.md)).

Это важно для replay window:

```text
replay window ~= scheduler interval + processing time
```

### Порядок dynamic address-list entries

Статус: не требуется для текущей fail-closed policy.

Если в `token-hit` попало несколько source IP, текущий прототип не выбирает первый entry. Он сжигает
bucket и не добавляет никого в `allowed`.

Это снимает зависимость от порядка dynamic address-list entries. Возвращаться к выбору "первого"
имеет смысл только если появится сильная UX-причина и будет отдельно проверена надежность порядка.

### Хранение `used bucket/token state`

Статус: проверено для прототипа.

Кандидаты:

- script variable;
- address-list marker;
- disabled/commented firewall rule;
- file.

Результат CHR:

- script global в первом варианте прототипа не остановил повторный hit в том же bucket;
- повторный hit с того же source IP дополнительно маскировался тем, что RouterOS не добавляет второй
  одинаковый `allowed` entry и возвращает ошибку `already have such entry`;
- временный address-list marker `mkpk-proto-used-<bucket>` сработал как used-state;
- второй token-hit в том же bucket удаляется с warning `replay ignored`;
- collision из двух разных `token-hit` сжигает bucket и не создает `allowed`.

Предварительная позиция уточнена: для hot-path state использовать runtime/config objects, в первую
очередь временные address-list markers. Частой записи в files по-прежнему лучше избегать.

### Reboot-survival

Статус: базово проверено на CHR, требует production-hardening.

Механизмы внутри MikroTik должны переживать reboot в fail-closed режиме:

- persistent firewall rules, scripts, scheduler, NAT rules и profile/client metadata остаются в config;
- dynamic address-list entries и script globals считаются потерянными;
- после reboot scheduler должен пересчитать current/previous bucket token rules;
- до успешного пересчета token-stage должен быть disabled или содержать заведомо невалидный content;
- потеря `allowed` entries после reboot приемлема: клиент делает knock заново.

Результат reboot-теста CHR RouterOS 7.23.2:

- firewall rules, script и scheduler сохранились;
- dynamic `allowed` и used markers после reboot отсутствовали;
- scheduler стартовал и пересчитал `token now`/`token prev`;
- post-reboot knock с current bucket token сработал;
- `:timestamp` bucket совпал с клиентским epoch bucket, хотя `/system ntp client` был disabled;
- в логах ранний startup tick отразился со временем `10:19:39`, а после применения timezone/clock следующие записи шли как `15:20:00+`.

Дополнительный startup guard проверен:

- `mkpk-tt-startup` выполнился после reboot;
- token rules сначала были переведены в `disabled=yes` и invalid content;
- затем `mkpk-tt-startup` запустил poller, который пересчитал и включил актуальные token rules;
- post-reboot knock сработал.

Остаточная production-hardening задача: полностью исключить окно до самого первого startup script tick
средствами persistent filter rules сложно. Нужно решить, достаточно ли такого guard, или нужен другой
механизм установки/активации token-stage.

### PSK-derived time-token prototype

Статус: проверено на CHR RouterOS 7.23.2.

Результат:

- RouterOS `:convert $msg from=raw to=hex transform=sha512` совпадает с локальным `shasum -a 512`;
- firewall `content` принимает 128-символьный SHA512 hex token;
- scheduler успешно обновляет два token rules: current bucket и previous bucket;
- end-to-end flow с current bucket token добавляет observed source IP в `allowed`;
- end-to-end flow с previous bucket token также работает;
- неверный payload не создает token-hit и не открывает доступ;
- replay/collision политика работает как в статическом прототипе.

Ранее profile values были hardcoded в отдельном persistent profile script; теперь конфиг-driven — см.
[profile-format.md](profile-format.md) и [multi-profile-render.md](multi-profile-render.md).

### Хранение параметров профиля

Статус: решено (эволюционировало).

Прошло три стадии: hardcoded demo → отдельный persistent `mkpk-tt-profile-demo` (`:global`) →
**текущее**: значения инлайнятся в client-таблицу единого data-driven poller. Отдельного profile-скрипта
больше нет. PSK/SMTP-пароли по-прежнему в source RouterOS-скриптов — нужно ограничение прав на чтение
scripts и аккуратность с export/backup.

### Производительность `content` и `layer7-protocol`

Статус: частично проверено.

Нагрузочный тест на 20 клиентов выполнен (CPU-дельта ~12% для data-driven poller, см.
[multi-profile-render.md](multi-profile-render.md)). Открыто: полноценный flood-тест staged-портов
(random noise, targeted UDP flood, много неверных payload). Staged UDP должен снижать нагрузку, т.к.
token/content проверка применяется только после дешёвых stages и гейтится `src-address-list=stage2`.

### Уведомления

Статус: `webhook`, `telegram`, `email` реализованы и проверены на CHR; `syslog` — открыт.

- payload — корректный JSON через `[:serialize ... to=json]`, `Content-Type: application/json`;
- telegram — POST `{chat_id,text}` на Bot API; email — `/tool e-mail send` с inline SMTP-параметрами;
- graceful degradation: ошибка доставки логируется и не откатывает allow (проверено с неверным
  токеном/недоступным SMTP);
- по умолчанию notify выключен.

Открыто: канал remote `syslog` (по сути системный logging action, ортогонально).

### Безопасные права для deploy-пользователя

Статус: открыто (hardening).

SSH теперь только провижининг (`mkpk-provision deploy`), а не runtime. Открытый вопрос сузился: можно ли
завести RouterOS user/group с минимальными правами именно под deploy (импорт `.rsc`, управление
`mkpk-tt-*`), не давая полный админ-доступ и, в частности, чтения секретов из export/scripts. Сейчас
deploy рассчитан на обычный админ-доступ.
