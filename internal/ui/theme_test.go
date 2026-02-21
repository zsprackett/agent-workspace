package ui

import (
	"testing"
)

func TestStatusIconCreating(t *testing.T) {
	icon, _ := StatusIcon("creating")
	if icon == "" {
		t.Error("expected non-empty icon for creating")
	}
}

func TestStatusIconDeleting(t *testing.T) {
	icon, _ := StatusIcon("deleting")
	if icon == "" {
		t.Error("expected non-empty icon for deleting")
	}
}

func TestStatusIconCreatingDistinctFromDefault(t *testing.T) {
	_, colorCreating := StatusIcon("creating")
	_, colorUnknown := StatusIcon("unknown-xyz")
	if colorCreating == colorUnknown {
		t.Error("creating should have distinct color from unknown status")
	}
}
