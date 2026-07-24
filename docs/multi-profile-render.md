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
- used-marker list: `mkpk-tt-used-C-<bucket>`.

Обработка всех клиентов идёт в одном скрипте `mkpk-tt-poller` (см. ниже), а не
per-client.

Несколько token-правил на одном token-порту сервиса различаются по `content`
(разные 128-hex токены), поэтому пакет клиента C попадает только в его hit-list.

## Per-service изоляция allowed-list

Прежний общий default `allowed_list = mkpk-tt-allowed` в многосервисной схеме
ломал изоляцию: успешный knock любого клиента открывал бы NAT всех сервисов.
Теперь default — `mkpk-tt-allowed-<service>`, поэтому knock клиента открывает
только NAT его сервиса. Значение по-прежнему можно переопределить в конфиге.

## Data-driven poller (один scheduler)

Обработка всех профилей идёт в одном скрипте `mkpk-tt-poller` с одним
scheduler-ом `interval=1s`. Устройство:

- Таблица клиентов — RouterOS array-of-arrays (по одному associative-array на
  клиента, ключи в кавычках: `{"key"="c1"; "service"="svca"; "psk"="..."; ...}`).
  Строится один раз и кэшируется в global `mkpkTtClients` (пересборка литерала
  каждый тик доминировала бы по CPU); сбрасывается при (re)import и теряется при
  reboot, после чего poller строит её заново.
- Логика вынесена в две `do={}`-функции: `refreshTokens` (пересчёт now/prev
  token и запись в firewall rule) и `processHits` (перенос observed src в
  allowed, used-marker, notify). Ранее это был проверенный single-profile poller.
- **Bucket-cache**: `refreshTokens` вызывается по всем клиентам только когда
  сменился bucket (global `mkpkTtBucket`), то есть раз в `bucket_seconds`, а не
  каждую секунду. Это убирает per-second `sha512`-пересчёт.
- **Hot-path guard**: каждую секунду делается один regex-find
  `list~"^mkpk-tt-hit-"`; per-client `processHits` запускается только если
  хоть один hit есть (редкий случай). В простое per-tick стоимость ~константна
  независимо от N.

Почему функции, а не отдельные скрипты: проверено на CHR, что `:return` из
скрипта, запущенного через `/system script run`, прерывает остаток списка команд
вызывающего. `:return` внутри `:foreach` завершил бы весь poller. А `:return`
внутри `do={}`-функции завершает только функцию — поэтому ранние выходы
(`hitCount=0`, `used`, collision) сохраняются, а `:foreach` продолжает по
остальным клиентам. Проверено также: `do={}`-функция может писать в global,
видимый отдельно запускаемому `mkpk-tt-notify`.

## Startup / install / fail-closed

- `mkpk-tt-startup`: переводит все token-правила (`comment~"^mkpk-tt token "`) в
  `disabled=yes` + invalid content, чистит все hit-списки
  (`list~"^mkpk-tt-hit-"`), сбрасывает global `mkpkTtBucket=0`, затем запускает
  `mkpk-tt-apply-service`.
- `mkpk-tt-poller` (scheduler `interval=1s`) на следующем тике видит, что
  `nowBucket != mkpkTtBucket`, и через `refreshTokens` пересчитывает и включает
  token-правила. До этого — fail-closed.
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
/system script environment remove [find where name~"^mkpkTt"]
```

Последняя строка сбрасывает mkpk globals (в т.ч. кэш `mkpkTtClients` и
`mkpkTtBucket`), чтобы re-import не переиспользовал устаревшие данные профилей.

Полный re-render всегда пересоздаёт весь `mkpk-tt-*` слой; частичное обновление
отдельного профиля пока не поддерживается.

## Проверка на CHR (2026-07-24)

Проверено на CHR ROUTER-A (RouterOS 7.23.2) конфигом с 2 services (`svca`, `svcb`)
и 2 clients (`ca`→svca, `cb`→svcb):

Data-driven poller проверен на CHR конфигом с 20 клиентами (2 services) и
функционально с 2 services / 2 clients:

- import создал ожидаемые объекты: **4 scripts** (apply-service, notify, poller,
  startup) и **2 scheduler** (startup + poller; install self-removed) независимо
  от N; 2×N token rules, 2×N stage-lists по числу сервисов;
- **poller срабатывает после import без reboot** и через bucket-cache refresh
  заполняет token content актуальными 128-hex токенами (для 20 клиентов — все
  40 token rules `disabled=no`); главный риск дизайна снят;
- RouterOS-side `sha512` совпал с client-side Go токеном;
- end-to-end knock (`stage1 -> stage2 -> token -> poller -> allowed`) сработал:
  observed source IP попал в `mkpk-tt-allowed-<svc>` с корректным comment;
- **per-service изоляция allowed-list подтверждена**: knock клиента на svca открыл
  только `mkpk-tt-allowed-svca`, svcb остался пуст;
- used-marker и replay-путь работают через hit-guard: повторный knock в том же
  bucket логируется `mkpk-tt replay ignored for <client>; bucket already used` и
  не переоткрывает доступ.

Практическая заметка по окружению: при отправке knock через VPN observed source
IP может отличаться от «прямого» egress, а из-за variance UDP-потоков коротких
retry-окон иногда не хватало, чтобы token-пакеты пришли с того же IP, что и stage
(тогда token rule не матчился). С `--stage-duration 3s --token-duration 3s`
knock проходил стабильно. Это свойство клиентского пути/VPN, не рендера.

Тестовые объекты после проверки удалены с CHR.

## Нагрузочный тест N планировщиков (2026-07-24)

Измерено на том же CHR (1 CPU, 2GHz, RouterOS 7.23.2), sampling `cpu-load` в одной
SSH-сессии с `:delay 1s` (чтобы не мерить overhead от per-command SSH):

Сначала измерялась прежняя per-client схема (N pollers × N schedulers), затем —
текущий data-driven poller (1 scheduler). Baseline на CHR шумит и дрейфует, важна
дельта над baseline той же сессии.

| Схема (20 clients) | baseline | loaded avg | пик | дельта |
|---|---|---|---|---|
| per-client (20 pollers @1s) | ~2% | ~26% | ~50% | ~24% |
| data-driven (1 poller, hit-guard) | ~9% | ~21% | ~30% | ~12% |

Также структурно: data-driven даёт **2 scheduler вместо 21**, **4 script вместо
23**, **~434 строки `.rsc` вместо ~2440** для 20 клиентов.

Что дало выигрыш:

- Первая наивная консолидация (один poller, но пересбор client-массива и пересчёт
  токенов каждый тик) CPU не улучшила — стоимость просто переехала из sha512 в
  построение литерала.
- Кэш `mkpkTtClients` (строить массив один раз) + **bucket-cache** (пересчёт
  токенов только на границе bucket) + **hit-guard** (per-client обход только когда
  есть hit) — в сумме примерно вдвое снизили дельту CPU и убрали синхронный
  rollover-burst (пик 50%→30%).

Замечание про `content`-matcher: token rule гейтится `src-address-list=stage2-<svc>`,
поэтому случайный интернет-шум на token-порт отсекается дешёвой проверкой
address-list ещё до `content`-сравнения. Полноценный flood-тест staged-портов
(рост stage address-list, CPU under flood) не проводился и остаётся follow-up.

## Ограничения / открытые вопросы

- Deterministic-порядок объектов обеспечивается сортировкой ключей.
- Масштабирование: data-driven poller снял per-scheduler overhead и rollover-burst;
  дельта CPU для 20 клиентов ~12%. Дальнейшее снижение (напр. более редкий refresh,
  батч-обработка) — по мере необходимости.
- Flood-тест staged-портов под нагрузкой ещё не выполнен.
