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
