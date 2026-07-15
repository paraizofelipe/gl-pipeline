package api

import "testing"

func TestStatusFromGitLab(t *testing.T) {
	cases := map[string]Status{
		"running":   StatusInProgress,
		"pending":   StatusPending,
		"created":   StatusPending,
		"scheduled": StatusWaiting,
		"success":   StatusCompleted,
		"failed":    StatusCompleted,
		"manual":    StatusCompleted,
	}
	for status, want := range cases {
		if got := StatusFromGitLab(status); got != want {
			t.Errorf("StatusFromGitLab(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestConclusionFromGitLab(t *testing.T) {
	cases := map[string]Conclusion{
		"success":  ConclusionSuccess,
		"failed":   ConclusionFailure,
		"canceled": ConclusionCancelled,
		"skipped":  ConclusionSkipped,
		"manual":   ConclusionNeutral,
		"running":  "",
	}
	for status, want := range cases {
		if got := ConclusionFromGitLab(status); got != want {
			t.Errorf("ConclusionFromGitLab(%q) = %q, want %q", status, got, want)
		}
	}
}
