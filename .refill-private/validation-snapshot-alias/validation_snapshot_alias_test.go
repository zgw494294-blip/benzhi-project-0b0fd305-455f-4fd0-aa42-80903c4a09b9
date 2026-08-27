package validation_snapshot_alias_test

import (
	"context"
	"testing"
	"time"

	"geopack/internal/application"
	"geopack/internal/domain"
	"geopack/internal/repository"
)

func TestValidationSnapshotDoesNotShareNestedState(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.August, 25, 9, 30, 0, 0, time.UTC)
	submission, err := domain.NewSubmission(
		"sub_snapshot_isolation",
		"嵌套核验快照隔离",
		"EPSG:4490",
		domain.BBox{MinX: 110, MinY: 20, MaxX: 111, MaxY: 21},
		10,
		now,
	)
	if err != nil {
		t.Fatalf("创建批次: %v", err)
	}
	content := []byte("data")
	digest := domain.DigestBytes(content)
	roles := []domain.ArtifactRole{
		domain.RoleOrthophoto,
		domain.RolePointCloud,
		domain.RoleControlReport,
		domain.RoleMetadata,
	}
	for _, role := range roles {
		_, err := submission.RegisterArtifact(domain.ArtifactInput{
			Role:               role,
			Filename:           string(role) + ".bin",
			SizeBytes:          int64(len(content)),
			SHA256:             digest,
			CRS:                "EPSG:4490",
			GroundResolutionCM: 5,
			CoverageBBox:       domain.BBox{MinX: 109, MinY: 19, MaxX: 112, MaxY: 22},
			Content:            content,
		}, digest, now)
		if err != nil {
			t.Fatalf("登记 %s: %v", role, err)
		}
	}
	run := domain.RunValidation(submission, now, nil)
	if run.Outcome != domain.OutcomePassed {
		t.Fatalf("准备核验历史: got %s", run.Outcome)
	}
	submission.ApplyValidation(run, now)
	if _, err := submission.Freeze("reviewer", now); err != nil {
		t.Fatalf("准备冻结清单: %v", err)
	}

	store, err := repository.Open(t.TempDir())
	if err != nil {
		t.Fatalf("打开仓储: %v", err)
	}
	if _, err := store.PutObject(ctx, content, digest, int64(len(content))); err != nil {
		t.Fatalf("保存成果对象: %v", err)
	}
	if err := store.Create(ctx, submission); err != nil {
		t.Fatalf("保存批次: %v", err)
	}
	service := application.NewService(store, nil)

	first, err := service.Get(ctx, submission.SubmissionID)
	if err != nil {
		t.Fatalf("首次查询: %v", err)
	}
	version := first.Version
	originalFilename := first.Artifacts[0].Filename
	originalMessage := first.ValidationRuns[0].Results[0].Message
	originalReference := first.ValidationRuns[0].ManifestRefs[0].ArtifactID
	originalFrozenFilename := first.FrozenManifest.Artifacts[0].Filename
	first.Artifacts[0].Filename = "tampered.tif"
	first.ValidationRuns[0].Results[0].Message = "调用方篡改的核验消息"
	first.ValidationRuns[0].ManifestRefs[0].ArtifactID = "artifact-tampered"
	first.FrozenManifest.Artifacts[0].Filename = "tampered-frozen.tif"

	second, err := service.Get(ctx, submission.SubmissionID)
	if err != nil {
		t.Fatalf("再次查询: %v", err)
	}
	if second.Version != version {
		t.Fatalf("只读查询后版本意外变化: got %d want %d", second.Version, version)
	}
	if got := second.ValidationRuns[0].Results[0].Message; got != originalMessage {
		t.Fatalf("查询结果污染了仓储核验消息: got %q", got)
	}
	if got := second.ValidationRuns[0].ManifestRefs[0].ArtifactID; got != originalReference {
		t.Fatalf("查询结果污染了仓储清单引用: got %q", got)
	}
	if got := second.Artifacts[0].Filename; got != originalFilename {
		t.Fatalf("查询结果污染了仓储成果修订: got %q", got)
	}
	if got := second.FrozenManifest.Artifacts[0].Filename; got != originalFrozenFilename {
		t.Fatalf("查询结果污染了仓储冻结清单: got %q", got)
	}
}
