package hosts

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSharedFixtureHarness(t *testing.T) {
	fixture, err := os.ReadFile("../../core/practice/testdata/session-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var report Report
	if err = json.Unmarshal([]byte(Run(string(fixture))), &report); err != nil {
		t.Fatal(err)
	}
	if !report.OK || !report.Cancellation || !report.ResourceOwnership || report.LoopCompletion == nil || report.Transport.Completed.CompletedLoops != 1 || len(report.Transport.Events) != 1 {
		t.Fatalf("harness failed: %+v", report)
	}
	// Repeated independent runs must emit byte-identical portable reports.
	if Run(string(fixture)) != Run(string(fixture)) {
		t.Fatal("nondeterministic report")
	}
}
func TestRejectBadFixture(t *testing.T) {
	for _, input := range []string{`{}`, `{"fixtureVersion":99}`, `{"surprise":true}`, string(make([]byte, MaxFixtureBytes+1))} {
		var report Report
		if err := json.Unmarshal([]byte(Run(input)), &report); err != nil {
			t.Fatal(err)
		}
		if report.OK || report.Error == "" {
			t.Fatalf("accepted fixture: %s", input)
		}
	}
}
