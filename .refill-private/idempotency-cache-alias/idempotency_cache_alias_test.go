package bugtest

import (
	"context"
	"testing"

	"geopack/internal/application"
	"geopack/internal/audit"
	"geopack/internal/domain"
	"geopack/internal/repository"
)

func TestIdempotencyReplayDoesNotShareMutableResult(t *testing.T) {
	log, err := audit.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(repository.NewMemoryStore(), log)
	created, err := service.Create(context.Background(), application.CreateSubmission{
		IdempotencyKey: "create", ProjectName: "缓存隔离测试", RequiredCRS: "EPSG:4490",
		AreaBBox: domain.BBox{MinX: 100, MinY: 20, MaxX: 110, MaxY: 30}, MaxGroundResolutionCM: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("orthophoto-v1")
	command := application.RegisterArtifact{
		SubmissionID: created.Submission.SubmissionID, ExpectedVersion: created.Submission.Version,
		IdempotencyKey: "register", Actor: "submitter",
		Artifact: domain.ArtifactInput{
			Role: domain.RoleOrthophoto, Filename: "ortho.tif", SizeBytes: int64(len(content)),
			SHA256: domain.DigestBytes(content), CRS: "EPSG:4490", GroundResolutionCM: 8,
			CoverageBBox: domain.BBox{MinX: 99, MinY: 19, MaxX: 111, MaxY: 31}, Content: content,
		},
	}
	first, err := service.Register(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	first.Artifact.Filename = "poisoned-by-caller.tif"

	replayed, err := service.Register(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Artifact == nil || replayed.Artifact.Filename != "ortho.tif" {
		t.Fatalf("幂等重放复用了调用方可变结果: %#v", replayed.Artifact)
	}
}
