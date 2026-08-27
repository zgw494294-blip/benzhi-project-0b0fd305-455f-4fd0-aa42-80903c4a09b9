package audit_tail_corruption_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"geopack/internal/audit"
)

func TestCompleteCorruptAuditTailIsNotSilentlyDiscarded(t *testing.T) {
	dir := t.TempDir()
	log, err := audit.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for index, eventType := range []string{"submission.created", "artifact.registered"} {
		_, err = log.Append(audit.Event{
			Type:             eventType,
			SubmissionID:     "sub-recovery",
			AggregateVersion: uint64(index + 1),
			OccurredAt:       time.Unix(int64(index+1), 0).UTC(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	path := filepath.Join(dir, "audit.frames")
	corrupt, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	marker := []byte(`"checksum":"`)
	checksum := bytes.LastIndex(corrupt, marker)
	if checksum < 0 {
		t.Fatal("未找到尾帧 checksum")
	}
	checksum += len(marker)
	if corrupt[checksum] == '0' {
		corrupt[checksum] = '1'
	} else {
		corrupt[checksum] = '0'
	}
	if err = os.WriteFile(path, corrupt, 0640); err != nil {
		t.Fatal(err)
	}

	if _, err = audit.Open(dir); err == nil {
		t.Fatal("完整尾帧 checksum 损坏时恢复必须报错，不能静默截除已提交事件")
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(after, corrupt) {
		t.Fatal("失败恢复不得改写或截断完整的损坏尾帧")
	}
}
