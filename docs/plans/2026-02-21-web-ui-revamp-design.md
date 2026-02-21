# Web UI Revamp Design

**Date:** 2026-02-21
**Goal:** Replace the current inline-expand session list with a split-pane mission-control UI that is more usable on desktop and still works on mobile.

---

## Layout

### Desktop (>768px)

```
┌─────────────────────────────────────────────────────────┐
│ header: logo · connection dot · logout                   │
├──────────────┬──────────────────────────────────────────┤
│ SIDEBAR      │ DETAIL PANEL                             │
│ 260px fixed  │ flex-1                                   │
│              │                                           │
│ [GROUP A]  + │ [session title] [status badge] [actions] │
│  ● session1  │ ──────────────────────────────────────── │
│  ◐ session2  │ TERMINAL · GIT · NOTES · LOG             │
│              │                                           │
│ [GROUP B]  + │ [tab content — full remaining height]    │
│  ○ session3  │                                           │
└──────────────┴──────────────────────────────────────────┘
```

- Sidebar: fixed 260px, independently scrollable
- Detail panel: flex-1, fills remaining width and height
- Both panels sit beneath a single slim header bar
- Clicking `+` beside a group header opens a compact inline create-session form in the sidebar

### Mobile (<768px)

- Sidebar fills full screen by default
- Tapping a session slides the detail panel in overtop (CSS transform transition)
- Detail panel header shows `←` back button to return to sidebar
- Tab bar in detail panel scrolls horizontally if needed

---

## Visual Design

### Color Palette

```css
--bg:           #060a0f;   /* near-black, deep navy */
--sidebar:      #0b1018;   /* sidebar background */
--surface:      #111820;   /* panels, cards */
--border:       #1c2530;   /* default borders */
--border-hi:    #2a3848;   /* hover/active borders */
--text:         #c8d8e8;   /* cool off-white */
--muted:        #4e6070;   /* secondary text */
--accent:       #00d4ff;   /* cyan — selection, interactive */
--running:      #00e676;   /* electric green */
--waiting:      #ffcc02;   /* amber */
--idle:         #3d5166;   /* dim blue-grey */
--stopped:      #2a3848;   /* very dim */
--error:        #ff4560;   /* red */
```

### Typography

- Font: **JetBrains Mono** (Google Fonts), used throughout
- Logo/brand: 14px weight 600
- Group headers: 10px uppercase, letter-spacing 0.1em, `--muted`
- Session titles: 13px regular
- Tab labels: 11px uppercase, letter-spacing 0.08em

### Status Indicators

- **Running:** solid green dot with a faint `box-shadow` glow
- **Waiting:** amber dot with a CSS `@keyframes` pulse (opacity + scale, 2s ease-in-out infinite)
- **Idle:** dim grey dot, no animation
- **Stopped:** very dim dot
- **Error:** red dot

### Detail Panel Header

When a session is selected, the header band receives a very subtle status tint:
- Waiting session → faint amber wash (`rgba(255,204,2,0.05)`)
- Running session → faint green wash (`rgba(0,230,118,0.05)`)
- Others → no tint

### Selected Session in Sidebar

Active session row gets a 2px left border in the session's status color, and a slightly lighter background.

---

## Detail Panel Tabs

Four tabs per session:

| Tab | Content |
|-----|---------|
| **TERMINAL** | ttyd iframe (full remaining height). Default tab when `TmuxSession` is set. |
| **GIT** | Git Status and Git Diff buttons open new tabs. Open PR button. Dirty-tree indicator if `HasUncommitted`. |
| **NOTES** | Textarea + Save button. Full height, resizable. |
| **LOG** | Activity event list (existing event-log UI, styled to match). |

Action buttons (Stop / Restart / Delete) sit in the detail panel header, top-right, small text buttons that turn red on hover for Delete.

---

## New Session Form

- A `+` button appears in each group header row (right-aligned)
- Clicking opens a compact inline form directly below the group header in the sidebar
- Fields: Title (optional), Tool (select), Path (optional)
- Mobile: same behavior, form scrolls with the sidebar

---

## Implementation Scope

Files to modify:
- `internal/webserver/static/style.css` — full rewrite
- `internal/webserver/static/index.html` — new structure (sidebar + detail panel divs)
- `internal/webserver/static/app.js` — split-pane logic, mobile toggle, tab state, selected session state

No backend changes required.
