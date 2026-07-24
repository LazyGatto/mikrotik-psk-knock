# Multi-profile RouterOS render

Дизайн многопрофильной генерации `.rsc`. Заменяет прежнюю single-profile схему,
где `routeros render` рендерил ровно одного client/service с захардкоженными
именами объектов.

Статус: реализовано в `client/internal/routeros/render.go` (`RenderConfig`) и
**проверено end-to-end на живом CHR** (ROUTER-A, RouterOS 7.23.2), см. раздел
«Проверка на CHR» ниже.

## Термины

- **service** — сетевой профиль сервиса: свои stage1/stage2/token порты, NAT
  target, notify-настройки и собственный `allowed` address-list.
- **client** — учётка со своим `psk` и `client_id`, привязанная к одному service.
  Токен считается per-client (`token = sha512(psk|v1|service|client_id|bucket|psk)`),
  поэтому именно client — единица, у которой есть собственные token-правила и
  poller.

Несколько clients могут указывать на один service (общие stage-порты и общий
`allowed` список этого service), и несколько services могут сосуществовать.

## Namespacing объектов

Ключи map (`services.<name>`, `clients.<name>`) валидируются как безопасные
идентификаторы (`^[A-Za-z0-9][A-Za-z0-9_-]*$`) и подставляются в имена объектов.

Per-service (S = ключ сервиса):

- firewall stage1: `comment="mkpk-tt stage1 S"`, `address-list=mkpk-tt-stage1-S`;
- firewall stage2: `src-address-list=mkpk-tt-stage1-S`, `address-list=mkpk-tt-stage2-S`;
- allowed address-list: `mkpk-tt-allowed-S` (по умолчанию per-service, см. ниже);
- NAT rule: `comment="mkpk-tt dst-nat S"`, `src-address-list=mkpk-tt-allowed-S`.

Per-client (C = ключ клиента, на сервисе S):

- token now/prev firewall rules: `comment="mkpk-tt token now C"` /
  `"mkpk-tt token prev C"`, `dst-port=<S.token_port>`,
  `src-address-list=mkpk-tt-stage2-S`, `content=<token>`,
  `address-list=mkpk-tt-hit-now-C` / `mkpk-tt-hit-prev-C`;
- used-marker list: `mkpk-tt-used-C-<bucket>`;
- poller script: `mkpk-tt-poller-C`;
- poller scheduler: `mkpk-tt-poller-C` (`interval=1s start-time=startup`).

Несколько token-правил на одном token-порту сервиса различаются по `content`
(разные 128-hex токены), поэтому пакет клиента C попадает только в его hit-list.

## Per-service изоляция allowed-list

Прежний общий default `allowed_list = mkpk-tt-allowed` в многосервисной схеме
ломал изоляцию: успешный knock любого клиента открывал бы NAT всех сервисов.
Теперь default — `mkpk-tt-allowed-<service>`, поэтому knock клиента открывает
только NAT его сервиса. Значение по-прежнему можно переопределить в конфиге.

## Почему per-client scheduler, а не общий poller

Проверенный на CHR факт: `:return` из скрипта, запущенного через
`/system script run`, прерывает остаток on-event/родительского списка команд
(поэтому в single-profile self-remove в `mkpk-tt-install` шёл первым). Из-за
этого последовательный прогон нескольких pollers, каждый со своими early
`:return 0`, обрывался бы после первого.

Решение: у каждого клиента собственный poller-скрипт и собственный scheduler
`interval=1s`. Каждый `mkpk-tt-poller-C` — это почти дословная копия уже
проверенного single-profile poller, специализированная под C (свои имена
hit/used-списков, comment token-правил, инлайновые значения профиля). Это
минимизирует риск RouterOS-семантики (никакой новой control-flow-логики), ценой
N планировщиков по 1с. Для «нескольких roaming клиентов» это приемлемо; при
масштабировании на многие десятки клиентов стоит вернуться к data-driven poller.

Значения профиля (`service`, `client_id`, `psk`, `token_port`, `allowed_list`,
таймауты, notify) инлайнятся как `:local` прямо в `mkpk-tt-poller-C`. Отдельный
per-client profile-скрипт не используется: RouterOS globals глобальны по имени,
и разделяемые имена секретов между профилями были бы хрупкими.

## Startup / install / fail-closed

- `mkpk-tt-startup`: переводит все token-правила (`comment~"^mkpk-tt token "`) в
  `disabled=yes` + invalid content, чистит все hit-списки
  (`list~"^mkpk-tt-hit-"`), затем запускает `mkpk-tt-apply-service`. Pollers НЕ
  запускает (иначе снова проблема `:return`-цепочки).
- Каждый `mkpk-tt-poller-C` (scheduler `interval=1s`) в течение ~1с сам
  пересчитывает и включает свои token-правила. До этого — fail-closed.
- `mkpk-tt-install` (one-shot): сначала удаляет себя, затем запускает
  `mkpk-tt-startup`. Обеспечивает init без reboot после import.
- `mkpk-tt-apply-service`: прямолинейный скрипт, для каждого сервиса создаёт или
  обновляет его NAT rule из инлайновых значений (без early `:return`).

## Cleanup (в начале сгенерированного скрипта)

```
/system scheduler remove [find where name~"^mkpk-tt-"]
/system script remove [find where name~"^mkpk-tt-"]
/ip firewall filter remove [find where comment~"^mkpk-tt "]
/ip firewall nat remove [find where comment~"^mkpk-tt "]
/ip firewall address-list remove [find where list~"^mkpk-tt-"]
```

Полный re-render всегда пересоздаёт весь `mkpk-tt-*` слой; частичное обновление
отдельного профиля пока не поддерживается.

## Проверка на CHR (2026-07-24)

Проверено на CHR ROUTER-A (RouterOS 7.23.2) конфигом с 2 services (`svca`, `svcb`)
и 2 clients (`ca`→svca, `cb`→svcb):

- import создал ожидаемые объекты: 5 scripts (apply-service, notify, poller-ca,
  poller-cb, startup), 3 scheduler (startup + 2 poller; install self-removed),
  8 filter rules (4 stage + 4 token), 2 NAT;
- **per-client pollers срабатывают после import без reboot** и заполняют token
  content актуальными 128-hex токенами (главный риск дизайна снят);
- RouterOS-side `sha512` совпал с client-side Go токеном для per-client формулы;
- end-to-end knock (`stage1 -> stage2 -> token -> poller -> allowed`) сработал:
  observed source IP попал в `mkpk-tt-allowed-svca` с корректным comment;
- **per-service изоляция allowed-list подтверждена**: knock `ca` открыл только
  `mkpk-tt-allowed-svca`, `mkpk-tt-allowed-svcb` остался пуст; knock `cb` открыл
  только svcb; оба клиента работают независимо;
- per-client used-marker и replay-путь работают: повторный knock в том же bucket
  логируется `mkpk-tt replay ignored for <client>; bucket already used` и не
  переоткрывает доступ.

Практическая заметка по окружению: при отправке knock через VPN observed source
IP может отличаться от «прямого» egress, а из-за variance UDP-потоков коротких
retry-окон иногда не хватало, чтобы token-пакеты пришли с того же IP, что и stage
(тогда token rule не матчился). С `--stage-duration 3s --token-duration 3s`
knock проходил стабильно. Это свойство клиентского пути/VPN, не рендера.

Тестовые объекты после проверки удалены с CHR.

## Ограничения / открытые вопросы

- Deterministic-порядок объектов обеспечивается сортировкой ключей.
- Масштабирование: N планировщиков 1с — приемлемо для единиц клиентов, не для
  десятков; тогда нужен data-driven poller. CPU при N=2 на 1-CPU CHR был
  незаметен; отдельный нагрузочный тест на большем N ещё не делался.
