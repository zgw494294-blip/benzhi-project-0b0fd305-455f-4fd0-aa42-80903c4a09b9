package domain

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type ArtifactRevision struct {
	ArtifactID         string       `json:"artifactId"`
	Revision           uint32       `json:"revision"`
	Role               ArtifactRole `json:"role"`
	Filename           string       `json:"filename"`
	SizeBytes          int64        `json:"sizeBytes"`
	SHA256             string       `json:"sha256"`
	CRS                string       `json:"crs"`
	GroundResolutionCM float64      `json:"groundResolutionCm"`
	CoverageBBox       BBox         `json:"coverageBBox"`
	ObjectKey          string       `json:"objectKey"`
	RegisteredAt       time.Time    `json:"registeredAt"`
}

type ArtifactInput struct {
	Role               ArtifactRole `json:"role"`
	Filename           string       `json:"filename"`
	SizeBytes          int64        `json:"sizeBytes"`
	SHA256             string       `json:"sha256"`
	CRS                string       `json:"crs"`
	GroundResolutionCM float64      `json:"groundResolutionCm"`
	CoverageBBox       BBox         `json:"coverageBBox"`
	Content            []byte       `json:"-"`
}

type ArtifactReference struct {
	ArtifactID string       `json:"artifactId"`
	Revision   uint32       `json:"revision"`
	Role       ArtifactRole `json:"role"`
}

func (i ArtifactInput) Validate() error {
	if !i.Role.Valid() {
		return NewError("invalid_artifact_role", "未知成果角色 %q", i.Role)
	}
	if strings.TrimSpace(i.Filename) == "" || filepath.Base(i.Filename) != i.Filename {
		return NewError("invalid_filename", "文件名必须是不含路径的名称")
	}
	if i.SizeBytes <= 0 {
		return NewError("invalid_size", "文件字节数必须大于零")
	}
	if err := ValidateSHA256(i.SHA256); err != nil {
		return err
	}
	if int64(len(i.Content)) != i.SizeBytes {
		return NewError("content_size_mismatch", "声明字节数与内容不一致")
	}
	if DigestBytes(i.Content) != i.SHA256 {
		return NewError("content_digest_mismatch", "声明摘要与内容不一致")
	}
	if NormalizeCRS(i.CRS) == "" {
		return NewError("invalid_crs", "坐标基准不能为空")
	}
	if i.GroundResolutionCM <= 0 {
		return NewError("invalid_resolution", "地面分辨率必须大于零")
	}
	return i.CoverageBBox.Validate()
}

func (a ArtifactRevision) Key() string { return fmt.Sprintf("%s:%d", a.ArtifactID, a.Revision) }

func (a ArtifactRevision) Reference() ArtifactReference {
	return ArtifactReference{ArtifactID: a.ArtifactID, Revision: a.Revision, Role: a.Role}
}
