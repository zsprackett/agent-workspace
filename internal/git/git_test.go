package git_test

import (
	"testing"
	"github.com/zsprackett/agent-workspace/internal/git"
)

func TestValidateBranchName(t *testing.T) {
	cases := []struct {
		name string
		want string // empty = valid
	}{
		{"main", ""},
		{"feature/my-thing", ""},
		{"", "branch name cannot be empty"},
		{"has space", "cannot contain ' '"},
		{"has..dots", "cannot contain '..'"},
		{".starts-with-dot", "cannot start with '.'"},
		{"ends.lock", "cannot end with '.lock'"},
	}
	for _, c := range cases {
		err := git.ValidateBranchName(c.name)
		if c.want == "" && err != nil {
			t.Errorf("%q: expected valid, got %v", c.name, err)
		}
		if c.want != "" && err == nil {
			t.Errorf("%q: expected error containing %q, got nil", c.name, c.want)
		}
	}
}

func TestSanitizeBranchName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"my feature", "my-feature"},
		{"has..dots", "has-dots"},
		{"--leading-dashes--", "leading-dashes"},
	}
	for _, c := range cases {
		got := git.SanitizeBranchName(c.input)
		if got != c.want {
			t.Errorf("%q: got %q want %q", c.input, got, c.want)
		}
	}
}

func TestGenerateWorktreePath(t *testing.T) {
	got := git.GenerateWorktreePath("/home/user/myrepo", "feature/my-branch")
	want := "/home/user/myrepo/.worktrees/feature-my-branch"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestParseRepoURL(t *testing.T) {
	cases := []struct {
		input string
		host  string
		owner string
		repo  string
		isErr bool
	}{
		{"https://github.com/owner/myrepo", "github.com", "owner", "myrepo", false},
		{"https://github.com/owner/myrepo.git", "github.com", "owner", "myrepo", false},
		{"https://gitlab.com/org/project.git", "gitlab.com", "org", "project", false},
		{"git@github.com:owner/myrepo.git", "github.com", "owner", "myrepo", false},
		{"git@github.com:owner/myrepo", "github.com", "owner", "myrepo", false},
		{"not-a-url", "", "", "", true},
		{"https://github.com/onlyowner", "", "", "", true},
	}
	for _, c := range cases {
		host, owner, repo, err := git.ParseRepoURL(c.input)
		if c.isErr {
			if err == nil {
				t.Errorf("%q: expected error, got host=%q owner=%q repo=%q", c.input, host, owner, repo)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error: %v", c.input, err)
			continue
		}
		if host != c.host || owner != c.owner || repo != c.repo {
			t.Errorf("%q: got host=%q owner=%q repo=%q, want host=%q owner=%q repo=%q",
				c.input, host, owner, repo, c.host, c.owner, c.repo)
		}
	}
}

func TestBareRepoPath(t *testing.T) {
	got := git.BareRepoPath("/home/user/.agent-workspace/repos", "github.com", "owner", "myrepo")
	want := "/home/user/.agent-workspace/repos/github.com/owner/myrepo.git"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestWorktreePath(t *testing.T) {
	got := git.WorktreePath("/home/user/.agent-workspace/worktrees", "github.com", "owner", "myrepo", "swift-fox")
	want := "/home/user/.agent-workspace/worktrees/github.com/owner/myrepo/swift-fox"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestFetchBare_NonexistentPath(t *testing.T) {
	err := git.FetchBare("/nonexistent/path/that/does/not/exist.git")
	if err == nil {
		t.Error("expected error for nonexistent repo, got nil")
	}
}
