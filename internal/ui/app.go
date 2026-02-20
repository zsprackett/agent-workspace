package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zsprackett/agent-workspace/internal/config"
	"github.com/zsprackett/agent-workspace/internal/db"
	"github.com/zsprackett/agent-workspace/internal/git"
	"github.com/zsprackett/agent-workspace/internal/monitor"
	"github.com/zsprackett/agent-workspace/internal/notify"
	"github.com/zsprackett/agent-workspace/internal/syncer"
	"github.com/zsprackett/agent-workspace/internal/session"
	"github.com/zsprackett/agent-workspace/internal/tmux"
	"github.com/zsprackett/agent-workspace/internal/ui/dialogs"
	"github.com/zsprackett/agent-workspace/internal/webserver"
)

type App struct {
	tapp   *tview.Application
	pages  *tview.Pages
	home   *Home
	store  *db.DB
	mgr    *session.Manager
	mon    *monitor.Monitor
	syn    *syncer.Syncer
	cfg    config.Config
	groups []*db.Group
	web    *webserver.Server
}

func NewApp(store *db.DB, cfg config.Config) *App {
	a := &App{
		store: store,
		cfg:   cfg,
		mgr:   session.NewManager(store),
	}

	a.tapp = tview.NewApplication()
	a.pages = tview.NewPages()
	a.home = NewHome(a.tapp, store)

	notifier := notify.New(notify.Config{
		Enabled: cfg.Notifications.Enabled,
		Webhook: cfg.Notifications.Webhook,
		NtfyURL: cfg.Notifications.NtfyURL,
	})

	a.web = webserver.New(store, a.mgr, webserver.Config{
		Enabled: cfg.Webserver.Enabled,
		Port:    cfg.Webserver.Port,
		Host:    cfg.Webserver.Host,
		TLS: webserver.TLSConfig{
			Mode:     cfg.Webserver.TLS.Mode,
			Domain:   cfg.Webserver.TLS.Domain,
			CertFile: cfg.Webserver.TLS.CertFile,
			KeyFile:  cfg.Webserver.TLS.KeyFile,
			CacheDir: cfg.Webserver.TLS.CacheDir,
		},
		Auth: webserver.AuthConfig{
			JWTSecret:       cfg.Webserver.Auth.JWTSecret,
			RefreshTokenTTL: cfg.Webserver.Auth.RefreshTokenTTL,
		},
	})

	a.mon = monitor.New(store, func() {
		a.tapp.QueueUpdateDraw(func() {
			a.refreshHome()
		})
	}, notifier, a.web)

	a.syn = syncer.New(store, cfg.ReposDir)

	a.pages.AddPage("home", a.home, true, true)
	a.tapp.SetRoot(a.pages, true).EnableMouse(false)
	a.tapp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == '?' {
			a.showHelp()
			return nil
		}
		return event
	})

	a.home.SetCallbacks(
		func(groupPath string) { a.onNew(groupPath) },
		a.onDelete,
		a.onStop,
		a.onRestart,
		a.onEdit,
		a.onNewGroup,
		a.onMove,
		a.onAttach,
		a.onNotes,
		func() { a.tapp.Stop() },
	)

	return a
}

func (a *App) Run() error {
	// Ensure default group exists
	groups, _ := a.store.LoadGroups()
	if len(groups) == 0 {
		groups = []*db.Group{{
			Path:     "my-sessions",
			Name:     "My Sessions",
			Expanded: true,
		}}
		a.store.SaveGroups(groups)
	}

	if err := a.web.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: webserver: %v\n", err)
	}

	a.refreshHome()
	a.mon.Start()
	defer a.mon.Stop()

	a.syn.Start()
	defer a.syn.Stop()

	return a.tapp.Run()
}

func (a *App) refreshHome() {
	sessions, _ := a.store.LoadSessions()
	groups, _ := a.store.LoadGroups()
	a.groups = groups
	a.home.Update(sessions, groups)
}

func (a *App) showDialog(name string, widget tview.Primitive, width, height int) {
	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 1, false).
			AddItem(widget, width, 0, true).
			AddItem(nil, 0, 1, false), height, 0, true).
		AddItem(nil, 0, 1, false)
	a.pages.AddPage(name, modal, true, true)
	a.tapp.SetFocus(widget)
}

func (a *App) closeDialog(name string) {
	a.pages.RemovePage(name)
	a.tapp.SetFocus(a.home.table)
}

func (a *App) showHelp() {
	help := dialogs.HelpDialog(func() {
		a.closeDialog("help")
	})
	a.showDialog("help", help, 60, 30)
}

func (a *App) onNew(groupPath string) {
	if groupPath == "" {
		groupPath = a.cfg.DefaultGroup
	}
	groups, _ := a.store.LoadGroups()
	form := dialogs.NewSessionDialog(groups, a.cfg.DefaultTool, groupPath,
		func(result dialogs.NewSessionResult) {
			a.closeDialog("new-session")
			opts := session.CreateOptions{
				Title:     result.Title,
				Tool:      result.Tool,
				Command:   result.Command,
				GroupPath: result.GroupPath,
			}
			// Look up the selected group's repo URL.
			var groupRepoURL string
			for _, g := range groups {
				if g.Path == result.GroupPath {
					groupRepoURL = g.RepoURL
					break
				}
			}
			createSession := func() {
				s, err := a.mgr.Create(opts)
				if err != nil {
					a.showError(fmt.Sprintf("Create failed: %v", err))
					return
				}
				a.refreshHome()
				a.onAttachSession(s)
			}
			if groupRepoURL != "" {
				host, owner, repo, err := git.ParseRepoURL(groupRepoURL)
				if err != nil {
					a.showError(fmt.Sprintf("Invalid group repo URL: %v", err))
					return
				}
				// Resolve the title now so the branch name and session title match.
				title := result.Title
				if title == "" {
					title = session.GenerateTitle()
				}
				opts.Title = title

				bareRepoPath := git.BareRepoPath(a.cfg.ReposDir, host, owner, repo)
				if err := os.MkdirAll(filepath.Dir(bareRepoPath), 0755); err != nil {
					a.showError(fmt.Sprintf("Create repos dir failed: %v", err))
					return
				}
				if !git.IsBareRepo(bareRepoPath) {
					if err := git.CloneBare(groupRepoURL, bareRepoPath); err != nil {
						a.showError(fmt.Sprintf("Clone failed: %v", err))
						return
					}
				} else {
					if err := git.FetchBare(bareRepoPath); err != nil {
						a.showError(fmt.Sprintf("Fetch failed: %v", err))
						return
					}
				}
				branch := git.SanitizeBranchName(title)
				wtPath := git.WorktreePath(a.cfg.WorktreesDir, host, owner, repo, branch)
				if err := os.MkdirAll(filepath.Dir(wtPath), 0755); err != nil {
					a.showError(fmt.Sprintf("Create worktrees dir failed: %v", err))
					return
				}
				if _, err := git.CreateWorktree(bareRepoPath, branch, wtPath, a.cfg.Worktree.DefaultBaseBranch); err != nil {
					if errors.Is(err, git.ErrWorktreeExists) {
						modal := tview.NewModal().
							SetText(fmt.Sprintf("Worktree for branch '%s' already exists.\n\nReuse it or cancel?", branch)).
							AddButtons([]string{"Reuse", "Cancel"}).
							SetDoneFunc(func(_ int, label string) {
								a.closeDialog("worktree-exists")
								if label == "Reuse" {
									opts.WorktreePath = wtPath
									opts.WorktreeRepo = bareRepoPath
									opts.WorktreeBranch = branch
									opts.ProjectPath = wtPath
									opts.RepoURL = groupRepoURL
									createSession()
								}
							})
						a.pages.AddPage("worktree-exists", modal, true, true)
						return
					}
					a.showError(fmt.Sprintf("Create worktree failed: %v", err))
					return
				}
				opts.WorktreePath = wtPath
				opts.WorktreeRepo = bareRepoPath
				opts.WorktreeBranch = branch
				opts.ProjectPath = wtPath
				opts.RepoURL = groupRepoURL
			} else {
				opts.ProjectPath = result.ProjectPath
			}
			createSession()
		},
		func() { a.closeDialog("new-session") },
	)
	a.showDialog("new-session", form, 60, 20)
}

func (a *App) onDelete(item listItem) {
	if item.isGroup {
		if item.group.Path == "my-sessions" {
			a.showError("Cannot delete the default group")
			return
		}
		// Move sessions to default group
		sessions, _ := a.store.LoadSessions()
		for _, s := range sessions {
			if s.GroupPath == item.group.Path {
				a.mgr.MoveToGroup(s.ID, "my-sessions")
			}
		}
		a.store.DeleteGroup(item.group.Path)
		a.store.Touch()
		a.refreshHome()
	} else if item.session != nil {
		doDelete := func() {
			// Kill the tmux session first so the tool releases any file locks
			// before we attempt worktree removal.
			if item.session.TmuxSession != "" {
				tmux.KillSession(item.session.TmuxSession)
			}
			if item.session.WorktreePath != "" && item.session.WorktreeRepo != "" {
				if err := git.RemoveWorktree(item.session.WorktreeRepo, item.session.WorktreePath, false); err != nil {
					modal := tview.NewModal().
						SetText(fmt.Sprintf("Could not remove worktree:\n\n%v\n\nForce delete (discards uncommitted changes)?", err)).
						AddButtons([]string{"Force Delete", "Cancel"}).
						SetDoneFunc(func(_ int, label string) {
							a.closeDialog("worktree-error")
							if label == "Force Delete" {
								if err := git.RemoveWorktree(item.session.WorktreeRepo, item.session.WorktreePath, true); err != nil {
									a.showError(fmt.Sprintf("Force delete failed: %v", err))
									return
								}
								a.mgr.Delete(item.session.ID)
								a.refreshHome()
							}
						})
					a.pages.AddPage("worktree-error", modal, true, true)
					return
				}
			}
			a.mgr.Delete(item.session.ID)
			a.refreshHome()
		}
		s := item.session
		if s.TmuxSession != "" && s.Status != db.StatusStopped {
			modal := dialogs.ConfirmDialog(
				fmt.Sprintf("Session %q is still running.\nDelete it anyway?", s.Title),
				func() { a.closeDialog("confirm-delete"); doDelete() },
				func() { a.closeDialog("confirm-delete") },
			)
			a.pages.AddPage("confirm-delete", modal, true, true)
		} else {
			doDelete()
		}
	}
}

func (a *App) onStop(item listItem) {
	if item.session != nil {
		a.mgr.Stop(item.session.ID)
		a.refreshHome()
	}
}

func (a *App) onRestart(item listItem) {
	if item.session != nil {
		a.mgr.Restart(item.session.ID)
		a.refreshHome()
	}
}

func (a *App) onEdit(item listItem) {
	if item.isGroup {
		form := dialogs.GroupDialog("Edit Group", item.group.Name, item.group.RepoURL, string(item.group.DefaultTool),
			func(result dialogs.GroupResult) {
				a.closeDialog("edit")
				groups, _ := a.store.LoadGroups()
				for _, g := range groups {
					if g.Path == item.group.Path {
						g.Name = result.Name
						g.RepoURL = result.RepoURL
						g.DefaultTool = db.Tool(result.DefaultTool)
					}
				}
				a.store.SaveGroups(groups)
				a.store.Touch()
				a.refreshHome()
			}, func() { a.closeDialog("edit") })
		a.showDialog("edit", form, 65, 14)
	} else if item.session != nil {
		groups, _ := a.store.LoadGroups()
		form := dialogs.EditSessionDialog(item.session, groups,
			func(result dialogs.EditSessionResult) {
				a.closeDialog("edit")
				if err := a.mgr.Update(item.session.ID, session.UpdateOptions{
					Title:       result.Title,
					Tool:        result.Tool,
					Command:     result.Command,
					ProjectPath: result.ProjectPath,
					GroupPath:   result.GroupPath,
				}); err != nil {
					a.showError(fmt.Sprintf("Edit failed: %v", err))
					return
				}
				a.refreshHome()
			}, func() { a.closeDialog("edit") })
		a.showDialog("edit", form, 60, 22)
	}
}

func (a *App) onNewGroup() {
	form := dialogs.GroupDialog("New Group", "", "", "", func(result dialogs.GroupResult) {
		a.closeDialog("new-group")
		path := strings.ToLower(strings.ReplaceAll(result.Name, " ", "-"))
		groups, _ := a.store.LoadGroups()
		groups = append(groups, &db.Group{
			Path:        path,
			Name:        result.Name,
			Expanded:    true,
			SortOrder:   len(groups),
			RepoURL:     result.RepoURL,
			DefaultTool: db.Tool(result.DefaultTool),
		})
		a.store.SaveGroups(groups)
		a.store.Touch()
		a.refreshHome()
	}, func() { a.closeDialog("new-group") })
	a.showDialog("new-group", form, 65, 14)
}

func (a *App) onNotes(item listItem) {
	if item.session == nil {
		return
	}
	s := item.session
	form := dialogs.NotesDialog(s.Title, s.Notes,
		func(notes string) {
			a.closeDialog("notes")
			a.store.UpdateSessionNotes(s.ID, notes)
			a.store.Touch()
			a.refreshHome()
		},
		func() { a.closeDialog("notes") },
	)
	a.showDialog("notes", form, 60, 18)
}

func (a *App) onMove(item listItem) {
	if item.session == nil {
		return
	}
	groups, _ := a.store.LoadGroups()
	list := dialogs.MoveDialog(groups, func(groupPath string) {
		a.closeDialog("move")
		a.mgr.MoveToGroup(item.session.ID, groupPath)
		a.refreshHome()
	}, func() { a.closeDialog("move") })
	a.showDialog("move", list, 40, 15)
}

func (a *App) onAttach(item listItem) {
	if item.isGroup {
		a.home.toggleGroup(item.group)
		return
	}
	if item.session != nil {
		a.onAttachSession(item.session)
	}
}

func (a *App) onAttachSession(s *db.Session) {
	a.tapp.Suspend(func() {
		a.mgr.Attach(s.ID)
	})
	a.refreshHome()
}

func (a *App) showError(msg string) {
	modal := tview.NewModal().
		SetText(msg).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(_ int, _ string) {
			a.closeDialog("error")
		})
	a.pages.AddPage("error", modal, true, true)
}
