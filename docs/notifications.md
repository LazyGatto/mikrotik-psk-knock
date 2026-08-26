# Уведомления об открытии доступа

После успешного стука роутер добавляет observed source IP в allowed-list и **затем** шлёт
уведомление. Каналы независимы: включённых может быть сколько угодно, каждый обёрнут в
`:do {} on-error={}` — **сбой доставки логируется и не откатывает уже открытый доступ**
(`/log print where message~"mkpk-tt notify"`).

Настройка — на уровне роутера: вкладка **Notifications** в карточке роутера
(`mkpk-provision serve` / desktop) либо секция `notify:` в `mkpk.yaml`. После изменений нужен
**передеплой роутера** (`mkpk-provision deploy`) — конфиг запекается в скрипт `mkpk-tt-notify`,
чтобы переживать перезагрузку.

## Telegram

```yaml
routers:
  my-router:
    notify:
      telegram:
        enabled: true
        bot_token: "123456789:AA…"   # от @BotFather
        chat_id: "-1001234567890"    # куда слать
        thread_id: "42"              # опционально: топик форум-супергруппы
```

Сообщение приходит в человекочитаемом виде:

```text
🔓 KZ-D2A: socks5 open for lazygatto
from 95.25.177.50 · 55m
```

### 1. Бот

1. [@BotFather](https://t.me/BotFather) → `/newbot` → получите `bot_token` вида `123456789:AA…`.
2. Добавьте бота в целевой чат/группу (в личку — просто напишите ему первым: бот не может
   написать первым).

### 2. `chat_id`

| Куда шлём | Как получить `chat_id` |
| ----------- | ------------------------ |
| Личка | Напишите боту любое сообщение → откройте `https://api.telegram.org/bot<TOKEN>/getUpdates` → `message.chat.id` (положительное число) |
| Обычная группа | Добавьте бота в группу, напишите там что-нибудь → тот же `getUpdates` → `chat.id` (отрицательное) |
| Супергруппа / канал | `chat.id` вида `-100…`. Быстрый способ: откройте чат в Telegram Web, в адресе `t.me/c/<NNNN>/…` → `chat_id` = `-100<NNNN>` |

Если `getUpdates` возвращает пустой `result` — бот ещё не видел ни одного сообщения (или у него
включён privacy mode: для групп либо дайте боту админку, либо обратитесь к нему через `/`-команду).

### 3. `thread_id` — топик в форум-супергруппе

Если в группе включены **Topics** (форум) и уведомления нужны в конкретный топик, заполните поле
**топик (форум-группа)** — это `message_thread_id` из Bot API.

Где взять число: откройте нужный топик в Telegram Web и посмотрите на URL —
`t.me/c/<NNNN>/<TTT>/<msg-id>`, где `<TTT>` и есть id топика. В десктопном/мобильном клиенте:
«Copy Message Link» на любом сообщении внутри топика даёт ту же ссылку.

- Пусто → сообщения уходят в **General** (или в обычный чат, если топиков нет).
- Бот должен быть участником группы; для закрытых топиков ему нужны права на запись в них.
- `chat_id` при этом остаётся `chat_id` **всей группы** (`-100…`), а не топика — указать топик
  «внутри chat_id» нельзя, это отдельный параметр.

Проверка вручную (без роутера):

```sh
curl -s "https://api.telegram.org/bot<TOKEN>/sendMessage" \
  -d chat_id=-1001234567890 -d message_thread_id=42 -d text=test
```

`{"ok":true,…}` — параметры верные; `chat not found` / `message thread not found` — неверные
`chat_id` / `thread_id` либо бота нет в группе.

## Webhook

```yaml
notify:
  webhook:
    enabled: true
    url: "https://example.com/hook"
```

`POST` с JSON-телом (машиночитаемое, в отличие от текста для телеги):

```json
{"router":"KZ-D2A","service":"socks5","client_id":"lazygatto","src":"95.25.177.50",
 "list":"mkpk-tt-allowed-socks5","ttl":"55m","mode":"udp-token","bucket":1787746169,"time":"…"}
```

## E-mail

```yaml
notify:
  email:
    enabled: true
    to: "ops@example.com"
    from: "mkpk@example.com"
    server: "smtp.example.com"
    port: 587          # по умолчанию 587
    tls: starttls      # по умолчанию starttls
    user: "…"
    password: "…"
```

Параметры передаются inline в `/tool e-mail send` — глобальный `/tool e-mail` роутера не
мутируется. Тело письма — те же `key=value`, что и в webhook.

## Диагностика

- `/log print where message~"mkpk-tt"` на роутере: `mkpk-tt allowed …` пишется всегда, строки
  `mkpk-tt notify <channel> failed …` — только при сбое канала.
- Для `fetch`-каналов (telegram, webhook) роутеру нужны рабочий DNS и исходящий HTTPS.
- Уведомление отправляется **синхронно внутри тика поллера** (1s). Если внешний сервис
  недоступен, тик задержится на таймаут `fetch` — при регулярных проблемах со связностью
  стоит вынести отправку в отдельный one-shot scheduler (см. issue про live-тест уведомлений).
