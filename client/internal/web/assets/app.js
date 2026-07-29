"use strict";
const TOKEN = window.MKPK_TOKEN;

// ---------- i18n ----------
const I18N = {
  ru: {
    save: "Сохранить", cancel: "Отмена", close: "Закрыть", del: "Удалить", settings: "Настройки",
    add: "Добавить", copy: "Скопировать", copied: "✓ Скопировано", loading: "загрузка…", error: "ошибка",
    "saved_to": "Сохранено: {path}",
    "note.add": "Добавить примечание", "note.title": "Примечание · {kind} {name}", "note.subtitle": "Хранится только в этом конфиге — на роутер не уходит",
    "note.placeholder": "Заметка для себя…", "note.clear": "Очистить", "note.saved": "Примечание сохранено", "note.cleared": "Примечание удалено",
    "note.kind.router": "роутер", "note.kind.service": "сервис", "note.kind.user": "юзер",
    undo: "Отменить", redo: "Вернуть", "toast.undone": "Отменено", "toast.redone": "Возвращено",
    "nav.dashboard": "Обзор", "nav.routers": "Роутеры", "nav.users": "Юзеры",
    "nav.add_router": "+ Добавить роутер", "nav.add_user": "+ Добавить юзера",
    "nav.drift_title": "Есть роутеры с незадеплоенными изменениями", "nav.no_access": "нет доступов",
    "nav.router_settings": "Настройки роутера", "nav.user_edit": "Переименовать/удалить",
    "nav.needs_dot": "Нужен Deploy",
    "theme.dark": "Тёмная тема", "theme.light": "Светлая тема", "lang.switch": "Сменить язык",
    "health.checking": "проверка связи…", "health.unreachable": "недоступен по SSH", "health.reachable": "доступен",
    "clock.skew": "часы разошлись", "clock.skew_tip": "Часы роутера отличаются от локальных на {s}с. Стук привязан ко времени (30-сек bucket) — при таком расхождении токены не совпадут и стук НЕ РАБОТАЕТ. Включите NTP на роутере.",
    "clock.ntp_off": "NTP выключен", "clock.ntp_off_tip": "На роутере выключен NTP-клиент. Сейчас время совпадает, но со временем часы уплывут и стук перестанет работать. Включите NTP: /system ntp client set enabled=yes",
    "clock.ok_tip": "NTP включён, часы синхронизированы (расхождение {s}с). Стук будет работать.",
    "onb.title": "Добавьте первый роутер",
    "onb.body": "Роутер — это ваш MikroTik, который приложение провижинит по SSH. Сервисы живут внутри роутера; юзеры — рядом с роутерами и могут иметь доступ к нескольким сразу.",
    "dash.title": "Обзор",
    "dash.stat.routers": "Роутеры", "dash.stat.services": "Сервисы", "dash.stat.users": "Юзеры", "dash.stat.needs": "Нужен Deploy",
    "dash.no_creds": "{n} без SSH-кредов", "dash.all_creds": "у всех заданы креды",
    "dash.svc_on": "{n} включено в конфиге", "dash.multi": "{n} с мультидоступом",
    "dash.has_diff": "есть расхождения", "dash.check_hint": "проверьте статусы",
    "dash.drift_title": "Требуется Deploy", "dash.drift_body": "{names} — локальный конфиг отличается от того, что на роутере.",
    "dash.check": "Проверить статусы", "dash.add_router": "+ Роутер", "dash.add_user": "+ Юзер",
    "dash.subtitle": "{r} роутер(-ов) · {u} юзер(-ов) · ", "dash.no_users": "Юзеров пока нет.",
    "rstate.clean": "локальный конфиг", "rstate.needs": "● нужен Deploy", "rstate.synced": "✓ синхронизирован",
    "rstate.never": "● не установлен на роутере", "rstate.empty": "нечего деплоить", "rstate.error": "ошибка подключения",
    "dash.row.no_creds": "SSH-креды не заданы · ", "dash.row.svc": "{on}/{total} сервисов", "dash.row.users": "{n} юзер(-ов)",
    "toast.checking": "Проверяю статусы…", "toast.checked": "Статусы обновлены",
    "pill.needs": "Не задеплоено — нужен Deploy", "pill.synced": "✓ Синхронизировано", "pill.empty": "Нечего деплоить", "pill.clean": "Локальный конфиг — Deploy",
    "svc.add": "+ Сервис",
    "svc.explain": "Гейтованные эндпоинты этого роутера: три «стучальных» порта + цель — проброс внутрь (forward) или порт самого роутера (local).",
    "svc.empty": "Сервисов пока нет. Добавьте первый.",
    "svc.target_local": "порт роутера {port}/{proto}",
    "svc.on_title": "Включён в конфиге. Выключить — правила не будут рендериться; применится после Deploy.",
    "svc.off_title": "Выключен в конфиге. Включить — правила появятся на роутере после Deploy.",
    "svc.edit": "Редактировать", "svc.delete": "Удалить",
    "svc.foot": "Тоггл меняет состояние сервиса в конфиге. Выключенный сервис остаётся в списке, но его правила и токены юзеров не рендерятся и не деплоятся. Изменения попадут на роутер после Deploy → Apply.",
    "access.explain": "Кто имеет доступ к этому роутеру. Read-only проекция матрицы: редактирование доступа — на экране юзера.",
    "access.empty": "Ни у кого нет доступа к этому роутеру. Откройте юзера в сайдбаре и включите нужные сервисы.",
    "access.svc_off": "Сервис выключен в конфиге", "access.open_user": "Открыть юзера →",
    "render.download": "Скачать .rsc",
    "render.foot": "Рендер по срезу текущего роутера: включённые сервисы + токен-правила юзеров, у которых есть доступ. PSK юзеров попадают в конфиг роутера — поэтому у каждой пары (юзер × роутер) свой PSK.",
    "deploy.nocreds.title": "SSH-креды не заданы",
    "deploy.nocreds.body": "Реквизиты подключения задаются один раз в настройках роутера — экран деплоя их не спрашивает.",
    "deploy.nocreds.btn": "Открыть настройки роутера",
    "deploy.connection": "Подключение", "deploy.state": "Состояние",
    "deploy.auth_key": "ключ: {path}", "deploy.auth_pw": "пароль", "deploy.pw_fallback": " · пароль-fallback",
    "deploy.uninstall_title": "Uninstall с роутера?",
    "deploy.uninstall_body": "Все правила mkpk-tt будут удалены с роутера. Локальный конфиг не тронут.",
    "deploy.dry_title": "Показать, что будет сделано, без изменений на роутере",
    "deploy.dry_btn": "Dry-Run",
    "deploy.synced_hint": "Уже синхронизировано — включите force, чтобы передеплоить",
    "deploy.force_title": "Применить, даже если hash совпадает",
    "deploy.result_ph": "Результат действия появится здесь. Начните со Status.",
    "deploy.streaming": "Выполняется по SSH — живой лог ниже…",
    "dstate.clean": "локальный конфиг (не проверялся по SSH)", "dstate.needs": "● есть локальные изменения — нужен Apply",
    "dstate.synced": "✓ синхронизировано", "dstate.never": "● на роутере ничего не установлено",
    "dstate.empty": "нечего деплоить", "dstate.error": "ошибка подключения",
    "dres.synced.t": "Синхронизировано", "dres.synced.m": "На роутере hash {h} совпадает с локальным конфигом.",
    "dres.never.t": "На роутере ничего не установлено", "dres.never.m": "mkpk-tt-meta не найдена. Локальный конфиг (hash {h}) ещё не деплоился.",
    "dres.drift.t": "Drift: конфиг отличается", "dres.drift.m": "На роутере hash {h}, локально {l}. Нужен Deploy.",
    "dres.applied.t": "Задеплоено", "dres.applied.m": "Конфиг применён: hash {h}. Локальный конфиг и роутер синхронизированы.",
    "dres.dry.t": "Dry-run завершён", "dres.dry.m": "Показано, что было бы сделано. На роутере ничего не изменено (dry-run).",
    "dres.uninstalled.t": "Защита удалена с роутера", "dres.uninstalled.m": "Порт-нок и все его правила убраны — роутер вернулся в исходное состояние. Локальная настройка цела, кнопка «Deploy» поставит всё обратно.",
    "dres.err.t": "Не удалось подключиться", "dres.ok": "Готово",
    "dres.apply_btn": "Deploy — задеплоить изменения", "dres.settings_link": "Настройки роутера →",
    "dres.show_log": "▶ Показать raw-лог", "dres.hide_log": "▼ Скрыть raw-лог",
    "user.matrix": "Матрица доступа",
    "user.matrix.explain": "Юзер может иметь доступ к нескольким роутерам. PSK — свой на каждый роутер (создаётся при первом доступе); один инвайт может включать несколько роутеров.",
    "user.needs_deploy": "нужен Deploy", "user.needs_deploy_title": "Локальные изменения не задеплоены",
    "user.svc_count": "{n} из {total} сервисов", "user.no_access": "нет доступа",
    "user.psk_own": "свой для этого роутера", "user.psk_active": "PSK задан для этого роутера", "user.psk_none": "Нет доступа — PSK не создан",
    "user.invite_single_title": "Инвайт только с этим роутером",
    "user.rotate_title": "Сгенерировать новый PSK для этой пары юзер×роутер", "user.rotate": "⟳ Ротировать",
    "user.svc_off": "· выключен в конфиге", "user.no_services": "у роутера нет сервисов",
    "user.matrix.foot": "Изменения доступа и ротация PSK попадают на роутер после Deploy этого роутера, а юзеру — через пере-выданный инвайт.",
    "user.grant": "Выдать доступ",
    "user.grant.explain": "Invite-blob для клиентского приложения — передавать только по безопасному каналу.",
    "user.grant.none": "Сначала включите юзеру хотя бы один сервис в матрице выше.",
    "user.grant.all": "Общий инвайт — все роутеры ({n})", "user.grant.one": "Выдать инвайт",
    "user.header_id": "client_id: {id} · единый во всех роутерах",
    "svc.del.title": "Удалить сервис {name}?", "svc.del.body": "Правило будет удалено с роутера после следующего Deploy.",
    "toast.svc_deleted": "Сервис удалён", "toast.psk_rotated": "PSK ротирован",
    "psk.rotate.title": "Ротировать PSK?", "psk.rotate.body": "Будет сгенерирован новый PSK этой пары юзер×роутер. Старый инвайт перестанет работать после Deploy роутера — выдайте новый.",
    "psk.rotate.btn": "Ротировать",
    "router.new": "Новый роутер", "field.name": "Имя", "field.address": "Публичный адрес (стук клиента)", "field.port": "Порт", "field.user": "Пользователь",
    "router.address_note": "Домен/IP, по которому конечный юзер стучит из недоверенной сети. Именно он попадает в инвайт.",
    "router.allowed_timeout": "Таймаут доступа по умолчанию",
    "router.allowed_timeout_note": "На сколько открывается порт после стука (TTL записи в allowed-list). По умолчанию 3m. Можно переопределить на конкретном сервисе.",
    "router.ssh_address": "SSH-адрес деплоя (если отличается)",
    "router.ssh_address_note": "Пусто → деплой идёт по публичному адресу. Задайте, если провижн по локальному/management-адресу из safe-env.",
    "router.tab_general": "Общие", "router.tab_notify": "Нотификации",
    "router.addr_required": "публичный адрес обязателен", "router.addr_bad": "адрес {addr} — не IP и не домен", "router.ssh_addr_bad": "SSH-адрес {addr} — не IP и не домен",
    "router.ssh_legend": "SSH для деплоя",
    "router.ssh_note": "Используется кнопками Status / Apply / Uninstall. Хранится в локальном секретном конфиге (0600) и не покидает эту машину.",
    "router.auth": "Аутентификация", "router.auth_note": "рекомендуется ssh-agent: секрет не попадает в конфиг", "router.auth_keyfile": "файл ключа",
    "router.keypath": "Путь к приватному ключу", "router.keypath_note": "В конфиге хранится только путь, не сам ключ.",
    "router.pw_collapse": "Пароль (fallback)", "router.pw_ssh": "Пароль SSH (fallback)",
    "router.keypass": "Пассфраза ключа", "router.keypass_note": "Опционально. Лежит в секретном конфиге вместе с PSK.",
    "router.notify_legend": "Уведомления (per router)",
    "router.notify_note": "Срабатывают при успешном открытии любого сервиса этого роутера. Можно включить несколько каналов сразу; секреты на edit оставь пустыми, чтобы не менять.",
    "ph.unchanged": "не менять", "toast.router_saved": "Роутер сохранён",
    "router.del": "Удалить роутер…", "router.del.title": "Удалить роутер {name}?",
    "router.del.body": "Вместе с ним удалятся {s} сервис(-ов) и доступ у {u} юзер(-ов). Сначала сделайте Uninstall, если mkpk стоит на роутере.",
    "toast.router_deleted": "Роутер удалён",
    "svc.new": "Новый сервис", "svc.title": "Сервис {name}",
    "svc.field_name": "Имя сервиса", "svc.field_name_note": "Входит в формулу токена — переименование инвалидирует выданные инвайты.",
    "svc.allowed_timeout": "Таймаут доступа", "svc.allowed_timeout_note": "Сколько адрес держится в allowed после стука (напр. 10m, 1h). Пусто = дефолт роутера ({def}).",
    "svc.name_bad": "имя: только A–Z a–z 0–9 _ -, начинается с буквы/цифры, до 32 символов",
    "svc.knock_ports": "Порты «стука» (stage1 / stage2 / token)", "svc.suggest": "Подобрать свободные",
    "svc.type": "Тип цели", "svc.proto": "Протокол", "svc.port_ext": "Внешний порт", "svc.port_local": "Порт роутера",
    "svc.port_local_note": "input accept на этот порт роутера, без NAT.",
    "svc.conflict": "{label}: {port} занят — {svc} ({field})", "svc.port_range": "{label}: {port} вне диапазона (1–65535)", "svc.required": "заполните: {fields}", "svc.ipv4": "to_address: {addr} — не IPv4-адрес", "toast.svc_saved": "Сервис сохранён",
    "user.new": "Новый юзер", "user.title": "Юзер {name}", "user.field_name": "Имя (client_id)",
    "user.field_name_note": "Единая идентичность во всех роутерах; входит в формулу токена — переименование инвалидирует инвайты.",
    "toast.user_saved": "Юзер сохранён", "user.del": "Удалить юзера…", "user.del.title": "Удалить юзера {name}?",
    "user.del.body": "Юзер и весь его доступ на всех роутерах будут удалены. Изменения попадут на роутеры после Deploy.",
    "toast.user_deleted": "Юзер удалён",
    "invite.warn": "Блоб содержит PSK юзера для каждого включённого роутера. Передавайте только по безопасному каналу.",
    "invite.mode": "Что войдёт в блоб", "invite.mode_all": "Все роутеры ({n})", "invite.mode_single": "Только один роутер",
    "invite.included": "Роутеры в блобе", "invite.no_services": "— нет включённых сервисов",
    "invite.download": "Скачать .mkpk", "invite.reveal": "Показать блоб", "invite.title": "Инвайт — {user}",
  },
  en: {
    save: "Save", cancel: "Cancel", close: "Close", del: "Delete", settings: "Settings",
    "note.add": "Add note", "note.title": "Note · {kind} {name}", "note.subtitle": "Stored in this config only — never sent to the router",
    "note.placeholder": "A note to self…", "note.clear": "Clear", "note.saved": "Note saved", "note.cleared": "Note removed",
    "note.kind.router": "router", "note.kind.service": "service", "note.kind.user": "user",
    add: "Add", copy: "Copy", copied: "✓ Copied", loading: "loading…", error: "error",
    "saved_to": "Saved: {path}",
    undo: "Undo", redo: "Redo", "toast.undone": "Undone", "toast.redone": "Redone",
    "nav.dashboard": "Overview", "nav.routers": "Routers", "nav.users": "Users",
    "nav.add_router": "+ Add router", "nav.add_user": "+ Add user",
    "nav.drift_title": "Routers with undeployed changes", "nav.no_access": "no access",
    "nav.router_settings": "Router settings", "nav.user_edit": "Rename/delete",
    "nav.needs_dot": "Deploy needed",
    "theme.dark": "Dark theme", "theme.light": "Light theme", "lang.switch": "Switch language",
    "health.checking": "checking…", "health.unreachable": "unreachable via SSH", "health.reachable": "reachable",
    "clock.skew": "clock drift", "clock.skew_tip": "The router clock differs from local by {s}s. Knocking is time-based (30s bucket) — at this drift tokens won't match and knocking DOES NOT WORK. Enable NTP on the router.",
    "clock.ntp_off": "NTP off", "clock.ntp_off_tip": "The router's NTP client is off. The clock matches now, but it will drift and knocking will stop working. Enable NTP: /system ntp client set enabled=yes",
    "clock.ok_tip": "NTP on, clock synced (skew {s}s). Knocking will work.",
    "onb.title": "Add your first router",
    "onb.body": "A router is your MikroTik, provisioned by this app over SSH. Services live inside a router; users sit alongside routers and can have access to several at once.",
    "dash.title": "Overview",
    "dash.stat.routers": "Routers", "dash.stat.services": "Services", "dash.stat.users": "Users", "dash.stat.needs": "Deploy needed",
    "dash.no_creds": "{n} without SSH creds", "dash.all_creds": "all have creds",
    "dash.svc_on": "{n} enabled in config", "dash.multi": "{n} with multi-router access",
    "dash.has_diff": "has differences", "dash.check_hint": "check statuses",
    "dash.drift_title": "Deploy required", "dash.drift_body": "{names} — local config differs from what's on the router.",
    "dash.check": "Check statuses", "dash.add_router": "+ Router", "dash.add_user": "+ User",
    "dash.subtitle": "{r} router(s) · {u} user(s) · ", "dash.no_users": "No users yet.",
    "rstate.clean": "local config", "rstate.needs": "● deploy needed", "rstate.synced": "✓ synced",
    "rstate.never": "● not installed on router", "rstate.empty": "nothing to deploy", "rstate.error": "connection error",
    "dash.row.no_creds": "SSH creds not set · ", "dash.row.svc": "{on}/{total} services", "dash.row.users": "{n} user(s)",
    "toast.checking": "Checking statuses…", "toast.checked": "Statuses updated",
    "pill.needs": "Not deployed — deploy needed", "pill.synced": "✓ Synced", "pill.empty": "Nothing to deploy", "pill.clean": "Local config — Deploy",
    "svc.add": "+ Service",
    "svc.explain": "This router's gated endpoints: three knock ports + a target — forward into the LAN (dst-nat) or a port on the router itself (local).",
    "svc.empty": "No services yet. Add the first one.",
    "svc.target_local": "router port {port}/{proto}",
    "svc.on_title": "Enabled in config. Disabling stops its rules from rendering; applies after Deploy.",
    "svc.off_title": "Disabled in config. Enabling makes its rules appear on the router after Deploy.",
    "svc.edit": "Edit", "svc.delete": "Delete",
    "svc.foot": "The toggle changes the service state in the config. A disabled service stays listed, but its rules and users' tokens are not rendered or deployed. Changes reach the router after Deploy → Apply.",
    "access.explain": "Who can reach this router. A read-only projection of the matrix — edit access on the user screen.",
    "access.empty": "Nobody has access to this router. Open a user in the sidebar and enable the services.",
    "access.svc_off": "Service disabled in config", "access.open_user": "Open user →",
    "render.download": "Download .rsc",
    "render.foot": "Render of the current router: enabled services + token rules of users who have access. Users' PSKs go into the router config — so each (user × router) pair has its own PSK.",
    "deploy.nocreds.title": "SSH creds not set",
    "deploy.nocreds.body": "Connection details are set once in the router settings — the deploy screen doesn't ask for them.",
    "deploy.nocreds.btn": "Open router settings",
    "deploy.connection": "Connection", "deploy.state": "State",
    "deploy.auth_key": "key: {path}", "deploy.auth_pw": "password", "deploy.pw_fallback": " · password fallback",
    "deploy.uninstall_title": "Uninstall from the router?",
    "deploy.uninstall_body": "All mkpk-tt rules will be removed from the router. The local config is untouched.",
    "deploy.dry_title": "Show what would be done, without changing the router",
    "deploy.dry_btn": "Dry-Run",
    "deploy.synced_hint": "Already synced — enable force to redeploy",
    "deploy.force_title": "Apply even if the hash matches",
    "deploy.result_ph": "The result will appear here. Start with Status.",
    "deploy.streaming": "Running over SSH — live log below…",
    "dstate.clean": "local config (not SSH-checked)", "dstate.needs": "● local changes — Apply needed",
    "dstate.synced": "✓ synced", "dstate.never": "● nothing installed on the router",
    "dstate.empty": "nothing to deploy", "dstate.error": "connection error",
    "dres.synced.t": "Synced", "dres.synced.m": "Router hash {h} matches the local config.",
    "dres.never.t": "Nothing installed on the router", "dres.never.m": "mkpk-tt-meta not found. The local config (hash {h}) hasn't been deployed yet.",
    "dres.drift.t": "Drift: config differs", "dres.drift.m": "Router hash {h}, local {l}. Deploy needed.",
    "dres.applied.t": "Deployed", "dres.applied.m": "Config applied: hash {h}. Local config and router are in sync.",
    "dres.dry.t": "Dry-run done", "dres.dry.m": "Shows what would be done. Nothing changed on the router (dry-run).",
    "dres.uninstalled.t": "Protection removed from the router", "dres.uninstalled.m": "Port-knocking and all its rules are gone — the router is back to its original state. Your local setup is intact; the Deploy button puts it all back.",
    "dres.err.t": "Connection failed", "dres.ok": "Done",
    "dres.apply_btn": "Deploy — deploy changes", "dres.settings_link": "Router settings →",
    "dres.show_log": "▶ Show raw log", "dres.hide_log": "▼ Hide raw log",
    "user.matrix": "Access matrix",
    "user.matrix.explain": "A user can have access to several routers. The PSK is per router (created on first access); one invite can include several routers.",
    "user.needs_deploy": "deploy needed", "user.needs_deploy_title": "Local changes not deployed",
    "user.svc_count": "{n} of {total} services", "user.no_access": "no access",
    "user.psk_own": "unique for this router", "user.psk_active": "PSK set for this router", "user.psk_none": "No access — PSK not created",
    "user.invite_single_title": "Invite with this router only",
    "user.rotate_title": "Generate a new PSK for this user×router pair", "user.rotate": "⟳ Rotate",
    "user.svc_off": "· disabled in config", "user.no_services": "router has no services",
    "user.matrix.foot": "Access changes and PSK rotation reach the router after its Deploy, and the user via a re-issued invite.",
    "user.grant": "Grant access",
    "user.grant.explain": "Invite blob for the client app — hand over only through a safe channel.",
    "user.grant.none": "First enable at least one service for the user in the matrix above.",
    "user.grant.all": "Combined invite — all routers ({n})", "user.grant.one": "Issue invite",
    "user.header_id": "client_id: {id} · shared across routers",
    "svc.del.title": "Delete service {name}?", "svc.del.body": "The rule will be removed from the router on the next Deploy.",
    "toast.svc_deleted": "Service deleted", "toast.psk_rotated": "PSK rotated",
    "psk.rotate.title": "Rotate PSK?", "psk.rotate.body": "A new PSK will be generated for this user×router pair. The old invite stops working after the router's Deploy — issue a new one.",
    "psk.rotate.btn": "Rotate",
    "router.new": "New router", "field.name": "Name", "field.address": "Public address (client knock)", "field.port": "Port", "field.user": "User",
    "router.address_note": "The domain/IP end users knock from an untrusted network. This is what goes into the invite.",
    "router.allowed_timeout": "Default access timeout",
    "router.allowed_timeout_note": "How long the port stays open after a knock (allowed-list entry TTL). Defaults to 3m. Can be overridden per service.",
    "router.ssh_address": "SSH deploy address (if different)",
    "router.ssh_address_note": "Empty → deploy uses the public address. Set it if you provision over a local/management address from safe-env.",
    "router.tab_general": "General", "router.tab_notify": "Notifications",
    "router.addr_required": "public address is required", "router.addr_bad": "address {addr} is not an IP or hostname", "router.ssh_addr_bad": "SSH address {addr} is not an IP or hostname",
    "router.ssh_legend": "SSH for deploy",
    "router.ssh_note": "Used by Status / Apply / Uninstall. Stored in the local secret config (0600) and never leaves this machine.",
    "router.auth": "Authentication", "router.auth_note": "ssh-agent recommended: the secret stays out of the config", "router.auth_keyfile": "key file",
    "router.keypath": "Private key path", "router.keypath_note": "Only the path is stored in the config, not the key itself.",
    "router.pw_collapse": "Password (fallback)", "router.pw_ssh": "SSH password (fallback)",
    "router.keypass": "Key passphrase", "router.keypass_note": "Optional. Stored in the secret config together with the PSKs.",
    "router.notify_legend": "Notifications (per router)",
    "router.notify_note": "Fire on a successful open of any service on this router. Several channels can be on at once; leave secrets blank on edit to keep them.",
    "ph.unchanged": "unchanged", "toast.router_saved": "Router saved",
    "router.del": "Delete router…", "router.del.title": "Delete router {name}?",
    "router.del.body": "This also deletes {s} service(s) and access for {u} user(s). Uninstall first if mkpk is on the router.",
    "toast.router_deleted": "Router deleted",
    "svc.new": "New service", "svc.title": "Service {name}",
    "svc.field_name": "Service name", "svc.field_name_note": "Part of the token formula — renaming invalidates issued invites.",
    "svc.allowed_timeout": "Access timeout", "svc.allowed_timeout_note": "How long the address stays allowed after a knock (e.g. 10m, 1h). Empty = router default ({def}).",
    "svc.name_bad": "name: only A–Z a–z 0–9 _ -, starts with a letter/digit, up to 32 chars",
    "svc.knock_ports": "Knock ports (stage1 / stage2 / token)", "svc.suggest": "Suggest free",
    "svc.type": "Target type", "svc.proto": "Protocol", "svc.port_ext": "External port", "svc.port_local": "Router port",
    "svc.port_local_note": "input accept to this router port, no NAT.",
    "svc.conflict": "{label}: {port} taken — {svc} ({field})", "svc.port_range": "{label}: {port} out of range (1–65535)", "svc.required": "fill in: {fields}", "svc.ipv4": "to_address: {addr} — not an IPv4 address", "toast.svc_saved": "Service saved",
    "user.new": "New user", "user.title": "User {name}", "user.field_name": "Name (client_id)",
    "user.field_name_note": "A single identity across all routers; part of the token formula — renaming invalidates invites.",
    "toast.user_saved": "User saved", "user.del": "Delete user…", "user.del.title": "Delete user {name}?",
    "user.del.body": "The user and all its access on every router will be deleted. Changes reach the routers after Deploy.",
    "toast.user_deleted": "User deleted",
    "invite.warn": "The blob contains the user's PSK for each included router. Hand it over only through a safe channel.",
    "invite.mode": "What goes into the blob", "invite.mode_all": "All routers ({n})", "invite.mode_single": "One router only",
    "invite.included": "Routers in the blob", "invite.no_services": "— no enabled services",
    "invite.download": "Download .mkpk", "invite.reveal": "Show blob", "invite.title": "Invite — {user}",
  },
};
let LANG = localStorage.getItem("mkpk-lang") || ((navigator.language || "").toLowerCase().startsWith("ru") ? "ru" : "en");
function t(key, p) {
  let s = (I18N[LANG] && I18N[LANG][key]) ?? I18N.ru[key] ?? key;
  if (p) for (const k in p) s = s.replaceAll("{" + k + "}", p[k]);
  return s;
}
function toggleLang() { LANG = LANG === "ru" ? "en" : "ru"; localStorage.setItem("mkpk-lang", LANG); render(); }

// ---------- tiny helpers ----------
function h(tag, props, ...kids) {
  const e = document.createElement(tag);
  if (props) for (const k in props) {
    const v = props[k];
    if (v == null || v === false) continue;
    if (k === "class") e.className = v;
    else if (k === "html") e.innerHTML = v;
    else if (k === "style") e.setAttribute("style", v);
    else if (k.startsWith("on")) e.addEventListener(k.slice(2).toLowerCase(), v);
    else e.setAttribute(k, v === true ? "" : v);
  }
  for (const kid of kids.flat()) {
    if (kid == null || kid === false) continue;
    e.append(kid.nodeType ? kid : document.createTextNode(kid));
  }
  return e;
}
const ICONS = {
  logo: '<circle cx="7" cy="7" r="2.3"/><circle cx="17" cy="7" r="2.3"/><circle cx="12" cy="16" r="2.3"/>',
  dash: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
  router: '<rect x="3" y="13" width="18" height="7" rx="2"/><path d="M7 17h.01M11 17h.01"/><path d="M12 13V8m0 0 3 2m-3-2-3 2"/>',
  gear: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.6 1.6 0 0 0 .32 1.77l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.6 1.6 0 0 0-1.77-.32 1.6 1.6 0 0 0-.97 1.47V21a2 2 0 0 1-4 0v-.08a1.6 1.6 0 0 0-1.05-1.47 1.6 1.6 0 0 0-1.77.32l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.6 1.6 0 0 0 .32-1.77 1.6 1.6 0 0 0-1.47-.97H3a2 2 0 0 1 0-4h.08a1.6 1.6 0 0 0 1.47-1.05 1.6 1.6 0 0 0-.32-1.77l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.6 1.6 0 0 0 1.77.32H9a1.6 1.6 0 0 0 .97-1.47V3a2 2 0 0 1 4 0v.08a1.6 1.6 0 0 0 .97 1.47 1.6 1.6 0 0 0 1.77-.32l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.6 1.6 0 0 0-.32 1.77V9a1.6 1.6 0 0 0 1.47.97H21a2 2 0 0 1 0 4h-.08a1.6 1.6 0 0 0-1.47.97z"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M4 12H2m20 0h-2M5 5l1.5 1.5m11 11L19 19M19 5l-1.5 1.5M6.5 17.5 5 19"/>',
  moon: '<path d="M21 12.8A8.5 8.5 0 1 1 11.2 3a6.6 6.6 0 0 0 9.8 9.8z"/>',
  refresh: '<path d="M21 12a9 9 0 1 1-2.64-6.36M21 3v4h-4"/>',
  undo: '<path d="M9 14 4 9l5-5"/><path d="M4 9h11a5 5 0 0 1 0 10H9"/>',
  redo: '<path d="m15 14 5-5-5-5"/><path d="M20 9H9a5 5 0 0 0 0 10h6"/>',
  pencil: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
  trash: '<path d="M3 6h18M8 6V4h8v2m-9 0 1 14h8l1-14"/>',
  lock: '<rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',
  warn: '<path d="M12 3 2 20h20L12 3Z"/><path d="M12 10v4m0 3h.01"/>',
  user: '<circle cx="12" cy="8" r="3.5"/><path d="M5 20a7 7 0 0 1 14 0"/>',
  note: '<path d="M21 15a2 2 0 0 1-2 2H8l-5 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/><path d="M8 9h8M8 13h5"/>',
};
function icon(name, cls) {
  const s = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  s.setAttribute("viewBox", "0 0 24 24");
  s.setAttribute("width", "16"); s.setAttribute("height", "16");
  s.setAttribute("fill", "none"); s.setAttribute("stroke", "currentColor");
  s.setAttribute("stroke-width", "1.7"); s.setAttribute("stroke-linecap", "round"); s.setAttribute("stroke-linejoin", "round");
  if (cls) s.setAttribute("class", cls);
  s.innerHTML = ICONS[name] || "";
  return s;
}
// ---------- theme ----------
let THEME = localStorage.getItem("mkpk-theme") || "light";
function applyTheme() { document.documentElement.setAttribute("data-theme", THEME); }
function toggleTheme() { THEME = THEME === "light" ? "dark" : "light"; localStorage.setItem("mkpk-theme", THEME); applyTheme(); renderSidebar(); }
applyTheme();

const initials = (s) => (s || "?").slice(0, 2).toUpperCase();
const short = (hash) => (hash || "").slice(0, 12);

let toastTimer;
function toast(msg, isErr) {
  const el = document.getElementById("toast");
  el.textContent = msg;
  el.className = isErr ? "err" : "ok";
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => el.classList.add("hidden"), 3500);
}

async function api(method, path, body) {
  const opts = { method, headers: { "X-MKPK-Token": TOKEN } };
  if (body !== undefined) { opts.headers["Content-Type"] = "application/json"; opts.body = JSON.stringify(body); }
  const res = await fetch(path, opts);
  const ct = res.headers.get("content-type") || "";
  if (ct.includes("application/json")) {
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || res.statusText);
    return data;
  }
  const text = await res.text();
  if (!res.ok) throw new Error(text || res.statusText);
  return text;
}

// ---------- state ----------
const S = {
  path: "", routers: [], users: [],
  view: { kind: "dashboard", id: null, tab: "services" },
  deploy: {},
  deployOpts: { force: false },
  deployRunning: null,
  canUndo: false, canRedo: false,
  health: {}, // router -> {reachable, identity, version, uptime, board, err}
};
const routerOf = (n) => S.routers.find((r) => r.name === n);
const userOf = (n) => S.users.find((u) => u.name === n);

function routerState(r) {
  const d = S.deploy[r.name] || {};
  if (d.err) return "error";
  if (d.installed === false) return r.services.length ? "never" : "empty";
  if (d.baseline !== undefined && d.baseline !== r.hash) return "needs";
  if (d.checked && d.installed) return "synced";
  return "clean";
}
const isDrift = (r) => routerState(r) === "needs" || routerState(r) === "never";

async function applyConfig(data) {
  S.path = data.path;
  const sum = data.summary || { routers: [], users: [] };
  S.routers = (sum.routers || []).map((r) => ({ ...r, services: r.services || [], clients: r.clients || [] }));
  S.users = (sum.users || []).map((u) => ({ ...u, access: (u.access || []).map((a) => ({ ...a, services: a.services || [] })) }));
  S.canUndo = !!data.can_undo; S.canRedo = !!data.can_redo;
  for (const r of S.routers) {
    const d = S.deploy[r.name] || (S.deploy[r.name] = {});
    if (d.baseline === undefined) d.baseline = r.hash;
  }
  if (S.view.kind === "router" && !routerOf(S.view.id)) S.view = { kind: "dashboard" };
  if (S.view.kind === "user" && !userOf(S.view.id)) S.view = { kind: "dashboard" };
  if (!S.routers.length) S.view = { kind: "onboarding" };
  render();
}
async function reload() { applyConfig(await api("GET", "/api/config")); }
function go(view) { S.view = { tab: "services", ...view }; render(); }

async function undo() {
  if (!S.canUndo) return;
  try { applyConfig(await api("POST", "/api/undo")); toast(t("toast.undone")); } catch (e) { toast(e.message, true); }
}
async function redo() {
  if (!S.canRedo) return;
  try { applyConfig(await api("POST", "/api/redo")); toast(t("toast.redone")); } catch (e) { toast(e.message, true); }
}
// Ctrl/Cmd+Z undo, Ctrl/Cmd+Shift+Z redo — but never while typing in a field or
// with a modal open (let the browser's native input undo work there).
document.addEventListener("keydown", (e) => {
  if (!(e.metaKey || e.ctrlKey) || e.key.toLowerCase() !== "z") return;
  const el = document.activeElement;
  if (el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT")) return;
  if (document.querySelector(".overlay")) return;
  e.preventDefault();
  e.shiftKey ? redo() : undo();
});

// ---------- router health polling ----------
// Probe each creds-configured router over SSH: device info + install state. The
// same probe feeds the deploy state (installed hash), so drift is known live.
async function pollInfo(name) {
  try {
    const d = await api("GET", "/api/router/info?router=" + encodeURIComponent(name));
    S.health[name] = { reachable: d.reachable, identity: d.identity, version: d.version, uptime: d.uptime, board: d.board, err: d.error,
      clock_checked: d.clock_checked, clock_ok: d.clock_ok, clock_skew_seconds: d.clock_skew_seconds, ntp_enabled: d.ntp_enabled, ntp_status: d.ntp_status };
    if (d.reachable) {
      const rec = S.deploy[name] || (S.deploy[name] = {});
      rec.checked = true; rec.err = null;
      rec.installed = d.installed;
      rec.baseline = d.installed ? d.installed_hash : "";
    }
  } catch (e) { S.health[name] = { reachable: false, err: e.message }; }
}
let pollTimer;
async function pollAll() {
  const targets = S.routers.filter((r) => r.deploy.configured);
  if (!targets.length) return;
  await Promise.all(targets.map((r) => pollInfo(r.name)));
  render();
}
function startPolling() {
  clearInterval(pollTimer);
  pollAll();
  pollTimer = setInterval(pollAll, 20000);
}
function shortVersion(v) { return (v || "").split(" ")[0]; }
// isSafeName — mirrors the backend: ^[A-Za-z0-9][A-Za-z0-9_-]*$, max 32 chars.
// Names compose into RouterOS object names, so no spaces / unicode / punctuation.
const MAX_NAME_LEN = 32;
function isSafeName(s) { return /^[A-Za-z0-9][A-Za-z0-9_-]*$/.test(s) && s.length <= MAX_NAME_LEN; }
// isIPv4 — strict dotted-quad, each octet 0..255, no leading zeros beyond "0".
function isIPv4(s) {
  const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(s);
  if (!m) return false;
  return m.slice(1).every((o) => { const n = +o; return n >= 0 && n <= 255 && String(n) === o; });
}
// isHostname — DNS name: dot-separated labels of [A-Za-z0-9-], 1..63 each, not
// starting/ending with a hyphen; total ≤ 253. isHostOrIP mirrors the backend.
function isHostname(s) {
  if (!s || s.length > 253) return false;
  return s.split(".").every((l) => /^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$/.test(l));
}
function isHostOrIP(s) { return isIPv4(s) || isHostname(s); }
// hostInput restricts a field to host/IP characters ([A-Za-z0-9.-]) as typed.
function hostInput(input) {
  input.setAttribute("maxlength", 253);
  input.addEventListener("input", () => {
    const cleaned = input.value.replace(/[^A-Za-z0-9.-]/g, "");
    if (cleaned === input.value) return;
    const caret = (input.selectionStart || cleaned.length) - (input.value.length - cleaned.length);
    input.value = cleaned;
    try { input.setSelectionRange(caret, caret); } catch (e) { /* detached */ }
  });
  return input;
}
// small live status dot for a router (sidebar / lists)
function routerDot(r) {
  if (isDrift(r)) return h("span", { class: "dot amber", "data-tip": t("nav.needs_dot") });
  const hv = S.health[r.name];
  if (hv && hv.reachable) return h("span", { class: "dot green pulse", "data-tip": t("health.reachable") });
  return null;
}
// rich health marker for the router header
function healthMarker(r) {
  if (!r.deploy.configured) return null;
  const hv = S.health[r.name];
  if (!hv) return h("span", { class: "row", style: "gap:6px" }, h("span", { class: "dot grey" }), h("span", { class: "foot-note" }, t("health.checking")));
  if (!hv.reachable) return h("span", { class: "row", style: "gap:6px", "data-tip": hv.err || "" }, h("span", { class: "dot grey" }), h("span", { class: "foot-note" }, t("health.unreachable")));
  return h("span", { class: "row", style: "gap:6px", "data-tip": hv.board || "" },
    h("span", { class: "dot green pulse" }),
    h("span", { class: "mono foot-note" }, [hv.identity, shortVersion(hv.version), hv.uptime].filter(Boolean).join(" · ")),
    clockWarn(hv));
}
// clockWarn surfaces a router clock problem that silently breaks knocking:
// tokens are time-bucketed, so a drifted clock (or NTP off) means knocks never
// match. Hard warning when the skew is out of tolerance; softer one for NTP off.
function clockWarn(hv) {
  if (!hv || !hv.reachable || !hv.clock_checked) return null;
  if (!hv.clock_ok) {
    return h("span", { class: "pill amber", "data-tip": t("clock.skew_tip", { s: hv.clock_skew_seconds }) }, icon("warn", "ic-sm"), t("clock.skew"));
  }
  if (!hv.ntp_enabled) {
    return h("span", { class: "pill amber", "data-tip": t("clock.ntp_off_tip") }, icon("warn", "ic-sm"), t("clock.ntp_off"));
  }
  // All good — a positive confirmation, since a silent clock failure is the worst case.
  return h("span", { class: "pill green", "data-tip": t("clock.ok_tip", { s: hv.clock_skew_seconds }) }, "NTP ✓");
}
// clockWarnDot is the compact (icon-only) form of clockWarn for tight spots —
// the sidebar router row and the dashboard list — so a clock problem (which
// silently kills knocking) is visible without opening the router.
function clockWarnDot(r) {
  const hv = S.health[r.name];
  if (!hv || !hv.reachable || !hv.clock_checked) return null;
  if (!hv.clock_ok) return h("span", { class: "clock-warn", "data-tip": t("clock.skew_tip", { s: hv.clock_skew_seconds }) }, icon("warn", "ic-sm"));
  if (!hv.ntp_enabled) return h("span", { class: "clock-warn soft", "data-tip": t("clock.ntp_off_tip") }, icon("warn", "ic-sm"));
  return null;
}

// ---------- render root ----------
function render() { hideTip(); renderSidebar(); renderMain(); }

function renderSidebar() {
  const sb = document.getElementById("sidebar");
  sb.innerHTML = "";
  sb.append(h("div", { class: "brand" },
    h("span", { class: "logo" }, h("img", { src: "/logo-96.png", alt: "mkpk", width: 48, height: 48 })),
    h("div", null, h("h1", null, "mkpk-provision"), h("div", { class: "sub" }, "provisioning console"))));
  const nav = h("div", { class: "nav" });

  const dashRow = h("button", { class: "nav-row" + (S.view.kind === "dashboard" ? " sel" : ""), onclick: () => go({ kind: "dashboard" }) },
    icon("dash"), h("span", { class: "grow title" }, t("nav.dashboard")));
  const driftCount = S.routers.filter(isDrift).length;
  if (driftCount) dashRow.append(h("span", { class: "badge amber", "data-tip": t("nav.drift_title") }, String(driftCount)));
  nav.append(dashRow);

  nav.append(navHeader(t("nav.routers"), () => openRouterModal(null)));
  for (const r of S.routers) {
    nav.append(h("button", { class: "nav-row" + (S.view.kind === "router" && S.view.id === r.name ? " sel" : ""), onclick: () => go({ kind: "router", id: r.name }) },
      icon("router"),
      h("span", { class: "grow" }, h("div", { class: "title" }, r.name), h("div", { class: "meta" }, r.address)),
      noteBadge("router", null, r.name, r.note),
      clockWarnDot(r),
      routerDot(r),
      h("span", { class: "gear", onclick: (e) => { e.stopPropagation(); openRouterModal(r.name); }, "data-tip": t("nav.router_settings") }, icon("gear"))));
  }
  nav.append(h("button", { class: "dashed", onclick: () => openRouterModal(null) }, t("nav.add_router")));

  nav.append(navHeader(t("nav.users"), () => openUserModal(null)));
  for (const u of S.users) {
    const summary = u.access.length ? u.access.map((a) => a.router).join(", ") : t("nav.no_access");
    nav.append(h("button", { class: "nav-row" + (S.view.kind === "user" && S.view.id === u.name ? " sel" : ""), onclick: () => go({ kind: "user", id: u.name }) },
      h("span", { class: "avatar" }, initials(u.name)),
      h("span", { class: "grow" }, h("div", { class: "title" }, u.name), h("div", { class: "meta" }, summary)),
      noteBadge("user", null, u.name, u.note),
      h("span", { class: "gear", onclick: (e) => { e.stopPropagation(); openUserModal(u.name); }, "data-tip": t("nav.user_edit") }, icon("pencil"))));
  }
  nav.append(h("button", { class: "dashed", onclick: () => openUserModal(null) }, t("nav.add_user")));

  const foot = h("div", { class: "sidebar-foot row" },
    h("span", { class: "grow" }, (window.MKPK_VERSION && window.MKPK_VERSION !== "__MKPK_" + "VERSION__") ? h("span", { class: "ver" }, window.MKPK_VERSION) : null),
    h("button", { class: "iconbtn", style: "width:auto;padding:0 6px;font-weight:650;font-size:11px", "data-tip": t("lang.switch"), onclick: toggleLang }, LANG.toUpperCase()),
    h("button", { class: "iconbtn", "data-tip": THEME === "light" ? t("theme.dark") : t("theme.light"), onclick: toggleTheme }, icon(THEME === "light" ? "moon" : "sun")));
  sb.append(nav, foot);
}

// undo/redo button group shown in the top bar of every view
function histButtons() {
  return h("div", { class: "row", style: "gap:4px" },
    h("button", { class: "iconbtn", disabled: !S.canUndo, "data-tip": t("undo") + " (⌘Z)", onclick: undo }, icon("undo")),
    h("button", { class: "iconbtn", disabled: !S.canRedo, "data-tip": t("redo") + " (⇧⌘Z)", onclick: redo }, icon("redo")));
}
function navHeader(label, onAdd) {
  return h("div", { class: "nav-eyebrow" }, h("span", null, label), h("button", { onclick: onAdd, "data-tip": t("add") }, "+"));
}

function renderMain() {
  const m = document.getElementById("main");
  m.innerHTML = "";
  const k = S.view.kind;
  if (k === "onboarding" || !S.routers.length) return m.append(onboarding());
  if (k === "user") return m.append(userView(userOf(S.view.id)));
  if (k === "router") return m.append(routerView(routerOf(S.view.id)));
  return m.append(dashboard());
}

// ---------- onboarding ----------
function onboarding() {
  return h("div", { class: "content" },
    h("div", { class: "empty" },
      icon("router", "glyph"),
      h("h3", null, t("onb.title")),
      h("p", null, t("onb.body")),
      h("button", { class: "btn pri", onclick: () => openRouterModal(null) }, t("nav.add_router"))));
}

// ---------- dashboard ----------
function dashboard() {
  const svcTotal = S.routers.reduce((n, r) => n + r.services.length, 0);
  const svcOn = S.routers.reduce((n, r) => n + r.services.filter((s) => s.enabled).length, 0);
  const noCreds = S.routers.filter((r) => !r.deploy.configured).length;
  const multi = S.users.filter((u) => u.access.length > 1).length;
  const drifters = S.routers.filter(isDrift);

  const wrap = h("div", { class: "wrap wide" });
  wrap.append(h("div", { class: "stat-grid" },
    stat(t("dash.stat.routers"), S.routers.length, noCreds ? t("dash.no_creds", { n: noCreds }) : t("dash.all_creds")),
    stat(t("dash.stat.services"), svcTotal, t("dash.svc_on", { n: svcOn })),
    stat(t("dash.stat.users"), S.users.length, t("dash.multi", { n: multi })),
    stat(t("dash.stat.needs"), drifters.length, drifters.length ? t("dash.has_diff") : t("dash.check_hint"), drifters.length ? "warn" : "ok")));

  if (drifters.length) {
    wrap.append(h("div", { class: "callout amber" }, h("div", { class: "t" }, t("dash.drift_title")),
      h("div", { class: "foot-note", style: "margin:4px 0 8px" }, t("dash.drift_body", { names: drifters.map((r) => r.name).join(", ") })),
      h("div", { class: "row wrap-row" }, ...drifters.map((r) => h("button", { class: "btn sm", onclick: () => go({ kind: "router", id: r.name, tab: "deploy" }) }, r.name + " → Deploy")))));
  }

  wrap.append(h("div", { class: "row", style: "justify-content:flex-end" },
    h("button", { class: "btn sm", onclick: checkAllStatuses }, t("dash.check")),
    h("button", { class: "btn sm", onclick: () => openRouterModal(null) }, t("dash.add_router")),
    h("button", { class: "btn sm", onclick: () => openUserModal(null) }, t("dash.add_user"))));

  const rlist = h("div", { class: "card" });
  rlist.append(h("div", { class: "pad", style: "border-bottom:1px solid var(--divider)" }, h("span", { class: "section-title" }, t("nav.routers"))));
  for (const r of S.routers) rlist.append(dashRouterRow(r));
  wrap.append(rlist);

  const ulist = h("div", { class: "card" });
  ulist.append(h("div", { class: "pad", style: "border-bottom:1px solid var(--divider)" }, h("span", { class: "section-title" }, t("nav.users"))));
  for (const u of S.users) {
    const chips = u.access.length
      ? u.access.map((a) => h("span", { class: "chip" }, a.router + " · " + a.services.length))
      : [h("span", { class: "chip", style: "color:var(--muted)" }, t("nav.no_access"))];
    ulist.append(h("div", { class: "list-row", onclick: () => go({ kind: "user", id: u.name }) },
      h("span", { class: "avatar" }, initials(u.name)),
      h("span", { class: "grow" }, h("div", { class: "name" }, u.name)),
      h("div", { class: "row wrap-row" }, ...chips),
      noteBadge("user", null, u.name, u.note, true)));
  }
  if (!S.users.length) ulist.append(h("div", { class: "pad foot-note" }, t("dash.no_users")));
  wrap.append(ulist);

  return h("div", null, h("div", { class: "topbar" },
    h("div", { class: "grow" },
      h("h2", null, t("dash.title")),
      h("div", { class: "sub" }, t("dash.subtitle", { r: S.routers.length, u: S.users.length }), h("span", { class: "mono" }, S.path))),
    histButtons()),
    h("div", { class: "content" }, wrap));
}
function stat(label, value, note, tone) {
  return h("div", { class: "stat" }, h("div", { class: "label" }, label),
    h("div", { class: "value", style: tone === "warn" ? "color:var(--warn)" : tone === "ok" ? "color:var(--ok)" : "" }, String(value)),
    h("div", { class: "note" }, note));
}
function dashRouterRow(r) {
  const st = routerState(r);
  const tone = { clean: "grey", needs: "amber", synced: "green", never: "amber", empty: "grey", error: "grey" }[st];
  const svcOn = r.services.filter((s) => s.enabled).length;
  const usersWith = S.users.filter((u) => u.access.some((a) => a.router === r.name)).length;
  const hv = S.health[r.name];
  const dev = hv && hv.reachable ? " · " + [shortVersion(hv.version), hv.uptime].filter(Boolean).join(" · ") : "";
  return h("div", { class: "list-row", onclick: () => go({ kind: "router", id: r.name }) },
    hv && hv.reachable && !isDrift(r) ? h("span", { class: "dot green pulse" }) : h("span", { class: "dot " + tone }),
    h("span", { class: "grow" },
      h("div", { class: "name" }, r.name, " ", h("span", { class: "mono", style: "color:var(--muted);font-weight:400" }, r.address)),
      h("div", { class: "sub" }, (r.deploy.configured ? "" : t("dash.row.no_creds")) + t("rstate." + st) + " · " + t("dash.row.svc", { on: svcOn, total: r.services.length }) + " · " + t("dash.row.users", { n: usersWith }) + dev)),
    clockWarnDot(r),
    noteBadge("router", null, r.name, r.note, true));
}
async function checkAllStatuses() {
  toast(t("toast.checking"));
  for (const r of S.routers.filter((x) => x.deploy.configured)) {
    try { await deployAction(r.name, "status", {}); } catch (e) { /* recorded */ }
  }
  render();
  toast(t("toast.checked"));
}

// ---------- router view ----------
function routerView(r) {
  const st = routerState(r);
  let pill;
  if (st === "needs" || st === "never") pill = h("button", { class: "pill amber amber-btn", onclick: () => go({ kind: "router", id: r.name, tab: "deploy" }) }, h("span", { class: "dot amber" }), t("pill.needs"));
  else if (st === "synced") pill = h("span", { class: "pill green" }, t("pill.synced"));
  else if (st === "empty") pill = h("span", { class: "pill grey" }, t("pill.empty"));
  else pill = h("button", { class: "pill grey", onclick: () => go({ kind: "router", id: r.name, tab: "deploy" }) }, t("pill.clean"));

  const tabs = [["services", "Services"], ["access", "Access"], ["render", "Render"], ["deploy", "Deploy"]];
  const tabbar = h("div", { class: "tabs" }, ...tabs.map(([id, label]) =>
    h("button", { class: "tab" + (S.view.tab === id ? " sel" : ""), onclick: () => go({ kind: "router", id: r.name, tab: id }) },
      label, id === "deploy" && isDrift(r) && h("span", { class: "dot amber" }))));

  let body;
  if (S.view.tab === "access") body = routerAccess(r);
  else if (S.view.tab === "render") body = routerRender(r);
  else if (S.view.tab === "deploy") body = routerDeploy(r);
  else body = routerServices(r);

  return h("div", null,
    h("div", { class: "topbar" },
      h("div", { class: "grow" }, h("h2", null, r.name),
        h("div", { class: "sub row", style: "gap:10px" }, h("span", { class: "mono" }, r.address), healthMarker(r))),
      histButtons(),
      noteBadge("router", null, r.name, r.note, true),
      pill,
      h("button", { class: "btn sm", onclick: () => openRouterModal(r.name) }, t("settings"))),
    tabbar,
    h("div", { class: "content" }, body));
}

function routerServices(r) {
  const wrap = h("div", { class: "wrap" });
  wrap.append(h("div", { class: "head" },
    h("span", { class: "section-title" }, "Services"),
    h("span", { class: "badge grey" }, String(r.services.length)),
    h("span", { class: "spacer" }),
    h("button", { class: "btn pri sm", onclick: () => openServiceModal(r.name, null) }, t("svc.add"))));
  wrap.append(h("div", { class: "explain" }, t("svc.explain")));
  if (!r.services.length) wrap.append(h("div", { class: "card pad foot-note" }, t("svc.empty")));
  for (const s of r.services) {
    const target = s.target_type === "local"
      ? t("svc.target_local", { port: s.target_port, proto: s.target_protocol })
      : ":" + s.target_port + "/" + s.target_protocol + " → " + s.target_to_address + ":" + s.target_to_port;
    wrap.append(h("div", { class: "card pad row", style: s.enabled ? "" : "opacity:.62" },
      h("button", { class: "switch" + (s.enabled ? " on" : ""), "data-tip": s.enabled ? t("svc.on_title") : t("svc.off_title"), onclick: () => toggleService(r.name, s.name, !s.enabled) }),
      h("span", { class: "grow" },
        h("div", { class: "row" }, h("span", { class: "mono", style: "font-weight:600" }, s.name),
          h("span", { class: "badge " + (s.target_type === "local" ? "green" : "indigo") }, s.target_type)),
        h("div", { class: "row wrap-row", style: "margin-top:5px;gap:6px" },
          h("span", { class: "chip" }, s.stage1_port + " / " + s.stage2_port + " / " + s.token_port),
          h("span", { class: "foot-note" }, target))),
      noteBadge("service", r.name, s.name, s.note, true),
      h("button", { class: "iconbtn", "data-tip": t("svc.edit"), onclick: () => openServiceModal(r.name, s.name) }, icon("pencil")),
      h("button", { class: "iconbtn", "data-tip": t("svc.delete"), onclick: () => delService(r.name, s.name) }, icon("trash"))));
  }
  wrap.append(h("div", { class: "foot-note" }, t("svc.foot")));
  return wrap;
}

function routerAccess(r) {
  const wrap = h("div", { class: "wrap" });
  wrap.append(h("span", { class: "section-title" }, "Access"));
  wrap.append(h("div", { class: "explain" }, t("access.explain")));
  const withAccess = S.users.filter((u) => u.access.some((a) => a.router === r.name));
  if (!withAccess.length) return wrap.append(h("div", { class: "card pad foot-note" }, t("access.empty"))), wrap;
  for (const u of withAccess) {
    const a = u.access.find((x) => x.router === r.name);
    const chips = a.services.map((sn) => {
      const svc = r.services.find((s) => s.name === sn);
      const off = svc && !svc.enabled;
      return h("span", { class: "chip" + (off ? " strike" : ""), "data-tip": off ? t("access.svc_off") : "" }, sn);
    });
    wrap.append(h("div", { class: "card pad row" },
      h("span", { class: "avatar" }, initials(u.name)),
      h("span", { class: "grow" }, h("div", { style: "font-weight:550" }, u.name),
        h("div", { class: "row wrap-row", style: "margin-top:4px;gap:6px" }, h("span", { class: "foot-note mono" }, "psk ••••••••"), ...chips)),
      h("button", { class: "btn sm", onclick: () => go({ kind: "user", id: u.name }) }, t("access.open_user"))));
  }
  return wrap;
}

function routerRender(r) {
  const wrap = h("div", { class: "wrap wide" });
  const pre = h("pre", { class: "code" }, t("loading"));
  api("GET", "/api/render?router=" + encodeURIComponent(r.name)).then((txt) => { pre.textContent = txt; }).catch((e) => { pre.textContent = t("error") + ": " + e.message; });
  wrap.append(h("div", { class: "head" },
    h("span", { class: "section-title" }, "Render"),
    h("span", { class: "chip" }, "hash " + short(r.hash)),
    h("span", { class: "spacer" }),
    h("button", { class: "btn sm", onclick: (e) => { navigator.clipboard.writeText(pre.textContent).then(() => { e.target.textContent = t("copied"); setTimeout(() => e.target.textContent = t("copy"), 1600); }); } }, t("copy")),
    h("button", { class: "btn pri sm", onclick: () => downloadText(pre.textContent, "mkpk-" + r.name + ".rsc") }, t("render.download"))));
  wrap.append(h("div", { class: "card", style: "padding:2px" }, pre));
  wrap.append(h("div", { class: "foot-note" }, t("render.foot")));
  return wrap;
}

function routerDeploy(r) {
  const wrap = h("div", { class: "wrap" });
  wrap.append(h("span", { class: "section-title" }, "Deploy (SSH)"));
  if (!r.deploy.configured) {
    wrap.append(h("div", { class: "empty" }, icon("lock", "glyph"),
      h("h3", null, t("deploy.nocreds.title")),
      h("p", null, t("deploy.nocreds.body")),
      h("button", { class: "btn pri", onclick: () => openRouterModal(r.name) }, t("deploy.nocreds.btn"))));
    return wrap;
  }
  const d = r.deploy;
  const auth = d.use_agent ? "ssh-agent" : d.key_path ? t("deploy.auth_key", { path: d.key_path }) : d.password_set ? t("deploy.auth_pw") : "—";
  const sshAddr = d.address || r.address;
  wrap.append(h("div", { class: "grid2" },
    h("div", { class: "card pad" }, h("div", { class: "lbl" }, t("deploy.connection")),
      h("div", { class: "mono", style: "margin-top:4px" }, (d.user || "?") + " @ " + sshAddr + " : " + (d.port || 22)),
      h("div", { class: "foot-note", style: "margin-top:3px" }, auth + (d.password_set && !d.use_agent && d.key_path ? t("deploy.pw_fallback") : ""))),
    h("div", { class: "card pad" }, h("div", { class: "lbl" }, t("deploy.state")),
      h("div", { class: "mono", style: "margin-top:4px;font-size:11px" }, "local " + short(r.hash)),
      deployStateLine(r))));

  const force = h("input", { type: "checkbox", checked: S.deployOpts.force, onchange: () => { S.deployOpts.force = force.checked; render(); } });
  const running = S.deployRunning && S.deployRunning.startsWith(r.name + ":");
  const busy = (b) => running ? true : b;
  const synced = routerState(r) === "synced";
  const deployDisabled = busy(false) || (synced && !S.deployOpts.force);
  wrap.append(h("div", { class: "card pad row wrap-row" },
    h("button", { class: "btn sm", disabled: busy(false), onclick: () => runDeploy(r, "status") }, "Status"),
    h("button", { class: "btn ok sm", disabled: busy(false), "data-tip": t("deploy.dry_title"), onclick: () => runDeploy(r, "apply", { dry: true, force: S.deployOpts.force }) }, t("deploy.dry_btn")),
    h("button", { class: "btn pri sm", disabled: deployDisabled, "data-tip": synced && !S.deployOpts.force ? t("deploy.synced_hint") : "", onclick: () => runDeploy(r, "apply", { dry: false, force: S.deployOpts.force }) }, "Deploy"),
    h("button", { class: "btn danger sm", disabled: busy(false), onclick: () => confirmDialog(t("deploy.uninstall_title"), t("deploy.uninstall_body"), "Uninstall", () => runDeploy(r, "uninstall")) }, "Uninstall…"),
    h("span", { class: "spacer" }),
    h("label", { class: "inline-check", "data-tip": t("deploy.force_title") }, force, "force"),
    running && h("span", null, h("span", { class: "spin" }), " " + S.deployRunning.split(":")[1] + "…")));
  const prev = S.deploy[r.name];
  if (prev && prev.streaming) {
    wrap.append(h("div", { class: "card pad stack" },
      h("div", { class: "row" }, h("span", { class: "spin" }), h("span", { class: "foot-note" }, t("deploy.streaming"))),
      h("pre", { class: "term", id: "deploy-live-" + r.name }, (prev.live || []).join("\n\n"))));
  } else if (prev && prev.result) wrap.append(deployResult(r, prev.result));
  else wrap.append(h("div", { class: "card pad foot-note" }, t("deploy.result_ph")));
  return wrap;
}
function deployStateLine(r) {
  const st = routerState(r);
  const tone = { clean: "muted", needs: "warn", synced: "ok", never: "warn", empty: "muted", error: "muted" }[st];
  return h("div", { class: "foot-note", style: "margin-top:3px;color:var(--" + tone + ")" }, t("dstate." + st));
}
async function runDeploy(r, action, opts) {
  opts = opts || {};
  S.deployRunning = r.name + ":" + (opts.dry ? "dry" : action);
  const rec = S.deploy[r.name] || (S.deploy[r.name] = {});
  rec.streaming = true; rec.live = []; rec.result = null; rec.err = null;
  render();
  // Append straight to the live <pre> so we don't re-render on every line.
  const term = document.getElementById("deploy-live-" + r.name);
  const append = (line) => {
    rec.live.push(line);
    if (term) { term.textContent += (term.textContent ? "\n\n" : "") + line; term.scrollTop = term.scrollHeight; }
  };
  try {
    const res = await streamDeploy(r.name, { action, force: !!opts.force, dry_run: !!opts.dry }, append);
    applyDeployResult(rec, action, res);
  } catch (e) {
    rec.checked = true; rec.err = e.message; rec.result = { _kind: "err", msg: e.message, action };
  }
  rec.streaming = false;
  S.deployRunning = null;
  render();
}
// streamDeploy POSTs to the streaming endpoint and reads newline-delimited JSON
// events ({type:log|result|error}); onLine is called per log line, and the final
// result object is returned. Errors arrive as an error event (status is 200).
async function streamDeploy(routerName, body, onLine) {
  const resp = await fetch("/api/deploy/stream", {
    method: "POST",
    headers: { "X-MKPK-Token": TOKEN, "Content-Type": "application/json" },
    body: JSON.stringify({ router: routerName, action: body.action, force: !!body.force, dry_run: !!body.dry_run }),
  });
  if (!resp.ok || !resp.body) throw new Error((await resp.text()) || resp.statusText);
  const reader = resp.body.getReader();
  const dec = new TextDecoder();
  let buf = "", result = null;
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += dec.decode(value, { stream: true });
    let nl;
    while ((nl = buf.indexOf("\n")) >= 0) {
      const raw = buf.slice(0, nl).trim();
      buf = buf.slice(nl + 1);
      if (!raw) continue;
      const ev = JSON.parse(raw);
      if (ev.type === "log") onLine(ev.line);
      else if (ev.type === "result") result = ev.result;
      else if (ev.type === "error") throw new Error(ev.msg);
    }
  }
  if (!result) throw new Error("stream ended without a result");
  return result;
}
// deployAction runs a non-streamed action — used by the dashboard "check all".
async function deployAction(routerName, action, opts) {
  const body = { router: routerName, force: !!opts.force, dry_run: opts.dry_run !== undefined ? opts.dry_run : action !== "apply" };
  const rec = S.deploy[routerName] || (S.deploy[routerName] = {});
  let res;
  try {
    res = await api("POST", "/api/deploy/" + action, body);
  } catch (e) {
    rec.checked = true; rec.err = e.message; rec.result = { _kind: "err", msg: e.message, action };
    throw e;
  }
  applyDeployResult(rec, action, res);
  return res;
}
// applyDeployResult folds a deploy result into per-router state and tags it with
// a _kind the result card renders.
function applyDeployResult(rec, action, res) {
  rec.checked = true; rec.err = null;
  if (action === "status") {
    rec.installed = res.installed;
    rec.baseline = res.installed ? res.installed_hash : "";
    res._kind = res.installed ? (res.up_to_date ? "synced" : "drift") : "never";
  } else if (action === "apply" || action === "") {
    if (res.applied) { rec.installed = true; rec.baseline = res.hash; res._kind = "applied"; }
    else if (res.action === "skip") { rec.installed = true; rec.baseline = res.hash; res._kind = "synced"; }
    else { res._kind = "dry"; }
  } else if (action === "uninstall") {
    if (res.applied) { rec.installed = false; rec.baseline = ""; res._kind = "uninstalled"; }
    else res._kind = "dry";
  }
  rec.result = res;
}
function deployResult(r, res) {
  const kinds = {
    synced: ["ok", t("dres.synced.t"), t("dres.synced.m", { h: short(res.installed_hash) })],
    never: ["warn", t("dres.never.t"), t("dres.never.m", { h: short(res.desired_hash) })],
    drift: ["warn", t("dres.drift.t"), t("dres.drift.m", { h: short(res.installed_hash), l: short(res.desired_hash) })],
    applied: ["ok", t("dres.applied.t"), t("dres.applied.m", { h: short(res.hash) })],
    dry: ["ok", t("dres.dry.t"), t("dres.dry.m")],
    uninstalled: ["ok", t("dres.uninstalled.t"), t("dres.uninstalled.m")],
    err: ["err", t("dres.err.t"), res.msg],
  };
  const [tone, title, msg] = kinds[res._kind] || ["ok", t("dres.ok"), JSON.stringify(res)];
  const toneClass = { ok: "green", warn: "amber", err: "danger" }[tone] || "green";
  const card = h("div", { class: "card pad" },
    h("div", { class: "row" }, h("span", { class: "badge " + (toneClass === "danger" ? "amber" : toneClass), style: toneClass === "danger" ? "color:var(--danger);background:var(--danger-bg);border-color:var(--danger-border)" : "" }, tone === "ok" ? "✓" : tone === "warn" ? "●" : "✕"),
      h("span", { style: "font-weight:600" }, title)),
    h("div", { class: "foot-note", style: "margin-top:5px" }, msg));
  if (res._kind === "drift") card.append(h("button", { class: "btn pri sm", style: "margin-top:8px", onclick: () => runDeploy(r, "apply", { dry: false, force: true }) }, t("dres.apply_btn")));
  if (res._kind === "err") card.append(h("button", { class: "btn link", style: "margin-top:8px", onclick: () => openRouterModal(r.name) }, t("dres.settings_link")));
  if (res.log) {
    const term = h("pre", { class: "term hidden" }, res.log);
    const toggle = h("button", { class: "btn link", style: "margin-top:8px", onclick: () => {
      term.classList.toggle("hidden");
      toggle.textContent = term.classList.contains("hidden") ? t("dres.show_log") : t("dres.hide_log");
    } }, t("dres.show_log"));
    card.append(h("div", null, toggle), term);
  }
  return card;
}

// ---------- user view ----------
function userView(u) {
  const wrap = h("div", { class: "wrap narrow" });
  wrap.append(h("span", { class: "section-title" }, t("user.matrix")));
  wrap.append(h("div", { class: "explain" }, t("user.matrix.explain")));
  for (const r of S.routers) {
    const a = u.access.find((x) => x.router === r.name);
    const has = !!a;
    const card = h("div", { class: "card pad stack" });
    card.append(h("div", { class: "row" },
      icon("router"), h("span", { style: "font-weight:600" }, r.name),
      h("span", { class: "mono foot-note" }, r.address),
      noteHint(r.note),
      isDrift(r) && h("button", { class: "badge amber", "data-tip": t("user.needs_deploy_title"), onclick: () => go({ kind: "router", id: r.name, tab: "deploy" }) }, t("user.needs_deploy")),
      h("span", { class: "spacer" }),
      h("span", { class: "foot-note" }, has ? t("user.svc_count", { n: a.services.length, total: r.services.length }) : t("user.no_access")),
      h("span", { class: "badge " + (has ? "green" : "grey"), "data-tip": has ? t("user.psk_active") : t("user.psk_none") }, "PSK"),
      has && h("button", { class: "btn sm", "data-tip": t("user.invite_single_title"), onclick: () => openInvite(u.name, "single", r.name) }, "Invite"),
      has && h("button", { class: "iconbtn", "data-tip": t("user.rotate_title"), onclick: () => rotatePSK(u.name, r.name) }, icon("refresh"))));
    const checks = h("div", { class: "stack", style: "gap:5px" });
    for (const s of r.services) {
      const on = has && a.services.includes(s.name);
      const cb = h("input", { type: "checkbox", checked: on, onchange: () => setAccess(u.name, r.name, s.name, cb.checked) });
      checks.append(h("label", { class: "inline-check" }, cb, h("span", { class: "mono" }, s.name), noteHint(s.note), !s.enabled && h("span", { class: "foot-note" }, t("user.svc_off"))));
    }
    if (!r.services.length) checks.append(h("span", { class: "foot-note" }, t("user.no_services")));
    card.append(checks);
    wrap.append(card);
  }
  wrap.append(h("div", { class: "foot-note" }, t("user.matrix.foot")));

  const nAccess = u.access.length;
  const inv = h("div", { class: "card pad stack" });
  inv.append(h("span", { class: "section-title" }, t("user.grant")));
  inv.append(h("div", { class: "explain" }, t("user.grant.explain")));
  if (nAccess === 0) inv.append(h("div", { class: "foot-note" }, t("user.grant.none")));
  else inv.append(h("div", null, h("button", { class: "btn pri", onclick: () => openInvite(u.name, nAccess > 1 ? "all" : "single", nAccess === 1 ? u.access[0].router : null) },
    nAccess > 1 ? t("user.grant.all", { n: nAccess }) : t("user.grant.one"))));
  wrap.append(inv);

  return h("div", null,
    h("div", { class: "topbar" }, h("span", { class: "avatar lg" }, initials(u.name)),
      h("div", { class: "grow" }, h("h2", null, u.name),
        h("div", { class: "sub mono" }, t("user.header_id", { id: u.client_id }))),
      histButtons(),
      noteBadge("user", null, u.name, u.note, true),
      h("button", { class: "btn sm", onclick: () => openUserModal(u.name) }, t("settings"))),
    h("div", { class: "content" }, wrap));
}

// ---------- mutations ----------
async function toggleService(router, name, enabled) {
  try { applyConfig(await api("POST", "/api/service/enable", { router, name, enabled })); } catch (e) { toast(e.message, true); }
}
async function delService(router, name) {
  confirmDialog(t("svc.del.title", { name }), t("svc.del.body"), t("del"), async () => {
    try { applyConfig(await api("DELETE", "/api/service?router=" + encodeURIComponent(router) + "&name=" + encodeURIComponent(name))); toast(t("toast.svc_deleted")); } catch (e) { toast(e.message, true); }
  });
}
async function setAccess(user, router, service, on) {
  const u = userOf(user);
  const a = u.access.find((x) => x.router === router);
  const cur = a ? a.services.slice() : [];
  const next = on ? [...new Set([...cur, service])] : cur.filter((s) => s !== service);
  try {
    if (next.length === 0) applyConfig(await api("DELETE", "/api/client?router=" + encodeURIComponent(router) + "&name=" + encodeURIComponent(user)));
    else applyConfig(await api("POST", "/api/client", { router, name: user, services: next }));
  } catch (e) { toast(e.message, true); render(); }
}
async function rotatePSK(user, router) {
  confirmDialog(t("psk.rotate.title"), t("psk.rotate.body"), t("psk.rotate.btn"), async () => {
    try { applyConfig(await api("POST", "/api/user/psk", { user, router })); toast(t("toast.psk_rotated")); } catch (e) { toast(e.message, true); }
  });
}

// ---------- modals ----------
function closeModal() { document.getElementById("modal-root").innerHTML = ""; }
function modal(node, size) {
  // Close only via Esc or explicit buttons — not a backdrop click (avoids
  // losing a half-filled form by accident).
  const root = document.getElementById("modal-root");
  root.innerHTML = "";
  root.append(h("div", { class: "overlay" }, h("div", { class: "modal " + (size || "") }, node)));
}
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && document.querySelector(".overlay")) closeModal();
});

// ---------- tooltips ----------
// A single styled tooltip shared by every element carrying a data-tip attribute.
// Shows a touch faster than the native title and is theme-aware. Multi-line and
// long text wrap; it flips below the target when there is no room above.
const TIP = { el: null, timer: null, cur: null };
function tipEl() {
  if (!TIP.el) { TIP.el = document.createElement("div"); TIP.el.className = "tooltip"; document.body.appendChild(TIP.el); }
  return TIP.el;
}
function hideTip() {
  if (TIP.timer) { clearTimeout(TIP.timer); TIP.timer = null; }
  TIP.cur = null;
  if (TIP.el) TIP.el.classList.remove("show");
}
function showTip(target) {
  const text = target.getAttribute("data-tip");
  if (!text || !target.isConnected) return;
  const el = tipEl();
  el.textContent = text;
  el.classList.add("show");
  const r = target.getBoundingClientRect();
  const tw = el.offsetWidth, th = el.offsetHeight;
  let left = r.left + r.width / 2 - tw / 2;
  let top = r.top - th - 8, place = "top";
  if (top < 6) { top = r.bottom + 8; place = "bottom"; }
  left = Math.max(6, Math.min(left, window.innerWidth - tw - 6));
  el.style.left = Math.round(left) + "px";
  el.style.top = Math.round(top) + "px";
  el.setAttribute("data-place", place);
}
document.addEventListener("mouseover", (e) => {
  const target = e.target.closest && e.target.closest("[data-tip]");
  if (!target || target === TIP.cur) return;
  const text = target.getAttribute("data-tip");
  hideTip();
  if (!text) return; // empty data-tip → no tooltip
  TIP.cur = target;
  TIP.timer = setTimeout(() => showTip(target), 110);
});
document.addEventListener("mouseout", (e) => {
  if (!TIP.cur) return;
  const to = e.relatedTarget;
  if (to && TIP.cur.contains(to)) return; // moved within the same target
  hideTip();
});
document.addEventListener("scroll", hideTip, true);
document.addEventListener("mousedown", hideTip, true);
function field(label, input, note) {
  return h("div", { class: "field" }, h("label", null, label), input, note && h("div", { class: "note" }, note));
}

// nameInput constrains a text field to a RouterOS-safe identifier as the user
// types: only [A-Za-z0-9_-], capped at MAX_NAME_LEN. Disallowed characters are
// dropped and the caret kept stable, so invalid input never reaches the backend.
function nameInput(input) {
  input.setAttribute("maxlength", MAX_NAME_LEN);
  input.addEventListener("input", () => {
    const cleaned = input.value.replace(/[^A-Za-z0-9_-]/g, "");
    if (cleaned === input.value) return;
    const caret = (input.selectionStart || cleaned.length) - (input.value.length - cleaned.length);
    input.value = cleaned;
    try { input.setSelectionRange(caret, caret); } catch (e) { /* detached */ }
  });
  return input;
}
// portInput clamps a number field to a valid port (1..65535) on entry, so a huge
// value can't be typed in.
function portInput(input) {
  input.setAttribute("max", "65535");
  input.setAttribute("min", "1");
  input.addEventListener("input", () => {
    if (input.value === "") return;
    const n = Number(input.value);
    if (Number.isFinite(n) && n > 65535) input.value = "65535";
  });
  return input;
}

// noteBadge renders the local-note icon next to an entity. `always` shows a faint
// icon even when empty (an add affordance); without it, the icon appears only when
// a note exists (an at-a-glance indicator). Tooltip = the note; click opens the
// mini editor. Entity is identified by (kind, router, name) — router used only for
// kind "service".
function noteBadge(kind, router, name, note, always) {
  const has = !!(note && note.trim());
  if (!has && !always) return null;
  return h("button", {
    class: "iconbtn note-badge" + (has ? " has-note" : ""),
    "data-tip": has ? note : t("note.add"),
    onclick: (e) => { e.stopPropagation(); openNoteModal(kind, router, name, note || ""); },
  }, icon("note"));
}
// noteHint is a read-only note indicator: the note icon + tooltip when a note
// exists, with no editor. Used where notes are reference-only (the access matrix).
function noteHint(note) {
  if (!(note && note.trim())) return null;
  return h("span", { class: "note-hint", "data-tip": note, onclick: (e) => { e.preventDefault(); e.stopPropagation(); } }, icon("note"));
}
function openNoteModal(kind, router, name, note) {
  const ta = h("textarea", { class: "note-area", maxlength: 1000, rows: 5, placeholder: t("note.placeholder") });
  ta.value = note || "";
  const post = async (value) => {
    try { applyConfig(await api("POST", "/api/note", { kind, router: router || "", name, note: value })); closeModal(); toast(value ? t("note.saved") : t("note.cleared")); }
    catch (e) { toast(e.message, true); }
  };
  const save = h("button", { class: "btn pri", onclick: () => post(ta.value.trim()) }, t("save"));
  const foot = h("div", { class: "modal-foot" },
    note ? h("button", { class: "btn danger", onclick: () => post("") }, t("note.clear")) : null,
    h("span", { class: "spacer" }),
    h("button", { class: "btn", onclick: closeModal }, t("cancel")), save);
  modal(h("div", null,
    h("div", { class: "modal-head" }, h("h3", null, t("note.title", { kind: t("note.kind." + kind), name })), h("div", { class: "sub" }, t("note.subtitle"))),
    h("div", { class: "modal-body" }, ta),
    foot), "sm");
  ta.focus();
}

function openRouterModal(name) {
  const r = name ? routerOf(name) : null;
  const g = {};
  const inp = (val, attrs) => h("input", { type: "text", value: val || "", ...attrs });
  g.name = nameInput(inp(r && r.name, { placeholder: "router-a" }));
  g.address = hostInput(inp(r && r.address, { placeholder: "router.example.com" }));
  g.allowed_timeout = inp(r && r.defaults && r.defaults.allowed_timeout, { placeholder: "3m" });
  const d = (r && r.deploy) || {};
  g.ssh_address = hostInput(inp(d.address, { placeholder: "напр. 10.0.0.1 / router.lan" }));
  g.port = portInput(h("input", { type: "number", value: d.port || "", placeholder: "22" }));
  g.user = inp(d.user, { placeholder: "admin" });
  g.key_path = inp(d.key_path, { placeholder: "~/.ssh/id_ed25519" });
  let authMode = d.use_agent === false && d.key_path ? "key" : "agent";
  const keyWrap = h("div", { class: "field" + (authMode === "key" ? "" : " hidden") }, h("label", null, t("router.keypath")), g.key_path, h("div", { class: "note" }, t("router.keypath_note")));
  const seg = h("div", { class: "seg" },
    h("button", { type: "button", class: authMode === "agent" ? "on" : "", onclick: () => setAuth("agent") }, "ssh-agent"),
    h("button", { type: "button", class: authMode === "key" ? "on" : "", onclick: () => setAuth("key") }, t("router.auth_keyfile")));
  function setAuth(m) { authMode = m; seg.children[0].className = m === "agent" ? "on" : ""; seg.children[1].className = m === "key" ? "on" : ""; keyWrap.classList.toggle("hidden", m !== "key"); }
  g.password = h("input", { type: "password", placeholder: r ? t("ph.unchanged") : "" });
  g.key_pass = h("input", { type: "password", placeholder: r ? t("ph.unchanged") : "" });
  const fbBody = h("div", { class: "stack hidden" }, field(t("router.pw_ssh"), g.password), field(t("router.keypass"), g.key_pass, t("router.keypass_note")));
  const fbToggle = h("button", { type: "button", class: "collapse-head", onclick: () => { fbBody.classList.toggle("hidden"); fbToggle.firstChild.textContent = fbBody.classList.contains("hidden") ? "▶ " : "▼ "; } }, "▶ ", t("router.pw_collapse"));

  const n = (r && r.notify) || {};
  g.nw = h("input", { type: "checkbox", checked: !!n.webhook_enabled });
  g.url = inp(n.url, { placeholder: "https://…" });
  g.nt = h("input", { type: "checkbox", checked: !!n.telegram_enabled });
  g.tg_chat = inp(n.telegram_chat_id, { placeholder: "@chat / id" });
  g.tg_token = h("input", { type: "password", placeholder: n.bot_token_set ? t("ph.unchanged") : "bot token" });
  g.ne = h("input", { type: "checkbox", checked: !!n.email_enabled });
  g.email_to = inp(n.email_to); g.email_from = inp(n.email_from);
  g.email_server = inp(n.email_server, { placeholder: "smtp.example.com" });
  g.email_port = portInput(h("input", { type: "number", value: n.email_port || "", placeholder: "587" }));
  g.email_tls = inp(n.email_tls, { placeholder: "starttls" });
  g.email_user = inp(n.email_user);
  g.email_pass = h("input", { type: "password", placeholder: n.email_password_set ? t("ph.unchanged") : "" });
  function chan(cb, label, ...rows) {
    const box = h("div", { class: "chan" }, h("label", { class: "inline-check" }, cb, label), ...rows);
    const inputs = [...box.querySelectorAll("input,select")].filter((i) => i !== cb);
    const sync = () => inputs.forEach((i) => { i.disabled = !cb.checked; });
    cb.addEventListener("change", sync); sync();
    return box;
  }

  const addrErr = h("div", { class: "foot-note", style: "color:var(--danger)" });
  // Two tabs keep the modal short (no scroll): general/SSH on one, notifications
  // on the other. Both panels stay in the DOM so save reads every field.
  const paneGeneral = h("div", { class: "modal-body" },
    h("div", { class: "grid2" }, field(t("field.name"), g.name), field(t("field.address"), g.address, t("router.address_note"))),
    addrErr,
    field(t("router.allowed_timeout"), g.allowed_timeout, t("router.allowed_timeout_note")),
    h("fieldset", { class: "fieldset" }, h("legend", null, t("router.ssh_legend")),
      h("div", { class: "note" }, t("router.ssh_note")),
      field(t("router.ssh_address"), g.ssh_address, t("router.ssh_address_note")),
      h("div", { class: "grid2" }, field(t("field.port"), g.port), field(t("field.user"), g.user)),
      field(t("router.auth"), seg, t("router.auth_note")),
      keyWrap, fbToggle, fbBody));
  const paneNotify = h("div", { class: "modal-body hidden" },
    h("div", { class: "note" }, t("router.notify_note")),
    chan(g.nw, "Webhook", field("URL", g.url)),
    chan(g.nt, "Telegram", h("div", { class: "grid2" }, field("chat id", g.tg_chat), field("bot token", g.tg_token))),
    chan(g.ne, "Email",
      h("div", { class: "grid2" }, field("to", g.email_to), field("from", g.email_from)),
      h("div", { class: "grid3" }, field("server", g.email_server), field("port", g.email_port), field("tls", g.email_tls)),
      h("div", { class: "grid2" }, field("user", g.email_user), field("password", g.email_pass))));
  let mtab = "general";
  const tGen = h("button", { type: "button", class: "mtab on", onclick: () => setMTab("general") }, t("router.tab_general"));
  const tNot = h("button", { type: "button", class: "mtab", onclick: () => setMTab("notify") }, t("router.tab_notify"));
  function setMTab(x) {
    mtab = x;
    tGen.classList.toggle("on", x === "general"); tNot.classList.toggle("on", x === "notify");
    paneGeneral.classList.toggle("hidden", x !== "general"); paneNotify.classList.toggle("hidden", x !== "notify");
  }
  const tabbar = h("div", { class: "modal-tabs" }, tGen, tNot);
  const body = h("div", null, tabbar, paneGeneral, paneNotify);

  const save = h("button", { class: "btn pri", onclick: async () => {
    try {
      const nameVal = g.name.value.trim();
      const val = {
        name: r ? r.name : nameVal,   // operate on the current key
        rename: r ? nameVal : "",     // new name when editing
        address: g.address.value.trim(),
        allowed_timeout: g.allowed_timeout.value.trim(),
        deploy_address: g.ssh_address.value.trim(),
        port: +g.port.value || 0, user: g.user.value.trim(),
        use_agent: authMode === "agent", key_path: authMode === "key" ? g.key_path.value.trim() : "",
        password: g.password.value, key_pass: g.key_pass.value,
        notify: {
          webhook: { enabled: g.nw.checked, url: g.url.value.trim() },
          telegram: { enabled: g.nt.checked, chat_id: g.tg_chat.value.trim(), bot_token: g.tg_token.value },
          email: { enabled: g.ne.checked, to: g.email_to.value.trim(), from: g.email_from.value.trim(), server: g.email_server.value.trim(), port: +g.email_port.value || 0, tls: g.email_tls.value.trim(), user: g.email_user.value.trim(), password: g.email_pass.value },
        },
      };
      const res = await api("POST", "/api/router", val);
      closeModal(); S.view = { kind: "router", id: nameVal, tab: S.view.tab || "services" }; applyConfig(res); toast(t("toast.router_saved"));
    } catch (e) { toast(e.message, true); }
  } }, t("save"));
  // Live address validation: public address required + IPv4/hostname; SSH deploy
  // address optional but IPv4/hostname when set. Gates save and flags the field.
  function checkAddr() {
    const errs = [];
    const pub = g.address.value.trim();
    const badPub = pub === "" || !isHostOrIP(pub);
    if (pub === "") errs.push(t("router.addr_required"));
    else if (!isHostOrIP(pub)) errs.push(t("router.addr_bad", { addr: pub }));
    const ssh = g.ssh_address.value.trim();
    const badSsh = ssh !== "" && !isHostOrIP(ssh);
    if (badSsh) errs.push(t("router.ssh_addr_bad", { addr: ssh }));
    addrErr.textContent = errs.join("; ");
    g.address.classList.toggle("err", badPub);
    g.ssh_address.classList.toggle("err", badSsh);
    save.disabled = errs.length > 0;
  }
  g.address.addEventListener("input", checkAddr);
  g.ssh_address.addEventListener("input", checkAddr);
  checkAddr();

  const foot = h("div", { class: "modal-foot" });
  if (r) foot.append(h("button", { class: "btn danger sm", onclick: () => delRouter(r.name) }, t("router.del")));
  foot.append(h("span", { class: "spacer" }), h("button", { class: "btn", onclick: closeModal }, t("cancel")), save);

  modal(h("div", null, h("div", { class: "modal-head" }, h("h3", null, r ? t("nav.router_settings") : t("router.new"))), body, foot), "md");
}
async function delRouter(name) {
  const r = routerOf(name);
  const users = S.users.filter((u) => u.access.some((a) => a.router === name)).length;
  confirmDialog(t("router.del.title", { name }), t("router.del.body", { s: r.services.length, u: users }), t("router.del"), async () => {
    try { closeModal(); applyConfig(await api("DELETE", "/api/router?name=" + encodeURIComponent(name))); toast(t("toast.router_deleted")); } catch (e) { toast(e.message, true); }
  });
}

function openServiceModal(router, name) {
  const r = routerOf(router);
  const s = name ? r.services.find((x) => x.name === name) : null;
  const g = {};
  g.name = nameInput(h("input", { type: "text", class: "mono", value: s ? s.name : "", placeholder: "ssh-home" }));
  g.s1 = portInput(h("input", { type: "number", value: s ? s.stage1_port : "", placeholder: "41011" }));
  g.s2 = portInput(h("input", { type: "number", value: s ? s.stage2_port : "", placeholder: "41012" }));
  g.tk = portInput(h("input", { type: "number", value: s ? s.token_port : "", placeholder: "41013" }));
  const routerDefTimeout = (r.defaults && r.defaults.allowed_timeout) || "3m";
  g.allowed_timeout = h("input", { type: "text", class: "mono", value: s ? (s.allowed_timeout || "") : "", placeholder: routerDefTimeout });
  let ttype = s ? s.target_type : "forward";
  let proto = s ? s.target_protocol : "tcp";
  g.port = portInput(h("input", { type: "number", value: s ? s.target_port : "", placeholder: "2022" }));
  g.to_addr = h("input", { type: "text", value: s ? s.target_to_address : "", placeholder: "192.0.2.10" });
  g.to_port = portInput(h("input", { type: "number", value: s ? s.target_to_port : "", placeholder: "22" }));
  const fwdRow = h("div", { class: "grid2" }, field("to_address", g.to_addr), field("to_port", g.to_port));
  const portLabel = h("label", null, t("svc.port_ext"));
  const portNote = h("div", { class: "note hidden" }, t("svc.port_local_note"));
  const portField = h("div", { class: "field" }, portLabel, g.port, portNote);
  const typeSeg = seg2(["forward", "local"], ["forward (dst-nat)", "local (input)"], ttype, (v) => { ttype = v; syncType(); });
  const protoSeg = seg2(["tcp", "udp"], ["tcp", "udp"], proto, (v) => { proto = v; });
  function syncType() {
    const local = ttype === "local";
    fwdRow.classList.toggle("hidden", local);
    portLabel.textContent = local ? t("svc.port_local") : t("svc.port_ext");
    portNote.classList.toggle("hidden", !local);
    checkPorts();
  }

  const conflict = h("div", { class: "foot-note", style: "color:var(--danger)" });
  function checkPorts() {
    const errs = [];
    const bad = new Set(); // inputs to flag red
    // fields in play: knock ports + target port always; to_address/to_port only for forward
    const ports = [["stage1", g.s1], ["stage2", g.s2], ["token", g.tk], ["target", g.port]];
    if (ttype === "forward") ports.push(["to_port", g.to_port]);
    const required = [["name", g.name], ...ports];
    if (ttype === "forward") required.push(["to_address", g.to_addr]);
    // required: nothing left empty
    const missing = required.filter(([, e]) => e.value.trim() === "");
    missing.forEach(([, e]) => bad.add(e));
    if (missing.length) errs.push(t("svc.required", { fields: missing.map(([l]) => l).join(", ") }));
    // name format: RouterOS-safe identifier
    if (g.name.value.trim() !== "" && !isSafeName(g.name.value.trim())) { errs.push(t("svc.name_bad")); bad.add(g.name); }
    // range: each filled port must be 1..65535
    for (const [l, e] of ports) {
      const p = +e.value;
      if (e.value !== "" && (p < 1 || p > 65535 || !Number.isInteger(p))) { errs.push(t("svc.port_range", { label: l, port: e.value })); bad.add(e); }
    }
    // to_address (forward only) must be a literal IPv4 — RouterOS dst-nat needs it
    if (ttype === "forward" && g.to_addr.value.trim() !== "" && !isIPv4(g.to_addr.value.trim())) {
      errs.push(t("svc.ipv4", { addr: g.to_addr.value.trim() })); bad.add(g.to_addr);
    }
    // collision: knock ports vs other services on this router
    const mine = [["stage1", g.s1], ["stage2", g.s2], ["token", g.tk]].map(([l, e]) => [l, +e.value]);
    for (const svc of r.services) {
      if (s && svc.name === s.name) continue;
      for (const [l, p] of mine) {
        if (!p) continue;
        for (const [pl, pv] of [["stage1", svc.stage1_port], ["stage2", svc.stage2_port], ["token", svc.token_port], ["target", svc.target_port]])
          if (p === pv) { errs.push(t("svc.conflict", { label: l, port: p, svc: svc.name, field: pl })); bad.add(l === "stage1" ? g.s1 : l === "stage2" ? g.s2 : g.tk); }
      }
    }
    conflict.textContent = errs.join("; ");
    save.disabled = errs.length > 0;
    [g.name, g.s1, g.s2, g.tk, g.port, g.to_port, g.to_addr].forEach((e) => e.classList.toggle("err", bad.has(e)));
  }
  [g.name, g.s1, g.s2, g.tk, g.port, g.to_port, g.to_addr].forEach((e) => e.addEventListener("input", checkPorts));

  const suggest = h("button", { type: "button", class: "btn link row", style: "gap:5px", onclick: async () => {
    try { const d = await api("GET", "/api/ports/suggest?count=3&router=" + encodeURIComponent(router)); [g.s1.value, g.s2.value, g.tk.value] = d.ports; checkPorts(); } catch (e) { toast(e.message, true); }
  } }, icon("refresh", "ic-sm"), t("svc.suggest"));

  const save = h("button", { class: "btn pri", onclick: async () => {
    try {
      const nameVal = g.name.value.trim();
      const val = { router, name: s ? s.name : nameVal, rename: s ? nameVal : "", stage1_port: +g.s1.value, stage2_port: +g.s2.value, token_port: +g.tk.value,
        allowed_timeout: g.allowed_timeout.value.trim(),
        target: { type: ttype, protocol: proto, port: +g.port.value, to_address: ttype === "forward" ? g.to_addr.value.trim() : "", to_port: ttype === "forward" ? +g.to_port.value : 0 } };
      closeModal(); applyConfig(await api("POST", "/api/service", val)); toast(t("toast.svc_saved"));
    } catch (e) { toast(e.message, true); }
  } }, t("save"));

  const body = h("div", { class: "modal-body" },
    field(t("svc.field_name"), g.name, t("svc.field_name_note")),
    h("div", { class: "field" }, h("label", null, t("svc.knock_ports")),
      h("div", { class: "grid3" }, g.s1, g.s2, g.tk), h("div", { class: "row" }, suggest), conflict),
    field(t("svc.type"), typeSeg), portField, fwdRow, field(t("svc.proto"), protoSeg),
    field(t("svc.allowed_timeout"), g.allowed_timeout, t("svc.allowed_timeout_note", { def: routerDefTimeout })));
  syncType(); checkPorts();
  modal(h("div", null, h("div", { class: "modal-head" }, h("h3", null, s ? t("svc.title", { name: s.name }) : t("svc.new"))), body,
    h("div", { class: "modal-foot" }, h("span", { class: "spacer" }), h("button", { class: "btn", onclick: closeModal }, t("cancel")), save)));
}
function seg2(vals, labels, cur, on) {
  const s = h("div", { class: "seg" });
  vals.forEach((v, i) => s.append(h("button", { type: "button", class: v === cur ? "on" : "", onclick: () => { [...s.children].forEach((c, j) => c.className = j === i ? "on" : ""); on(v); } }, labels[i])));
  return s;
}

function openUserModal(name) {
  const u = name ? userOf(name) : null;
  const nameInp = nameInput(h("input", { type: "text", value: u ? u.name : "", placeholder: "phone" }));
  const save = h("button", { class: "btn pri", onclick: async () => {
    const val = nameInp.value.trim();
    if (!val) return;
    try {
      const res = u ? await api("POST", "/api/user", { name: u.name, rename: val }) : await api("POST", "/api/user", { name: val });
      closeModal(); S.view = { kind: "user", id: val }; applyConfig(res); toast(t("toast.user_saved"));
    } catch (e) { toast(e.message, true); }
  } }, t("save"));
  const foot = h("div", { class: "modal-foot" });
  if (u) foot.append(h("button", { class: "btn danger sm", onclick: () => {
    confirmDialog(t("user.del.title", { name: u.name }), t("user.del.body"), t("user.del"), async () => {
      try { closeModal(); S.view = { kind: "dashboard" }; applyConfig(await api("DELETE", "/api/user?name=" + encodeURIComponent(u.name))); toast(t("toast.user_deleted")); } catch (e) { toast(e.message, true); }
    });
  } }, t("user.del")));
  foot.append(h("span", { class: "spacer" }), h("button", { class: "btn", onclick: closeModal }, t("cancel")), save);
  modal(h("div", null, h("div", { class: "modal-head" }, h("h3", null, u ? t("user.title", { name: u.name }) : t("user.new"))),
    h("div", { class: "modal-body" }, field(t("user.field_name"), nameInp, t("user.field_name_note"))),
    foot), "sm");
}

async function openInvite(user, mode, router) {
  const u = userOf(user);
  let curMode = mode, curRouter = router || (u.access[0] && u.access[0].router);
  const routerPick = h("div", { class: "row wrap-row" });
  const included = h("div", { class: "stack", style: "gap:4px" });
  const blobBox = h("div", { class: "blobbox" });
  const modeSeg = seg2(["all", "single"], [t("invite.mode_all", { n: u.access.length }), t("invite.mode_single")], curMode, (v) => { curMode = v; refresh(); });

  function refresh() {
    routerPick.innerHTML = "";
    if (curMode === "single") {
      for (const a of u.access) routerPick.append(h("button", { type: "button", class: "chip", style: a.router === curRouter ? "border-color:var(--accent);color:var(--accent)" : "", onclick: () => { curRouter = a.router; refresh(); } }, a.router));
    }
    included.innerHTML = "";
    const routers = curMode === "all" ? u.access.map((a) => a.router) : [curRouter];
    for (const rn of routers) {
      const r = routerOf(rn); const a = u.access.find((x) => x.router === rn);
      const on = a.services.filter((sn) => { const s = r.services.find((x) => x.name === sn); return s && s.enabled; });
      included.append(h("div", { class: "row" }, h("span", { style: "font-weight:550" }, rn), h("span", { class: "mono foot-note" }, r.address),
        h("span", { class: "spacer" }), h("span", { class: "foot-note" }, on.length ? on.join(", ") : t("invite.no_services"))));
    }
    loadBlob();
  }
  async function loadBlob() {
    blobBox.innerHTML = "";
    const pre = h("pre", { class: "code", style: "max-height:120px" }, "…");
    const veil = h("div", { class: "veil" }, h("button", { class: "btn sm", onclick: () => veil.remove() }, t("invite.reveal")));
    blobBox.append(pre, veil);
    try {
      const q = "user=" + encodeURIComponent(user) + (curMode === "single" ? "&router=" + encodeURIComponent(curRouter) : "");
      const d = await api("GET", "/api/export?" + q);
      pre.textContent = d.blob; blobBox._blob = d.blob;
    } catch (e) { pre.textContent = t("error") + ": " + e.message; blobBox._blob = ""; }
  }

  const body = h("div", { class: "modal-body" },
    h("div", { class: "callout amber" }, h("div", { class: "foot-note", style: "color:var(--warn)" }, t("invite.warn"))),
    field(t("invite.mode"), modeSeg), routerPick,
    h("div", { class: "card pad" }, h("div", { class: "lbl", style: "margin-bottom:6px" }, t("invite.included")), included),
    blobBox);
  const foot = h("div", { class: "modal-foot" }, h("span", { class: "spacer" }),
    h("button", { class: "btn", onclick: () => { if (blobBox._blob) downloadText(blobBox._blob + "\n", user + (curMode === "single" ? "-" + curRouter : "") + ".mkpk"); } }, t("invite.download")),
    h("button", { class: "btn pri", onclick: (e) => { navigator.clipboard.writeText(blobBox._blob || "").then(() => { e.target.textContent = t("copied"); setTimeout(() => e.target.textContent = t("copy"), 1600); }); } }, t("copy")),
    h("button", { class: "btn ghost", onclick: closeModal }, t("close")));
  modal(h("div", null, h("div", { class: "modal-head" }, h("h3", null, t("invite.title", { user }))), body, foot));
  refresh();
}

function confirmDialog(title, msg, actionLabel, onOk) {
  modal(h("div", null,
    h("div", { class: "modal-head" }, h("div", { class: "row" }, icon("warn"), h("h3", null, title))),
    h("div", { class: "modal-body" }, h("div", { class: "foot-note" }, msg)),
    h("div", { class: "modal-foot" }, h("span", { class: "spacer" }),
      h("button", { class: "btn", onclick: closeModal }, t("cancel")),
      h("button", { class: "btn danger-solid", onclick: () => { closeModal(); onOk(); } }, actionLabel))), "sm");
}

async function downloadText(text, filename) {
  // The desktop webview (Wails) can't do a browser blob-download; the app is
  // local, so save server-side to ~/Downloads and report the path.
  if (window.MKPK_DESKTOP) {
    try { const r = await api("POST", "/api/save", { filename, content: text }); toast(t("saved_to", { path: r.path })); }
    catch (e) { toast(e.message, true); }
    return;
  }
  const a = document.createElement("a");
  a.href = URL.createObjectURL(new Blob([text], { type: "text/plain" }));
  a.download = filename; a.click();
}

// ---------- boot ----------
reload().then(startPolling).catch((e) => toast(e.message, true));
