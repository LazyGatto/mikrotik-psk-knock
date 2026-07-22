# MikroTik PSK Knock

Проект для проработки и дальнейшей реализации безопасного временного открытия `dst-nat` на MikroTik через authenticated knock.

Основная идея: не держать порт-форварды доступными из Интернета постоянно, а открывать их на короткое время только для конкретного `src-address` после успешного knock.

## Цели

- Реализовать максимально автономный RouterOS-only вариант без Docker и внешнего сервиса там, где это возможно.
- Использовать staged UDP knock как дешевый предварительный фильтр.
- Использовать PSK-derived time-token на базе возможностей RouterOS `:convert transform=sha512`.
- Добавлять успешный `src-address` в firewall address-list с timeout.
- Держать `dst-nat` правила статическими, но ограниченными через `src-address-list`.
- Отправлять уведомления при каждом добавлении нового адреса в разрешенный список.
- В дальнейшем сделать клиентское приложение с CLI и GUI.
- Отдельно рассмотреть SSH/Ed25519 режим как более строгую криптографическую альтернативу.

## Базовый поток

```text
client
  -> UDP knock stage 1
  -> UDP knock stage 2
  -> UDP token stage with short-lived PSK token

MikroTik
  -> добавляет src-address в token-hit address-list
  -> scheduler выбирает допустимый hit и помечает bucket/token used
  -> добавляет selected src-address в allowed address-list с timeout
  -> отправляет уведомление владельцу
  -> dst-nat начинает работать только для этого src-address
```

## Статус

Сейчас это проектная заготовка с первично проверенной ROS-only базой на живом CHR: собраны требования, ограничения RouterOS, модель угроз, варианты архитектуры и реестр оставшихся вопросов.

Сделано:

- зафиксирован ROS-only дизайн через staged UDP и PSK-derived time-token;
- добавлена polling-модель `token-hit -> scheduler -> allowed` для сужения replay window;
- закрыты основные концептуальные вопросы по dynamic/roaming clients, observed source IP и per-client PSK;
- проверены ключевые RouterOS-примитивы на CHR: `sha512`, time bucket через `:timestamp`, UDP `content`, обновление rule content и scheduler 1s;
- собран и проверен минимальный RouterOS end-to-end прототип `stage1 -> stage2 -> token-hit -> scheduler -> allowed`;
- собран и проверен PSK-derived time-token прототип с RouterOS-side `sha512` и token rules для текущего/предыдущего bucket;
- добавлены и проверены persistent profile script, startup guard, disabled `dst-nat` demo-rule и notification hook.

Ближайший следующий шаг: вынести NAT target и notification target параметры в profile/service format.

## Документы

- [docs/context.md](docs/context.md) - консолидированный контекст обсуждения и технические заметки.
- [docs/design.md](docs/design.md) - первичный дизайн ROS-only решения.
- [docs/threat-model.md](docs/threat-model.md) - модель угроз и ограничения.
- [docs/open-questions.md](docs/open-questions.md) - открытые вопросы и принятые концептуальные решения.
- [docs/profile-format.md](docs/profile-format.md) - текущий формат persistent RouterOS profile script.
- [docs/roadmap.md](docs/roadmap.md) - план дальнейшей работы.
- [agent/instructions.md](agent/instructions.md) - инструкции для будущего агента/разработчика.
