package actor_result_cross_talk_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"geopack/internal/application"
	"geopack/internal/audit"
	"geopack/internal/domain"
	"geopack/internal/repository"
)

type stagedStore struct {
	repository.Store
	putCalls        atomic.Int32
	firstPutStarted chan struct{}
	releaseFirstPut chan struct{}
	secondPersisted chan struct{}
	secondOnce      sync.Once
}

func (s *stagedStore) PutObject(ctx context.Context, data []byte, digest string, size int64) (string, error) {
	if s.putCalls.Add(1) == 1 {
		close(s.firstPutStarted)
		<-s.releaseFirstPut
	}
	return s.Store.PutObject(ctx, data, digest, size)
}

func (s *stagedStore) SaveIdempotency(ctx context.Context, scope, key, digest string, response []byte) error {
	err := s.Store.SaveIdempotency(ctx, scope, key, digest, response)
	if key == "second-write" {
		s.secondOnce.Do(func() { close(s.secondPersisted) })
	}
	return err
}

func artifact(role domain.ArtifactRole, content string) domain.ArtifactInput {
	data := []byte(content)
	return domain.ArtifactInput{
		Role:               role,
		Filename:           string(role) + ".dat",
		SizeBytes:          int64(len(data)),
		SHA256:             domain.DigestBytes(data),
		CRS:                "EPSG:4490",
		GroundResolutionCM: 8,
		CoverageBBox:       domain.BBox{MinX: 100, MinY: 20, MaxX: 110, MaxY: 30},
		Content:            data,
	}
}

func TestCanceledActorResultCannotLeakToNextRequest(t *testing.T) {
	base := repository.NewMemoryStore()
	store := &stagedStore{
		Store:           base,
		firstPutStarted: make(chan struct{}),
		releaseFirstPut: make(chan struct{}),
		secondPersisted: make(chan struct{}),
	}
	now := time.Unix(1700000000, 0).UTC()
	sub, err := domain.NewSubmission(
		"sub_actor_result_isolation",
		"actor 取消隔离复现",
		"EPSG:4490",
		domain.BBox{MinX: 100, MinY: 20, MaxX: 110, MaxY: 30},
		10,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = base.Create(context.Background(), sub); err != nil {
		t.Fatal(err)
	}
	log, err := audit.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(store, log)

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, callErr := service.Register(firstCtx, application.RegisterArtifact{
			SubmissionID: sub.SubmissionID, ExpectedVersion: 1, IdempotencyKey: "first-write",
			Artifact: artifact(domain.RoleOrthophoto, "first-object"), Actor: "submitter-a",
		})
		firstDone <- callErr
	}()

	<-store.firstPutStarted
	cancelFirst()
	if err = <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("首请求应观察到 context.Canceled，实际为 %v", err)
	}
	close(store.releaseFirstPut)

	result, err := service.Register(context.Background(), application.RegisterArtifact{
		SubmissionID: sub.SubmissionID, ExpectedVersion: 2, IdempotencyKey: "second-write",
		Artifact: artifact(domain.RolePointCloud, "second-object"), Actor: "submitter-b",
	})
	if err != nil {
		t.Fatalf("后继请求不应失败：%v", err)
	}
	<-store.secondPersisted
	if result.Artifact == nil || result.Artifact.Role != domain.RolePointCloud {
		t.Fatalf("后继请求收到了已取消前序请求的结果：%+v", result.Artifact)
	}
}
