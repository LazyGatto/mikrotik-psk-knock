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

### Клиент должен поддерживать UDP-token и SSH modes

Статус: решено.

UDP-token mode - ROS-only компромисс для окружений без внешнего verifier.

SSH/Ed25519 mode - более строгий режим для окружений, где можно безопасно открыть management path и использовать SSH public-key authentication.

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
- `content` выглядит пригодным для token-stage.

Остается проверить ограничения длины token и поведение под нагрузкой. `layer7-protocol` пока остается запасным вариантом.

### Обновление firewall rules scheduler-ом

Статус: частично проверено на CHR RouterOS 7.23.2.

Результат:

- `content` matcher можно менять через SSH/script;
- после смены на `mkpk-token-updated` старый token не сработал, новый сработал.

Остается проверить регулярное обновление scheduler-ом и схему rules для `now`/`now-1`.

### Минимальный scheduler interval

Статус: базово проверено на CHR RouterOS 7.23.2.

Результат: scheduler с `interval=1s` работает. За `:delay 5s` тестовый счетчик увеличился на 4, то есть практически около 1 секунды, но с джиттером/стартовой задержкой.

Остается проверить стабильность под нагрузкой и влияние на CPU.

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

Ограничение прототипа: profile values (`service`, `client_id`, `psk`) пока hardcoded demo values внутри
отдельного persistent profile script. Формат описан в [profile-format.md](profile-format.md).

### Persistent profile script

Статус: проверено в time-token прототипе.

Profile/client параметры вынесены в отдельный persistent script `mkpk-tt-profile-demo`, который задает:

- `service`;
- `client_id`;
- `psk`;
- `token_port`;
- `allowed_list`;
- `allowed_timeout`;
- `used_timeout`.

Poller запускает profile script перед расчетом token. После reboot profile script сохранился, poller
прочитал значения, пересчитал token rules, и post-reboot knock сработал.

Ограничение: PSK хранится в RouterOS script source. Нужно отдельно определить права RouterOS users/groups,
ротацию секретов и правила обращения с export/backup.

### Производительность `content` и `layer7-protocol`

Статус: требует проверки на тестовом железе.

Проверить CPU impact под:

- случайным интернет-шумом;
- targeted UDP flood на stage ports;
- большим количеством неверных payload;
- нормальным staged flow.

Staged UDP должен снижать нагрузку, потому что token/content проверка применяется только после дешевых stages.

### Уведомления

Статус: требует проверки.

Проверить каналы:

- `/tool e-mail`;
- `/tool fetch` на webhook;
- Telegram bot API через HTTPS;
- remote syslog.

Требование: notification failure не должен отменять успешное добавление source IP в allowed list. Ошибка должна логироваться локально.

### Безопасные права для SSH/Ed25519 режима

Статус: требует проверки.

Проверить RouterOS user/group permissions для сценария:

```text
ssh client -> run конкретный script -> add observed/declared address to address-list
```

Важно понять, можно ли достаточно ограничить пользователя без OpenSSH-style `ForceCommand`.
