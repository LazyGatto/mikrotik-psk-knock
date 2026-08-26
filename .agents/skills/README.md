# Процессные скиллы проекта

Скиллы, от которых зависит рабочий процесс из
[`docs/agents/`](../../docs/agents/README_FOR_AGENTS.md):

| Скилл | Где применяется |
| ------- | ----------------- |
| [`devils-advocate`](devils-advocate/SKILL.md) | DA-гейт, после того как дифф исполнителя прошёл `scripts/verify.sh` |
| [`writing-plans`](writing-plans/SKILL.md) | Перед стартом волны — разрезать дизайн на распределяемые шаги |
| [`verification`](verification/SKILL.md) | Перед любым заявлением о готовности — «готово», закрытие issue, `APPROVED`, сводка к MR |

Техническое macOS/Swift-знание — в
[`client-macos/.agents/skills/`](../../client-macos/.agents/skills/), рядом с
кодом, к которому относится.
