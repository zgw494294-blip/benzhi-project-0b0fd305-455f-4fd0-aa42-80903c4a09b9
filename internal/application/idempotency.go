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
func cloneMutationResult(r MutationResult) MutationResult {
	b, _ := json.Marshal(r)
	var copy MutationResult
	_ = json.Unmarshal(b, &copy)
	return copy
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
	return cloneMutationResult(r.Result), true, nil
}
func (c *idempotencyCache) store(scope, key, digest string, result MutationResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records[scope+"\x00"+key] = idempotentRecord{RequestDigest: digest, Result: cloneMutationResult(result)}
}
