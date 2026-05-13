package executor

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// consoleReportingMockDriver embeds mockDriver and implements
// consoleLogReporter so collectConsoleLogs picks it up.
type consoleReportingMockDriver struct {
	mockDriver
	logs []report.ConsoleLog
}

func (c *consoleReportingMockDriver) ConsoleLogReport() []report.ConsoleLog {
	return c.logs
}

func TestCollectConsoleLogs_DriverImplementsInterface(t *testing.T) {
	d := &consoleReportingMockDriver{
		logs: []report.ConsoleLog{
			{Level: "error", Message: "boom"},
			{Level: "warning", Message: "hot stove"},
		},
	}

	got := collectConsoleLogs(d)
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].Level != "error" || got[0].Message != "boom" {
		t.Errorf("got[0] = %+v, want {error, boom}", got[0])
	}
}

// TestCollectConsoleLogs_DriverWithoutInterface verifies the fallback path:
// mobile / native drivers don't implement consoleLogReporter and the helper
// returns nil instead of panicking.
func TestCollectConsoleLogs_DriverWithoutInterface(t *testing.T) {
	d := &mockDriver{} // plain mock, does not implement ConsoleLogReport

	got := collectConsoleLogs(d)
	if got != nil {
		t.Errorf("expected nil for non-implementing driver, got %v", got)
	}
}

// TestCollectConsoleLogs_EmptyLogs verifies that a driver implementing the
// interface but with no captured entries returns an empty (or nil) slice
// rather than a phantom non-empty result.
func TestCollectConsoleLogs_EmptyLogs(t *testing.T) {
	d := &consoleReportingMockDriver{logs: nil}

	got := collectConsoleLogs(d)
	if len(got) != 0 {
		t.Errorf("expected empty result for empty buffer, got %v", got)
	}
}
