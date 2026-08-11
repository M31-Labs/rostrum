package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogAppendsVerifiesAndReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	log, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	first, err := log.Append(Event{At: time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC), Kind: "decision.accepted", ActorID: "chair_1", Subject: "sub_1", Rule: "AllowDecisionWithQuorum", Trace: []string{"AllowDecisionWithQuorum → AllowDecision"}})
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	second, err := log.Append(Event{Kind: "export.workspace", ActorID: "organizer_1", Subject: "workspace"})
	if err != nil {
		t.Fatalf("second append: %v", err)
	}
	if second.Seq != 2 || second.PrevHash != first.Hash {
		t.Fatalf("second record = %#v, want chained sequence", second)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	records, err := reopened.Records()
	if err != nil {
		t.Fatalf("Records: %v", err)
	}
	if len(records) != 2 || records[0].Rule != "AllowDecisionWithQuorum" || records[1].Hash == "" {
		t.Fatalf("records = %#v, want two durable records", records)
	}
}

func TestLogDetectsTampering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	log, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := log.Append(Event{Kind: "decision.accepted", Subject: "sub_1"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(contents), "decision.accepted", "decision.changed", 1)), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("Open(tampered ledger) = nil error, want hash verification failure")
	}
}

func TestLogRotatesWithoutBreakingChain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	log, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	log.segmentBytes = 1
	if _, err := log.Append(Event{Kind: "first"}); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if _, err := log.Append(Event{Kind: "second"}); err != nil {
		t.Fatalf("second append: %v", err)
	}
	if err := log.Verify(); err != nil {
		t.Fatalf("Verify after rotation: %v", err)
	}
	segments, err := filepath.Glob(filepath.Join(filepath.Dir(path), "audit-*.log"))
	if err != nil || len(segments) != 1 {
		t.Fatalf("segments = %v, %v; want one rotated segment", segments, err)
	}
}
