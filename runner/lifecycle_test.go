package runner

import (
	"errors"
	"testing"

	"github.com/impire-io/soulrealm/backend"
)

func TestOutcome(t *testing.T) {
	cases := []struct {
		name     string
		startErr error
		st       backend.ExitStatus
		want     Terminal
		reason   string
	}{
		{"clean", nil, backend.ExitStatus{Code: 0}, TerminalDone, ""},
		{"nonzero", nil, backend.ExitStatus{Code: 3}, TerminalAbandon, ReasonNonzeroExit},
		{"signal", nil, backend.ExitStatus{Signal: "terminated"}, TerminalAbandon, ReasonSignal},
		{"start-failed", errors.New("boom"), backend.ExitStatus{}, TerminalAbandon, ReasonStartFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			term, reason := Outcome(tc.startErr, tc.st)
			if term != tc.want || reason != tc.reason {
				t.Fatalf("Outcome = (%v, %q), want (%v, %q)", term, reason, tc.want, tc.reason)
			}
		})
	}
}
