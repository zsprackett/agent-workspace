# Web UI Revamp Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the inline-expand session list with a split-pane mission-control layout: fixed 260px sidebar on the left, full-height detail panel on the right, with tabbed content (Terminal, Git, Notes, Log). Mobile: full-screen sidebar slides away when a session is selected, detail panel slides in with a back button.

**Architecture:** Three-file frontend rewrite — `index.html` (new DOM structure), `style.css` (full replacement), `app.js` (new state model + split-pane rendering). No backend changes. Key state change: `expandedSessions` Set → single `selectedSessionID` string. Detail panel rebuilt only when selected session changes; status-only updates patch the DOM in-place to avoid destroying the terminal iframe or losing textarea focus.

**Tech Stack:** Vanilla JS/HTML/CSS, JetBrains Mono via Google Fonts, no new dependencies.

---

### Task 1: Rewrite index.html

**Files:**
- Modify: `internal/webserver/static/index.html`

**Step 1: Replace the file with the new structure**

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="theme-color" content="#060a0f">
  <title>agent-workspace</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600&display=swap" rel="stylesheet">
  <link rel="manifest" href="/manifest.json">
  <link rel="stylesheet" href="/style.css">
</head>
<body>
  <div class="app">
    <header class="app-header">
      <div class="header-left">
        <button class="back-btn" id="back-btn" aria-label="Back">←</button>
        <span class="logo">agent-workspace</span>
      </div>
      <div class="header-right">
        <div id="connection-status" class="conn-dot" title="SSE connection"></div>
        <button id="logout-btn" class="logout-btn">logout</button>
      </div>
    </header>
    <div class="layout">
      <aside class="sidebar" id="sidebar">
        <div id="session-list"></div>
      </aside>
      <main class="detail-panel" id="detail-panel">
        <div class="detail-empty" id="detail-empty">
          <span class="detail-empty-text">select a session</span>
        </div>
        <div class="detail-content" id="detail-content"></div>
      </main>
    </div>
  </div>
  <script src="/app.js"></script>
</body>
</html>
```

**Step 2: Verify it builds**

```bash
make build
```

Expected: compiles with no errors.

**Step 3: Commit**

```bash
git add internal/webserver/static/index.html
git commit -m "feat(ui): new split-pane HTML structure"
```

---

### Task 2: Rewrite style.css

**Files:**
- Modify: `internal/webserver/static/style.css`

**Step 1: Replace the file completely**

```css
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

:root {
  --bg:        #060a0f;
  --sidebar:   #0b1018;
  --surface:   #111820;
  --border:    #1c2530;
  --border-hi: #2a3848;
  --text:      #c8d8e8;
  --muted:     #4e6070;
  --accent:    #00d4ff;
  --running:   #00e676;
  --waiting:   #ffcc02;
  --idle:      #3d5166;
  --stopped:   #2a3848;
  --error:     #ff4560;
}

body {
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 13px;
  background: var(--bg);
  color: var(--text);
  height: 100dvh;
  overflow: hidden;
}

/* ── App shell ─────────────────────────────── */
.app { display: flex; flex-direction: column; height: 100dvh; }

/* ── Header ────────────────────────────────── */
.app-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 0 16px; height: 44px;
  background: var(--bg); border-bottom: 1px solid var(--border);
  flex-shrink: 0; z-index: 20;
}
.header-left  { display: flex; align-items: center; gap: 10px; }
.header-right { display: flex; align-items: center; gap: 12px; }
.logo { font-size: 13px; font-weight: 600; letter-spacing: 0.02em; }

.back-btn {
  display: none; background: none; border: none; color: var(--muted);
  cursor: pointer; font-family: inherit; font-size: 18px;
  padding: 0 4px; line-height: 1; border-radius: 3px;
}
.back-btn:hover { color: var(--text); }

.conn-dot {
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--stopped); transition: background 0.3s;
}
.conn-dot.connected { background: var(--running); }

.logout-btn {
  background: none; border: none; color: var(--muted);
  cursor: pointer; font-family: inherit; font-size: 12px; padding: 0;
}
.logout-btn:hover { color: var(--text); }

/* ── Layout ────────────────────────────────── */
.layout { display: flex; flex: 1; overflow: hidden; }

/* ── Sidebar ───────────────────────────────── */
.sidebar {
  width: 260px; flex-shrink: 0;
  background: var(--sidebar); border-right: 1px solid var(--border);
  overflow-y: auto; overflow-x: hidden;
}

.group-header-row {
  display: flex; align-items: center; justify-content: space-between;
  padding: 18px 14px 6px;
}
.group-label {
  font-size: 10px; font-weight: 600; color: var(--muted);
  text-transform: uppercase; letter-spacing: 0.1em;
}
.group-add-btn {
  background: none; border: none; color: var(--muted);
  cursor: pointer; font-family: inherit; font-size: 16px;
  line-height: 1; padding: 2px 5px; border-radius: 3px;
}
.group-add-btn:hover { color: var(--text); background: var(--surface); }

.session-row {
  display: flex; align-items: center; gap: 8px;
  padding: 8px 14px 8px 12px; cursor: pointer;
  border-left: 2px solid transparent;
  transition: background 0.1s;
}
.session-row:hover { background: rgba(255,255,255,0.03); }
.session-row.active { background: var(--surface); }
.session-row.active.status-running { border-left-color: var(--running); }
.session-row.active.status-waiting { border-left-color: var(--waiting); }
.session-row.active.status-idle    { border-left-color: var(--idle); }
.session-row.active.status-stopped { border-left-color: var(--stopped); }
.session-row.active.status-error   { border-left-color: var(--error); }

.session-row-title {
  flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  font-size: 12px; font-weight: 500;
}
.session-row-tool { font-size: 10px; color: var(--muted); flex-shrink: 0; }

/* Status dots */
.status-dot { font-size: 9px; flex-shrink: 0; line-height: 1; }
.status-dot.running { color: var(--running); filter: drop-shadow(0 0 2px var(--running)); }
.status-dot.waiting { color: var(--waiting); animation: pulse-dot 2s ease-in-out infinite; }
.status-dot.idle    { color: var(--idle); }
.status-dot.stopped { color: var(--stopped); }
.status-dot.error   { color: var(--error); }

@keyframes pulse-dot {
  0%, 100% { opacity: 1; }
  50%       { opacity: 0.35; }
}

/* Create form */
.create-form {
  display: none; padding: 8px 12px 12px;
  background: var(--surface);
  border-top: 1px solid var(--border);
  border-bottom: 1px solid var(--border);
}
.create-form.open { display: block; }
.form-row { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.form-label {
  font-size: 10px; color: var(--muted); width: 36px;
  flex-shrink: 0; text-transform: uppercase; letter-spacing: 0.05em;
}
.form-input, .form-select {
  flex: 1; background: var(--bg); color: var(--text);
  border: 1px solid var(--border); border-radius: 3px;
  padding: 4px 7px; font-family: inherit; font-size: 11px;
}
.form-input:focus, .form-select:focus { outline: none; border-color: var(--accent); }
.form-submit {
  margin-top: 4px; font-size: 11px; padding: 4px 10px;
  background: var(--surface); color: var(--text);
  border: 1px solid var(--border); border-radius: 3px;
  cursor: pointer; font-family: inherit;
}
.form-submit:hover { border-color: var(--accent); color: var(--accent); }

/* ── Detail panel ──────────────────────────── */
.detail-panel {
  flex: 1; display: flex; flex-direction: column;
  overflow: hidden; background: var(--bg);
}
.detail-empty {
  flex: 1; display: flex; align-items: center; justify-content: center;
}
.detail-empty-text { font-size: 11px; color: var(--muted); letter-spacing: 0.06em; }
.detail-content { flex: 1; display: none; flex-direction: column; overflow: hidden; }

/* Detail header */
.detail-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 10px 16px; border-bottom: 1px solid var(--border);
  flex-shrink: 0; transition: background 0.4s; gap: 12px;
}
.detail-header.tint-running { background: rgba(0,230,118,0.04); }
.detail-header.tint-waiting { background: rgba(255,204,2,0.05); }
.detail-header.tint-error   { background: rgba(255,69,96,0.05); }

.detail-title-group { display: flex; align-items: center; gap: 10px; min-width: 0; }
.detail-session-name {
  font-size: 14px; font-weight: 600;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.detail-status-badge {
  font-size: 9px; text-transform: uppercase; letter-spacing: 0.08em;
  padding: 2px 6px; border-radius: 3px; border: 1px solid; flex-shrink: 0;
}
.detail-status-badge.running { color: var(--running); border-color: var(--running); }
.detail-status-badge.waiting {
  color: var(--waiting); border-color: var(--waiting);
  animation: pulse-dot 2s ease-in-out infinite;
}
.detail-status-badge.idle    { color: var(--idle);    border-color: var(--idle); }
.detail-status-badge.stopped { color: var(--stopped); border-color: var(--stopped); }
.detail-status-badge.error   { color: var(--error);   border-color: var(--error); }

.detail-actions { display: flex; gap: 4px; flex-shrink: 0; }

.action-btn {
  font-size: 11px; padding: 3px 8px;
  background: none; color: var(--muted);
  border: 1px solid var(--border); border-radius: 3px;
  cursor: pointer; font-family: inherit;
  transition: color 0.1s, border-color 0.1s;
}
.action-btn:hover { color: var(--text); border-color: var(--border-hi); }
.action-btn.danger:hover { color: var(--error); border-color: var(--error); }

/* Tab bar */
.tab-bar {
  display: flex; border-bottom: 1px solid var(--border);
  padding: 0 16px; flex-shrink: 0; background: var(--surface);
  overflow-x: auto;
}
.tab-btn {
  font-size: 10px; font-weight: 600; letter-spacing: 0.08em;
  text-transform: uppercase; padding: 9px 12px;
  background: none; border: none; border-bottom: 2px solid transparent;
  color: var(--muted); cursor: pointer; font-family: inherit;
  margin-bottom: -1px; white-space: nowrap;
  transition: color 0.15s, border-color 0.15s;
}
.tab-btn:hover { color: var(--text); }
.tab-btn.active { color: var(--accent); border-bottom-color: var(--accent); }

/* Tab content */
.tab-content { flex: 1; overflow: hidden; display: flex; flex-direction: column; }

/* Terminal tab */
.terminal-container { flex: 1; width: 100%; display: flex; flex-direction: column; }
.terminal-iframe { flex: 1; width: 100%; border: none; display: block; }

/* Git tab */
.git-panel { padding: 20px; display: flex; flex-direction: column; gap: 12px; }
.git-btn-row { display: flex; gap: 8px; flex-wrap: wrap; }
.git-btn {
  font-size: 12px; padding: 6px 14px;
  background: var(--surface); color: var(--text);
  border: 1px solid var(--border); border-radius: 3px;
  cursor: pointer; font-family: inherit;
  transition: border-color 0.1s, color 0.1s;
}
.git-btn:hover { border-color: var(--accent); color: var(--accent); }
.dirty-notice { font-size: 11px; color: var(--waiting); }

/* Notes tab */
.notes-panel { display: flex; flex-direction: column; flex: 1; padding: 12px; gap: 8px; }
.notes-area {
  flex: 1; background: var(--surface); color: var(--text);
  border: 1px solid var(--border); border-radius: 4px;
  padding: 10px; font-family: inherit; font-size: 13px; resize: none;
}
.notes-area:focus { outline: none; border-color: var(--accent); }
.save-btn {
  align-self: flex-start; font-size: 11px; padding: 4px 12px;
  background: var(--surface); color: var(--text);
  border: 1px solid var(--border); border-radius: 3px;
  cursor: pointer; font-family: inherit;
}
.save-btn:hover { border-color: var(--accent); color: var(--accent); }

/* Log tab */
.log-panel { padding: 12px 16px; overflow-y: auto; flex: 1; }
.log-title {
  font-size: 10px; color: var(--muted);
  text-transform: uppercase; letter-spacing: 0.08em; margin-bottom: 10px;
}
.log-entry {
  display: flex; gap: 12px; font-size: 11px;
  padding: 4px 0; border-bottom: 1px solid var(--border);
}
.log-ts { color: var(--muted); flex-shrink: 0; }
.log-type { color: var(--text); }

/* ── Login page ────────────────────────────── */
.login-wrap {
  display: flex; align-items: center; justify-content: center; min-height: 80dvh;
}
.login-box {
  background: var(--surface); border: 1px solid var(--border);
  padding: 32px; width: 320px; display: flex; flex-direction: column; gap: 16px;
}
.login-box h2 { font-size: 1rem; color: var(--text); }
.login-box input {
  width: 100%; padding: 8px; background: var(--bg);
  border: 1px solid var(--border); color: var(--text);
  font-family: inherit; font-size: 14px;
}
.login-box input:focus { outline: none; border-color: var(--accent); }
.login-box button {
  padding: 8px; background: var(--surface); border: 1px solid var(--border);
  color: var(--text); font-family: inherit; font-size: 14px; cursor: pointer;
}
.login-box button:hover { border-color: var(--accent); color: var(--accent); }
.error { color: var(--error); font-size: 13px; min-height: 1em; }

/* ── Mobile ────────────────────────────────── */
@media (max-width: 767px) {
  body { overflow: hidden; }
  .back-btn { display: block; }
  .layout { position: relative; }

  .sidebar {
    width: 100%; position: absolute; inset: 0;
    transform: translateX(0);
    transition: transform 0.25s cubic-bezier(0.4,0,0.2,1);
    z-index: 10;
  }
  .sidebar.offscreen { transform: translateX(-100%); }

  .detail-panel {
    position: absolute; inset: 0;
    transform: translateX(100%);
    transition: transform 0.25s cubic-bezier(0.4,0,0.2,1);
    z-index: 10; background: var(--bg);
  }
  .detail-panel.visible { transform: translateX(0); }
}
```

**Step 2: Verify it builds**

```bash
make build
```

Expected: compiles with no errors.

**Step 3: Commit**

```bash
git add internal/webserver/static/style.css
git commit -m "feat(ui): new mission-control design system"
```

---

### Task 3: Rewrite app.js

**Files:**
- Modify: `internal/webserver/static/app.js`

This is a full replacement. The existing auth functions (`getAccessToken`, `refreshAccessToken`, `authFetch`, `logout`) are kept verbatim. Everything else is replaced.

**Step 1: Replace app.js with the full new implementation**

```js
// --- Auth (unchanged) ---
function getAccessToken() { return sessionStorage.getItem('access_token'); }
function getRefreshToken() { return localStorage.getItem('refresh_token'); }
function setTokens(access, refresh) {
  sessionStorage.setItem('access_token', access);
  localStorage.setItem('refresh_token', refresh);
}
function clearTokens() {
  sessionStorage.removeItem('access_token');
  localStorage.removeItem('refresh_token');
}

async function refreshAccessToken() {
  const rt = getRefreshToken();
  if (!rt) return false;
  const res = await fetch('/api/auth/refresh', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refresh_token: rt }),
  });
  if (!res.ok) { clearTokens(); return false; }
  const { access_token, refresh_token } = await res.json();
  setTokens(access_token, refresh_token);
  return true;
}

async function authFetch(url, options = {}) {
  const token = getAccessToken();
  if (!token) { window.location.href = '/login'; return null; }
  options.headers = { ...options.headers, 'Authorization': 'Bearer ' + token };
  let res = await fetch(url, options);
  if (res.status === 401) {
    const ok = await refreshAccessToken();
    if (!ok) { window.location.href = '/login'; return null; }
    options.headers['Authorization'] = 'Bearer ' + getAccessToken();
    res = await fetch(url, options);
  }
  return res;
}

async function logout() {
  const rt = getRefreshToken();
  if (rt) {
    await fetch('/api/auth/logout', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: rt }),
    });
  }
  clearTokens();
  window.location.href = '/login';
}

// --- State ---
const STATUS_ICONS = {
  running: { char: '●', cls: 'running' },
  waiting: { char: '◐', cls: 'waiting' },
  idle:    { char: '○', cls: 'idle' },
  stopped: { char: '◻', cls: 'stopped' },
  error:   { char: '✗', cls: 'error' },
};

let state = { sessions: [], groups: [] };
let selectedSessionID = null;   // replaces expandedSessions Set
let tabState = {};              // { [sessionID]: 'terminal'|'git'|'notes'|'log' }
let openCreateForms = new Set();
let mobileShowDetail = false;
let sseRetryDelay = 1000;

// Module-level iframe cache — survives DOM rebuilds so the terminal doesn't reload.
const savedIframes = {};
// Tracks what is currently rendered in the detail panel to avoid unnecessary rebuilds.
let renderedDetailID = null;

// --- Data fetching ---
async function fetchSessions() {
  const res = await authFetch('/api/sessions');
  if (!res || !res.ok) return;
  state = await res.json();
  if (!state.sessions) state.sessions = [];
  if (!state.groups)   state.groups = [];
  render();
}

async function apiAction(url, method) {
  const res = await authFetch(url, { method });
  if (res && !res.ok) alert(`Failed: ${res.status}`);
  fetchSessions();
}

async function saveNotes(sessionID, notes) {
  await authFetch(`/api/sessions/${sessionID}/notes`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ notes }),
  });
}

function formatTime(tsStr) {
  if (!tsStr) return '';
  return new Date(tsStr).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

async function loadLogEntries(sessionID, container) {
  const res = await authFetch(`/api/sessions/${sessionID}/events`);
  if (!res || !res.ok) return;
  const { events } = await res.json();
  if (!events || !events.length) return;
  events.slice(0, 20).forEach(e => {
    const row = document.createElement('div');
    row.className = 'log-entry';
    row.innerHTML = `<span class="log-ts">${formatTime(e.Ts)}</span><span class="log-type">${e.EventType}</span>`;
    container.appendChild(row);
  });
}

// --- Session selection ---
function selectSession(id) {
  selectedSessionID = id;
  mobileShowDetail = true;
  render();
}

// --- Main render ---
function render() {
  renderSidebar();

  if (selectedSessionID !== renderedDetailID) {
    // Selection changed: kill old ttyd and rebuild detail panel.
    if (renderedDetailID && renderedDetailID !== selectedSessionID) {
      authFetch(`/api/sessions/${renderedDetailID}/ttyd`, { method: 'DELETE' }).catch(() => {});
      delete savedIframes[renderedDetailID];
    }
    renderedDetailID = selectedSessionID;
    rebuildDetail();
  } else {
    // Same session: update status indicators in-place (preserves textarea focus, iframe).
    updateDetailStatus();
  }

  // Mobile: slide sidebar/detail based on mobileShowDetail.
  if (window.innerWidth <= 767) {
    const sidebar = document.getElementById('sidebar');
    const panel = document.getElementById('detail-panel');
    if (mobileShowDetail && selectedSessionID) {
      sidebar.classList.add('offscreen');
      panel.classList.add('visible');
    } else {
      sidebar.classList.remove('offscreen');
      panel.classList.remove('visible');
    }
  }
}

// --- Sidebar rendering ---
function renderSidebar() {
  const list = document.getElementById('session-list');
  list.innerHTML = '';

  const grouped = {};
  const groupOrder = [];
  state.groups.forEach(g => {
    grouped[g.Path] = { group: g, sessions: [] };
    groupOrder.push(g.Path);
  });
  state.sessions.forEach(s => {
    if (!grouped[s.GroupPath]) {
      grouped[s.GroupPath] = { group: { Name: s.GroupPath, Path: s.GroupPath }, sessions: [] };
      groupOrder.push(s.GroupPath);
    }
    grouped[s.GroupPath].sessions.push(s);
  });

  // Deduplicate groupOrder (sessions may reference groups not in state.groups).
  const seen = new Set();
  const uniqueOrder = groupOrder.filter(p => { if (seen.has(p)) return false; seen.add(p); return true; });

  uniqueOrder.forEach(path => {
    const { group, sessions } = grouped[path];
    if (!sessions.length && !openCreateForms.has(path)) return;

    // Group header row
    const headerRow = document.createElement('div');
    headerRow.className = 'group-header-row';

    const label = document.createElement('span');
    label.className = 'group-label';
    label.textContent = group.Name || group.Path;

    const addBtn = document.createElement('button');
    addBtn.className = 'group-add-btn';
    addBtn.textContent = '+';
    addBtn.title = 'New session';
    addBtn.onclick = (e) => {
      e.stopPropagation();
      if (openCreateForms.has(path)) openCreateForms.delete(path);
      else openCreateForms.add(path);
      renderSidebar();
    };

    headerRow.appendChild(label);
    headerRow.appendChild(addBtn);
    list.appendChild(headerRow);
    list.appendChild(buildCreateForm(path));

    // Session rows
    sessions.forEach(s => {
      const icon = STATUS_ICONS[s.Status] || STATUS_ICONS.idle;
      const isActive = s.ID === selectedSessionID;
      const row = document.createElement('div');
      row.setAttribute('data-session-id', s.ID);
      row.className = `session-row${isActive ? ' active status-' + s.Status : ''}`;
      row.innerHTML = `
        <span class="status-dot ${icon.cls}">${icon.char}</span>
        <span class="session-row-title">${s.HasUncommitted ? '* ' : ''}${s.Title}</span>
        <span class="session-row-tool">${s.Tool}</span>
      `;
      row.onclick = () => selectSession(s.ID);
      list.appendChild(row);
    });
  });
}

// --- Create form (inline in sidebar) ---
function buildCreateForm(groupPath) {
  const form = document.createElement('div');
  form.className = 'create-form' + (openCreateForms.has(groupPath) ? ' open' : '');

  const mk = (labelText, el) => {
    const row = document.createElement('div');
    row.className = 'form-row';
    const lbl = document.createElement('span');
    lbl.className = 'form-label';
    lbl.textContent = labelText;
    row.appendChild(lbl);
    row.appendChild(el);
    return row;
  };

  const titleInput = document.createElement('input');
  titleInput.type = 'text'; titleInput.className = 'form-input'; titleInput.placeholder = 'auto';

  const toolSelect = document.createElement('select');
  toolSelect.className = 'form-select';
  ['claude', 'opencode', 'gemini', 'codex', 'custom', 'shell'].forEach(t => {
    const opt = document.createElement('option');
    opt.value = t; opt.textContent = t; toolSelect.appendChild(opt);
  });

  const pathInput = document.createElement('input');
  pathInput.type = 'text'; pathInput.className = 'form-input'; pathInput.placeholder = 'optional';

  const submitBtn = document.createElement('button');
  submitBtn.className = 'form-submit';
  submitBtn.textContent = 'Create';
  submitBtn.onclick = async (e) => {
    e.stopPropagation();
    const res = await authFetch('/api/sessions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        title: titleInput.value.trim() || '',
        tool: toolSelect.value,
        group_path: groupPath,
        project_path: pathInput.value.trim(),
      }),
    });
    if (res && !res.ok) alert(`Create failed: ${res.status}`);
    else openCreateForms.delete(groupPath);
    fetchSessions();
  };

  form.appendChild(mk('Title', titleInput));
  form.appendChild(mk('Tool', toolSelect));
  form.appendChild(mk('Path', pathInput));
  form.appendChild(submitBtn);
  return form;
}

// --- Detail panel ---
function rebuildDetail() {
  const emptyEl  = document.getElementById('detail-empty');
  const contentEl = document.getElementById('detail-content');

  if (!selectedSessionID) {
    emptyEl.style.display = '';
    contentEl.style.display = 'none';
    contentEl.innerHTML = '';
    return;
  }

  const s = state.sessions.find(s => s.ID === selectedSessionID);
  if (!s) {
    selectedSessionID = null;
    renderedDetailID = null;
    emptyEl.style.display = '';
    contentEl.style.display = 'none';
    contentEl.innerHTML = '';
    return;
  }

  emptyEl.style.display = 'none';
  contentEl.style.display = 'flex';
  contentEl.innerHTML = '';

  // Default tab: terminal if the session has one, otherwise notes.
  if (!tabState[s.ID]) tabState[s.ID] = s.TmuxSession ? 'terminal' : 'notes';
  const tab = tabState[s.ID];

  // Header
  contentEl.appendChild(buildDetailHeader(s));

  // Tab bar
  const tabBar = document.createElement('div');
  tabBar.className = 'tab-bar';
  const tabs = s.TmuxSession
    ? ['terminal', 'git', 'notes', 'log']
    : ['git', 'notes', 'log'];
  tabs.forEach(t => {
    const btn = document.createElement('button');
    btn.className = 'tab-btn' + (t === tab ? ' active' : '');
    btn.textContent = t.toUpperCase();
    btn.onclick = () => { tabState[s.ID] = t; rebuildDetail(); };
    tabBar.appendChild(btn);
  });
  contentEl.appendChild(tabBar);

  // Tab content
  const tabContent = document.createElement('div');
  tabContent.className = 'tab-content';
  renderTabContent(s, tab, tabContent);
  contentEl.appendChild(tabContent);
}

function buildDetailHeader(s) {
  const tints = { running: 'tint-running', waiting: 'tint-waiting', error: 'tint-error' };
  const header = document.createElement('div');
  header.className = 'detail-header' + (tints[s.Status] ? ' ' + tints[s.Status] : '');

  const titleGroup = document.createElement('div');
  titleGroup.className = 'detail-title-group';

  const nameEl = document.createElement('span');
  nameEl.className = 'detail-session-name';
  nameEl.textContent = (s.HasUncommitted ? '* ' : '') + s.Title;

  const badge = document.createElement('span');
  badge.className = `detail-status-badge ${s.Status}`;
  badge.textContent = s.Status;

  titleGroup.appendChild(nameEl);
  titleGroup.appendChild(badge);

  const actions = document.createElement('div');
  actions.className = 'detail-actions';

  const mkBtn = (label, danger, onClick) => {
    const btn = document.createElement('button');
    btn.className = 'action-btn' + (danger ? ' danger' : '');
    btn.textContent = label;
    btn.onclick = (e) => { e.stopPropagation(); onClick(); };
    return btn;
  };

  if (s.Status !== 'stopped') {
    actions.appendChild(mkBtn('Stop', false, () => {
      if (confirm(`Stop "${s.Title}"?`)) apiAction(`/api/sessions/${s.ID}/stop`, 'POST');
    }));
  }
  actions.appendChild(mkBtn('Restart', false, () => {
    if (confirm(`Restart "${s.Title}"?`)) apiAction(`/api/sessions/${s.ID}/restart`, 'POST');
  }));
  actions.appendChild(mkBtn('Delete', true, () => {
    if (confirm(`Delete "${s.Title}"?`)) {
      authFetch(`/api/sessions/${s.ID}/ttyd`, { method: 'DELETE' }).catch(() => {});
      delete savedIframes[s.ID];
      selectedSessionID = null;
      renderedDetailID = null;
      apiAction(`/api/sessions/${s.ID}`, 'DELETE');
    }
  }));

  header.appendChild(titleGroup);
  header.appendChild(actions);
  return header;
}

function renderTabContent(s, tab, container) {
  if (tab === 'terminal') {
    if (!s.TmuxSession) {
      container.innerHTML = '<div style="padding:20px;color:var(--muted);font-size:12px">No tmux session.</div>';
      return;
    }
    const termContainer = document.createElement('div');
    termContainer.className = 'terminal-container';
    if (savedIframes[s.ID]) {
      termContainer.appendChild(savedIframes[s.ID]);
    } else {
      const iframe = document.createElement('iframe');
      iframe.src = `/terminal/${s.ID}/`;
      iframe.className = 'terminal-iframe';
      termContainer.appendChild(iframe);
      savedIframes[s.ID] = iframe;
    }
    container.appendChild(termContainer);
    return;
  }

  if (tab === 'git') {
    const panel = document.createElement('div');
    panel.className = 'git-panel';
    if (s.HasUncommitted) {
      const notice = document.createElement('div');
      notice.className = 'dirty-notice';
      notice.textContent = '● uncommitted changes';
      panel.appendChild(notice);
    }
    if (s.ProjectPath || s.WorktreePath) {
      const token = getAccessToken();
      const row = document.createElement('div');
      row.className = 'git-btn-row';
      const mkGitBtn = (label, onClick) => {
        const btn = document.createElement('button');
        btn.className = 'git-btn'; btn.textContent = label; btn.onclick = onClick;
        return btn;
      };
      row.appendChild(mkGitBtn('Git Status', () =>
        window.open(`/api/sessions/${s.ID}/git/status?token=${encodeURIComponent(token)}`, '_blank')));
      row.appendChild(mkGitBtn('Git Diff', () =>
        window.open(`/api/sessions/${s.ID}/git/diff?token=${encodeURIComponent(token)}`, '_blank')));
      row.appendChild(mkGitBtn('Open PR', async () => {
        const res = await authFetch(`/api/sessions/${s.ID}/pr-url`);
        if (!res || !res.ok) { alert('No open PR found for this branch.'); return; }
        const { url } = await res.json();
        window.open(url, '_blank');
      }));
      panel.appendChild(row);
    } else {
      const msg = document.createElement('div');
      msg.style.cssText = 'font-size:12px;color:var(--muted)';
      msg.textContent = 'No git working directory for this session.';
      panel.appendChild(msg);
    }
    container.appendChild(panel);
    return;
  }

  if (tab === 'notes') {
    const panel = document.createElement('div');
    panel.className = 'notes-panel';
    const textarea = document.createElement('textarea');
    textarea.className = 'notes-area';
    textarea.value = s.Notes || '';
    textarea.placeholder = 'Notes...';
    const saveBtn = document.createElement('button');
    saveBtn.className = 'save-btn';
    saveBtn.textContent = 'Save notes';
    saveBtn.onclick = () => saveNotes(s.ID, textarea.value);
    panel.appendChild(textarea);
    panel.appendChild(saveBtn);
    container.appendChild(panel);
    return;
  }

  if (tab === 'log') {
    const panel = document.createElement('div');
    panel.className = 'log-panel';
    const title = document.createElement('div');
    title.className = 'log-title';
    title.textContent = 'Activity';
    panel.appendChild(title);
    loadLogEntries(s.ID, panel);
    container.appendChild(panel);
  }
}

// In-place status update — called when the selected session hasn't changed
// but its Status field may have been updated by SSE.
function updateDetailStatus() {
  if (!selectedSessionID) return;
  const s = state.sessions.find(s => s.ID === selectedSessionID);
  if (!s) return;

  // Update sidebar dot for this session.
  const row = document.querySelector(`.session-row[data-session-id="${s.ID}"]`);
  if (row) {
    const icon = STATUS_ICONS[s.Status] || STATUS_ICONS.idle;
    const dot = row.querySelector('.status-dot');
    if (dot) { dot.className = `status-dot ${icon.cls}`; dot.textContent = icon.char; }
  }

  // Update detail header tint and badge.
  const header = document.querySelector('#detail-content .detail-header');
  if (!header) return;
  const tints = { running: 'tint-running', waiting: 'tint-waiting', error: 'tint-error' };
  header.className = 'detail-header' + (tints[s.Status] ? ' ' + tints[s.Status] : '');
  const badge = header.querySelector('.detail-status-badge');
  if (badge) { badge.className = `detail-status-badge ${s.Status}`; badge.textContent = s.Status; }
}

// --- SSE ---
function connectSSE() {
  const token = getAccessToken();
  if (!token) { window.location.href = '/login'; return; }
  const es = new EventSource('/events?token=' + encodeURIComponent(token));
  const dot = document.getElementById('connection-status');

  es.onopen = () => { dot.className = 'conn-dot connected'; sseRetryDelay = 1000; };

  es.onmessage = (e) => {
    const evt = JSON.parse(e.data);
    if (evt.type === 'snapshot' || evt.type === 'refresh') {
      fetchSessions();
    } else if (evt.type === 'status_changed') {
      const s = state.sessions.find(s => s.ID === evt.session_id);
      if (s) {
        s.Status = evt.status;
        // Update sidebar dot in-place.
        const row = document.querySelector(`.session-row[data-session-id="${evt.session_id}"]`);
        if (row) {
          const icon = STATUS_ICONS[evt.status] || STATUS_ICONS.idle;
          const dot = row.querySelector('.status-dot');
          if (dot) { dot.className = `status-dot ${icon.cls}`; dot.textContent = icon.char; }
        }
        // If this is the selected session, update the detail header in-place too.
        if (evt.session_id === selectedSessionID) updateDetailStatus();
      } else {
        fetchSessions();
      }
    } else {
      fetchSessions();
    }
  };

  es.onerror = () => {
    dot.className = 'conn-dot';
    es.close();
    refreshAccessToken().then(ok => {
      if (!ok) { window.location.href = '/login'; return; }
      setTimeout(connectSSE, sseRetryDelay);
      sseRetryDelay = Math.min(sseRetryDelay * 2, 30000);
    });
  };
}

// --- Boot ---
document.addEventListener('DOMContentLoaded', async () => {
  if (!getAccessToken() && !getRefreshToken()) { window.location.href = '/login'; return; }
  if (!getAccessToken() && getRefreshToken()) {
    const ok = await refreshAccessToken();
    if (!ok) { window.location.href = '/login'; return; }
  }

  const logoutBtn = document.getElementById('logout-btn');
  if (logoutBtn) logoutBtn.onclick = logout;

  const backBtn = document.getElementById('back-btn');
  if (backBtn) backBtn.onclick = () => { mobileShowDetail = false; render(); };

  fetchSessions();
  connectSSE();
});
```

**Step 2: Verify it builds**

```bash
make build
```

Expected: compiles with no errors.

**Step 3: Smoke test the UI**

```bash
./agent-workspace
```

Open the web UI. Verify:
- Split-pane layout: sidebar on left, empty state on right ("select a session")
- Clicking a session loads the detail panel with header, tab bar, and terminal (if applicable)
- Status dot pulses for waiting sessions
- Tab switching works: TERMINAL | GIT | NOTES | LOG
- GIT tab shows Git Status / Git Diff / Open PR buttons for sessions with a path
- Notes tab: type something, switch tabs, switch back — typed text is gone (expected: notes are rebuilt on selection change; this is fine)
- Deleting a session clears the detail panel
- Connection dot in header turns green

**Step 4: Verify mobile layout**

Resize browser to <768px width. Verify:
- Sidebar fills the screen
- Tapping a session slides the detail panel in
- Back button (←) slides back to sidebar

**Step 5: Commit**

```bash
git add internal/webserver/static/app.js
git commit -m "feat(ui): split-pane layout with tabbed detail panel"
```

---

### Task 4: Final build and install

**Step 1: Run full test suite**

```bash
make test
```

Expected: all pass (no backend changes were made).

**Step 2: Build and install**

```bash
make install
```

Expected: installs to `~/.local/bin/agent-workspace`.

**Step 3: Commit (if any fixups needed)**

```bash
git add -p
git commit -m "fix(ui): <describe any fixup>"
```
