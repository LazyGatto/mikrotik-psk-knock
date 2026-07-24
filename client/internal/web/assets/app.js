const TOKEN = window.MKPK_TOKEN;
const el = (id) => document.getElementById(id);

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

function fillSelect(sel, values, firstLabel) {
  const cur = sel.value;
  sel.innerHTML = "";
  if (firstLabel !== undefined) sel.append(new Option(firstLabel, ""));
  values.forEach((v) => sel.append(new Option(v, v)));
  if ([...sel.options].some((o) => o.value === cur)) sel.value = cur;
}

function td(html) {
  const c = document.createElement("td");
  c.innerHTML = html;
  return c;
}

function renderConfig(data) {
  const s = data.summary;
  el("cfg-path").textContent = data.path;
  el("cfg-hash").textContent = (data.hash || "").slice(0, 16);
  el("router-line").textContent = `router: ${s.router.name || "—"} @ ${s.router.address || "—"}`;
  const d = s.defaults;
  el("defaults-line").textContent = `bucket=${d.bucket_seconds}s stage=${d.stage_timeout} hit=${d.token_hit_timeout} allowed=${d.allowed_timeout} used=${d.used_timeout}`;

  const st = el("services").querySelector("tbody");
  st.innerHTML = "";
  (s.services || []).forEach((svc) => {
    const tr = document.createElement("tr");
    tr.append(
      td(`<span class="mono">${svc.name}</span>`),
      td(`<span class="mono">${svc.stage1_port}/${svc.stage2_port}/${svc.token_port}</span>`),
      td(`<span class="mono">${svc.allowed_list}</span>`),
      td(`${svc.nat_enabled ? "on" : "off"} ${svc.nat_dst_port}→${svc.nat_to_address}:${svc.nat_to_port}`),
      td(svc.notify_enabled ? svc.notify_channel : "off"),
    );
    const rm = document.createElement("button");
    rm.textContent = "✕";
    rm.className = "danger small";
    rm.onclick = () => del("service", svc.name);
    const c = document.createElement("td");
    c.append(rm);
    tr.append(c);
    st.append(tr);
  });

  const ct = el("clients").querySelector("tbody");
  ct.innerHTML = "";
  (s.clients || []).forEach((cl) => {
    const tr = document.createElement("tr");
    tr.append(
      td(`<span class="mono">${cl.name}</span>`),
      td(`<span class="mono">${cl.client_id}</span>`),
      td(`<span class="mono">${cl.service}</span>`),
      td(cl.psk_set ? "set" : "—"),
    );
    const rm = document.createElement("button");
    rm.textContent = "✕";
    rm.className = "danger small";
    rm.onclick = () => del("client", cl.name);
    const c = document.createElement("td");
    c.append(rm);
    tr.append(c);
    ct.append(tr);
  });

  fillSelect(el("cli-service"), (s.services || []).map((x) => x.name));
  fillSelect(el("render-client"), (s.clients || []).map((x) => x.name), "all clients");
}

async function load() {
  try {
    renderConfig(await api("GET", "/api/config"));
  } catch (e) {
    toast(e.message, true);
  }
}

async function del(kind, name) {
  if (!confirm(`remove ${kind} ${name}?`)) return;
  try {
    renderConfig(await api("DELETE", `/api/${kind}?name=` + encodeURIComponent(name)));
    toast(`${kind} removed`);
  } catch (e) {
    toast(e.message, true);
  }
}

function syncNotify() {
  const ch = el("notify-channel").value;
  document.querySelectorAll(".notify-fields").forEach((f) => f.classList.toggle("hidden", f.dataset.channel !== ch));
}

el("svc-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const g = (n) => e.target.elements[n];
  const body = {
    name: g("name").value.trim(),
    stage1_port: +g("stage1_port").value,
    stage2_port: +g("stage2_port").value,
    token_port: +g("token_port").value,
    allowed_list: g("allowed_list").value.trim(),
    nat: {
      enabled: g("nat_enabled").checked,
      dst_port: +g("nat_dst_port").value,
      to_address: g("nat_to_address").value.trim(),
      to_port: +g("nat_to_port").value,
    },
    notify: {
      enabled: g("notify_enabled").checked,
      channel: g("notify_channel").value,
      url: g("notify_url").value.trim(),
      telegram: { bot_token: g("tg_bot_token").value.trim(), chat_id: g("tg_chat_id").value.trim() },
      email: {
        to: g("email_to").value.trim(),
        from: g("email_from").value.trim(),
        server: g("email_server").value.trim(),
        port: +g("email_port").value || 0,
        tls: g("email_tls").value.trim(),
        user: g("email_user").value.trim(),
        password: g("email_password").value,
      },
    },
  };
  try {
    renderConfig(await api("POST", "/api/service", body));
    e.target.reset();
    syncNotify();
    toast("service added");
  } catch (err) {
    toast(err.message, true);
  }
});

el("cli-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const g = (n) => e.target.elements[n];
  const body = { name: g("name").value.trim(), service: g("service").value, psk: g("psk").value };
  try {
    renderConfig(await api("POST", "/api/client", body));
    e.target.reset();
    toast("client added");
  } catch (err) {
    toast(err.message, true);
  }
});

el("gen-psk").onclick = async () => {
  try {
    const d = await api("GET", "/api/secret");
    el("cli-psk").value = d.secret;
  } catch (e) {
    toast(e.message, true);
  }
};

el("notify-channel").addEventListener("change", syncNotify);

async function renderRsc() {
  const client = el("render-client").value;
  return api("GET", "/api/render" + (client ? "?client=" + encodeURIComponent(client) : ""));
}

el("render-btn").onclick = async () => {
  try {
    el("rsc").textContent = await renderRsc();
    el("rsc").classList.remove("hidden");
  } catch (e) {
    toast(e.message, true);
  }
};

el("download-btn").onclick = async () => {
  try {
    const text = await renderRsc();
    const a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([text], { type: "text/plain" }));
    a.download = "mkpk.rsc";
    a.click();
  } catch (e) {
    toast(e.message, true);
  }
};

function deployBody() {
  const g = (n) => el("deploy-form").elements[n];
  return {
    address: g("address").value.trim(),
    port: +g("port").value || 22,
    user: g("user").value.trim(),
    key_path: g("key_path").value.trim(),
    key_pass: g("key_pass").value,
    use_agent: g("use_agent").checked,
    password: g("password").value,
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
      const data = await api("POST", "/api/deploy/" + action, deployBody());
      out.textContent = JSON.stringify(data, null, 2);
      if (action !== "status") load();
    } catch (e) {
      out.textContent = "error: " + e.message;
    }
  };
});

syncNotify();
load();
