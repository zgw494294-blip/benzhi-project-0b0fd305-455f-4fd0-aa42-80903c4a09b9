package repository

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"geopack/internal/domain"
)

type idempotencyRecord struct {
	SchemaVersion int             `json:"schemaVersion"`
	Scope         string          `json:"scope"`
	Key           string          `json:"key"`
	RequestDigest string          `json:"requestDigest"`
	Response      json.RawMessage `json:"response"`
}

func idempotencyPathDigest(scope, key string) string {
	return domain.DigestBytes([]byte(scope + "\x00" + key))
}

func (s *FileStore) LoadIdempotency(ctx context.Context, scope, key string) (string, []byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.layout.idempotency(idempotencyPathDigest(scope, key)))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	var record idempotencyRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return "", nil, false, err
	}
	if record.SchemaVersion != 1 || record.Scope != scope || record.Key != key {
		return "", nil, false, domain.NewError("idempotency_corrupt", "幂等记录校验失败")
	}
	return record.RequestDigest, append([]byte(nil), record.Response...), true, nil
}

func (s *FileStore) SaveIdempotency(ctx context.Context, scope, key, digest string, response []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.layout.idempotency(idempotencyPathDigest(scope, key))
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	if existing, err := os.ReadFile(path); err == nil {
		var record idempotencyRecord
		if err := json.Unmarshal(existing, &record); err != nil {
			return err
		}
		if record.RequestDigest != digest {
			return domain.NewError("idempotency_conflict", "同一幂等键不能用于不同载荷")
		}
		return nil
	}
	record := idempotencyRecord{SchemaVersion: 1, Scope: scope, Key: key, RequestDigest: digest, Response: append([]byte(nil), response...)}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return atomicWrite(path, data)
}
