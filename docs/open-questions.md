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
