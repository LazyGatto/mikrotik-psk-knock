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
- Сделать клиентское приложение с CLI (далее — локальный веб-UI и десктоп).
- Провижининг на роутер выполнять по SSH; runtime port-knocking держать полностью на стороне клиента.

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

Рабочая ROS-only реализация с клиентским CLI и SSH-провижинингом; всё проверено end-to-end на живом
CHR (RouterOS 7.23.2).

Сделано:

- зафиксирован ROS-only дизайн через staged UDP и PSK-derived time-token; polling-модель
  `token-hit -> poller -> allowed` для сужения replay window;
- проверены на CHR ключевые RouterOS-примитивы: `sha512`, time bucket через `:timestamp`, UDP `content`,
  обновление rule content, scheduler 1s, reboot-survival, startup guard;
- **клиент** (Go): `mkpk` (runtime — `knock`, `check`) и `mkpk-provision` (admin — `secret`, `config`,
  `profile`, `service`, `client`, `token`, `routeros render`, `deploy`);
- **multi-profile render**: все services/clients в per-profile RouterOS-объекты с per-service изоляцией
  `allowed`-list;
- **data-driven poller**: один скрипт + один scheduler на все профили (вместо N), с кэшем и hit-guard;
- **уведомления**: каналы `webhook`, `telegram`, `email` с graceful degradation;
- **SSH-провижининг**: `mkpk-provision deploy` ставит/обновляет/снимает mkpk-слой по SSH с detect по
  config-hash и verify;
- фиксация used-marker `used_timeout >= 2*bucket_seconds`, валидация конфига (PSK-alphabet, имена, порты,
  таймауты).

Следующий шаг: локальный веб-UI поверх deploy-ядра, затем упаковка в десктоп (Wails). План — в
[docs/roadmap.md](docs/roadmap.md).

## Документы

- [docs/context.md](docs/context.md) - консолидированный контекст обсуждения и технические заметки.
- [docs/design.md](docs/design.md) - первичный дизайн ROS-only решения.
- [docs/threat-model.md](docs/threat-model.md) - модель угроз и ограничения.
- [docs/open-questions.md](docs/open-questions.md) - открытые вопросы и принятые концептуальные решения.
- [docs/profile-format.md](docs/profile-format.md) - справочник полей конфига (service/client/notify/nat).
- [docs/multi-profile-render.md](docs/multi-profile-render.md) - схема multi-profile render и data-driven poller.
- [docs/admin-app.md](docs/admin-app.md) - модель админ-приложения, multi-router и раздача клиентам (invite-blob).
- [docs/roadmap.md](docs/roadmap.md) - план дальнейшей работы.
- [client/README.md](client/README.md) - CLI, provisioning и deploy по SSH.
- [agent/instructions.md](agent/instructions.md) - инструкции для будущего агента/разработчика.
