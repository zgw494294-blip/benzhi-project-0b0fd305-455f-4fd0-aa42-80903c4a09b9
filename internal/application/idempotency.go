package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
)

type idempotentRecord struct {
	RequestDigest string
	Result        MutationResult
}
type idempotencyCache struct {
	mu      sync.Mutex
	records map[string]idempotentRecord
}

func newIdempotencyCache() *idempotencyCache {
	return &idempotencyCache{records: map[string]idempotentRecord{}}
}
func requestDigest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (c *idempotencyCache) lookup(scope, key, digest string) (MutationResult, bool, error) {
	if key == "" {
		return MutationResult{}, false, NewError("idempotency_key_required", "Idempotency-Key 不能为空")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.records[scope+"\x00"+key]
	if !ok {
		return MutationResult{}, false, nil
	}
	if r.RequestDigest != digest {
		return MutationResult{}, false, NewError("idempotency_conflict", "同一幂等键不能用于不同载荷")
	}
	r.Result.Submission = domainClone(r.Result.Submission)
	return r.Result, true, nil
}
func (c *idempotencyCache) store(scope, key, digest string, result MutationResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result.Submission = domainClone(result.Submission)
	c.records[scope+"\x00"+key] = idempotentRecord{RequestDigest: digest, Result: result}
}
