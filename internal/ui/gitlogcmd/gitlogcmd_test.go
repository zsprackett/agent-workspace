package gitlogcmd

import "testing"

func TestParseLogLine(t *testing.T) {
	c, ok := parseLogLine("abc1234\tFix auth bug\t2 hours ago")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if c.Hash != "abc1234" {
		t.Errorf("hash: got %q want %q", c.Hash, "abc1234")
	}
	if c.Subject != "Fix auth bug" {
		t.Errorf("subject: got %q want %q", c.Subject, "Fix auth bug")
	}
	if c.RelDate != "2 hours ago" {
		t.Errorf("reldate: got %q want %q", c.RelDate, "2 hours ago")
	}

	_, ok2 := parseLogLine("malformed no tabs")
	if ok2 {
		t.Error("expected ok=false for malformed line")
	}

	_, ok3 := parseLogLine("")
	if ok3 {
		t.Error("expected ok=false for empty line")
	}
}
