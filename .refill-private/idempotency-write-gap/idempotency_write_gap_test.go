package bugtest

import (
	"context"
	"errors"
	"testing"

	"geopack/internal/application"
	"geopack/internal/audit"
	"geopack/internal/domain"
	"geopack/internal/repository"
)

type failOnceIdempotencyStore struct {
	repository.Store
	fail bool
}

func (s *failOnceIdempotencyStore) SaveIdempotency(ctx context.Context, scope, key, digest string, response []byte) error {
	if s.fail {
		s.fail = false
		return errors.New("forced idempotency write failure")
	}
	return s.Store.SaveIdempotency(ctx, scope, key, digest, response)
}

func TestCreateRetryAfterIdempotencyWriteFailureDoesNotDuplicate(t *testing.T) {
	ctx := context.Background()
	base := repository.NewMemoryStore()
	store := &failOnceIdempotencyStore{Store: base, fail: true}
	log, err := audit.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(store, log)
	command := application.CreateSubmission{
		IdempotencyKey: "same-create", ProjectName: "幂等故障测试", RequiredCRS: "EPSG:4490",
		AreaBBox: domain.BBox{MinX: 100, MinY: 20, MaxX: 110, MaxY: 30}, MaxGroundResolutionCM: 10,
	}
	if _, err = service.Create(ctx, command); err == nil {
		t.Fatal("预期首次幂等记录写入失败")
	}
	if _, err = service.Create(ctx, command); err != nil {
		t.Fatalf("同键重试失败: %v", err)
	}
	items, err := base.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("幂等记录写入失败后的同键重试创建了 %d 个批次", len(items))
	}
}
