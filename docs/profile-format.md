# Config fields reference

Источник истины конфигурации — YAML клиента (`mkpk.yaml`), который загружает `mkpk-provision`. Из него
рендерится RouterOS `.rsc` и деплоится на роутер. Раскладку RouterOS-объектов см. в
[multi-profile-render.md](multi-profile-render.md).

> Историческая заметка: ранние прототипы хранили параметры в отдельном persistent-скрипте
> `mkpk-tt-profile-demo` с `:global`-переменными. Это заменено: в multi-profile схеме значения профилей
> инлайнятся в client-таблицу единого data-driven poller (`mkpk-tt-poller`), отдельного profile-скрипта
> больше нет.

## Пример

```yaml
router:
  name: router-a
  address: router.example.com
defaults:
  bucket_seconds: 30
  stage_timeout: 5s
  token_hit_timeout: 2s
  allowed_timeout: 3m
  used_timeout: 65s
services:
  demo-service:
    stage1_port: 41001
    stage2_port: 41002
    token_port: 41003
    allowed_list: mkpk-tt-allowed-demo-service
    nat: { enabled: false, dst_port: 2222, to_address: 192.0.2.10, to_port: 22 }
    notify: { enabled: false, channel: webhook, url: "" }
clients:
  demo-client:
    service: demo-service
    psk: mkpk-prototype-psk
```

## defaults

- `bucket_seconds` — размер time bucket; клиент и RouterOS считают один и тот же bucket.
- `stage_timeout` — TTL записей в stage1/stage2 address-list.
- `token_hit_timeout` — TTL записей в token-hit address-list.
- `allowed_timeout` — время, на которое открывается observed source IP.
- `used_timeout` — TTL used-marker. **Должен быть ≥ `2*bucket_seconds`**, чтобы marker перекрывал полное
  окно приёма `now`+`prev` (иначе токен можно повторить в разрыве между истечением marker и концом окна).

## services.\<name>

Ключ `<name>` и `allowed_list` должны быть safe-именами (`^[A-Za-z0-9][A-Za-z0-9_-]*$`) — они попадают в
имена RouterOS-объектов.

- `service_name` — включается в token message (по умолчанию = ключ).
- `stage1_port` / `stage2_port` / `token_port` — UDP-порты стадий; должны быть различны.
- `allowed_list` — address-list, через который ограничивается `dst-nat` (по умолчанию
  `mkpk-tt-allowed-<name>` — per-service изоляция).
- `nat.enabled` — включает сгенерированный `dst-nat` rule.
- `nat.comment` — стабильный comment, по которому `mkpk-tt-apply-service` находит NAT rule.
- `nat.dst_port` / `nat.to_address` / `nat.to_port` — внешний порт и внутренний target `dstnat`.
- `notify.*` — см. ниже.

## services.\<name>.notify

- `enabled` — включает notification path после успешного allow.
- `channel` — `webhook` | `telegram` | `email`.
- `url` — webhook endpoint (канал `webhook`).
- `telegram.bot_token` / `telegram.chat_id` — параметры Bot API (канал `telegram`).
- `email.to` / `email.from` / `email.server` / `email.port` / `email.tls` / `email.user` /
  `email.password` — SMTP-параметры (канал `email`); передаются inline в `/tool e-mail send`, глобальный
  `/tool e-mail` роутера не мутируется.

Ошибка доставки любого канала логируется и **не** откатывает уже добавленный `allowed` entry.

## clients.\<name>

Ключ `<name>` — safe-имя (используется в именах token-правил/hit-списков).

- `service` — ссылка на service.
- `client_id` — идентификатор в token message и audit-comment (по умолчанию = ключ).
- `psk` — per-client секрет. Разрешён только base64url-safe ASCII (`A-Z a-z 0-9 - _`), чтобы RouterOS
  string interpolation не искажала значение при рендере. `mkpk-provision secret generate` выдаёт такой
  формат.

## Токен

```text
token = sha512(psk + "|v1|" + service + "|" + client_id + "|" + bucket + "|" + psk)   (hex)
```

Считается одинаково клиентом (Go) и RouterOS (`:convert ... transform=sha512`) — сверено на CHR.

## Ограничения по секретам

PSK и (для email) SMTP-пароль попадают в source RouterOS-скриптов. Это требует ограничения прав
RouterOS users/groups на чтение scripts и аккуратного обращения с export/backup. Ротация секретов —
через изменение конфига и повторный `mkpk-provision deploy` (config-hash поймает дрифт).
