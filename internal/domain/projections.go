package domain

import (
	"fmt"
	"reflect"
	"sort"
)

type RoleProgress struct {
	Role            ArtifactRole `json:"role"`
	Registered      bool         `json:"registered"`
	RevisionCount   int          `json:"revisionCount"`
	CurrentRevision uint32       `json:"currentRevision,omitempty"`
}

type SubmissionProgress struct {
	Roles                  []RoleProgress    `json:"roles"`
	RegisteredRequired     int               `json:"registeredRequired"`
	LatestValidation       ValidationOutcome `json:"latestValidation,omitempty"`
	LatestValidationRunID  string            `json:"latestValidationRunId,omitempty"`
	OpenDiscrepancyCount   int               `json:"openDiscrepancyCount"`
	FreezePrerequisitesMet bool              `json:"freezePrerequisitesMet"`
}

func ProjectProgress(s *Submission) SubmissionProgress {
	projection := SubmissionProgress{Roles: make([]RoleProgress, 0, len(RequiredRoles))}
	for _, role := range RequiredRoles {
		item := RoleProgress{Role: role}
		for _, artifact := range s.Artifacts {
			if artifact.Role == role {
				item.RevisionCount++
				if artifact.Revision > item.CurrentRevision {
					item.CurrentRevision = artifact.Revision
				}
			}
		}
		item.Registered = item.CurrentRevision > 0
		if item.Registered {
			projection.RegisteredRequired++
		}
		projection.Roles = append(projection.Roles, item)
	}
	if len(s.ValidationRuns) > 0 {
		latest := s.ValidationRuns[len(s.ValidationRuns)-1]
		projection.LatestValidation = latest.Outcome
		projection.LatestValidationRunID = latest.RunID
	}
	for _, discrepancy := range s.Discrepancies {
		if discrepancy.Status != DiscrepancyClosed {
			projection.OpenDiscrepancyCount++
		}
	}
	projection.FreezePrerequisitesMet = projection.RegisteredRequired == len(RequiredRoles) && projection.LatestValidation == OutcomePassed && projection.OpenDiscrepancyCount == 0 && s.Status == StatusApprovable
	return projection
}

type RevisionHistoryItem struct {
	ArtifactRevision
	Current            bool     `json:"current"`
	FrozenReference    bool     `json:"frozenReference"`
	ReferencedByRunIDs []string `json:"referencedByRunIds"`
}

func RevisionHistory(s *Submission, role *ArtifactRole) ([]RevisionHistoryItem, error) {
	if role != nil && !role.Valid() {
		return nil, NewError("invalid_artifact_role", "未知成果角色 %q", *role)
	}
	items := make([]RevisionHistoryItem, 0, len(s.Artifacts))
	for _, artifact := range s.Artifacts {
		if role != nil && artifact.Role != *role {
			continue
		}
		current, _ := s.CurrentRevision(artifact.Role)
		item := RevisionHistoryItem{ArtifactRevision: artifact, Current: current.Revision == artifact.Revision}
		if s.FrozenManifest != nil {
			for _, frozen := range s.FrozenManifest.Artifacts {
				if frozen.ArtifactID == artifact.ArtifactID && frozen.Revision == artifact.Revision {
					item.FrozenReference = true
				}
			}
		}
		for _, run := range s.ValidationRuns {
			for _, ref := range run.ManifestRefs {
				if ref.ArtifactID == artifact.ArtifactID && ref.Revision == artifact.Revision && ref.Role == artifact.Role {
					item.ReferencedByRunIDs = append(item.ReferencedByRunIDs, run.RunID)
					break
				}
			}
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Role == items[j].Role {
			return items[i].Revision < items[j].Revision
		}
		return items[i].Role < items[j].Role
	})
	return items, nil
}

type FieldDifference struct {
	Field   string `json:"field"`
	Changed bool   `json:"changed"`
	From    any    `json:"from"`
	To      any    `json:"to"`
}

type RevisionComparison struct {
	Role         ArtifactRole      `json:"role"`
	FromRevision uint32            `json:"fromRevision"`
	ToRevision   uint32            `json:"toRevision"`
	Fields       []FieldDifference `json:"fields"`
}

func CompareRevisions(s *Submission, role ArtifactRole, fromRevision, toRevision uint32) (RevisionComparison, error) {
	if !role.Valid() {
		return RevisionComparison{}, NewError("invalid_artifact_role", "未知成果角色 %q", role)
	}
	if fromRevision == toRevision {
		return RevisionComparison{}, NewError("duplicate_revision_comparison", "不能比较同一修订")
	}
	find := func(revision uint32) (ArtifactRevision, bool) {
		for _, artifact := range s.Artifacts {
			if artifact.Role == role && artifact.Revision == revision {
				return artifact, true
			}
		}
		return ArtifactRevision{}, false
	}
	from, fromOK := find(fromRevision)
	to, toOK := find(toRevision)
	if !fromOK || !toOK {
		for _, artifact := range s.Artifacts {
			if artifact.Role != role && ((!fromOK && artifact.Revision == fromRevision) || (!toOK && artifact.Revision == toRevision)) {
				return RevisionComparison{}, NewError("cross_role_comparison", "只能比较同一成果角色的修订")
			}
		}
		return RevisionComparison{}, NewError("artifact_revision_not_found", "指定修订不存在")
	}
	field := func(name string, a, b any) FieldDifference {
		return FieldDifference{Field: name, Changed: !reflect.DeepEqual(a, b), From: a, To: b}
	}
	return RevisionComparison{Role: role, FromRevision: fromRevision, ToRevision: toRevision, Fields: []FieldDifference{
		field("filename", from.Filename, to.Filename), field("sizeBytes", from.SizeBytes, to.SizeBytes),
		field("sha256", from.SHA256, to.SHA256), field("crs", from.CRS, to.CRS),
		field("groundResolutionCm", from.GroundResolutionCM, to.GroundResolutionCM), field("coverageBBox", from.CoverageBBox, to.CoverageBBox),
	}}, nil
}

type RunIntegrity struct {
	Reproducible bool   `json:"reproducible"`
	Warning      string `json:"warning,omitempty"`
}

func ValidationRunIntegrity(s *Submission, run ValidationRun) RunIntegrity {
	if len(run.ManifestRefs) == 0 {
		if run.ManifestDigest == ManifestDigestFor(nil) {
			return RunIntegrity{Reproducible: true}
		}
		return RunIntegrity{Reproducible: false, Warning: "历史运行未保存清单修订引用"}
	}
	artifacts, ok := s.ArtifactsForReferences(run.ManifestRefs)
	if !ok {
		return RunIntegrity{Reproducible: false, Warning: "历史运行引用的成果修订不存在"}
	}
	actual := ManifestDigestFor(artifacts)
	if actual != run.ManifestDigest {
		return RunIntegrity{Reproducible: false, Warning: fmt.Sprintf("清单摘要不一致：期望 %s，实际 %s", run.ManifestDigest, actual)}
	}
	return RunIntegrity{Reproducible: true}
}

type ResultEvolution struct {
	Code       string       `json:"code"`
	Role       ArtifactRole `json:"role,omitempty"`
	Change     string       `json:"change"`
	FromResult *CheckResult `json:"fromResult,omitempty"`
	ToResult   *CheckResult `json:"toResult,omitempty"`
}

func CompareValidationRuns(from, to ValidationRun) []ResultEvolution {
	type keyed struct {
		code string
		role ArtifactRole
	}
	fromMap := map[keyed]CheckResult{}
	toMap := map[keyed]CheckResult{}
	keys := map[keyed]bool{}
	for _, result := range from.Results {
		key := keyed{result.Code, result.Role}
		fromMap[key] = result
		keys[key] = true
	}
	for _, result := range to.Results {
		key := keyed{result.Code, result.Role}
		toMap[key] = result
		keys[key] = true
	}
	ordered := make([]keyed, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].code == ordered[j].code {
			return ordered[i].role < ordered[j].role
		}
		return ordered[i].code < ordered[j].code
	})
	items := make([]ResultEvolution, 0, len(ordered))
	for _, key := range ordered {
		left, leftOK := fromMap[key]
		right, rightOK := toMap[key]
		item := ResultEvolution{Code: key.code, Role: key.role}
		switch {
		case !leftOK:
			item.Change = "added"
			item.ToResult = &right
		case !rightOK:
			item.Change = "removed"
			item.FromResult = &left
		default:
			item.FromResult, item.ToResult = &left, &right
			switch {
			case left.Status == CheckFailed && right.Status == CheckPassed:
				item.Change = "failed_to_passed"
			case left.Status == CheckPassed && right.Status == CheckFailed:
				item.Change = "passed_to_failed"
			default:
				item.Change = "unchanged"
			}
		}
		items = append(items, item)
	}
	return items
}
