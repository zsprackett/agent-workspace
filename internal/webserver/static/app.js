const STATUS_ICONS = {
  running: { char: '●', cls: 'running' },
  waiting: { char: '◐', cls: 'waiting' },
  idle:    { char: '○', cls: 'idle' },
  stopped: { char: '◻', cls: 'stopped' },
  error:   { char: '✗', cls: 'error' },
};

let state = { sessions: [], groups: [] };
let expandedSessions = new Set();
let sseRetryDelay = 1000;
// Track open create-forms per group path
let openCreateForms = new Set();

async function fetchSessions() {
  const res = await fetch('/api/sessions');
  if (!res.ok) return;
  state = await res.json();
  if (!state.sessions) state.sessions = [];
  if (!state.groups)   state.groups = [];
  render();
}

function connectSSE() {
  const es = new EventSource('/events');
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
      if (s) { s.Status = evt.status; render(); }
      else    { fetchSessions(); }
    } else {
      fetchSessions();
    }
  };
  es.onerror = () => {
    dot.className = '';
    es.close();
    setTimeout(connectSSE, sseRetryDelay);
    sseRetryDelay = Math.min(sseRetryDelay * 2, 30000);
  };
}

function formatTime(tsStr) {
  if (!tsStr) return '';
  const d = new Date(tsStr);
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

async function loadEvents(sessionID, container) {
  const res = await fetch(`/api/sessions/${sessionID}/events`);
  if (!res.ok) return;
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
  await fetch(`/api/sessions/${sessionID}/notes`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ notes }),
  });
}

async function apiAction(url, method) {
  const res = await fetch(url, { method });
  if (!res.ok) alert(`Failed: ${res.status}`);
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
    const res = await fetch('/api/sessions', {
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

  // Close WebSockets for sessions that are no longer expanded before clearing DOM.
  list.querySelectorAll('.session-detail[data-session-id]').forEach(detail => {
    const sid = detail.dataset.sessionId;
    if (!expandedSessions.has(sid) && detail._ws) {
      detail._ws.close();
    }
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
          actions.appendChild(mkBtn('Stop', false, () => apiAction(`/api/sessions/${s.ID}/stop`, 'POST')));
        }
        actions.appendChild(mkBtn('Restart', false, () => apiAction(`/api/sessions/${s.ID}/restart`, 'POST')));
        actions.appendChild(mkBtn('Delete', true, () => {
          if (confirm(`Delete session "${s.Title}"?`)) {
            expandedSessions.delete(s.ID);
            apiAction(`/api/sessions/${s.ID}`, 'DELETE');
          }
        }));
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

        // xterm.js terminal (only for sessions with a tmux session)
        if (s.TmuxSession) {
          const termContainer = document.createElement('div');
          termContainer.className = 'terminal-container';
          detail.appendChild(termContainer);

          const term = new Terminal({
            theme: { background: '#0d1117', foreground: '#e6edf3' },
            cursorBlink: true,
          });
          const fitAddon = new FitAddon.FitAddon();
          term.loadAddon(fitAddon);
          term.open(termContainer);
          fitAddon.fit();

          const wsProto = location.protocol === 'https:' ? 'wss:' : 'ws:';
          const ws = new WebSocket(`${wsProto}//${location.host}/ws/sessions/${s.ID}/terminal`);
          ws.binaryType = 'arraybuffer';
          ws.onmessage = (e) => {
            if (e.data instanceof ArrayBuffer) {
              term.write(new Uint8Array(e.data));
            } else {
              term.write(e.data);
            }
          };
          ws.onclose = () => term.write('\r\n[disconnected]\r\n');
          term.onData(data => {
            if (ws.readyState === WebSocket.OPEN) {
              ws.send(JSON.stringify({ type: 'input', data }));
            }
          });
          detail._ws = ws;
        }

        loadEvents(s.ID, detail);
      }

      row.onclick = () => {
        if (expandedSessions.has(s.ID)) {
          // Close WebSocket if open
          if (detail._ws) detail._ws.close();
          expandedSessions.delete(s.ID);
        } else {
          expandedSessions.add(s.ID);
        }
        render();
      };

      list.appendChild(row);
      list.appendChild(detail);
    });
  });
}

document.addEventListener('DOMContentLoaded', () => {
  fetchSessions();
  connectSSE();
});
