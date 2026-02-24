package git

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func ValidateBranchName(name string) error {
	if name == "" {
		return errors.New("branch name cannot be empty")
	}
	if strings.TrimSpace(name) != name {
		return errors.New("branch name cannot have leading or trailing spaces")
	}
	if strings.Contains(name, "..") {
		return errors.New("branch name cannot contain '..'")
	}
	if strings.HasPrefix(name, ".") {
		return errors.New("branch name cannot start with '.'")
	}
	if strings.HasSuffix(name, ".lock") {
		return errors.New("branch name cannot end with '.lock'")
	}
	for _, ch := range []string{" ", "\t", "~", "^", ":", "?", "*", "[", "\\"} {
		if strings.Contains(name, ch) {
			return fmt.Errorf("branch name cannot contain '%s'", ch)
		}
	}
	return nil
}

func SanitizeBranchName(name string) string {
	r := strings.NewReplacer(
		" ", "-", "..", "-", "~", "-", "^", "-",
		":", "-", "?", "-", "*", "-", "[", "-", "\\", "-", "/", "-",
	)
	s := r.Replace(name)
	for strings.HasPrefix(s, ".") {
		s = s[1:]
	}
	for strings.HasSuffix(s, ".lock") {
		s = s[:len(s)-5]
	}
	// collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

func GenerateWorktreePath(repoDir, branchName string) string {
	safe := strings.ReplaceAll(branchName, "/", "-")
	safe = strings.ReplaceAll(safe, " ", "-")
	return filepath.Join(repoDir, ".worktrees", safe)
}

func IsGitRepo(dir string) bool {
	return exec.Command("git", "-C", dir, "rev-parse", "--git-dir").Run() == nil
}

func GetRepoRoot(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func GetCurrentBranch(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func BranchExists(repoDir, branch string) bool {
	return exec.Command("git", "-C", repoDir, "show-ref", "--verify", "--quiet",
		"refs/heads/"+branch).Run() == nil
}

var ErrWorktreeExists = errors.New("worktree path already exists")

func CreateWorktree(repoDir, branchName, worktreePath, baseBranch string) (string, error) {
	if err := ValidateBranchName(branchName); err != nil {
		return "", err
	}
	if !IsGitRepo(repoDir) {
		return "", errors.New("not a git repository")
	}
	if worktreePath == "" {
		worktreePath = GenerateWorktreePath(repoDir, branchName)
	}
	var cmd *exec.Cmd
	var upstream string
	if BranchExists(repoDir, branchName) {
		cmd = exec.Command("git", "-C", repoDir, "worktree", "add", worktreePath, branchName)
	} else {
		base := baseBranch
		if base == "" {
			base = "HEAD"
		}
		startPoint := base
		if base != "HEAD" {
			upstream = "origin/" + base
			// Use the remote tracking ref as the start point so the new worktree
			// begins at the latest fetched commit rather than the (potentially
			// stale) local branch ref.
			startPoint = "origin/" + base
		}
		cmd = exec.Command("git", "-C", repoDir, "worktree", "add", "-b", branchName, worktreePath, startPoint)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "already exists") {
			return "", ErrWorktreeExists
		}
		return "", fmt.Errorf("create worktree: %s", out)
	}
	if upstream != "" {
		exec.Command("git", "-C", repoDir, "branch", "--set-upstream-to="+upstream, branchName).Run()
	}
	return worktreePath, nil
}

func RemoveWorktree(repoDir, worktreePath string, force bool) error {
	args := []string{"-C", repoDir, "worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, worktreePath)
	out, err := exec.Command("git", args...).CombinedOutput()
	if err == nil {
		pruneClaudeProject(worktreePath)
		return nil
	}
	msg := string(out)
	// If git doesn't recognize the path as a working tree, prune stale refs
	// and remove the directory directly.
	if strings.Contains(msg, "is not a working tree") {
		exec.Command("git", "-C", repoDir, "worktree", "prune").Run()
		if rmErr := os.RemoveAll(worktreePath); rmErr != nil {
			return fmt.Errorf("remove worktree directory: %w", rmErr)
		}
		pruneClaudeProject(worktreePath)
		return nil
	}
	return fmt.Errorf("remove worktree: %s", msg)
}

// pruneClaudeProject removes the entry for worktreePath from ~/.claude.json's
// "projects" map, if present. Errors are silently ignored since this is best-effort.
func pruneClaudeProject(worktreePath string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	claudeJSON := filepath.Join(home, ".claude.json")
	data, err := os.ReadFile(claudeJSON)
	if err != nil {
		return
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return
	}
	projects, ok := doc["projects"].(map[string]any)
	if !ok {
		return
	}
	if _, exists := projects[worktreePath]; !exists {
		return
	}
	delete(projects, worktreePath)
	updated, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return
	}
	updated = append(updated, '\n')
	tmp := claudeJSON + ".tmp"
	if err := os.WriteFile(tmp, updated, 0o600); err != nil {
		return
	}
	os.Rename(tmp, claudeJSON)
}

func GetDefaultBranch(repoDir string) (string, error) {
	out, err := exec.Command("git", "-C", repoDir, "symbolic-ref", "refs/remotes/origin/HEAD").Output()
	if err == nil {
		ref := strings.TrimSpace(string(out))
		branch := strings.TrimPrefix(ref, "refs/remotes/origin/")
		if branch != ref {
			return branch, nil
		}
	}
	if BranchExists(repoDir, "main") {
		return "main", nil
	}
	if BranchExists(repoDir, "master") {
		return "master", nil
	}
	return "", errors.New("could not determine default branch")
}

// ParseRepoURL parses a GitHub/GitLab URL into (host, owner, repo) components.
// Handles https://github.com/owner/repo, https://github.com/owner/repo.git,
// and git@github.com:owner/repo.git
func ParseRepoURL(rawURL string) (host, owner, repo string, err error) {
	// Handle SCP-style SSH URLs: git@github.com:owner/repo.git
	if strings.HasPrefix(rawURL, "git@") {
		s := strings.TrimPrefix(rawURL, "git@")
		colonIdx := strings.Index(s, ":")
		if colonIdx < 0 {
			return "", "", "", fmt.Errorf("invalid SSH URL: %s", rawURL)
		}
		host = s[:colonIdx]
		path := strings.TrimSuffix(s[colonIdx+1:], ".git")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", "", fmt.Errorf("cannot parse owner/repo from SSH URL: %s", rawURL)
		}
		return host, parts[0], parts[1], nil
	}

	// Handle HTTPS URLs
	u, parseErr := url.Parse(rawURL)
	if parseErr != nil {
		return "", "", "", fmt.Errorf("invalid URL: %w", parseErr)
	}
	if u.Host == "" {
		return "", "", "", fmt.Errorf("missing host in URL: %s", rawURL)
	}
	host = u.Host
	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", fmt.Errorf("cannot parse owner/repo from URL: %s", rawURL)
	}
	return host, parts[0], parts[1], nil
}

// BareRepoPath returns the local bare clone path for a repo.
// e.g. baseDir="~/.agent-workspace/repos", host="github.com", owner="user", repo="myrepo"
// → ~/.agent-workspace/repos/github.com/user/myrepo.git
func BareRepoPath(baseDir, host, owner, repo string) string {
	return filepath.Join(baseDir, host, owner, repo+".git")
}

// WorktreePath returns the local path for a worktree.
// e.g. ~/.agent-workspace/worktrees/github.com/user/myrepo/swift-fox
func WorktreePath(baseDir, host, owner, repo, branch string) string {
	return filepath.Join(baseDir, host, owner, repo, branch)
}

// ensureRemoteTrackingRefs ensures the remote tracking refspec is configured on the
// bare repo so worktrees see remote tracking refs (refs/remotes/origin/main, etc.)
// for upstream tracking and git status. We intentionally do NOT add a
// +refs/heads/*:refs/heads/* refspec because git refuses to update a local branch
// ref that is currently checked out in a linked worktree, causing fetch errors.
// Instead, CreateWorktree uses origin/<base> as the starting commit so new worktrees
// always begin at the latest remote state.
// If the problematic +refs/heads/*:refs/heads/* refspec is present (added by a
// previous version), it is removed here.
func ensureRemoteTrackingRefs(repoDir string) {
	out, _ := exec.Command("git", "-C", repoDir, "config", "--get-all", "remote.origin.fetch").Output()
	existing := string(out)
	// Remove the problematic refspec if it was added by a previous version.
	if strings.Contains(existing, "refs/heads/*:refs/heads/*") {
		exec.Command("git", "-C", repoDir, "config", "--unset", "remote.origin.fetch",
			`^\+refs/heads/\*:refs/heads/\*$`).Run()
	}
	if !strings.Contains(existing, "refs/remotes/origin/") {
		exec.Command("git", "-C", repoDir, "config", "--add", "remote.origin.fetch",
			"+refs/heads/*:refs/remotes/origin/*").Run()
	}
}

// CloneBare clones a remote URL as a bare repo to destPath.
// No-ops if destPath already exists.
func CloneBare(remoteURL, destPath string) error {
	if _, err := os.Stat(destPath); err == nil {
		return nil
	}
	if out, err := exec.Command("git", "clone", "--bare", remoteURL, destPath).CombinedOutput(); err != nil {
		return fmt.Errorf("clone bare: %s", out)
	}
	ensureRemoteTrackingRefs(destPath)
	return nil
}

// IsBareRepo reports whether path is a bare git repository.
func IsBareRepo(path string) bool {
	out, err := exec.Command("git", "-C", path, "rev-parse", "--is-bare-repository").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// FetchBare runs "git fetch --prune" on a bare repository at repoDir.
// It also ensures the remote tracking refspec is configured so that
// refs/remotes/origin/* refs are populated for worktree upstream tracking.
func FetchBare(repoDir string) error {
	ensureRemoteTrackingRefs(repoDir)
	out, err := exec.Command("git", "-C", repoDir, "fetch", "--prune").CombinedOutput()
	if err != nil {
		return fmt.Errorf("fetch %s: %s", repoDir, out)
	}
	return nil
}

// IsWorktreeDirty reports whether a git worktree at path has uncommitted changes.
func IsWorktreeDirty(path string) (bool, error) {
	out, err := exec.Command("git", "-C", path, "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}
