package repository

import (
	"context"
	"geopack/internal/domain"
)

type Store interface {
	Create(context.Context, *domain.Submission) error
	Load(context.Context, string) (*domain.Submission, error)
	Save(context.Context, *domain.Submission, uint64) error
	PutObject(context.Context, []byte, string, int64) (string, error)
	VerifyObject(context.Context, string, int64, string) (ObjectIntegrity, error)
	List(context.Context) ([]*domain.Submission, error)
	LoadIdempotency(context.Context, string, string) (string, []byte, bool, error)
	SaveIdempotency(context.Context, string, string, string, []byte) error
}

type ObjectIntegrity struct {
	ObjectKey      string `json:"objectKey"`
	Exists         bool   `json:"exists"`
	ExpectedSize   int64  `json:"expectedSizeBytes"`
	ActualSize     int64  `json:"actualSizeBytes,omitempty"`
	SizeMatches    bool   `json:"sizeMatches"`
	ExpectedSHA256 string `json:"expectedSha256"`
	ActualSHA256   string `json:"actualSha256,omitempty"`
	DigestMatches  bool   `json:"digestMatches"`
}

type NotFoundError struct{ ID string }

func (e *NotFoundError) Error() string { return "批次不存在: " + e.ID }

type ConflictError struct{ Expected, Actual uint64 }

func (e *ConflictError) Error() string { return "版本冲突" }
