const $ = (id) => document.getElementById(id);

let repos = [];

async function fetchJSON(url, opts) {
  const r = await fetch(url, opts);
  return { ok: r.ok, status: r.status, body: await r.json() };
}

async function loadRepos() {
  const { body } = await fetchJSON("/_admin/api/repositories");
  repos = body || [];
  const sel = $("repository");
  sel.innerHTML = "";
  for (const r of repos) {
    const opt = document.createElement("option");
    opt.value = r.full_name;
    opt.textContent = r.full_name;
    sel.appendChild(opt);
  }
  refreshDependents();
}

function currentRepo() {
  const name = $("repository").value;
  return repos.find((r) => r.full_name === name);
}

function refreshDependents() {
  const repo = currentRepo();
  const prSel = $("pr-number");
  prSel.innerHTML = "";
  const shaSel = $("commit-sha");
  shaSel.innerHTML = "";
  const tbody = $("webhook-table").querySelector("tbody");
  tbody.innerHTML = "";
  if (!repo) return;
  for (const pr of repo.pull_requests || []) {
    const opt = document.createElement("option");
    opt.value = pr.number;
    opt.textContent = `#${pr.number} ${pr.title} [${pr.state}]`;
    prSel.appendChild(opt);
  }
  for (const c of repo.commits || []) {
    const opt = document.createElement("option");
    opt.value = c.sha;
    opt.textContent = c.sha.slice(0, 12);
    shaSel.appendChild(opt);
  }
  const hooks = repo.webhooks || [];
  if (!hooks.length) {
    const tr = document.createElement("tr");
    tr.innerHTML = '<td colspan="2"><em>none configured</em></td>';
    tbody.appendChild(tr);
  } else {
    for (const w of hooks) {
      const tr = document.createElement("tr");
      const events = (w.events && w.events.length) ? w.events.join(", ") : "(all)";
      tr.innerHTML = `<td>${escape(w.url)}</td><td>${escape(events)}</td>`;
      tbody.appendChild(tr);
    }
  }
}

function refreshEventFields() {
  const ev = $("event").value;
  $("pr-fields").style.display = ev === "pull_request" ? "" : "none";
  $("push-fields").style.display = ev === "push" ? "" : "none";
}

async function send() {
  const ev = $("event").value;
  const payload = {
    event: ev,
    repository: $("repository").value,
    sender: $("sender").value,
  };
  if (ev === "pull_request") {
    payload.action = $("action").value;
    payload.number = parseInt($("pr-number").value || "0", 10);
  } else if (ev === "push") {
    payload.ref = $("ref").value;
    payload.sha = $("commit-sha").value;
  }
  $("send").disabled = true;
  const { status, body } = await fetchJSON("/_admin/api/send", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  $("send").disabled = false;
  $("result").textContent = `HTTP ${status}\n` + JSON.stringify(body, null, 2);
}

function escape(s) {
  return String(s).replace(/[&<>"]/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;",
  }[c]));
}

$("event").addEventListener("change", refreshEventFields);
$("repository").addEventListener("change", refreshDependents);
$("send").addEventListener("click", send);

loadRepos();
refreshEventFields();
