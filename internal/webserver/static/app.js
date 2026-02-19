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

function render() {
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

  groupOrder.forEach(path => {
    const { group, sessions } = grouped[path];
    if (!sessions.length) return;

    const header = document.createElement('div');
    header.className = 'group-header';
    header.textContent = group.Name || group.Path;
    list.appendChild(header);

    sessions.forEach(s => {
      const icon = STATUS_ICONS[s.Status] || STATUS_ICONS.idle;
      const row = document.createElement('div');
      row.className = 'session-row' + (expandedSessions.has(s.ID) ? ' expanded' : '');

      row.innerHTML = `
        <span class="status-dot ${icon.cls}">${icon.char}</span>
        <span class="session-title">${s.HasUncommitted ? '* ' : ''}${s.Title}</span>
        <span class="session-tool">${s.Tool}</span>
      `;

      const detail = document.createElement('div');
      detail.className = 'session-detail' + (expandedSessions.has(s.ID) ? ' open' : '');

      if (expandedSessions.has(s.ID)) {
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
        loadEvents(s.ID, detail);
      }

      row.onclick = () => {
        if (expandedSessions.has(s.ID)) {
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
