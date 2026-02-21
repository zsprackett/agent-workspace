// --- Auth ---
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

const STATUS_ICONS = {
  running: { char: '●', cls: 'running' },
  waiting: { char: '◐', cls: 'waiting' },
  idle:    { char: '○', cls: 'idle' },
  stopped: { char: '◻', cls: 'stopped' },
  error:   { char: '✗', cls: 'error' },
};

let state = { sessions: [], groups: [] };
let expandedSessions = new Set();
let expandedTerminals = new Set();
let sseRetryDelay = 1000;
// Track open create-forms per group path
let openCreateForms = new Set();

async function fetchSessions() {
  const res = await authFetch('/api/sessions');
  if (!res) return;
  if (!res.ok) return;
  state = await res.json();
  if (!state.sessions) state.sessions = [];
  if (!state.groups)   state.groups = [];
  render();
}

function connectSSE() {
  const token = getAccessToken();
  if (!token) { window.location.href = '/login'; return; }
  const es = new EventSource('/events?token=' + encodeURIComponent(token));
  const dot = document.getElementById('connection-status');

  es.onopen = () => {
    dot.className = 'connected';
    sseRetryDelay = 1000;
  };
  es.onmessage = (e) => {
    const evt = JSON.parse(e.data);
    if (evt.type === 'snapshot' || evt.type === 'refresh') {
      fetchSessions();
    } else if (evt.type === 'status_changed') {
      const s = state.sessions.find(s => s.ID === evt.session_id);
      if (s) {
        s.Status = evt.status;
        // Update only the status dot in-place -- avoid a full re-render and
        // the iframe teardown that causes visible flicker every 500ms.
        const row = document.querySelector(`.session-row[data-session-id="${evt.session_id}"]`);
        if (row) {
          const icon = STATUS_ICONS[evt.status] || STATUS_ICONS.idle;
          const dot = row.querySelector('.status-dot');
          if (dot) { dot.className = `status-dot ${icon.cls}`; dot.textContent = icon.char; }
        } else {
          render();
        }
      } else {
        fetchSessions();
      }
    } else {
      fetchSessions();
    }
  };
  es.onerror = () => {
    dot.className = '';
    es.close();
    refreshAccessToken().then(ok => {
      if (!ok) { window.location.href = '/login'; return; }
      setTimeout(connectSSE, sseRetryDelay);
      sseRetryDelay = Math.min(sseRetryDelay * 2, 30000);
    });
  };
}

function formatTime(tsStr) {
  if (!tsStr) return '';
  const d = new Date(tsStr);
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

async function loadEvents(sessionID, container) {
  const res = await authFetch(`/api/sessions/${sessionID}/events`);
  if (!res || !res.ok) return;
  const { events } = await res.json();
  if (!events || events.length === 0) return;

  const log = document.createElement('div');
  log.className = 'event-log';
  log.innerHTML = '<div class="event-log-title">Activity</div>';
  events.slice(0, 10).forEach(e => {
    const row = document.createElement('div');
    row.className = 'event-entry';
    row.innerHTML = `<span class="event-ts">${formatTime(e.Ts)}</span><span class="event-type">${e.EventType}</span>`;
    log.appendChild(row);
  });
  container.appendChild(log);
}

async function saveNotes(sessionID, notes) {
  await authFetch(`/api/sessions/${sessionID}/notes`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ notes }),
  });
}

async function apiAction(url, method) {
  const res = await authFetch(url, { method });
  if (res && !res.ok) alert(`Failed: ${res.status}`);
  fetchSessions();
}

function mkBtn(label, isDanger, onClick) {
  const btn = document.createElement('button');
  btn.className = 'action-btn' + (isDanger ? ' danger' : '');
  btn.textContent = label;
  btn.onclick = (e) => { e.stopPropagation(); onClick(); };
  return btn;
}

function buildCreateForm(groupPath) {
  const form = document.createElement('div');
  form.className = 'create-session-form' + (openCreateForms.has(groupPath) ? ' open' : '');

  const titleRow = document.createElement('div');
  titleRow.className = 'form-row';
  titleRow.innerHTML = '<span class="form-label">Title</span>';
  const titleInput = document.createElement('input');
  titleInput.type = 'text';
  titleInput.className = 'form-input';
  titleInput.placeholder = 'auto';
  titleRow.appendChild(titleInput);

  const toolRow = document.createElement('div');
  toolRow.className = 'form-row';
  toolRow.innerHTML = '<span class="form-label">Tool</span>';
  const toolSelect = document.createElement('select');
  toolSelect.className = 'form-select';
  ['claude', 'opencode', 'gemini', 'codex', 'custom', 'shell'].forEach(t => {
    const opt = document.createElement('option');
    opt.value = t;
    opt.textContent = t;
    toolSelect.appendChild(opt);
  });
  toolRow.appendChild(toolSelect);

  const pathRow = document.createElement('div');
  pathRow.className = 'form-row';
  pathRow.innerHTML = '<span class="form-label">Path</span>';
  const pathInput = document.createElement('input');
  pathInput.type = 'text';
  pathInput.className = 'form-input';
  pathInput.placeholder = 'project path (optional)';
  pathRow.appendChild(pathInput);

  const submitBtn = document.createElement('button');
  submitBtn.className = 'action-btn';
  submitBtn.textContent = 'Create';
  submitBtn.onclick = async (e) => {
    e.stopPropagation();
    const body = {
      title: titleInput.value.trim() || '',
      tool: toolSelect.value,
      group_path: groupPath,
      project_path: pathInput.value.trim(),
    };
    const res = await authFetch('/api/sessions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      alert(`Create failed: ${res.status}`);
    } else {
      openCreateForms.delete(groupPath);
    }
    fetchSessions();
  };

  form.appendChild(titleRow);
  form.appendChild(toolRow);
  form.appendChild(pathRow);
  form.appendChild(submitBtn);
  return form;
}

function render() {
  const list = document.getElementById('session-list');

  // Save live terminal iframes so they survive the DOM rebuild without reloading.
  const savedIframes = {};
  list.querySelectorAll('.session-detail[data-session-id]').forEach(detail => {
    const sid = detail.dataset.sessionId;
    const iframe = detail.querySelector('.terminal-iframe');
    if (iframe) savedIframes[sid] = iframe;
  });

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

  groupOrder.forEach(path => {
    const { group, sessions } = grouped[path];
    if (!sessions.length && !openCreateForms.has(path)) return;

    const headerRow = document.createElement('div');
    headerRow.className = 'group-header-row';

    const headerLabel = document.createElement('span');
    headerLabel.className = 'group-header';
    headerLabel.style.padding = '0';
    headerLabel.textContent = group.Name || group.Path;
    headerRow.appendChild(headerLabel);

    const newBtn = mkBtn('+ New session', false, () => {
      if (openCreateForms.has(path)) {
        openCreateForms.delete(path);
      } else {
        openCreateForms.add(path);
      }
      render();
    });
    headerRow.appendChild(newBtn);
    list.appendChild(headerRow);

    const createForm = buildCreateForm(path);
    list.appendChild(createForm);

    sessions.forEach(s => {
      const icon = STATUS_ICONS[s.Status] || STATUS_ICONS.idle;
      const row = document.createElement('div');
      row.setAttribute('data-session-id', s.ID);
      row.className = 'session-row' + (expandedSessions.has(s.ID) ? ' expanded' : '');

      row.innerHTML = `
        <span class="status-dot ${icon.cls}">${icon.char}</span>
        <span class="session-title">${s.HasUncommitted ? '* ' : ''}${s.Title}</span>
        <span class="session-dims"></span>
        <span class="session-tool">${s.Tool}</span>
      `;

      const detail = document.createElement('div');
      detail.className = 'session-detail' + (expandedSessions.has(s.ID) ? ' open' : '');
      detail.setAttribute('data-session-id', s.ID);

      if (expandedSessions.has(s.ID)) {
        // Action buttons
        const actions = document.createElement('div');
        actions.className = 'session-actions';

        if (s.Status !== 'stopped') {
          actions.appendChild(mkBtn('Stop', false, () => {
            if (confirm(`Stop session "${s.Title}"?`)) apiAction(`/api/sessions/${s.ID}/stop`, 'POST');
          }));
        }
        actions.appendChild(mkBtn('Restart', false, () => {
          if (confirm(`Restart session "${s.Title}"?`)) apiAction(`/api/sessions/${s.ID}/restart`, 'POST');
        }));
        actions.appendChild(mkBtn('Delete', true, () => {
          if (confirm(`Delete session "${s.Title}"?`)) {
            expandedSessions.delete(s.ID);
            expandedTerminals.delete(s.ID);
            apiAction(`/api/sessions/${s.ID}`, 'DELETE');
          }
        }));

        // Git / terminal actions (only for sessions with a git working directory)
        if (s.ProjectPath || s.WorktreePath) {
          const token = getAccessToken();
          actions.appendChild(mkBtn('Git Status', false, () => {
            window.open(`/api/sessions/${s.ID}/git/status?token=${encodeURIComponent(token)}`, '_blank');
          }));
          actions.appendChild(mkBtn('Git Diff', false, () => {
            window.open(`/api/sessions/${s.ID}/git/diff?token=${encodeURIComponent(token)}`, '_blank');
          }));
          actions.appendChild(mkBtn('Open PR', false, async () => {
            const res = await authFetch(`/api/sessions/${s.ID}/pr-url`);
            if (!res || !res.ok) {
              alert('No open PR found for this branch.');
              return;
            }
            const { url } = await res.json();
            window.open(url, '_blank');
          }));
        }

        if (s.TmuxSession) {
          const isTermOpen = expandedTerminals.has(s.ID);
          actions.appendChild(mkBtn(isTermOpen ? 'Hide Terminal' : 'Terminal', false, () => {
            if (expandedTerminals.has(s.ID)) {
              authFetch(`/api/sessions/${s.ID}/ttyd`, { method: 'DELETE' }).catch(() => {});
              expandedTerminals.delete(s.ID);
            } else {
              expandedTerminals.add(s.ID);
            }
            render();
          }));
        }

        detail.appendChild(actions);

        // Notes
        const textarea = document.createElement('textarea');
        textarea.className = 'notes-area';
        textarea.value = s.Notes || '';
        textarea.placeholder = 'Notes...';

        const saveBtn = document.createElement('button');
        saveBtn.className = 'save-btn';
        saveBtn.textContent = 'Save notes';
        saveBtn.onclick = (e) => {
          e.stopPropagation();
          saveNotes(s.ID, textarea.value);
        };

        detail.appendChild(textarea);
        detail.appendChild(saveBtn);

        // Terminal iframe -- populated after DOM append below.
        if (s.TmuxSession && expandedTerminals.has(s.ID)) {
          const termContainer = document.createElement('div');
          termContainer.className = 'terminal-container';
          detail.appendChild(termContainer);
        }

        loadEvents(s.ID, detail);
      }

      row.onclick = () => {
        if (expandedSessions.has(s.ID)) {
          authFetch(`/api/sessions/${s.ID}/ttyd`, { method: 'DELETE' }).catch(() => {});
          expandedSessions.delete(s.ID);
          expandedTerminals.delete(s.ID);
        } else {
          expandedSessions.add(s.ID);
        }
        render();
      };

      list.appendChild(row);
      list.appendChild(detail);

      // Spawn ttyd and embed it in an iframe.
      if (expandedSessions.has(s.ID) && expandedTerminals.has(s.ID) && s.TmuxSession) {
        const termContainer = detail.querySelector('.terminal-container');
        if (termContainer) {
          if (savedIframes[s.ID]) {
            // Reuse the existing iframe -- avoids terminal reload on re-render.
            termContainer.appendChild(savedIframes[s.ID]);
          } else {
            const iframe = document.createElement('iframe');
            // Proxied through our server so remote clients (iOS etc.) can reach it.
            iframe.src = `/terminal/${s.ID}/`;
            iframe.className = 'terminal-iframe';
            termContainer.appendChild(iframe);
          }
        }
      }
    });
  });
}

document.addEventListener('DOMContentLoaded', async () => {
  // If no tokens at all, redirect to login
  if (!getAccessToken() && !getRefreshToken()) {
    window.location.href = '/login';
    return;
  }
  // If we have a refresh token but no access token, try a silent refresh
  if (!getAccessToken() && getRefreshToken()) {
    const ok = await refreshAccessToken();
    if (!ok) { window.location.href = '/login'; return; }
  }

  const logoutBtn = document.getElementById('logout-btn');
  if (logoutBtn) logoutBtn.onclick = logout;

  fetchSessions();
  connectSSE();
});
