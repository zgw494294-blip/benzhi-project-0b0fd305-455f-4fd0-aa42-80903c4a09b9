package domain

import (
	"testing"
	"time"
)

func testSubmission(t *testing.T) *Submission {
	t.Helper()
	s, err := NewSubmission("sub-test", "测试项目", "EPSG:4490", BBox{100, 20, 110, 30}, 10, time.Unix(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func addTestArtifact(t *testing.T, s *Submission, role ArtifactRole, resolution float64) {
	t.Helper()
	content := []byte("content-" + string(role))
	_, err := s.RegisterArtifact(ArtifactInput{Role: role, Filename: string(role) + ".dat", SizeBytes: int64(len(content)), SHA256: DigestBytes(content), CRS: "EPSG:4490", GroundResolutionCM: resolution, CoverageBBox: BBox{99, 19, 111, 31}, Content: content}, DigestBytes(content), time.Unix(int64(s.Version+100), 0))
	if err != nil {
		t.Fatal(err)
	}
}
func TestValidationAndFreeze(t *testing.T) {
	s := testSubmission(t)
	for _, role := range RequiredRoles {
		addTestArtifact(t, s, role, 8)
	}
	run := RunValidation(s, time.Unix(200, 0), nil)
	if run.Outcome != OutcomePassed {
		t.Fatalf("核验应通过: %#v", run.Results)
	}
	s.ApplyValidation(run, time.Unix(200, 0))
	manifest, err := s.Freeze("reviewer", time.Unix(300, 0))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Digest != s.ManifestDigest() || len(manifest.Artifacts) != 4 {
		t.Fatal("冻结清单不稳定")
	}
	receipt := NewReceipt(1, "", "reviewer", manifest, time.Unix(301, 0))
	if ReceiptDigest(receipt) != receipt.ReceiptDigest {
		t.Fatal("凭据摘要不能重算")
	}
}
func TestDiscrepancyTransitions(t *testing.T) {
	s := testSubmission(t)
	for _, role := range RequiredRoles {
		resolution := 8.0
		if role == RoleOrthophoto {
			resolution = 20
		}
		addTestArtifact(t, s, role, resolution)
	}
	run := RunValidation(s, time.Unix(200, 0), nil)
	s.ApplyValidation(run, time.Unix(200, 0))
	if len(s.Discrepancies) != 1 {
		t.Fatalf("期望一个差异，得到 %d", len(s.Discrepancies))
	}
	d := &s.Discrepancies[0]
	if err := d.Remediate("note", DigestBytes([]byte("evidence")), "r2"); err == nil {
		t.Fatal("未认领差异不应直接整改")
	}
	if err := d.Acknowledge("owner", "原因"); err != nil {
		t.Fatal(err)
	}
	if err := d.Remediate("说明", DigestBytes([]byte("evidence")), "artifact:2"); err != nil {
		t.Fatal(err)
	}
	d.Close(time.Unix(300, 0))
	if d.Status != DiscrepancyClosed || d.ClosedAt == nil {
		t.Fatal("差异未关闭")
	}
}

func TestTargetedValidationPreservesSourceRun(t *testing.T) {
	s := testSubmission(t)
	for _, role := range RequiredRoles {
		addTestArtifact(t, s, role, 8)
	}
	first := RunValidation(s, time.Unix(200, 0), nil)
	s.ApplyValidation(first, time.Unix(200, 0))
	addTestArtifact(t, s, RoleOrthophoto, 8)
	second := RunValidation(s, time.Unix(300, 0), AffectedChecks(RoleOrthophoto))
	for _, result := range second.Results {
		if result.Role == RolePointCloud && result.Code == CheckCRS && result.SourceRunID != first.RunID {
			t.Fatal("未受影响检查未保留原始运行标识")
		}
		if result.Role == RoleOrthophoto && result.Code == CheckCRS && result.SourceRunID != second.RunID {
			t.Fatal("受影响检查未标记本次运行")
		}
	}
}
