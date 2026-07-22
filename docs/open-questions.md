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

Статус: решено концептуально, требует проверки деталей RouterOS.

Нельзя разрешать все адреса, попавшие в `token-hit` для одного token/bucket.

Политика:

- если RouterOS дает надежно определить первый dynamic address-list entry, открыть только первый hit;
- если порядок ненадежен, сжечь token/bucket, никого не открывать и отправить alert `collision/replay suspicion`.

### Token должен быть per-client

Статус: решено.

Предпочтительно использовать token/PSK per client или per client profile.

Per-profile общий секрет проще, но хуже для аудита и ротации: утечка одного PSK компрометирует всех пользователей профиля.

### Клиент должен поддерживать UDP-token и SSH modes

Статус: решено.

UDP-token mode - ROS-only компромисс для окружений без внешнего verifier.

SSH/Ed25519 mode - более строгий режим для окружений, где можно безопасно открыть management path и использовать SSH public-key authentication.

## Требует проверки на RouterOS

### Packet payload в script

Статус: гипотеза - скорее всего нельзя.

Вопрос: может ли RouterOS firewall/script надежно передавать UDP payload и metadata конкретного packet в script для parsing?

Почему важно: если да, можно построить более полноценный verifier с `check token -> mark used -> allow src` ближе к атомарной операции. Если нет, остается firewall/content плюс scheduler polling.

### `:convert sha512`

Статус: требует проверки на целевых версиях RouterOS.

Проверить:

- точный синтаксис `:convert` для `sha512`;
- формат входа `raw`;
- формат выхода `hex`;
- совпадение результата с reference implementation на клиенте.

### Time bucket в RouterOS script

Статус: требует проверки.

Проверить, можно ли удобно и надежно получить Unix time или иной стабильный timestamp для расчета:

```text
bucket = floor(now / bucket_size)
```

Если Unix time неудобен, нужно выбрать RouterOS-friendly способ вычисления bucket.

### Firewall `content` для UDP token

Статус: требует проверки.

Проверить:

- матчится ли UDP payload через `content` достаточно предсказуемо;
- можно ли сделать exact token matching без ложных совпадений;
- есть ли ограничения по длине hex token;
- не требуется ли `layer7-protocol`.

Предпочтение: использовать `content`, если его достаточно. `layer7-protocol` тяжелее и должен быть запасным вариантом.

### Обновление firewall rules scheduler-ом

Статус: требует проверки.

Проверить, можно ли регулярно обновлять token rules без побочных эффектов:

- менять `content` matcher;
- включать/отключать rules;
- держать отдельные rules для `now` и `now-1`;
- не создавать заметных packet drops или CPU spikes.

### Минимальный scheduler interval

Статус: требует проверки.

Текущая гипотеза: около 1 секунды.

Проверить, стабилен ли scheduler interval 1s на целевых MikroTik и не создает ли он заметной нагрузки.

Это важно для replay window:

```text
replay window ~= scheduler interval + processing time
```

### Порядок dynamic address-list entries

Статус: требует проверки.

Проверить, можно ли надежно определить первый `token-hit`, если за один polling interval в address-list попали несколько source IP.

Если порядок ненадежен, политика должна быть fail-closed: сжечь token/bucket, никого не открыть, отправить alert.

### Хранение `used bucket/token state`

Статус: требует проверки.

Кандидаты:

- script variable;
- address-list marker;
- disabled/commented firewall rule;
- file.

Предварительная позиция: избегать частой записи в files, если можно хранить state в runtime/config objects. Файлы могут быть полезны для debug или persistence, но хуже как hot-path state.

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

