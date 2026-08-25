package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Submission struct {
	SchemaVersion         int                `json:"schemaVersion"`
	SubmissionID          string             `json:"submissionId"`
	ProjectName           string             `json:"projectName"`
	AreaBBox              BBox               `json:"areaBBox"`
	RequiredCRS           string             `json:"requiredCrs"`
	MaxGroundResolutionCM float64            `json:"maxGroundResolutionCm"`
	Status                SubmissionStatus   `json:"status"`
	Version               uint64             `json:"version"`
	CreatedAt             time.Time          `json:"createdAt"`
	UpdatedAt             time.Time          `json:"updatedAt"`
	Artifacts             []ArtifactRevision `json:"artifacts"`
	ValidationRuns        []ValidationRun    `json:"validationRuns"`
	Discrepancies         []Discrepancy      `json:"discrepancies"`
	FrozenManifest        *FrozenManifest    `json:"frozenManifest,omitempty"`
	Receipt               *AcceptanceReceipt `json:"receipt,omitempty"`
}

func NewSubmission(id, project, crs string, bbox BBox, maxResolution float64, now time.Time) (*Submission, error) {
	if strings.TrimSpace(project) == "" {
		return nil, NewError("invalid_project_name", "项目名称不能为空")
	}
	if err := bbox.Validate(); err != nil {
		return nil, err
	}
	if NormalizeCRS(crs) == "" {
		return nil, NewError("invalid_crs", "规定坐标基准不能为空")
	}
	if maxResolution <= 0 {
		return nil, NewError("invalid_resolution", "分辨率阈值必须大于零")
	}
	now = now.UTC()
	return &Submission{SchemaVersion: 1, SubmissionID: id, ProjectName: strings.TrimSpace(project), AreaBBox: bbox, RequiredCRS: NormalizeCRS(crs), MaxGroundResolutionCM: maxResolution, Status: StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now, Artifacts: []ArtifactRevision{}, ValidationRuns: []ValidationRun{}, Discrepancies: []Discrepancy{}}, nil
}

func (s *Submission) EnsureMutable() error {
	if s.Status == StatusFrozen || s.Status == StatusIssued {
		return NewError("submission_frozen", "批次已冻结，不能修改")
	}
	return nil
}

func (s *Submission) CurrentRevision(role ArtifactRole) (ArtifactRevision, bool) {
	var result ArtifactRevision
	found := false
	for _, a := range s.Artifacts {
		if a.Role == role && (!found || a.Revision > result.Revision) {
			result = a
			found = true
		}
	}
	return result, found
}

func (s *Submission) RegisterArtifact(input ArtifactInput, objectKey string, now time.Time) (ArtifactRevision, error) {
	if err := s.EnsureMutable(); err != nil {
		return ArtifactRevision{}, err
	}
	if err := input.Validate(); err != nil {
		return ArtifactRevision{}, err
	}
	revision := uint32(1)
	artifactID := "artifact-" + string(input.Role)
	if old, ok := s.CurrentRevision(input.Role); ok {
		revision = old.Revision + 1
		artifactID = old.ArtifactID
	}
	a := ArtifactRevision{ArtifactID: artifactID, Revision: revision, Role: input.Role, Filename: input.Filename, SizeBytes: input.SizeBytes, SHA256: input.SHA256, CRS: NormalizeCRS(input.CRS), GroundResolutionCM: input.GroundResolutionCM, CoverageBBox: input.CoverageBBox, ObjectKey: objectKey, RegisteredAt: now.UTC()}
	s.Artifacts = append(s.Artifacts, a)
	s.Version++
	s.UpdatedAt = now.UTC()
	s.Status = StatusPendingValidation
	return a, nil
}

func ValidateArtifactBatch(inputs []ArtifactInput) ([]ArtifactInput, error) {
	if len(inputs) < 2 || len(inputs) > len(RequiredRoles) {
		return nil, NewError("invalid_batch_size", "批量登记必须包含二至四个成果")
	}
	seen := map[ArtifactRole]bool{}
	ordered := append([]ArtifactInput(nil), inputs...)
	for index, input := range ordered {
		if err := input.Validate(); err != nil {
			return nil, NewError("invalid_batch_item", "items[%d]: %s", index, err.Error())
		}
		if seen[input.Role] {
			return nil, NewError("duplicate_artifact_role", "items[%d]: 成果角色 %s 重复", index, input.Role)
		}
		seen[input.Role] = true
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Role < ordered[j].Role })
	return ordered, nil
}

func (s *Submission) RegisterArtifactBatch(inputs []ArtifactInput, objectKeys map[string]string, now time.Time) ([]ArtifactRevision, error) {
	if err := s.EnsureMutable(); err != nil {
		return nil, err
	}
	ordered, err := ValidateArtifactBatch(inputs)
	if err != nil {
		return nil, err
	}
	registered := make([]ArtifactRevision, 0, len(ordered))
	for _, input := range ordered {
		revision := uint32(1)
		artifactID := "artifact-" + string(input.Role)
		if old, ok := s.CurrentRevision(input.Role); ok {
			revision = old.Revision + 1
			artifactID = old.ArtifactID
		}
		key, ok := objectKeys[input.SHA256]
		if !ok || key == "" {
			return nil, NewError("object_not_ready", "成果 %s 的内容对象尚未就绪", input.Role)
		}
		registered = append(registered, ArtifactRevision{ArtifactID: artifactID, Revision: revision, Role: input.Role, Filename: input.Filename, SizeBytes: input.SizeBytes, SHA256: input.SHA256, CRS: NormalizeCRS(input.CRS), GroundResolutionCM: input.GroundResolutionCM, CoverageBBox: input.CoverageBBox, ObjectKey: key, RegisteredAt: now.UTC()})
	}
	s.Artifacts = append(s.Artifacts, registered...)
	s.Version++
	s.UpdatedAt = now.UTC()
	s.Status = StatusPendingValidation
	return registered, nil
}

func (s *Submission) ApplyValidation(run ValidationRun, now time.Time) {
	s.ValidationRuns = append(s.ValidationRuns, run)
	s.Version++
	s.UpdatedAt = now.UTC()
	if run.Outcome == OutcomePassed {
		for i := range s.Discrepancies {
			if s.Discrepancies[i].Status == DiscrepancyRemediated {
				s.Discrepancies[i].Close(now)
			}
		}
		s.Status = StatusApprovable
		return
	}
	s.Status = StatusRemediation
	for _, r := range run.Results {
		if r.Status != CheckFailed {
			continue
		}
		exists := false
		for _, d := range s.Discrepancies {
			if d.CheckCode == r.Code && d.Role == r.Role && d.Status != DiscrepancyClosed {
				exists = true
			}
		}
		if !exists {
			id := DigestBytes([]byte(fmt.Sprintf("%s:%s:%s", s.SubmissionID, r.Code, r.Role)))[:20]
			s.Discrepancies = append(s.Discrepancies, Discrepancy{DiscrepancyID: "disc_" + id, CheckCode: r.Code, Role: r.Role, Status: DiscrepancyOpen, OpenedAt: now.UTC()})
		}
	}
}

func (s *Submission) ManifestArtifacts() []ArtifactRevision {
	result := []ArtifactRevision{}
	for _, role := range RequiredRoles {
		if a, ok := s.CurrentRevision(role); ok {
			result = append(result, a)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Role < result[j].Role })
	return result
}

func (s *Submission) ManifestDigest() string {
	return ManifestDigestFor(s.ManifestArtifacts())
}

func ManifestDigestFor(items []ArtifactRevision) string {
	items = append([]ArtifactRevision(nil), items...)
	sort.Slice(items, func(i, j int) bool { return items[i].Role < items[j].Role })
	payload := make([]map[string]any, 0, len(items))
	for _, a := range items {
		payload = append(payload, map[string]any{"artifactId": a.ArtifactID, "revision": a.Revision, "role": a.Role, "filename": a.Filename, "sizeBytes": a.SizeBytes, "sha256": a.SHA256, "crs": a.CRS, "groundResolutionCm": a.GroundResolutionCM, "coverageBBox": a.CoverageBBox})
	}
	b, _ := json.Marshal(payload)
	return DigestBytes(b)
}

func (s *Submission) ArtifactsForReferences(refs []ArtifactReference) ([]ArtifactRevision, bool) {
	items := make([]ArtifactRevision, 0, len(refs))
	for _, ref := range refs {
		found := false
		for _, artifact := range s.Artifacts {
			if artifact.ArtifactID == ref.ArtifactID && artifact.Revision == ref.Revision && artifact.Role == ref.Role {
				items = append(items, artifact)
				found = true
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	return items, true
}

func (s *Submission) Freeze(approver string, now time.Time) (FrozenManifest, error) {
	if s.Status != StatusApprovable {
		return FrozenManifest{}, NewError("not_approvable", "仅全部核验通过的批次可冻结")
	}
	if strings.TrimSpace(approver) == "" {
		return FrozenManifest{}, NewError("invalid_approver", "复核员不能为空")
	}
	for _, d := range s.Discrepancies {
		if d.Status != DiscrepancyClosed {
			return FrozenManifest{}, NewError("open_discrepancy", "仍有未关闭差异")
		}
	}
	s.Version++
	s.Status = StatusFrozen
	s.UpdatedAt = now.UTC()
	m := FrozenManifest{SubmissionID: s.SubmissionID, FrozenVersion: s.Version, RulesetVersion: CurrentRuleset, Artifacts: s.ManifestArtifacts()}
	m.Digest = s.ManifestDigest()
	s.FrozenManifest = &m
	return m, nil
}

func (s *Submission) AttachReceipt(r AcceptanceReceipt, now time.Time) error {
	if s.Status != StatusFrozen {
		return NewError("not_frozen", "批次尚未冻结")
	}
	if s.Receipt != nil {
		return NewError("receipt_exists", "接收凭据已签发")
	}
	s.Receipt = &r
	s.Status = StatusIssued
	s.Version++
	s.UpdatedAt = now.UTC()
	return nil
}
