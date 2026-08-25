package application

import (
	"context"

	"geopack/internal/domain"
	"geopack/internal/repository"
)

type receiptVerificationResult struct {
	checks []ArtifactIntegrityCheck
	allOK  bool
	err    error
}

type receiptVerificationTask struct {
	artifacts []domain.ArtifactRevision
	done      chan receiptVerificationResult
}

type receiptVerifier struct {
	store repository.Store
	queue chan receiptVerificationTask
}

func newReceiptVerifier(store repository.Store, capacity int) *receiptVerifier {
	verifier := &receiptVerifier{store: store, queue: make(chan receiptVerificationTask, capacity)}
	go verifier.run()
	return verifier
}

func (v *receiptVerifier) run() {
	for task := range v.queue {
		checks := make([]ArtifactIntegrityCheck, 0, len(task.artifacts))
		allOK := true
		for _, artifact := range task.artifacts {
			check, err := v.store.VerifyObject(context.Background(), artifact.ObjectKey, artifact.SizeBytes, artifact.SHA256)
			if err != nil {
				task.done <- receiptVerificationResult{err: err}
				break
			}
			item := ArtifactIntegrityCheck{Role: artifact.Role, ArtifactID: artifact.ArtifactID, Revision: artifact.Revision, ObjectKey: artifact.ObjectKey, Exists: check.Exists, ExpectedSizeBytes: check.ExpectedSize, ActualSizeBytes: check.ActualSize, SizeMatches: check.SizeMatches, ExpectedSHA256: check.ExpectedSHA256, ActualSHA256: check.ActualSHA256, DigestMatches: check.DigestMatches}
			checks = append(checks, item)
			allOK = allOK && item.Exists && item.SizeMatches && item.DigestMatches
		}
		if len(checks) == len(task.artifacts) {
			task.done <- receiptVerificationResult{checks: checks, allOK: allOK}
		}
	}
}

func (v *receiptVerifier) verify(ctx context.Context, artifacts []domain.ArtifactRevision) ([]ArtifactIntegrityCheck, bool, error) {
	task := receiptVerificationTask{
		artifacts: append([]domain.ArtifactRevision(nil), artifacts...),
		done:      make(chan receiptVerificationResult, 1),
	}
	select {
	case v.queue <- task:
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
	select {
	case result := <-task.done:
		return result.checks, result.allOK, result.err
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
}
