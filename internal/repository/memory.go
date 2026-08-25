package repository

import (
	"context"
	"geopack/internal/domain"
	"sync"
)

type MemoryStore struct {
	mu          sync.RWMutex
	submissions map[string]*domain.Submission
	objects     map[string][]byte
	idempotency map[string]memoryIdempotency
}

type memoryIdempotency struct {
	digest   string
	response []byte
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{submissions: map[string]*domain.Submission{}, objects: map[string][]byte{}, idempotency: map[string]memoryIdempotency{}}
}
func (s *MemoryStore) Create(_ context.Context, sub *domain.Submission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.submissions[sub.SubmissionID]; ok {
		return domain.NewError("already_exists", "批次已存在")
	}
	s.submissions[sub.SubmissionID] = domain.CloneSubmission(sub)
	return nil
}
func (s *MemoryStore) Load(_ context.Context, id string) (*domain.Submission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.submissions[id]
	if !ok {
		return nil, &NotFoundError{ID: id}
	}
	return domain.CloneSubmission(sub), nil
}
func (s *MemoryStore) Save(_ context.Context, sub *domain.Submission, expected uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.submissions[sub.SubmissionID]
	if !ok {
		return &NotFoundError{ID: sub.SubmissionID}
	}
	if old.Version != expected {
		return &ConflictError{Expected: expected, Actual: old.Version}
	}
	s.submissions[sub.SubmissionID] = domain.CloneSubmission(sub)
	return nil
}
func (s *MemoryStore) PutObject(_ context.Context, data []byte, digest string, size int64) (string, error) {
	if int64(len(data)) != size || domain.DigestBytes(data) != digest {
		return "", domain.NewError("object_mismatch", "对象内容不匹配")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[digest] = append([]byte(nil), data...)
	return digest, nil
}
func (s *MemoryStore) VerifyObject(_ context.Context, key string, expectedSize int64, expectedDigest string) (ObjectIntegrity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	check := ObjectIntegrity{ObjectKey: key, ExpectedSize: expectedSize, ExpectedSHA256: expectedDigest}
	data, ok := s.objects[key]
	if !ok {
		return check, nil
	}
	check.Exists = true
	check.ActualSize = int64(len(data))
	check.ActualSHA256 = domain.DigestBytes(data)
	check.SizeMatches = check.ActualSize == expectedSize
	check.DigestMatches = check.ActualSHA256 == expectedDigest
	return check, nil
}
func (s *MemoryStore) List(_ context.Context) ([]*domain.Submission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*domain.Submission{}
	for _, sub := range s.submissions {
		out = append(out, domain.CloneSubmission(sub))
	}
	return out, nil
}

func (s *MemoryStore) LoadIdempotency(_ context.Context, scope, key string) (string, []byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.idempotency[scope+"\x00"+key]
	return record.digest, append([]byte(nil), record.response...), ok, nil
}

func (s *MemoryStore) SaveIdempotency(_ context.Context, scope, key, digest string, response []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := scope + "\x00" + key
	if existing, ok := s.idempotency[index]; ok && existing.digest != digest {
		return domain.NewError("idempotency_conflict", "同一幂等键不能用于不同载荷")
	}
	s.idempotency[index] = memoryIdempotency{digest: digest, response: append([]byte(nil), response...)}
	return nil
}
