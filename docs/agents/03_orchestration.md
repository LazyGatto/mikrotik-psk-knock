# Оркестрация волн в Herdr

Этот документ отвечает на вопрос **как физически идёт волна**: где живут
исполнители, в каких ветках, кто что видит. *Что* строим — GitLab issues и
`docs/plans/`. *Кто* строит — [`02_executor_pool.md`](02_executor_pool.md).

Herdr — менеджер терминального рабочего пространства для агентов. `herdr --skill`
печатает актуальный скилл; синтаксис команд — за самим бинарником: читайте
идентификаторы из JSON-ответов, не угадывайте их. Team Lead всегда работает
внутри панели Herdr; вне её (`HERDR_ENV` не выставлен) оркестрационные команды
запускать нельзя — они уйдут в ту сессию, на которой сейчас фокус у maintainer'а.

## Решения

- **D-1. Один worktree на волну/issue.** Всегда, включая однострочные правки
  кода. Исключение: изменения только в `docs/`, `CHANGELOG.md` или
  `.agents/skills/` без кода — можно веткой в основной копии. В остальное время
  основная копия Team Lead'а стоит на чистом `main`.
- **D-2. Стандартная раскладка волны.** `p1` — исполнитель, `p2` — панель
  верификации (`bash scripts/verify.sh`). DA стартует в том же workspace после
  исполнителя, чтобы его вопросы отвечались по живому контексту. Team Lead
  никогда не крадёт фокус (`--no-focus`).
- **D-3. Долгая или пишущая работа идёт в панель Herdr.** Всё, что дольше ~10
  минут или трогает файлы: `herdr agent start` в worktree. Оно остаётся видимым
  maintainer'у, переживает сессию Team Lead'а и сохраняет контекст для раунда
  `NEEDS_REVISION`. Инструмент `Agent` внутри сессии Team Lead'а — только чтение
  и поиск.
- **D-4. Бриф живёт в GitLab, права живут на роли.** Team Lead публикует
  комментарий «Executor brief» в issue (шаблон ниже); промпт — одна строка:
  *«Возьми #N: прочитай описание и комментарий "Executor brief", следуй ему
  точно»*. Исполнители читают GitLab через MCP и не имеют там прав на запись.
- **D-5. Приземление — через merge request.** Прямой пуш в `main` был привычкой
  до worktree; при параллельных волнах он теряет точку ревью и устраивает гонку
  за `CHANGELOG.md`. Коммиты только в документацию могут по-прежнему уходить
  напрямую.
- **D-6. Живой роутер — эксклюзивный ресурс.** Ни одна волна не ходит на роутер
  или CHR сама. `mkpk-provision deploy`, живой knock и разведка RouterOS-семантики
  выполняет maintainer, по одной за раз. Исполнитель, которому это нужно, обязан
  поднять HALT (H-2), а не «попробовать».

## Жизненный цикл волны

### 1. Team Lead: план и worktree

Issue → триаж → план в `docs/plans/` (с секцией *Архитектурная проверка* — см.
скилл [`writing-plans`](../../.agents/skills/writing-plans/SKILL.md)) → worktree:

```bash
test "${HERDR_ENV:-}" = 1     # всегда первая команда

herdr worktree create --workspace <id workspace mikrotik-psk-knock> \
  --branch feature/<slug>-<issue> --base main \
  --label "#<issue> <slug>" --no-focus
```

`--workspace` **обязателен**. Без него Herdr берёт репозиторий того workspace,
на котором сейчас фокус у maintainer'а, и worktree уезжает в чужой проект.
Проверьте путь в JSON-ответе, прежде чем продолжать.

### 2. Исполнитель

```bash
sleep 3   # оболочка панели должна дойти до приглашения, иначе agent_pane_busy

herdr agent start impl-<issue> --kind opencode --pane <w:p1> -- --agent or-pro-coder
herdr agent prompt impl-<issue> \
  "Возьми #<issue>: прочитай описание и комментарий Executor brief, следуй ему точно." \
  --wait --timeout 1800000
```

Только однострочные промпты — многострочный промпт TUI `opencode` не отправляет.
Длинные инструкции живут в issue или в файле внутри worktree. Результат читается
через `herdr agent read`.

### 3. Верификация

```bash
herdr pane split --pane <w:p1> --direction right --cwd <путь worktree> --no-focus
herdr pane run <w:p2> "bash scripts/verify.sh; echo GATE=\$?"
herdr pane wait-output <w:p2> --regex "GATE=[0-9]+" --timeout 1800000
```

Гейт — это `GATE=0`. Не «VERIFY OK», мелькнувшее где-то на экране, и не сводка
исполнителя. В свежем worktree нет ни `client-macos/.build/`, ни прогретого
Go-кеша модулей, поэтому первый прогон — полная холодная сборка обеих частей:
считайте в минутах, а не в секундах.

### 4. Гейт DA

```bash
herdr agent start da-<issue> --kind codex --pane <w:p3>
```

Дайте ему роль [`devils-advocate`](../../.agents/skills/devils-advocate/SKILL.md),
включая прокурорскую оговорку («назови три конкретных способа, которыми этот
дифф ломает клиент, утекает PSK или молча оставляет порт открытым»). Вердикты:

- `APPROVED` → к личному гейту Team Lead'а;
- `NEEDS_REVISION` → промпт исполнителю **в том же workspace**, где ещё жив его
  контекст;
- `HALT` → действуем по [`.claude/rules/halt.md`](../../.claude/rules/halt.md).

`codex` в свежем worktree открывает два диалога доверия и **съедает первый
промпт** — дождитесь его вопроса (`agent wait --until blocked` / `agent read`),
ответьте и только потом отправляйте настоящий промпт. Если OAuth `codex` истёк,
он принимает промпт и умирает с 401; перелогинить может только maintainer
(`codex login`). Запасной DA: `opencode` с read-only ролью ревьюера.

### 5. Team Lead: личный гейт и приземление

Личный гейт: объём (не просочилось ли лишнее), скрытая проверка приёмки,
секреты (нет реальных хостов/PSK/инвайтов в отслеживаемых файлах), локализация
macOS-клиента (нет хардкода пользовательских строк мимо `L(...)`), запись в
CHANGELOG, строка урока в [`04_lessons.md`](04_lessons.md), если исполнитель
споткнулся о среду.

Затем коммит **из worktree** и пуш, создающий MR тем же действием:

```bash
git push -u origin feature/<slug>-<issue> \
  -o merge_request.create -o merge_request.target=main \
  -o merge_request.remove_source_branch \
  -o merge_request.merge_when_pipeline_succeeds \
  -o merge_request.title="<type>(<scope>): <что> (#<issue>)"
```

Создание MR прямо при пуше не даёт GitLab запустить лишний branch-пайплайн
рядом с MR-пайплайном. Описание дополняется через `glab mr update`. После
слияния: закрыть issue сводным комментарием, синхронизировать публичное зеркало
(`git push github main`) и `herdr worktree remove`.

Пуш, не несущий нового коммита (например, после упавшего pre-commit хука),
создаёт MR как **Draft**, и merge-when-pipeline-succeeds не срабатывает —
`glab mr update <n> --ready` после настоящего пуша.

### 6. Релизы

Релиз — действие Team Lead'а в основной копии на чистом `main`: CHANGELOG →
коммит → тег `vX.Y.Z` → пуш тега. CI собирает только Go-бинарники; обе
macOS-сборки (Wails `mkpk-provision-desktop` и нативный `client-macos`)
собираются, подписываются и нотаризуются вручную и прикладываются к тому же
релизу — пошагово это описано в [`AGENTS.md`](../../AGENTS.md) («Release flow»).
**Никогда не бампайте версию и не ставьте тег без явного согласия владельца.**

## Бриф исполнителю (шаблон комментария к issue; годится и как файл)

```text
Задача: #<issue> — <одна строка>.
План: docs/plans/<файл>.md, шаги <N..M>. Только они. Ничего вне объёма.

Контекст (это прочитать; остальное — только при необходимости):
- файлы: <3–7 путей>
- инварианты: AGENTS.md; для client-macos — ещё client-macos/AGENTS.md
- домен: docs/design.md, docs/threat-model.md, agent/instructions.md
- уроки: docs/agents/04_lessons.md — строки про <тему>

Ограничения: не коммитить, не пушить, не писать в GitLab. Не ходить на роутер и
не запускать deploy. Не добавлять сторонние зависимости. Не менять формулу
токена и золотые векторы. Не вносить реальные хосты/логины/PSK в отслеживаемые
файлы — только placeholder'ы (router.example.com, 203.0.113.x). В macOS-клиенте
пользовательские строки — только через L("en", "ru"). Временные файлы — в tmp/
внутри этого worktree, никогда не в /tmp. Не трогать файлы вне указанной зоны.

Готово, когда: `bash scripts/verify.sh` выходит с кодом 0 (вставьте код возврата,
а не хвост лога); <скрытая проверка приёмки здесь не показывается>.
Перед сдачей: если наткнулся на проблему среды/сборки/инструментов и починил её —
добавь одну строку в docs/agents/04_lessons.md в формате того файла.
В ответе: список изменённых файлов + что осталось неясным.
```

Скрытая проверка приёмки — тест или сценарий, которого исполнитель не видел, —
прогоняется Team Lead'ом и входит в личный гейт.

## Параллельные волны

Только при непересекающихся файловых зонах; иначе последовательно. Известные
опасности при двух одновременных волнах:

- **Живой роутер/CHR один.** Две волны не могут одновременно деплоить или
  стучаться: address-list и token-hit состояние общие, и результат проверки
  станет неинтерпретируемым. Живые прогоны — строго по одному.
- **Конфликты в `CHANGELOG.md`** по секции `[Unreleased]`. Разрешаются слиянием
  обеих сторон. Если это станет рутиной, системное лекарство — фрагменты на
  issue (`changelog/unreleased/<issue>.md`), собираемые при релизе; это
  предложение maintainer'у, а не принятое решение.
- **Общие кеши сборки.** `client-macos/.build/` у каждого worktree свой (это
  хорошо), а `~/Library/Caches/org.swift.swiftpm` и `$GOPATH/pkg/mod` — общие.
  Первые сборки в новых worktree запускайте последовательно.
- **`swift test` в этом репозитории нет намеренно**: CLT не несёт тестового
  рантайма. Проверка Swift-рантайма — `swift run mkpk-selfcheck`; при добавлении
  регрессии её место там.

## Шпаргалка

```bash
test "${HERDR_ENV:-}" = 1                        # всегда первым
herdr workspace list; herdr agent list           # что живо
herdr worktree create --workspace W --branch B --base main --label L --no-focus
herdr pane split --pane ID --direction right --cwd "$PWD" --no-focus
herdr agent start NAME --kind opencode|codex|claude --pane ID -- <args>
herdr agent prompt NAME "одна строка" --wait --timeout MS
herdr agent wait NAME --until blocked --timeout MS
herdr agent read NAME --source recent-unwrapped --lines 200
herdr pane run ID "bash scripts/verify.sh; echo GATE=\$?"
herdr pane wait-output ID --regex "GATE=[0-9]+" --timeout MS
herdr worktree remove ...                        # только после state=merged
```

Не закрывайте workspace и панели, которые создали не вы. Не запускайте
`herdr server stop`. Не отвечайте за maintainer'а на вопрос заблокированного
агента, если план такого ответа не содержит.
