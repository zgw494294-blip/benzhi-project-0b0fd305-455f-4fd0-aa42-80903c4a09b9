package validation_query_cache_race_test

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

type loadBarrierStore struct {
	repository.Store
	mu      sync.Mutex
	waiting int
	target  int
	release chan struct{}
}

func (s *loadBarrierStore) Load(ctx context.Context, id string) (*domain.Submission, error) {
	s.mu.Lock()
	s.waiting++
	if s.waiting == s.target {
		close(s.release)
	}
	release := s.release
	s.mu.Unlock()
	select {
	case <-release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return s.Store.Load(ctx, id)
}

func TestConcurrentValidationQueryCacheIsRaceFree(t *testing.T) {
	const callers = 32
	memory := repository.NewMemoryStore()
	submission, err := domain.NewSubmission(
		"sub-cache-race",
		"并发核验查询",
		"EPSG:4490",
		domain.BBox{MinX: 100, MinY: 20, MaxX: 110, MaxY: 30},
		10,
		time.Unix(1000, 0),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = memory.Create(context.Background(), submission); err != nil {
		t.Fatal(err)
	}
	store := &loadBarrierStore{Store: memory, target: callers, release: make(chan struct{})}
	log, err := audit.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(store, log)

	start := make(chan struct{})
	errors := make(chan error, callers)
	var workers sync.WaitGroup
	workers.Add(callers)
	for index := 0; index < callers; index++ {
		limit := index + 1
		go func() {
			defer workers.Done()
			<-start
			_, queryErr := service.ValidationRuns(context.Background(), submission.SubmissionID, application.ValidationRunsQuery{Limit: limit})
			errors <- queryErr
		}()
	}
	close(start)
	workers.Wait()
	close(errors)
	for queryErr := range errors {
		if queryErr != nil {
			t.Fatalf("并发核验查询失败: %v", queryErr)
		}
	}
}
