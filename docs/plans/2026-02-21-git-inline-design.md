# Git Inline Display Design

**Date:** 2026-02-21
**Goal:** Show git status and diff inline on the Git tab instead of buttons that open new tabs.

## Problem

The current Git tab has three buttons (Git Status, Git Diff, Open PR) that open new browser tabs. Zac wants status and diff visible immediately on the Git tab without any extra clicks.

## Approach

Add two new JSON API endpoints that return raw git output. The frontend fetches both concurrently when the Git tab renders and displays them in styled `<pre>` blocks with client-side diff coloring. A Refresh button re-fetches on demand. Open PR stays as a button since it navigates to an external URL.

## Backend

Two new handlers in `internal/webserver/git.go`:

- `GET /api/sessions/{id}/git/status/text` — runs `git status`, returns `{"output": "..."}` as JSON
- `GET /api/sessions/{id}/git/diff/text` — runs `git diff HEAD` plus untracked file diffs (same logic as existing `handleGitDiff`), returns `{"output": "..."}` as JSON

Both are auth-protected (same middleware as other session endpoints). Both registered in `webserver.go`.

The existing HTML-returning endpoints (`/git/status`, `/git/diff`) are left unchanged.

## Frontend

`app.js` — replace `renderTabContent`'s `git` branch:

- Fire both `/git/status/text` and `/git/diff/text` fetches concurrently with `Promise.all`
- Show "loading..." placeholder in each section while fetching
- Render status output in a plain `<pre class="git-output">`
- Render diff output in a `<pre class="git-output">` with client-side line coloring: same rules as Go's `ColorDiffLines` (`+++`/`---` → `.diff-hdr`, `+` → `.diff-add`, `-` → `.diff-del`, `@@` → `.diff-hunk`), applied by building innerHTML with `<span>` elements
- Refresh button at the top re-runs the fetch + render
- Open PR button remains, opens the PR URL in a new tab
- Dirty notice (uncommitted changes) stays

## CSS

`style.css` — additions to git panel styles:

- `.git-panel` gets `overflow-y: auto; flex: 1` so the panel scrolls as a unit
- `.git-action-row` — flex row for Refresh + Open PR buttons + dirty notice
- `.git-section-label` — small uppercase section heading (STATUS / DIFF)
- `.git-output` — styled `<pre>` block: `background: var(--surface)`, monospace, `white-space: pre-wrap`
- `.diff-add` — `color: var(--running)` (green)
- `.diff-del` — `color: var(--error)` (red)
- `.diff-hunk` — `color: var(--accent)` (cyan)
- `.diff-hdr` — `color: var(--muted)` (gray)

## Layout

```
[Git tab panel]
──────────────────────────────────
  ● uncommitted changes   [↻ Refresh]  [Open PR]

  STATUS
  ┌────────────────────────────────┐
  │ On branch main                 │
  │ M  app.js                      │
  └────────────────────────────────┘

  DIFF
  ┌────────────────────────────────┐
  │ diff --git a/app.js b/app.js   │
  │ @@ -1,3 +1,5 @@                │
  │ + new line                     │
  │ - old line                     │
  └────────────────────────────────┘
```

## Files Changed

- `internal/webserver/git.go` — two new handlers
- `internal/webserver/webserver.go` — register two new routes
- `internal/webserver/static/app.js` — replace git tab renderer
- `internal/webserver/static/style.css` — add git panel and diff coloring CSS
