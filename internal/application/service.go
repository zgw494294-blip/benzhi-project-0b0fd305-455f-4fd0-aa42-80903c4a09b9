package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"geopack/internal/audit"
	"geopack/internal/domain"
	"geopack/internal/repository"
)

type Service struct {
	store  repository.Store
	audit  *audit.Log
	actors *actorRegistry
	idem   *idempotencyCache
	clock  func() time.Time
	ids    atomic.Uint64
}

func NewService(store repository.Store, log *audit.Log) *Service {
	return &Service{store: store, audit: log, actors: newActorRegistry(64), idem: newIdempotencyCache(), clock: time.Now}
}
func domainClone(s *domain.Submission) *domain.Submission { return domain.CloneSubmission(s) }

func (s *Service) newID(prefix string) string {
	n := s.ids.Add(1)
	return domain.NewID(prefix, s.clock(), fmt.Sprintf("%d", n))
}

func (s *Service) idemLookup(ctx context.Context, scope, key, digest string) (MutationResult, bool, error) {
	if result, ok, err := s.idem.lookup(scope, key, digest); err != nil || ok {
		return result, ok, err
	}
	storedDigest, data, ok, err := s.store.LoadIdempotency(ctx, scope, key)
	if err != nil || !ok {
		return MutationResult{}, false, err
	}
	if storedDigest != digest {
		return MutationResult{}, false, NewError("idempotency_conflict", "同一幂等键不能用于不同载荷")
	}
	var result MutationResult
	if err := json.Unmarshal(data, &result); err != nil {
		return MutationResult{}, false, err
	}
	s.idem.store(scope, key, digest, result)
	return result, true, nil
}

func (s *Service) idemStore(ctx context.Context, scope, key, digest string, result MutationResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if err := s.store.SaveIdempotency(ctx, scope, key, digest, data); err != nil {
		return err
	}
	s.idem.store(scope, key, digest, result)
	return nil
}
func (s *Service) checkVersion(sub *domain.Submission, expected uint64) error {
	if expected == 0 {
		return NewError("expected_version_required", "expectedVersion 必须大于零")
	}
	if sub.Version != expected {
		return &Error{Code: "version_conflict", Message: fmt.Sprintf("版本冲突：期望 %d，当前 %d", expected, sub.Version), Version: sub.Version}
	}
	return nil
}
func (s *Service) event(kind string, sub *domain.Submission, actor string, details map[string]any) error {
	_, err := s.audit.Append(audit.Event{Type: kind, SubmissionID: sub.SubmissionID, AggregateVersion: sub.Version, Actor: actor, Details: details, OccurredAt: s.clock().UTC()})
	return err
}
func mapStoreError(err error) error {
	var nf *repository.NotFoundError
	if errors.As(err, &nf) {
		return NewError("submission_not_found", nf.Error())
	}
	var conflict *repository.ConflictError
	if errors.As(err, &conflict) {
		return &Error{Code: "version_conflict", Message: conflict.Error(), Version: conflict.Actual}
	}
	return err
}

func (s *Service) Create(ctx context.Context, cmd CreateSubmission) (MutationResult, error) {
	digest := requestDigest(cmd)
	if r, ok, err := s.idemLookup(ctx, "create", cmd.IdempotencyKey, digest); err != nil || ok {
		return r, err
	}
	return s.actors.do(ctx, "create:"+cmd.IdempotencyKey, func() (MutationResult, error) {
		if r, ok, err := s.idemLookup(ctx, "create", cmd.IdempotencyKey, digest); err != nil || ok {
			return r, err
		}
		id := s.newID("sub")
		sub, err := domain.NewSubmission(id, cmd.ProjectName, cmd.RequiredCRS, cmd.AreaBBox, cmd.MaxGroundResolutionCM, s.clock())
		if err != nil {
			return MutationResult{}, err
		}
		if err = s.store.Create(ctx, sub); err != nil {
			return MutationResult{}, mapStoreError(err)
		}
		if err = s.event("submission.created", sub, "", map[string]any{"projectName": sub.ProjectName}); err != nil {
			return MutationResult{}, err
		}
		result := MutationResult{Submission: sub}
		if err = s.idemStore(ctx, "create", cmd.IdempotencyKey, digest, result); err != nil {
			return MutationResult{}, err
		}
		return result, nil
	})
}

func (s *Service) execute(ctx context.Context, id, scope, key string, request any, fn func(*domain.Submission) (MutationResult, error)) (MutationResult, error) {
	digest := requestDigest(request)
	if result, ok, err := s.idemLookup(ctx, scope+":"+id, key, digest); err != nil || ok {
		return result, err
	}
	return s.actors.do(ctx, id, func() (MutationResult, error) {
		if result, ok, err := s.idemLookup(ctx, scope+":"+id, key, digest); err != nil || ok {
			return result, err
		}
		sub, err := s.store.Load(ctx, id)
		if err != nil {
			return MutationResult{}, mapStoreError(err)
		}
		oldVersion := sub.Version
		result, err := fn(sub)
		if err != nil {
			return MutationResult{}, err
		}
		if err = s.store.Save(ctx, sub, oldVersion); err != nil {
			return MutationResult{}, mapStoreError(err)
		}
		result.Submission = domainClone(sub)
		if err := s.idemStore(ctx, scope+":"+id, key, digest, result); err != nil {
			return MutationResult{}, err
		}
		return result, nil
	})
}

func (s *Service) Register(ctx context.Context, cmd RegisterArtifact) (MutationResult, error) {
	return s.execute(ctx, cmd.SubmissionID, "register", cmd.IdempotencyKey, cmd, func(sub *domain.Submission) (MutationResult, error) {
		if err := s.checkVersion(sub, cmd.ExpectedVersion); err != nil {
			return MutationResult{}, err
		}
		key, err := s.store.PutObject(ctx, cmd.Artifact.Content, cmd.Artifact.SHA256, cmd.Artifact.SizeBytes)
		if err != nil {
			return MutationResult{}, err
		}
		a, err := sub.RegisterArtifact(cmd.Artifact, key, s.clock())
		if err != nil {
			return MutationResult{}, err
		}
		if err = s.event("artifact.registered", sub, cmd.Actor, map[string]any{"role": a.Role, "revision": a.Revision, "sha256": a.SHA256}); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{Artifact: &a}, nil
	})
}

func (s *Service) RegisterBatch(ctx context.Context, cmd RegisterArtifactBatch) (MutationResult, error) {
	return s.execute(ctx, cmd.SubmissionID, "register-batch", cmd.IdempotencyKey, cmd, func(sub *domain.Submission) (MutationResult, error) {
		if err := s.checkVersion(sub, cmd.ExpectedVersion); err != nil {
			return MutationResult{}, err
		}
		if err := sub.EnsureMutable(); err != nil {
			return MutationResult{}, err
		}
		ordered, err := domain.ValidateArtifactBatch(cmd.Artifacts)
		if err != nil {
			return MutationResult{}, err
		}
		keys := map[string]string{}
		for _, artifact := range ordered {
			key, err := s.store.PutObject(ctx, artifact.Content, artifact.SHA256, artifact.SizeBytes)
			if err != nil {
				return MutationResult{}, err
			}
			keys[artifact.SHA256] = key
		}
		registered, err := sub.RegisterArtifactBatch(ordered, keys, s.clock())
		if err != nil {
			return MutationResult{}, err
		}
		roles := make([]domain.ArtifactRole, 0, len(registered))
		revisions := map[domain.ArtifactRole]uint32{}
		for _, artifact := range registered {
			roles = append(roles, artifact.Role)
			revisions[artifact.Role] = artifact.Revision
		}
		digest := sub.ManifestDigest()
		if err = s.event("artifacts.batch_registered", sub, cmd.Actor, map[string]any{"roles": roles, "revisions": revisions, "manifestDigest": digest}); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{Artifacts: registered, ManifestDigest: digest}, nil
	})
}

func (s *Service) Validate(ctx context.Context, cmd StartValidation) (MutationResult, error) {
	return s.execute(ctx, cmd.SubmissionID, "validate", cmd.IdempotencyKey, cmd, func(sub *domain.Submission) (MutationResult, error) {
		if err := s.checkVersion(sub, cmd.ExpectedVersion); err != nil {
			return MutationResult{}, err
		}
		if err := sub.EnsureMutable(); err != nil {
			return MutationResult{}, err
		}
		run := domain.RunValidation(sub, s.clock(), nil)
		sub.ApplyValidation(run, s.clock())
		if err := s.event("validation.completed", sub, cmd.Actor, map[string]any{"runId": run.RunID, "outcome": run.Outcome, "manifestDigest": run.ManifestDigest}); err != nil {
			return MutationResult{}, err
		}
		return MutationResult{Validation: &run}, nil
	})
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Submission, error) {
	sub, err := s.store.Load(ctx, id)
	if err != nil {
		return nil, mapStoreError(err)
	}
	return sub, nil
}
func (s *Service) List(ctx context.Context) ([]*domain.Submission, error) { return s.store.List(ctx) }
