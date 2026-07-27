"use strict";
const TOKEN = window.MKPK_TOKEN;

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
  gear: '<circle cx="12" cy="12" r="3"/><path d="M12 2v3m0 14v3M2 12h3m14 0h3M5 5l2 2m10 10 2 2M19 5l-2 2M7 17l-2 2"/>',
  pencil: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
  trash: '<path d="M3 6h18M8 6V4h8v2m-9 0 1 14h8l1-14"/>',
  lock: '<rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',
  warn: '<path d="M12 3 2 20h20L12 3Z"/><path d="M12 10v4m0 3h.01"/>',
  user: '<circle cx="12" cy="8" r="3.5"/><path d="M5 20a7 7 0 0 1 14 0"/>',
};
function icon(name, cls) {
  const s = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  s.setAttribute("viewBox", "0 0 24 24");
  s.setAttribute("fill", "none");
  s.setAttribute("stroke", "currentColor");
  s.setAttribute("stroke-width", "1.7");
  s.setAttribute("stroke-linecap", "round");
  s.setAttribute("stroke-linejoin", "round");
  if (cls) s.setAttribute("class", cls);
  s.innerHTML = ICONS[name] || "";
  return s;
}
const initials = (s) => (s || "?").slice(0, 2).toUpperCase();
const short = (hash) => (hash || "").slice(0, 12);

let toastTimer;
function toast(msg, isErr) {
  const t = document.getElementById("toast");
  t.textContent = msg;
  t.className = isErr ? "err" : "ok";
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => t.classList.add("hidden"), 3500);
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
  path: "",
  routers: [],
  users: [],
  view: { kind: "dashboard", id: null, tab: "services" },
  deploy: {},   // routerName -> {installed, installedHash, at, err, checked}
};
const routerOf = (n) => S.routers.find((r) => r.name === n);
const userOf = (n) => S.users.find((u) => u.name === n);

// drift: needs a status/apply check this session to be known
function driftState(r) {
  const d = S.deploy[r.name];
  if (!d || !d.checked) return "unknown";
  if (d.err) return "error";
  if (!d.installed) return "never";
  return d.installedHash === r.hash ? "synced" : "drift";
}
const isDrift = (r) => driftState(r) === "drift";

async function applyConfig(data) {
  S.path = data.path;
  const sum = data.summary || { routers: [], users: [] };
  S.routers = sum.routers || [];
  S.users = sum.users || [];
  // keep selection valid
  if (S.view.kind === "router" && !routerOf(S.view.id)) S.view = { kind: "dashboard" };
  if (S.view.kind === "user" && !userOf(S.view.id)) S.view = { kind: "dashboard" };
  if (!S.routers.length) S.view = { kind: "onboarding" };
  render();
}
async function reload() { applyConfig(await api("GET", "/api/config")); }
function go(view) { S.view = { tab: "services", ...view }; render(); }

// ---------- render root ----------
function render() {
  renderSidebar();
  renderMain();
}

function renderSidebar() {
  const sb = document.getElementById("sidebar");
  sb.innerHTML = "";
  sb.append(
    h("div", { class: "brand" },
      h("span", { class: "logo" }, icon("logo")),
      h("div", null, h("h1", null, "mkpk-provision"), h("div", { class: "sub" }, "provisioning console"))),
  );
  const nav = h("div", { class: "nav" });

  const dashRow = h("button", { class: "nav-row" + (S.view.kind === "dashboard" ? " sel" : ""), onclick: () => go({ kind: "dashboard" }) },
    icon("dash"), h("span", { class: "grow title" }, "Обзор"));
  const driftCount = S.routers.filter(isDrift).length;
  if (driftCount) dashRow.append(h("span", { class: "badge amber", title: "Есть роутеры с незадеплоенными изменениями" }, String(driftCount)));
  nav.append(dashRow);

  // routers
  nav.append(navHeader("Роутеры", () => openRouterModal(null)));
  for (const r of S.routers) {
    const row = h("button", { class: "nav-row" + (S.view.kind === "router" && S.view.id === r.name ? " sel" : ""), onclick: () => go({ kind: "router", id: r.name }) },
      icon("router"),
      h("span", { class: "grow" }, h("div", { class: "title" }, r.name), h("div", { class: "meta" }, r.address)),
      isDrift(r) && h("span", { class: "dot amber", title: "Нужен Deploy" }),
      h("span", { class: "gear", onclick: (e) => { e.stopPropagation(); openRouterModal(r.name); }, title: "Настройки роутера" }, icon("gear")));
    nav.append(row);
  }
  nav.append(h("button", { class: "dashed", onclick: () => openRouterModal(null) }, "+ Добавить роутер"));

  // users
  nav.append(navHeader("Юзеры", () => openUserModal(null)));
  for (const u of S.users) {
    const summary = u.access.length ? u.access.map((a) => a.router).join(", ") : "нет доступов";
    nav.append(h("button", { class: "nav-row" + (S.view.kind === "user" && S.view.id === u.name ? " sel" : ""), onclick: () => go({ kind: "user", id: u.name }) },
      h("span", { class: "avatar" }, initials(u.name)),
      h("span", { class: "grow" }, h("div", { class: "title" }, u.name), h("div", { class: "meta" }, summary)),
      h("span", { class: "gear", onclick: (e) => { e.stopPropagation(); openUserModal(u.name); }, title: "Переименовать/удалить" }, icon("pencil"))));
  }
  nav.append(h("button", { class: "dashed", onclick: () => openUserModal(null) }, "+ Добавить юзера"));

  sb.append(nav, h("div", { class: "sidebar-foot" }, location.host + " · local session"));
}
function navHeader(label, onAdd) {
  return h("div", { class: "nav-eyebrow" }, h("span", null, label), h("button", { onclick: onAdd, title: "Добавить" }, "+"));
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
      h("h3", null, "Добавьте первый роутер"),
      h("p", null, "Роутер — это ваш MikroTik, который приложение провижинит по SSH. Сервисы живут внутри роутера; юзеры — рядом с роутерами и могут иметь доступ к нескольким сразу."),
      h("button", { class: "btn pri", onclick: () => openRouterModal(null) }, "+ Добавить роутер")));
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
    stat("Роутеры", S.routers.length, noCreds ? noCreds + " без SSH-кредов" : "у всех заданы креды"),
    stat("Сервисы", svcTotal, svcOn + " включено в конфиге"),
    stat("Юзеры", S.users.length, multi + " с мультидоступом"),
    stat("Нужен Deploy", drifters.length, drifters.length ? "есть расхождения" : "проверьте статусы", drifters.length ? "warn" : "ok")));

  if (drifters.length) {
    const c = h("div", { class: "callout amber" }, h("div", { class: "t" }, "Требуется Deploy"),
      h("div", { class: "foot-note", style: "margin:4px 0 8px" }, drifters.map((r) => r.name).join(", ") + " — локальный конфиг отличается от того, что на роутере."),
      h("div", { class: "row wrap-row" }, ...drifters.map((r) => h("button", { class: "btn sm", onclick: () => go({ kind: "router", id: r.name, tab: "deploy" }) }, r.name + " → Deploy"))));
    wrap.append(c);
  }

  wrap.append(h("div", { class: "row", style: "justify-content:flex-end" },
    h("button", { class: "btn sm", onclick: checkAllStatuses }, "Проверить статусы"),
    h("button", { class: "btn sm", onclick: () => openRouterModal(null) }, "+ Роутер"),
    h("button", { class: "btn sm", onclick: () => openUserModal(null) }, "+ Юзер")));

  // routers list
  const rlist = h("div", { class: "card" });
  rlist.append(h("div", { class: "pad", style: "border-bottom:1px solid var(--divider)" }, h("span", { class: "section-title" }, "Роутеры")));
  for (const r of S.routers) rlist.append(dashRouterRow(r));
  wrap.append(rlist);

  // users list
  const ulist = h("div", { class: "card" });
  ulist.append(h("div", { class: "pad", style: "border-bottom:1px solid var(--divider)" }, h("span", { class: "section-title" }, "Юзеры")));
  for (const u of S.users) {
    const chips = u.access.length
      ? u.access.map((a) => h("span", { class: "chip" }, a.router + " · " + a.services.length))
      : [h("span", { class: "chip", style: "color:var(--muted)" }, "нет доступов")];
    ulist.append(h("div", { class: "list-row", onclick: () => go({ kind: "user", id: u.name }) },
      h("span", { class: "avatar" }, initials(u.name)),
      h("span", { class: "grow" }, h("div", { class: "name" }, u.name)),
      h("div", { class: "row wrap-row" }, ...chips)));
  }
  if (!S.users.length) ulist.append(h("div", { class: "pad foot-note" }, "Юзеров пока нет."));
  wrap.append(ulist);

  return h("div", null, h("div", { class: "topbar" }, h("div", { class: "grow" },
    h("h2", null, "Обзор"),
    h("div", { class: "sub" }, S.routers.length + " роутер(-ов) · " + S.users.length + " юзер(-ов) · ",
      h("span", { class: "mono" }, S.path)))),
    h("div", { class: "content" }, wrap));
}
function stat(label, value, note, tone) {
  return h("div", { class: "stat" }, h("div", { class: "label" }, label),
    h("div", { class: "value", style: tone === "warn" ? "color:var(--warn)" : tone === "ok" ? "color:var(--ok)" : "" }, String(value)),
    h("div", { class: "note" }, note));
}
function dashRouterRow(r) {
  const st = driftState(r);
  const stateText = { unknown: ["grey", "не проверялся"], never: ["grey", "ещё не деплоился"], synced: ["green", "✓ синхронизирован"], drift: ["amber", "● нужен Deploy"], error: ["grey", "ошибка подключения"] }[st];
  const svcOn = r.services.filter((s) => s.enabled).length;
  const usersWith = S.users.filter((u) => u.access.some((a) => a.router === r.name)).length;
  return h("div", { class: "list-row", onclick: () => go({ kind: "router", id: r.name }) },
    h("span", { class: "dot " + stateText[0] }),
    h("span", { class: "grow" },
      h("div", { class: "name" }, r.name, " ", h("span", { class: "mono", style: "color:var(--muted);font-weight:400" }, r.address)),
      h("div", { class: "sub" }, (r.deploy.configured ? "" : "SSH-креды не заданы · ") + stateText[1] + " · " + svcOn + "/" + r.services.length + " сервисов · " + usersWith + " юзер(-ов)")));
}
async function checkAllStatuses() {
  toast("Проверяю статусы…");
  for (const r of S.routers.filter((x) => x.deploy.configured)) {
    try { await deployAction(r.name, "status", { silent: true }); } catch (e) { /* recorded in S.deploy */ }
  }
  render();
  toast("Статусы обновлены");
}

// ---------- router view ----------
function routerView(r) {
  const st = driftState(r);
  let pill;
  if (st === "drift") pill = h("button", { class: "pill amber amber-btn", onclick: () => go({ kind: "router", id: r.name, tab: "deploy" }) }, h("span", { class: "dot amber" }), "Не задеплоено — нужен Deploy");
  else if (st === "synced") pill = h("span", { class: "pill green" }, "✓ Синхронизировано");
  else if (st === "never") pill = h("span", { class: "pill grey" }, "Ещё не деплоился");
  else pill = h("button", { class: "pill grey", onclick: () => go({ kind: "router", id: r.name, tab: "deploy" }) }, "Статус не проверялся");

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
      h("div", { class: "grow" }, h("h2", null, r.name), h("div", { class: "sub" }, h("span", { class: "mono" }, r.address))),
      pill,
      h("button", { class: "btn sm", onclick: () => openRouterModal(r.name) }, "Настройки")),
    tabbar,
    h("div", { class: "content" }, body));
}

function routerServices(r) {
  const wrap = h("div", { class: "wrap" });
  wrap.append(h("div", { class: "head" },
    h("span", { class: "section-title" }, "Services"),
    h("span", { class: "badge grey" }, String(r.services.length)),
    h("span", { class: "spacer" }),
    h("button", { class: "btn pri sm", onclick: () => openServiceModal(r.name, null) }, "+ Сервис")));
  wrap.append(h("div", { class: "explain" }, "Гейтованные эндпоинты этого роутера: три «стучальных» порта + цель — проброс внутрь (forward) или порт самого роутера (local)."));
  if (!r.services.length) wrap.append(h("div", { class: "card pad foot-note" }, "Сервисов пока нет. Добавьте первый."));
  for (const s of r.services) {
    const target = s.target_type === "local"
      ? "порт роутера " + s.target_port + "/" + s.target_protocol
      : ":" + s.target_port + "/" + s.target_protocol + " → " + s.target_to_address + ":" + s.target_to_port;
    const card = h("div", { class: "card pad row", style: s.enabled ? "" : "opacity:.62" },
      h("button", { class: "switch" + (s.enabled ? " on" : ""), title: s.enabled ? "Включён в конфиге. Выключить — правила не будут рендериться; применится после Deploy." : "Выключен в конфиге. Включить — правила появятся на роутере после Deploy.", onclick: () => toggleService(r.name, s.name, !s.enabled) }),
      h("span", { class: "grow" },
        h("div", { class: "row" }, h("span", { class: "mono", style: "font-weight:600" }, s.name),
          h("span", { class: "badge " + (s.target_type === "local" ? "green" : "indigo") }, s.target_type)),
        h("div", { class: "row wrap-row", style: "margin-top:5px;gap:6px" },
          h("span", { class: "chip" }, s.stage1_port + " / " + s.stage2_port + " / " + s.token_port),
          h("span", { class: "foot-note" }, target))),
      h("button", { class: "iconbtn", title: "Редактировать", onclick: () => openServiceModal(r.name, s.name) }, icon("pencil")),
      h("button", { class: "iconbtn", title: "Удалить", onclick: () => delService(r.name, s.name) }, icon("trash")));
    wrap.append(card);
  }
  wrap.append(h("div", { class: "foot-note" }, "Тоггл меняет состояние сервиса в конфиге. Выключенный сервис остаётся в списке, но его правила и токены юзеров не рендерятся и не деплоятся. Изменения попадут на роутер после Deploy → Apply."));
  return wrap;
}

function routerAccess(r) {
  const wrap = h("div", { class: "wrap" });
  wrap.append(h("span", { class: "section-title" }, "Access"));
  wrap.append(h("div", { class: "explain" }, "Кто имеет доступ к этому роутеру. Read-only проекция матрицы: редактирование доступа — на экране юзера."));
  const withAccess = S.users.filter((u) => u.access.some((a) => a.router === r.name));
  if (!withAccess.length) return wrap.append(h("div", { class: "card pad foot-note" }, "Ни у кого нет доступа к этому роутеру. Откройте юзера в сайдбаре и включите нужные сервисы.")), wrap;
  for (const u of withAccess) {
    const a = u.access.find((x) => x.router === r.name);
    const chips = a.services.map((sn) => {
      const svc = r.services.find((s) => s.name === sn);
      const off = svc && !svc.enabled;
      return h("span", { class: "chip" + (off ? " strike" : ""), title: off ? "Сервис выключен в конфиге" : "" }, sn);
    });
    wrap.append(h("div", { class: "card pad row" },
      h("span", { class: "avatar" }, initials(u.name)),
      h("span", { class: "grow" }, h("div", { style: "font-weight:550" }, u.name),
        h("div", { class: "row wrap-row", style: "margin-top:4px;gap:6px" }, h("span", { class: "foot-note mono" }, "psk ••••••••"), ...chips)),
      h("button", { class: "btn sm", onclick: () => go({ kind: "user", id: u.name }) }, "Открыть юзера →")));
  }
  return wrap;
}

function routerRender(r) {
  const wrap = h("div", { class: "wrap wide" });
  const pre = h("pre", { class: "code" }, "загрузка…");
  api("GET", "/api/render?router=" + encodeURIComponent(r.name)).then((txt) => { pre.textContent = txt; }).catch((e) => { pre.textContent = "ошибка: " + e.message; });
  wrap.append(h("div", { class: "head" },
    h("span", { class: "section-title" }, "Render"),
    h("span", { class: "chip" }, "hash " + short(r.hash)),
    h("span", { class: "spacer" }),
    h("button", { class: "btn sm", onclick: (e) => { navigator.clipboard.writeText(pre.textContent).then(() => { e.target.textContent = "✓ Скопировано"; setTimeout(() => e.target.textContent = "Скопировать", 1600); }); } }, "Скопировать"),
    h("button", { class: "btn pri sm", onclick: () => downloadText(pre.textContent, "mkpk-" + r.name + ".rsc") }, "Скачать .rsc")));
  wrap.append(h("div", { class: "card", style: "padding:2px" }, pre));
  wrap.append(h("div", { class: "foot-note" }, "Рендер по срезу текущего роутера: включённые сервисы + токен-правила юзеров, у которых есть доступ. PSK юзеров попадают в конфиг роутера — поэтому у каждой пары (юзер × роутер) свой PSK."));
  return wrap;
}

function routerDeploy(r) {
  const wrap = h("div", { class: "wrap" });
  wrap.append(h("span", { class: "section-title" }, "Deploy (SSH)"));
  if (!r.deploy.configured) {
    wrap.append(h("div", { class: "empty" }, icon("lock", "glyph"),
      h("h3", null, "SSH-креды не заданы"),
      h("p", null, "Реквизиты подключения задаются один раз в настройках роутера — экран деплоя их не спрашивает."),
      h("button", { class: "btn pri", onclick: () => openRouterModal(r.name) }, "Открыть настройки роутера")));
    return wrap;
  }
  const d = r.deploy;
  const auth = d.use_agent ? "ssh-agent" : d.key_path ? "ключ: " + d.key_path : d.password_set ? "пароль" : "—";
  wrap.append(h("div", { class: "grid2" },
    h("div", { class: "card pad" }, h("div", { class: "lbl" }, "Подключение"),
      h("div", { class: "mono", style: "margin-top:4px" }, (d.user || "?") + " @ " + r.address + " : " + (d.port || 22)),
      h("div", { class: "foot-note", style: "margin-top:3px" }, auth + (d.password_set && !d.use_agent && d.key_path ? " · пароль-fallback" : ""))),
    h("div", { class: "card pad" }, h("div", { class: "lbl" }, "Состояние"),
      h("div", { class: "mono", style: "margin-top:4px;font-size:11px" }, "local " + short(r.hash)),
      deployStateLine(r))));

  const dry = h("input", { type: "checkbox", checked: true });
  const force = h("input", { type: "checkbox" });
  const out = h("div", null);
  const runningLabel = h("span", null);
  const bar = h("div", { class: "card pad row wrap-row" },
    h("button", { class: "btn sm", onclick: () => runDeploy(r, "status", dry, force, out, runningLabel) }, "Status"),
    h("button", { class: "btn pri sm", onclick: () => runDeploy(r, "apply", dry, force, out, runningLabel) }, "Apply"),
    h("button", { class: "btn danger sm", onclick: () => confirmDialog("Uninstall с роутера?", "Все правила mkpk-tt будут удалены с роутера. Локальный конфиг не тронут.", "Uninstall", () => runDeploy(r, "uninstall", dry, force, out, runningLabel)) }, "Uninstall…"),
    h("span", { class: "spacer" }),
    h("label", { class: "inline-check", title: "Показать, что будет сделано, без изменений на роутере" }, dry, "dry-run"),
    h("label", { class: "inline-check", title: "Применить, даже если hash совпадает" }, force, "force"),
    runningLabel);
  wrap.append(bar, out);
  const prev = S.deploy[r.name];
  if (prev && prev.result) out.append(deployResult(r, prev.result, dry, force, out, runningLabel));
  else out.append(h("div", { class: "card pad foot-note" }, "Результат действия появится здесь. Начните со Status."));
  return wrap;
}
function deployStateLine(r) {
  const st = driftState(r);
  const map = { unknown: ["grey", "не проверялся — нажмите Status"], never: ["grey", "ещё не деплоился"], synced: ["ok", "✓ синхронизировано"], drift: ["warn", "● drift — нужен Apply"], error: ["grey", "ошибка подключения"] };
  const [tone, text] = map[st];
  return h("div", { class: "foot-note", style: "margin-top:3px;color:var(--" + tone + ")" }, text);
}
async function runDeploy(r, action, dry, force, out, runningLabel) {
  runningLabel.innerHTML = "";
  runningLabel.append(h("span", { class: "spin" }), " " + action + "…");
  try {
    const res = await deployAction(r.name, action, { force: force.checked, dry_run: dry.checked });
    out.innerHTML = "";
    out.append(deployResult(r, res, dry, force, out, runningLabel));
    renderSidebar();
  } catch (e) {
    out.innerHTML = "";
    out.append(deployResult(r, { _kind: "err", msg: e.message, action }, dry, force, out, runningLabel));
  } finally { runningLabel.innerHTML = ""; }
}
// deployAction records installed state and returns the raw result
async function deployAction(routerName, action, opts) {
  const body = { router: routerName, force: !!opts.force, dry_run: opts.dry_run !== undefined ? opts.dry_run : action !== "apply" };
  let res;
  try {
    res = await api("POST", "/api/deploy/" + action, body);
  } catch (e) {
    S.deploy[routerName] = { checked: true, installed: false, err: e.message, result: { _kind: "err", msg: e.message, action } };
    throw e;
  }
  const rec = S.deploy[routerName] || {};
  rec.checked = true; rec.err = null;
  if (action === "status") {
    rec.installed = res.installed; rec.installedHash = res.installed_hash;
    res._kind = res.installed ? (res.up_to_date ? "synced" : "drift") : "never";
  } else if (action === "apply") {
    if (res.applied) { rec.installed = true; rec.installedHash = res.hash; res._kind = "applied"; }
    else if (res.action === "skip") { rec.installed = true; rec.installedHash = res.hash; res._kind = "synced"; }
    else { res._kind = "dry"; }   // dry-run
  } else if (action === "uninstall") {
    if (res.applied) { rec.installed = false; rec.installedHash = null; res._kind = "uninstalled"; }
    else res._kind = "dry";
  }
  rec.result = res;
  S.deploy[routerName] = rec;
  return res;
}
function deployResult(r, res, dry, force, out, runningLabel) {
  const kinds = {
    synced: ["ok", "Синхронизировано", "На роутере hash " + short(res.installed_hash) + " совпадает с локальным конфигом."],
    never: ["warn", "На роутере ничего не установлено", "mkpk-tt-meta не найдена. Локальный конфиг (hash " + short(res.desired_hash) + ") ещё не деплоился."],
    drift: ["warn", "Drift: конфиг отличается", "На роутере hash " + short(res.installed_hash) + ", локально " + short(res.desired_hash) + ". Нужен Apply."],
    applied: ["ok", "Задеплоено", "Конфиг применён: hash " + short(res.hash) + ". Локальный конфиг и роутер синхронизированы."],
    dry: ["ok", "Dry-run завершён", "Показано, что было бы сделано. На роутере ничего не изменено (dry-run)."],
    uninstalled: ["ok", "Снято с роутера", "Все правила mkpk-tt удалены. Конфиг в приложении не тронут — Apply вернёт всё обратно."],
    err: ["err", "Не удалось подключиться", res.msg],
  };
  const [tone, title, msg] = kinds[res._kind] || ["ok", "Готово", JSON.stringify(res)];
  const toneClass = { ok: "green", warn: "amber", err: "danger" }[tone] || "green";
  const card = h("div", { class: "card pad" },
    h("div", { class: "row" }, h("span", { class: "badge " + (toneClass === "danger" ? "amber" : toneClass), style: toneClass === "danger" ? "color:var(--danger);background:var(--danger-bg);border-color:var(--danger-border)" : "" }, tone === "ok" ? "✓" : tone === "warn" ? "●" : "✕"),
      h("span", { style: "font-weight:600" }, title)),
    h("div", { class: "foot-note", style: "margin-top:5px" }, msg));
  if (res._kind === "drift") card.append(h("button", { class: "btn pri sm", style: "margin-top:8px", onclick: () => runDeploy(r, "apply", dry, force, out, runningLabel) }, "Apply — задеплоить изменения"));
  if (res._kind === "err") card.append(h("button", { class: "btn link", style: "margin-top:8px", onclick: () => openRouterModal(r.name) }, "Настройки роутера →"));
  return card;
}

// ---------- user view ----------
function userView(u) {
  const wrap = h("div", { class: "wrap narrow" });
  wrap.append(h("span", { class: "section-title" }, "Матрица доступа"));
  wrap.append(h("div", { class: "explain" }, "Юзер может иметь доступ к нескольким роутерам. PSK — свой на каждый роутер (создаётся при первом доступе); один инвайт может включать несколько роутеров."));
  for (const r of S.routers) {
    const a = u.access.find((x) => x.router === r.name);
    const has = !!a;
    const card = h("div", { class: "card pad stack" });
    card.append(h("div", { class: "row" }, icon("router"), h("span", { style: "font-weight:600" }, r.name),
      h("span", { class: "mono foot-note" }, r.address), h("span", { class: "spacer" }),
      h("span", { class: "foot-note" }, has ? a.services.length + " из " + r.services.length + " сервисов" : "нет доступа")));
    if (has) card.append(h("div", { class: "row", style: "background:var(--surface-2);border-radius:6px;padding:6px 9px" },
      h("span", { class: "lbl" }, "PSK"), h("span", { class: "mono", style: "letter-spacing:1px" }, "••••••••••••"),
      h("span", { class: "foot-note" }, "свой для этого роутера"), h("span", { class: "spacer" }),
      h("button", { class: "btn sm", title: "Инвайт только с этим роутером", onclick: () => openInvite(u.name, "single", r.name) }, "Invite"),
      h("button", { class: "btn ghost sm", title: "Сгенерировать новый PSK для этой пары юзер×роутер", onclick: () => rotatePSK(u.name, r.name) }, "⟳ Ротировать")));
    const checks = h("div", { class: "stack", style: "gap:5px" });
    for (const s of r.services) {
      const on = has && a.services.includes(s.name);
      const cb = h("input", { type: "checkbox", checked: on, onchange: () => setAccess(u.name, r.name, s.name, cb.checked) });
      checks.append(h("label", { class: "inline-check" }, cb, h("span", { class: "mono" }, s.name), !s.enabled && h("span", { class: "foot-note" }, "· выключен в конфиге")));
    }
    if (!r.services.length) checks.append(h("span", { class: "foot-note" }, "у роутера нет сервисов"));
    card.append(checks);
    wrap.append(card);
  }
  wrap.append(h("div", { class: "foot-note" }, "Изменения доступа и ротация PSK попадают на роутер после Deploy этого роутера, а юзеру — через пере-выданный инвайт."));

  // invite section
  const nAccess = u.access.length;
  const inv = h("div", { class: "card pad stack" });
  inv.append(h("span", { class: "section-title" }, "Выдать доступ"));
  inv.append(h("div", { class: "explain" }, "Invite-blob для клиентского приложения — передавать только по безопасному каналу."));
  if (nAccess === 0) inv.append(h("div", { class: "foot-note" }, "Сначала включите юзеру хотя бы один сервис в матрице выше."));
  else inv.append(h("div", null, h("button", { class: "btn pri", onclick: () => openInvite(u.name, nAccess > 1 ? "all" : "single", nAccess === 1 ? u.access[0].router : null) },
    nAccess > 1 ? "Общий инвайт — все роутеры (" + nAccess + ")" : "Выдать инвайт")));
  wrap.append(inv);

  return h("div", null,
    h("div", { class: "topbar" }, h("span", { class: "avatar lg" }, initials(u.name)),
      h("div", { class: "grow" }, h("h2", null, u.name),
        h("div", { class: "sub mono" }, "client_id: " + u.client_id + " · единый во всех роутерах")),
      h("button", { class: "btn sm", onclick: () => openUserModal(u.name) }, "Настройки")),
    h("div", { class: "content" }, wrap));
}

// ---------- mutations ----------
async function toggleService(router, name, enabled) {
  try { applyConfig(await api("POST", "/api/service/enable", { router, name, enabled })); } catch (e) { toast(e.message, true); }
}
async function delService(router, name) {
  confirmDialog("Удалить сервис " + name + "?", "Правило будет удалено с роутера после следующего Deploy.", "Удалить", async () => {
    try { applyConfig(await api("DELETE", "/api/service?router=" + encodeURIComponent(router) + "&name=" + encodeURIComponent(name))); toast("Сервис удалён"); } catch (e) { toast(e.message, true); }
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
  confirmDialog("Ротировать PSK?", "Будет сгенерирован новый PSK этой пары юзер×роутер. Старый инвайт перестанет работать после Deploy роутера — выдайте новый.", "Ротировать", async () => {
    try { applyConfig(await api("POST", "/api/user/psk", { user, router })); toast("PSK ротирован"); } catch (e) { toast(e.message, true); }
  });
}

// ---------- modals ----------
function closeModal() { document.getElementById("modal-root").innerHTML = ""; }
function modal(node, size) {
  const root = document.getElementById("modal-root");
  root.innerHTML = "";
  root.append(h("div", { class: "overlay", onclick: (e) => { if (e.target === e.currentTarget) closeModal(); } },
    h("div", { class: "modal " + (size || "") }, node)));
}
function field(label, input, note) {
  return h("div", { class: "field" }, h("label", null, label), input, note && h("div", { class: "note" }, note));
}

function openRouterModal(name) {
  const r = name ? routerOf(name) : null;
  const g = {};
  const inp = (val, attrs) => { const e = h("input", { type: "text", value: val || "", ...attrs }); return e; };
  g.name = inp(r && r.name, { placeholder: "router-a" });
  if (r) g.name.setAttribute("readonly", "");
  g.address = inp(r && r.address, { placeholder: "router.example.com" });
  const d = (r && r.deploy) || {};
  g.port = h("input", { type: "number", value: d.port || "", placeholder: "22" });
  g.user = inp(d.user, { placeholder: "admin" });
  g.key_path = inp(d.key_path, { placeholder: "~/.ssh/id_ed25519" });
  let authMode = d.use_agent === false && d.key_path ? "key" : "agent";
  const keyWrap = h("div", { class: "field" + (authMode === "key" ? "" : " hidden") }, h("label", null, "Путь к приватному ключу"), g.key_path, h("div", { class: "note" }, "В конфиге хранится только путь, не сам ключ."));
  const seg = h("div", { class: "seg" },
    h("button", { type: "button", class: authMode === "agent" ? "on" : "", onclick: () => setAuth("agent") }, "ssh-agent"),
    h("button", { type: "button", class: authMode === "key" ? "on" : "", onclick: () => setAuth("key") }, "файл ключа"));
  function setAuth(m) { authMode = m; seg.children[0].className = m === "agent" ? "on" : ""; seg.children[1].className = m === "key" ? "on" : ""; keyWrap.classList.toggle("hidden", m !== "key"); }
  g.password = h("input", { type: "password", placeholder: r ? "не менять" : "" });
  g.key_pass = h("input", { type: "password", placeholder: r ? "не менять" : "" });
  const fbBody = h("div", { class: "stack hidden" }, field("Пароль SSH (fallback)", g.password), field("Пассфраза ключа", g.key_pass, "Опционально. Лежит в секретном конфиге вместе с PSK."));
  const fbToggle = h("button", { type: "button", class: "collapse-head", onclick: () => { fbBody.classList.toggle("hidden"); fbToggle.firstChild.textContent = fbBody.classList.contains("hidden") ? "▶ " : "▼ "; } }, "▶ ", "Пароль (fallback)");

  const n = (r && r.notify) || {};
  g.notify_enabled = h("input", { type: "checkbox", checked: !!n.enabled });
  g.notify_channel = h("select", null, ...["webhook", "telegram", "email"].map((c) => h("option", { value: c, selected: (n.channel || "webhook") === c }, c)));
  g.notify_url = inp(n.url, { placeholder: "https://…" });
  g.tg_chat = inp(n.telegram_chat_id, { placeholder: "@chat или id" });
  g.tg_token = h("input", { type: "password", placeholder: n.bot_token_set ? "не менять" : "bot token" });
  g.email_to = inp(n.email_to); g.email_server = inp(n.email_server, { placeholder: "smtp.example.com" });

  const body = h("div", { class: "modal-body" },
    h("div", { class: "grid2" }, field("Имя", g.name), field("Адрес", g.address)),
    h("fieldset", { class: "fieldset" }, h("legend", null, "SSH для деплоя"),
      h("div", { class: "note" }, "Используется кнопками Status / Apply / Uninstall. Хранится в локальном секретном конфиге (0600) и не покидает эту машину."),
      h("div", { class: "grid2" }, field("Порт", g.port), field("Пользователь", g.user)),
      field("Аутентификация", seg, "рекомендуется ssh-agent: секрет не попадает в конфиг"),
      keyWrap, fbToggle, fbBody),
    h("fieldset", { class: "fieldset" }, h("legend", null, "Уведомления (per router)"),
      h("div", { class: "note" }, "Отправляются при успешном открытии любого сервиса этого роутера. Пустое поле = канал выключен."),
      h("label", { class: "inline-check" }, g.notify_enabled, "включены"),
      field("Канал", g.notify_channel),
      field("Webhook URL", g.notify_url), field("Telegram chat", g.tg_chat), field("Telegram bot token", g.tg_token),
      field("Email — to", g.email_to), field("Email — server", g.email_server)));

  const save = h("button", { class: "btn pri", onclick: async () => {
    try {
      const val = {
        name: g.name.value.trim(), address: g.address.value.trim(),
        port: +g.port.value || 0, user: g.user.value.trim(),
        use_agent: authMode === "agent", key_path: authMode === "key" ? g.key_path.value.trim() : "",
        password: g.password.value, key_pass: g.key_pass.value,
        notify: { enabled: g.notify_enabled.checked, channel: g.notify_channel.value, url: g.notify_url.value.trim(),
          telegram: { chat_id: g.tg_chat.value.trim(), bot_token: g.tg_token.value },
          email: { to: g.email_to.value.trim(), server: g.email_server.value.trim() } },
      };
      const res = await api("POST", "/api/router", val);
      closeModal(); S.view = { kind: "router", id: val.name, tab: S.view.tab || "services" }; applyConfig(res); toast("Роутер сохранён");
    } catch (e) { toast(e.message, true); }
  } }, "Сохранить");
  const foot = h("div", { class: "modal-foot" });
  if (r) foot.append(h("button", { class: "btn danger sm", onclick: () => delRouter(r.name) }, "Удалить роутер…"));
  foot.append(h("span", { class: "spacer" }), h("button", { class: "btn", onclick: closeModal }, "Отмена"), save);

  modal(h("div", null, h("div", { class: "modal-head" }, h("h3", null, r ? "Настройки роутера" : "Новый роутер")), body, foot), "md");
}
async function delRouter(name) {
  const r = routerOf(name);
  const users = S.users.filter((u) => u.access.some((a) => a.router === name)).length;
  confirmDialog("Удалить роутер " + name + "?", "Вместе с ним удалятся " + r.services.length + " сервис(-ов) и доступ у " + users + " юзер(-ов). Сначала сделайте Uninstall, если mkpk стоит на роутере.", "Удалить роутер", async () => {
    try { closeModal(); applyConfig(await api("DELETE", "/api/router?name=" + encodeURIComponent(name))); toast("Роутер удалён"); } catch (e) { toast(e.message, true); }
  });
}

function openServiceModal(router, name) {
  const r = routerOf(router);
  const s = name ? r.services.find((x) => x.name === name) : null;
  const g = {};
  g.name = h("input", { type: "text", class: "mono", value: s ? s.name : "", placeholder: "ssh-home" });
  if (s) g.name.setAttribute("readonly", "");
  g.s1 = h("input", { type: "number", value: s ? s.stage1_port : "", placeholder: "41011" });
  g.s2 = h("input", { type: "number", value: s ? s.stage2_port : "", placeholder: "41012" });
  g.tk = h("input", { type: "number", value: s ? s.token_port : "", placeholder: "41013" });
  let ttype = s ? s.target_type : "forward";
  let proto = s ? s.target_protocol : "tcp";
  g.port = h("input", { type: "number", value: s ? s.target_port : "", placeholder: "2022" });
  g.to_addr = h("input", { type: "text", value: s ? s.target_to_address : "", placeholder: "192.0.2.10" });
  g.to_port = h("input", { type: "number", value: s ? s.target_to_port : "", placeholder: "22" });
  const fwdRow = h("div", { class: "grid2" }, field("to_address", g.to_addr), field("to_port", g.to_port));
  const localRow = h("div", { class: "field" }, h("label", null, "Порт роутера"), g.port, h("div", { class: "note" }, "input accept на этот порт роутера, без NAT."));
  const fwdPort = field("Внешний порт", g.port);
  const typeSeg = seg2(["forward", "local"], ["forward (dst-nat)", "local (input)"], ttype, (v) => { ttype = v; syncType(); });
  const protoSeg = seg2(["tcp", "udp"], ["tcp", "udp"], proto, (v) => { proto = v; });
  function syncType() { fwdRow.classList.toggle("hidden", ttype === "local"); fwdPort.classList.toggle("hidden", ttype === "local"); localRow.classList.toggle("hidden", ttype !== "local"); }

  const conflict = h("div", { class: "foot-note", style: "color:var(--danger)" });
  function checkPorts() {
    const mine = [["stage1", g.s1], ["stage2", g.s2], ["token", g.tk]].map(([l, e]) => [l, +e.value]);
    const errs = [];
    for (const svc of r.services) {
      if (s && svc.name === s.name) continue;
      for (const [l, p] of mine) {
        if (!p) continue;
        for (const [pl, pv] of [["stage1", svc.stage1_port], ["stage2", svc.stage2_port], ["token", svc.token_port], ["target", svc.target_port]])
          if (p === pv) errs.push(l + ": " + p + " занят — " + svc.name + " (" + pl + ")");
      }
    }
    conflict.textContent = errs.join("; ");
    save.disabled = errs.length > 0;
    [g.s1, g.s2, g.tk].forEach((e) => e.classList.toggle("err", errs.some((x) => x.includes(e.value) && e.value)));
  }
  [g.s1, g.s2, g.tk].forEach((e) => e.addEventListener("input", checkPorts));

  const suggest = h("button", { type: "button", class: "btn link", onclick: async () => {
    try { const d = await api("GET", "/api/ports/suggest?count=3&router=" + encodeURIComponent(router)); [g.s1.value, g.s2.value, g.tk.value] = d.ports; checkPorts(); } catch (e) { toast(e.message, true); }
  } }, "⚄ Подобрать свободные");

  const save = h("button", { class: "btn pri", onclick: async () => {
    try {
      const val = { router, name: g.name.value.trim(), stage1_port: +g.s1.value, stage2_port: +g.s2.value, token_port: +g.tk.value,
        target: { type: ttype, protocol: proto, port: +g.port.value, to_address: ttype === "forward" ? g.to_addr.value.trim() : "", to_port: ttype === "forward" ? +g.to_port.value : 0 } };
      closeModal(); applyConfig(await api("POST", "/api/service", val)); toast("Сервис сохранён");
    } catch (e) { toast(e.message, true); }
  } }, "Сохранить");

  const body = h("div", { class: "modal-body" },
    field("Имя сервиса", g.name, "Входит в формулу токена — переименование инвалидирует выданные инвайты."),
    h("div", { class: "field" }, h("label", null, "Порты «стука» (stage1 / stage2 / token)"),
      h("div", { class: "grid3" }, g.s1, g.s2, g.tk), h("div", { class: "row" }, suggest), conflict),
    field("Тип цели", typeSeg), fwdPort, fwdRow, localRow, field("Протокол", protoSeg));
  syncType(); checkPorts();
  modal(h("div", null, h("div", { class: "modal-head" }, h("h3", null, s ? "Сервис " + s.name : "Новый сервис")), body,
    h("div", { class: "modal-foot" }, h("span", { class: "spacer" }), h("button", { class: "btn", onclick: closeModal }, "Отмена"), save)));
}
function seg2(vals, labels, cur, on) {
  const s = h("div", { class: "seg" });
  vals.forEach((v, i) => s.append(h("button", { type: "button", class: v === cur ? "on" : "", onclick: () => { [...s.children].forEach((c, j) => c.className = j === i ? "on" : ""); on(v); } }, labels[i])));
  return s;
}

function openUserModal(name) {
  const u = name ? userOf(name) : null;
  const nameInp = h("input", { type: "text", value: u ? u.name : "", placeholder: "phone" });
  const save = h("button", { class: "btn pri", onclick: async () => {
    const val = nameInp.value.trim();
    if (!val) return;
    try {
      let res;
      if (u) res = await api("POST", "/api/user", { name: u.name, rename: val });
      else res = await api("POST", "/api/user", { name: val });
      closeModal(); S.view = { kind: "user", id: val }; applyConfig(res); toast("Юзер сохранён");
    } catch (e) { toast(e.message, true); }
  } }, "Сохранить");
  const foot = h("div", { class: "modal-foot" });
  if (u) foot.append(h("button", { class: "btn danger sm", onclick: () => {
    confirmDialog("Удалить юзера " + u.name + "?", "Юзер и весь его доступ на всех роутерах будут удалены. Изменения попадут на роутеры после Deploy.", "Удалить юзера", async () => {
      try { closeModal(); S.view = { kind: "dashboard" }; applyConfig(await api("DELETE", "/api/user?name=" + encodeURIComponent(u.name))); toast("Юзер удалён"); } catch (e) { toast(e.message, true); }
    });
  } }, "Удалить юзера…"));
  foot.append(h("span", { class: "spacer" }), h("button", { class: "btn", onclick: closeModal }, "Отмена"), save);
  modal(h("div", null, h("div", { class: "modal-head" }, h("h3", null, u ? "Юзер " + u.name : "Новый юзер")),
    h("div", { class: "modal-body" }, field("Имя (client_id)", nameInp, "Единая идентичность во всех роутерах; входит в формулу токена — переименование инвалидирует инвайты.")),
    foot), "sm");
}

async function openInvite(user, mode, router) {
  const u = userOf(user);
  let curMode = mode, curRouter = router || (u.access[0] && u.access[0].router);
  const routerPick = h("div", { class: "row wrap-row" });
  const included = h("div", { class: "stack", style: "gap:4px" });
  const blobBox = h("div", { class: "blobbox" });
  const modeSeg = seg2(["all", "single"], ["Все роутеры (" + u.access.length + ")", "Только один роутер"], curMode, (v) => { curMode = v; refresh(); });

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
        h("span", { class: "spacer" }), h("span", { class: "foot-note" }, on.length ? on.join(", ") : "— нет включённых сервисов")));
    }
    loadBlob();
  }
  async function loadBlob() {
    blobBox.innerHTML = "";
    const pre = h("pre", { class: "code", style: "max-height:120px" }, "…");
    const veil = h("div", { class: "veil" }, h("button", { class: "btn sm", onclick: () => veil.remove() }, "Показать блоб"));
    blobBox.append(pre, veil);
    try {
      const q = "user=" + encodeURIComponent(user) + (curMode === "single" ? "&router=" + encodeURIComponent(curRouter) : "");
      const d = await api("GET", "/api/export?" + q);
      pre.textContent = d.blob; blobBox._blob = d.blob;
    } catch (e) { pre.textContent = "ошибка: " + e.message; blobBox._blob = ""; }
  }

  const body = h("div", { class: "modal-body" },
    h("div", { class: "callout amber" }, h("div", { class: "foot-note", style: "color:var(--warn)" }, "Блоб содержит PSK юзера для каждого включённого роутера. Передавайте только по безопасному каналу.")),
    field("Что войдёт в блоб", modeSeg), routerPick,
    h("div", { class: "card pad" }, h("div", { class: "lbl", style: "margin-bottom:6px" }, "Роутеры в блобе"), included),
    blobBox);
  const foot = h("div", { class: "modal-foot" }, h("span", { class: "spacer" }),
    h("button", { class: "btn", onclick: () => { if (blobBox._blob) downloadText(blobBox._blob + "\n", user + (curMode === "single" ? "-" + curRouter : "") + ".mkpk"); } }, "Скачать .mkpk"),
    h("button", { class: "btn pri", onclick: (e) => { navigator.clipboard.writeText(blobBox._blob || "").then(() => { e.target.textContent = "✓ Скопировано"; setTimeout(() => e.target.textContent = "Скопировать", 1600); }); } }, "Скопировать"),
    h("button", { class: "btn ghost", onclick: closeModal }, "Закрыть"));
  modal(h("div", null, h("div", { class: "modal-head" }, h("h3", null, "Инвайт — " + user)), body, foot));
  refresh();
}

function confirmDialog(title, msg, actionLabel, onOk) {
  modal(h("div", null,
    h("div", { class: "modal-head" }, h("div", { class: "row" }, icon("warn"), h("h3", null, title))),
    h("div", { class: "modal-body" }, h("div", { class: "foot-note" }, msg)),
    h("div", { class: "modal-foot" }, h("span", { class: "spacer" }),
      h("button", { class: "btn", onclick: closeModal }, "Отмена"),
      h("button", { class: "btn danger-solid", onclick: () => { closeModal(); onOk(); } }, actionLabel))), "sm");
}

function downloadText(text, filename) {
  const a = document.createElement("a");
  a.href = URL.createObjectURL(new Blob([text], { type: "text/plain" }));
  a.download = filename; a.click();
}

// ---------- boot ----------
reload().catch((e) => toast(e.message, true));
