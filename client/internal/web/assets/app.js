const TOKEN = window.MKPK_TOKEN;
const el = (id) => document.getElementById(id);

let summary = { routers: [] };
let current = "";

async function api(method, path, body) {
  const opts = { method, headers: { "X-MKPK-Token": TOKEN } };
  if (body !== undefined) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
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

let toastTimer;
function toast(msg, isErr) {
  const t = el("toast");
  t.textContent = msg;
  t.className = "toast " + (isErr ? "err" : "ok");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => t.classList.add("hidden"), 4000);
}

function td(html) {
  const c = document.createElement("td");
  if (html !== undefined) c.innerHTML = html;
  return c;
}

function routerObj() {
  return summary.routers.find((r) => r.name === current);
}

function targetDesc(svc) {
  const proto = svc.target_protocol || "tcp";
  if (svc.target_type === "local") {
    return `local ${proto}/${svc.target_port} (router)`;
  }
  return `forward ${proto}/${svc.target_port} → ${svc.target_to_address}:${svc.target_to_port}`;
}

function applyConfig(data) {
  el("cfg-path").textContent = data.path;
  summary = data.summary || { routers: [] };
  const names = summary.routers.map((r) => r.name);
  if (!names.includes(current)) current = names[0] || "";
  const sel = el("router-select");
  sel.innerHTML = "";
  names.forEach((n) => sel.append(new Option(n, n)));
  sel.value = current;
  render();
}

function render() {
  const r = routerObj();
  el("router-meta").textContent = r ? `${r.address} · ${(r.hash || "").slice(0, 12)}` : "no routers";
  const dc = el("deploy-creds");
  if (r && r.deploy && r.deploy.configured) {
    const via = r.deploy.use_agent ? "ssh-agent" : r.deploy.key_path ? "key" : "password";
    dc.textContent = `Credentials from router: ${r.deploy.user || "?"}@${r.address}:${r.deploy.port || 22} (${via}).`;
    dc.className = "muted small";
  } else {
    dc.textContent = "No deploy credentials on this router — set them via “edit router”.";
    dc.className = "small err-text";
  }
  renderServices(r);
  renderUsers(r);
}

function renderServices(r) {
  const tb = el("services").querySelector("tbody");
  tb.innerHTML = "";
  (r?.services || []).forEach((svc) => {
    const tr = document.createElement("tr");
    if (!svc.enabled) tr.className = "off";
    const onCell = td();
    const cb = document.createElement("input");
    cb.type = "checkbox";
    cb.checked = svc.enabled;
    cb.title = "enabled";
    cb.onchange = () => toggleService(svc.name, cb.checked);
    onCell.append(cb);
    tr.append(
      onCell,
      td(`<span class="mono">${svc.name}</span>`),
      td(`<span class="mono">${svc.stage1_port}/${svc.stage2_port}/${svc.token_port}</span>`),
      td(`<span class="mono">${svc.allowed_list}</span>`),
      td(targetDesc(svc)),
    );
    const rm = document.createElement("button");
    rm.textContent = "✕";
    rm.className = "danger small";
    rm.onclick = () => delService(svc.name);
    const c = td();
    c.append(rm);
    tr.append(c);
    tb.append(tr);
  });

  const box = el("usr-services");
  box.innerHTML = "";
  (r?.services || []).forEach((svc) => {
    const lab = document.createElement("label");
    lab.className = "inline";
    const cb = document.createElement("input");
    cb.type = "checkbox";
    cb.value = svc.name;
    lab.append(cb, document.createTextNode(" " + svc.name));
    box.append(lab);
  });
}

function renderUsers(r) {
  const tb = el("users").querySelector("tbody");
  tb.innerHTML = "";
  (r?.clients || []).forEach((cl) => {
    const tr = document.createElement("tr");
    const svcBadges = (cl.services || []).map((s) => `<span class="badge">${s}</span>`).join(" ") || "—";
    tr.append(
      td(`<span class="mono">${cl.name}</span>`),
      td(`<span class="mono">${cl.client_id}</span>`),
      td(svcBadges),
    );
    const exp = document.createElement("button");
    exp.textContent = "invite";
    exp.className = "small";
    exp.onclick = () => exportUser(cl.name);
    const rm = document.createElement("button");
    rm.textContent = "✕";
    rm.className = "danger small";
    rm.onclick = () => delUser(cl.name);
    const c = td();
    c.append(exp, document.createTextNode(" "), rm);
    tr.append(c);
    tb.append(tr);
  });
}

async function load() {
  try {
    applyConfig(await api("GET", "/api/config"));
  } catch (e) {
    toast(e.message, true);
  }
}

// --- router bar ---
el("router-select").addEventListener("change", (e) => {
  current = e.target.value;
  render();
});
function openRouterForm(edit) {
  const form = el("add-router-form");
  form.reset();
  const g = (n) => form.elements[n];
  const r = edit ? routerObj() : null;
  g("name").readOnly = !!r;
  if (r) {
    g("name").value = r.name;
    g("address").value = r.address;
    const d = r.deploy || {};
    g("user").value = d.user || "";
    g("port").value = d.port || "";
    g("key_path").value = d.key_path || "";
    g("use_agent").checked = !!d.use_agent;
    const n = r.notify || {};
    g("notify_enabled").checked = !!n.enabled;
    g("notify_channel").value = n.channel || "webhook";
    g("notify_url").value = n.url || "";
    g("tg_chat_id").value = n.telegram_chat_id || "";
    g("email_to").value = n.email_to || "";
    g("email_from").value = n.email_from || "";
    g("email_server").value = n.email_server || "";
    g("email_port").value = n.email_port || "";
    g("email_tls").value = n.email_tls || "";
    g("email_user").value = n.email_user || "";
    // secrets (ssh pass/keypass, bot_token, email password) are never sent back; blank means "keep"
  }
  syncNotify();
  form.classList.remove("hidden");
}
el("add-router-btn").onclick = () => openRouterForm(false);
el("edit-router-btn").onclick = () => { if (current) openRouterForm(true); };
el("add-router-cancel").onclick = () => el("add-router-form").classList.add("hidden");
el("add-router-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const g = (n) => e.target.elements[n];
  try {
    current = g("name").value.trim();
    applyConfig(await api("POST", "/api/router", {
      name: current,
      address: g("address").value.trim(),
      user: g("user").value.trim(),
      port: +g("port").value || 0,
      key_path: g("key_path").value.trim(),
      use_agent: g("use_agent").checked,
      key_pass: g("key_pass").value,
      password: g("password").value,
      notify: notifyBody(e.target),
    }));
    e.target.reset();
    e.target.classList.add("hidden");
    toast("router saved");
  } catch (err) {
    toast(err.message, true);
  }
});
el("del-router-btn").onclick = async () => {
  if (!current || !confirm(`remove router ${current} (and its services/users)?`)) return;
  try {
    applyConfig(await api("DELETE", "/api/router?name=" + encodeURIComponent(current)));
    toast("router removed");
  } catch (e) {
    toast(e.message, true);
  }
};

// --- services ---
async function toggleService(name, enabled) {
  try {
    applyConfig(await api("POST", "/api/service/enable", { router: current, name, enabled }));
  } catch (e) {
    toast(e.message, true);
    render();
  }
}
async function delService(name) {
  if (!confirm(`remove service ${name}?`)) return;
  try {
    applyConfig(await api("DELETE", `/api/service?router=${encodeURIComponent(current)}&name=${encodeURIComponent(name)}`));
    toast("service removed");
  } catch (e) {
    toast(e.message, true);
  }
}
function syncNotify() {
  const ch = el("notify-channel").value;
  document.querySelectorAll(".notify-fields").forEach((f) => f.classList.toggle("hidden", f.dataset.channel !== ch));
}
el("notify-channel").addEventListener("change", syncNotify);
el("random-ports").onclick = async () => {
  if (!current) return;
  try {
    const d = await api("GET", "/api/ports/suggest?count=3&router=" + encodeURIComponent(current));
    const f = el("svc-form").elements;
    [f["stage1_port"].value, f["stage2_port"].value, f["token_port"].value] = d.ports;
  } catch (e) {
    toast(e.message, true);
  }
};
function syncTarget() {
  const local = el("target-type").value === "local";
  document.querySelectorAll(".target-forward").forEach((f) => f.classList.toggle("hidden", local));
}
el("target-type").addEventListener("change", syncTarget);
el("svc-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const g = (n) => e.target.elements[n];
  const body = {
    router: current,
    name: g("name").value.trim(),
    stage1_port: +g("stage1_port").value,
    stage2_port: +g("stage2_port").value,
    token_port: +g("token_port").value,
    target: {
      type: g("target_type").value,
      protocol: g("target_protocol").value,
      port: +g("target_port").value,
      to_address: g("target_type").value === "forward" ? g("target_to_address").value.trim() : "",
      to_port: g("target_type").value === "forward" ? +g("target_to_port").value : 0,
    },
  };
  try {
    applyConfig(await api("POST", "/api/service", body));
    e.target.reset();
    toast("service added");
  } catch (err) {
    toast(err.message, true);
  }
});

function notifyBody(form) {
  const g = (n) => form.elements[n];
  return {
    enabled: g("notify_enabled").checked,
    channel: g("notify_channel").value,
    url: g("notify_url").value.trim(),
    telegram: { bot_token: g("tg_bot_token").value, chat_id: g("tg_chat_id").value.trim() },
    email: { to: g("email_to").value.trim(), from: g("email_from").value.trim(), server: g("email_server").value.trim(), port: +g("email_port").value || 0, tls: g("email_tls").value.trim(), user: g("email_user").value.trim(), password: g("email_password").value },
  };
}

// --- users ---
el("gen-psk").onclick = async () => {
  try {
    const d = await api("GET", "/api/secret");
    el("usr-psk").value = d.secret;
  } catch (e) {
    toast(e.message, true);
  }
};
el("usr-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const g = (n) => e.target.elements[n];
  const services = [...el("usr-services").querySelectorAll("input:checked")].map((c) => c.value);
  try {
    applyConfig(await api("POST", "/api/client", { router: current, name: g("name").value.trim(), services, psk: g("psk").value }));
    e.target.reset();
    toast("user added");
  } catch (err) {
    toast(err.message, true);
  }
});
async function delUser(name) {
  if (!confirm(`remove user ${name}?`)) return;
  try {
    applyConfig(await api("DELETE", `/api/client?router=${encodeURIComponent(current)}&name=${encodeURIComponent(name)}`));
    toast("user removed");
  } catch (e) {
    toast(e.message, true);
  }
}
async function exportUser(name) {
  try {
    const d = await api("GET", `/api/export?router=${encodeURIComponent(current)}&user=${encodeURIComponent(name)}`);
    el("invite-user").textContent = name;
    el("invite-blob").value = d.blob;
    el("invite-modal").dataset.user = name;
    el("invite-modal").classList.remove("hidden");
  } catch (e) {
    toast(e.message, true);
  }
}
el("invite-copy").onclick = async () => {
  try {
    await navigator.clipboard.writeText(el("invite-blob").value);
    toast("copied");
  } catch {
    el("invite-blob").select();
    toast("select+copy manually", true);
  }
};
el("invite-download").onclick = () => {
  const a = document.createElement("a");
  a.href = URL.createObjectURL(new Blob([el("invite-blob").value + "\n"], { type: "text/plain" }));
  a.download = el("invite-modal").dataset.user + ".mkpk";
  a.click();
};
el("invite-close").onclick = () => el("invite-modal").classList.add("hidden");

// --- render ---
el("render-btn").onclick = async () => {
  try {
    el("rsc").textContent = await api("GET", "/api/render?router=" + encodeURIComponent(current));
    el("rsc").classList.remove("hidden");
  } catch (e) {
    toast(e.message, true);
  }
};
el("download-btn").onclick = async () => {
  try {
    const text = await api("GET", "/api/render?router=" + encodeURIComponent(current));
    const a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([text], { type: "text/plain" }));
    a.download = current + ".rsc";
    a.click();
  } catch (e) {
    toast(e.message, true);
  }
};

// --- deploy ---
function deployBody() {
  const g = (n) => el("deploy-form").elements[n];
  return {
    router: current,
    force: g("force").checked,
    dry_run: g("dry_run").checked,
  };
}
document.querySelectorAll("#deploy-form button[data-action]").forEach((btn) => {
  btn.onclick = async () => {
    const action = btn.dataset.action;
    if (action === "uninstall" && !deployBody().dry_run && !confirm("uninstall mkpk from the router?")) return;
    const out = el("deploy-out");
    out.classList.remove("hidden");
    out.textContent = "… " + action;
    try {
      out.textContent = JSON.stringify(await api("POST", "/api/deploy/" + action, deployBody()), null, 2);
    } catch (e) {
      out.textContent = "error: " + e.message;
    }
  };
});

syncNotify();
syncTarget();
load();
