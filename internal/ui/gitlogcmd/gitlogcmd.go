package gitlogcmd

import "strings"

type commit struct {
	Hash    string
	Subject string
	RelDate string
}

// parseLogLine parses a single line from:
//
//	git log --pretty=format:"%h\t%s\t%ar"
func parseLogLine(line string) (commit, bool) {
	parts := strings.SplitN(line, "\t", 3)
	if len(parts) != 3 {
		return commit{}, false
	}
	return commit{Hash: parts[0], Subject: parts[1], RelDate: parts[2]}, true
}
