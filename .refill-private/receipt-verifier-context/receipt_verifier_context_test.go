package receiptverifiercontext_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"geopack/internal/application"
	"geopack/internal/audit"
	"geopack/internal/domain"
	"geopack/internal/repository"
)

type contextObservingStore struct {
	*repository.MemoryStore
	submission *domain.Submission
	started    chan struct{}
	release    chan struct{}
	observed   chan error
	once       sync.Once
}

func (s *contextObservingStore) Load(ctx context.Context, id string) (*domain.Submission, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return domain.CloneSubmission(s.submission), nil
}

func (s *contextObservingStore) VerifyObject(ctx context.Context, key string, size int64, digest string) (repository.ObjectIntegrity, error) {
	first := false
	s.once.Do(func() {
		first = true
		close(s.started)
	})
	if first {
		<-s.release
		err := ctx.Err()
		s.observed <- err
		if err != nil {
			return repository.ObjectIntegrity{}, err
		}
	}
	return repository.ObjectIntegrity{
		ObjectKey:      key,
		Exists:         true,
		ExpectedSize:   size,
		ActualSize:     size,
		SizeMatches:    true,
		ExpectedSHA256: digest,
		ActualSHA256:   digest,
		DigestMatches:  true,
	}, nil
}

func TestReceiptVerifierPropagatesCancellationToActiveObjectRead(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	artifacts := []domain.ArtifactRevision{
		{ArtifactID: "artifact-orthophoto", Revision: 1, Role: domain.RoleOrthophoto, SizeBytes: 4, SHA256: domain.DigestBytes([]byte("ortho")), ObjectKey: "object-1", RegisteredAt: now},
		{ArtifactID: "artifact-point-cloud", Revision: 1, Role: domain.RolePointCloud, SizeBytes: 4, SHA256: domain.DigestBytes([]byte("cloud")), ObjectKey: "object-2", RegisteredAt: now},
	}
	manifest := domain.FrozenManifest{SubmissionID: "sub-context", FrozenVersion: 7, RulesetVersion: domain.CurrentRuleset, Artifacts: artifacts}
	manifest.Digest = domain.ManifestDigestFor(artifacts)
	receipt := domain.NewReceipt(1, "", "reviewer", manifest, now)
	sub := &domain.Submission{
		SchemaVersion:  1,
		SubmissionID:   manifest.SubmissionID,
		Status:         domain.StatusIssued,
		Version:        8,
		FrozenManifest: &manifest,
		Receipt:        &receipt,
	}
	store := &contextObservingStore{
		MemoryStore: repository.NewMemoryStore(),
		submission:  sub,
		started:     make(chan struct{}),
		release:     make(chan struct{}),
		observed:    make(chan error, 1),
	}
	log, err := audit.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	service := application.NewService(store, log)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, receiptErr := service.Receipt(ctx, sub.SubmissionID, nil)
		result <- receiptErr
	}()

	<-store.started
	cancel()
	if err = <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Receipt should return context.Canceled, got %v", err)
	}
	close(store.release)
	if observed := <-store.observed; !errors.Is(observed, context.Canceled) {
		t.Fatalf("active VerifyObject received a detached context after caller cancellation: %v", observed)
	}
}
