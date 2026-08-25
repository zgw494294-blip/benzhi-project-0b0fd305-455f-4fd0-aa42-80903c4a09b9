package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

type SubmissionStatus string

const (
	StatusDraft             SubmissionStatus = "draft"
	StatusPendingValidation SubmissionStatus = "pending_validation"
	StatusRemediation       SubmissionStatus = "remediation"
	StatusApprovable        SubmissionStatus = "approvable"
	StatusFrozen            SubmissionStatus = "frozen"
	StatusIssued            SubmissionStatus = "issued"
)

type ArtifactRole string

const (
	RoleOrthophoto    ArtifactRole = "orthophoto"
	RolePointCloud    ArtifactRole = "point_cloud"
	RoleControlReport ArtifactRole = "control_point_report"
	RoleMetadata      ArtifactRole = "metadata"
)

var RequiredRoles = []ArtifactRole{RoleControlReport, RoleMetadata, RoleOrthophoto, RolePointCloud}
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (r ArtifactRole) Valid() bool {
	switch r {
	case RoleOrthophoto, RolePointCloud, RoleControlReport, RoleMetadata:
		return true
	}
	return false
}

func (s SubmissionStatus) Valid() bool {
	switch s {
	case StatusDraft, StatusPendingValidation, StatusRemediation, StatusApprovable, StatusFrozen, StatusIssued:
		return true
	}
	return false
}

type BBox struct {
	MinX float64 `json:"minX"`
	MinY float64 `json:"minY"`
	MaxX float64 `json:"maxX"`
	MaxY float64 `json:"maxY"`
}

func (b BBox) Validate() error {
	values := []float64{b.MinX, b.MinY, b.MaxX, b.MaxY}
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return NewError("invalid_bbox", "覆盖范围包含非有限数值")
		}
	}
	if b.MinX >= b.MaxX || b.MinY >= b.MaxY {
		return NewError("invalid_bbox", "覆盖范围最小值必须小于最大值")
	}
	return nil
}

func (b BBox) Contains(other BBox) bool {
	return other.MinX >= b.MinX && other.MinY >= b.MinY && other.MaxX <= b.MaxX && other.MaxY <= b.MaxY
}

func ValidateSHA256(value string) error {
	if !digestPattern.MatchString(value) {
		return NewError("invalid_sha256", "SHA-256 必须为 64 位小写十六进制")
	}
	return nil
}

func DigestBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func NormalizeCRS(value string) string { return strings.ToUpper(strings.TrimSpace(value)) }

func NewID(prefix string, now time.Time, entropy string) string {
	raw := fmt.Sprintf("%s\x00%d\x00%s", prefix, now.UTC().UnixNano(), entropy)
	return prefix + "_" + DigestBytes([]byte(raw))[:20]
}
