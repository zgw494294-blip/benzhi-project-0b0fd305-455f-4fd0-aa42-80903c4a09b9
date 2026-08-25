package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogRecoveryAndTrace(t *testing.T) {
	dir := t.TempDir()
	log, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i, kind := range []string{"submission.created", "submission.frozen", "receipt.issued"} {
		_, err = log.Append(Event{Type: kind, SubmissionID: "sub-1", AggregateVersion: uint64(i + 1), OccurredAt: time.Unix(int64(i), 0), Details: map[string]any{"receiptDigest": "digest"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = log.VerifyTrace("sub-1"); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "audit.frames"), os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write([]byte{0, 0}); err != nil {
		t.Fatal(err)
	}
	f.Close()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Verify().Frames != 3 {
		t.Fatal("尾部残帧恢复后事件数不正确")
	}
}
func TestLogDetectsChecksumCorruption(t *testing.T) {
	dir := t.TempDir()
	log, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = log.Append(Event{Type: "submission.created", SubmissionID: "sub-1", AggregateVersion: 1, OccurredAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "audit.frames")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-2] ^= 1
	if err = os.WriteFile(path, data, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(dir); err == nil {
		t.Fatal("损坏的审计帧不应恢复成功")
	}
}
