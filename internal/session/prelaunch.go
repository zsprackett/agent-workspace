package session

import (
	"os/exec"
	"strings"
)

// RunPreLaunchCommand executes cmd (a shell command string) with the given positional args
// appended. Returns combined stdout+stderr output and any error.
// If cmd is empty, returns immediately with no error.
func RunPreLaunchCommand(cmd string, args ...string) (string, error) {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return "", nil
	}
	allArgs := append(fields[1:], args...)
	c := exec.Command(fields[0], allArgs...)
	out, err := c.CombinedOutput()
	return string(out), err
}
