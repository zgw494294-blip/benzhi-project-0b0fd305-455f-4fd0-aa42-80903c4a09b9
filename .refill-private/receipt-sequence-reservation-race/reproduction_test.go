package receipt_sequence_reservation_race_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"geopack/internal/application"
	"geopack/internal/audit"
	"geopack/internal/domain"
	"geopack/internal/repository"
)

type gatedStore struct {
	repository.Store
	mu      sync.Mutex
	blocked map[string]bool
	entered chan string
	release chan struct{}
}

func (s *gatedStore) Load(ctx context.Context, id string) (*domain.Submission, error) {
	s.mu.Lock()
	blocked := s.blocked[id]
	s.mu.Unlock()
	if blocked {
		select {
		case s.entered <- id:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.Store.Load(ctx, id)
}

func artifact(role domain.ArtifactRole) domain.ArtifactInput {
	content := []byte("content-for-" + string(role))
	return domain.ArtifactInput{
		Role:               role,
		Filename:           string(role) + ".dat",
		SizeBytes:          int64(len(content)),
		SHA256:             domain.DigestBytes(content),
		CRS:                "EPSG:4490",
		GroundResolutionCM: 8,
		CoverageBBox:       domain.BBox{MinX: 99, MinY: 19, MaxX: 111, MaxY: 31},
		Content:            content,
	}
}

func prepareApprovable(t *testing.T, service *application.Service, key string) *domain.Submission {
	t.Helper()
	created, err := service.Create(context.Background(), application.CreateSubmission{
		IdempotencyKey: key + "-create", ProjectName: "并发冻结项目" + key,
		RequiredCRS: "EPSG:4490", AreaBBox: domain.BBox{MinX: 100, MinY: 20, MaxX: 110, MaxY: 30},
		MaxGroundResolutionCM: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	registered, err := service.RegisterBatch(context.Background(), application.RegisterArtifactBatch{
		SubmissionID: created.Submission.SubmissionID, ExpectedVersion: created.Submission.Version,
		IdempotencyKey: key + "-register", Actor: "submitter",
		Artifacts: []domain.ArtifactInput{
			artifact(domain.RoleOrthophoto), artifact(domain.RolePointCloud),
			artifact(domain.RoleControlReport), artifact(domain.RoleMetadata),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	validated, err := service.Validate(context.Background(), application.StartValidation{
		SubmissionID: registered.Submission.SubmissionID, ExpectedVersion: registered.Submission.Version,
		IdempotencyKey: key + "-validate", Actor: "reviewer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if validated.Submission.Status != domain.StatusApprovable {
		t.Fatalf("测试前置批次未达到可批准状态: %s", validated.Submission.Status)
	}
	return validated.Submission
}

type freezeOutcome struct {
	result application.MutationResult
	err    error
}

func TestConcurrentFreezeReservesUniqueReceiptSequence(t *testing.T) {
	log, err := audit.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &gatedStore{Store: repository.NewMemoryStore()}
	service := application.NewService(store, log)
	first := prepareApprovable(t, service, "first")
	second := prepareApprovable(t, service, "second")

	store.mu.Lock()
	store.blocked = map[string]bool{first.SubmissionID: true, second.SubmissionID: true}
	store.entered = make(chan string)
	store.release = make(chan struct{})
	store.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	outcomes := make(chan freezeOutcome, 2)
	freeze := func(sub *domain.Submission, key string) {
		result, freezeErr := service.Freeze(ctx, application.FreezeSubmission{
			SubmissionID: sub.SubmissionID, ExpectedVersion: sub.Version,
			IdempotencyKey: key, ApprovedBy: "reviewer",
		})
		outcomes <- freezeOutcome{result: result, err: freezeErr}
	}
	go freeze(first, "freeze-first")
	go freeze(second, "freeze-second")

	arrived := map[string]bool{}
	for len(arrived) < 2 {
		select {
		case id := <-store.entered:
			arrived[id] = true
		case <-ctx.Done():
			t.Fatal("并发冻结请求未到达受控 Load 屏障")
		}
	}
	close(store.release)

	one := <-outcomes
	two := <-outcomes
	if one.err != nil || two.err != nil {
		t.Fatalf("并发冻结应分别成功，实际错误: %v / %v", one.err, two.err)
	}
	if one.result.Receipt == nil || two.result.Receipt == nil {
		t.Fatal("并发冻结未返回接收凭据")
	}
	if one.result.Receipt.Sequence == two.result.Receipt.Sequence {
		t.Fatalf("两个已签发凭据取得重复序号 %d", one.result.Receipt.Sequence)
	}
	if one.result.Receipt.Sequence+1 != two.result.Receipt.Sequence && two.result.Receipt.Sequence+1 != one.result.Receipt.Sequence {
		t.Fatalf("两个并发凭据序号不连续: %d / %d", one.result.Receipt.Sequence, two.result.Receipt.Sequence)
	}
}
