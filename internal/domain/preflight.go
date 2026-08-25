package domain

import (
	"encoding/json"
	"sort"
	"strings"
)

type FreezeBlocker struct {
	Code          string       `json:"code"`
	Role          ArtifactRole `json:"role,omitempty"`
	DiscrepancyID string       `json:"discrepancyId,omitempty"`
	Message       string       `json:"message"`
}

type FreezePreflight struct {
	Ready           bool            `json:"ready"`
	SubmissionID    string          `json:"submissionId"`
	ExpectedVersion uint64          `json:"expectedVersion"`
	ManifestDigest  string          `json:"manifestDigest"`
	RulesetVersion  string          `json:"rulesetVersion"`
	Blockers        []FreezeBlocker `json:"blockers"`
	PreflightToken  string          `json:"preflightToken,omitempty"`
}

func (s *Submission) FreezePreflight(approver string, expectedVersion uint64) FreezePreflight {
	report := FreezePreflight{SubmissionID: s.SubmissionID, ExpectedVersion: expectedVersion, ManifestDigest: s.ManifestDigest(), RulesetVersion: CurrentRuleset, Blockers: []FreezeBlocker{}}
	for _, role := range RequiredRoles {
		if _, ok := s.CurrentRevision(role); !ok {
			report.Blockers = append(report.Blockers, FreezeBlocker{Code: "required_role_missing", Role: role, Message: "缺少必需成果"})
		}
	}
	if strings.TrimSpace(approver) == "" {
		report.Blockers = append(report.Blockers, FreezeBlocker{Code: "approver_required", Message: "复核员不能为空"})
	}
	if s.Version != expectedVersion {
		report.Blockers = append(report.Blockers, FreezeBlocker{Code: "version_mismatch", Message: "预检版本与当前版本不一致"})
	}
	if len(s.ValidationRuns) == 0 {
		report.Blockers = append(report.Blockers, FreezeBlocker{Code: "validation_required", Message: "尚未执行核验"})
	} else {
		latest := s.ValidationRuns[len(s.ValidationRuns)-1]
		if latest.RulesetVersion != CurrentRuleset {
			report.Blockers = append(report.Blockers, FreezeBlocker{Code: "ruleset_outdated", Message: "最近核验未使用最新规则版本"})
		}
		if latest.ManifestDigest != report.ManifestDigest {
			report.Blockers = append(report.Blockers, FreezeBlocker{Code: "manifest_changed", Message: "候选清单已在最近核验后变化"})
		}
		for _, result := range latest.Results {
			if result.Status == CheckFailed {
				report.Blockers = append(report.Blockers, FreezeBlocker{Code: "check_failed", Role: result.Role, Message: result.Code})
			}
		}
	}
	for _, discrepancy := range s.Discrepancies {
		if discrepancy.Status != DiscrepancyClosed {
			report.Blockers = append(report.Blockers, FreezeBlocker{Code: "discrepancy_open", Role: discrepancy.Role, DiscrepancyID: discrepancy.DiscrepancyID, Message: "差异尚未关闭"})
		}
	}
	if s.Status != StatusApprovable {
		report.Blockers = append(report.Blockers, FreezeBlocker{Code: "status_not_approvable", Message: "批次状态不可批准"})
	}
	sort.Slice(report.Blockers, func(i, j int) bool {
		a, b := report.Blockers[i], report.Blockers[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Role != b.Role {
			return a.Role < b.Role
		}
		return a.DiscrepancyID < b.DiscrepancyID
	})
	report.Ready = len(report.Blockers) == 0
	if report.Ready {
		report.PreflightToken = preflightToken(report.SubmissionID, expectedVersion, report.ManifestDigest, report.RulesetVersion)
	}
	return report
}

func preflightToken(submissionID string, version uint64, manifest, ruleset string) string {
	payload, _ := json.Marshal(struct {
		SubmissionID string `json:"submissionId"`
		Version      uint64 `json:"expectedVersion"`
		Manifest     string `json:"manifestDigest"`
		Ruleset      string `json:"rulesetVersion"`
	}{submissionID, version, manifest, ruleset})
	return "pf_" + DigestBytes(payload)
}

func (s *Submission) ValidatePreflightToken(token string) bool {
	return token != "" && token == preflightToken(s.SubmissionID, s.Version, s.ManifestDigest(), CurrentRuleset)
}

type CheckIdentity struct {
	Code   string       `json:"code"`
	Role   ArtifactRole `json:"role,omitempty"`
	Impact string       `json:"impact"`
}

type PredictedCheck struct {
	Code    string       `json:"code"`
	Role    ArtifactRole `json:"role,omitempty"`
	Status  string       `json:"status"`
	Message string       `json:"message"`
}

type ClosureBlocker struct {
	Code          string       `json:"code"`
	Role          ArtifactRole `json:"role,omitempty"`
	DiscrepancyID string       `json:"discrepancyId,omitempty"`
	Message       string       `json:"message"`
}

type RemediationPreview struct {
	AffectedChecks    []CheckIdentity  `json:"affectedChecks"`
	ReusedResults     []CheckResult    `json:"reusedResults"`
	PredictedFailures []PredictedCheck `json:"predictedFailures"`
	ClosureBlockers   []ClosureBlocker `json:"closureBlockers"`
}

type ProposedArtifactMetadata struct {
	Role               ArtifactRole `json:"role"`
	CRS                string       `json:"crs"`
	GroundResolutionCM float64      `json:"groundResolutionCm"`
	CoverageBBox       BBox         `json:"coverageBBox"`
}

func (s *Submission) PreviewRemediation(discrepancyID string, proposed ProposedArtifactMetadata) (RemediationPreview, error) {
	discrepancy, err := FindDiscrepancy(s, discrepancyID)
	if err != nil {
		return RemediationPreview{}, err
	}
	if discrepancy.Status == DiscrepancyClosed || discrepancy.Status == DiscrepancyRemediated {
		return RemediationPreview{}, NewError("discrepancy_not_remediable", "差异 %s 当前状态不可整改", discrepancyID)
	}
	if !proposed.Role.Valid() {
		return RemediationPreview{}, NewError("invalid_artifact_role", "未知成果角色 %q", proposed.Role)
	}
	if discrepancy.Role != "" && discrepancy.Role != proposed.Role {
		return RemediationPreview{}, NewError("incompatible_artifact_role", "拟提交角色与差异关联角色不相容")
	}
	if NormalizeCRS(proposed.CRS) == "" {
		return RemediationPreview{}, NewError("invalid_crs", "坐标基准不能为空")
	}
	if proposed.GroundResolutionCM <= 0 {
		return RemediationPreview{}, NewError("invalid_resolution", "地面分辨率必须大于零")
	}
	if err := proposed.CoverageBBox.Validate(); err != nil {
		return RemediationPreview{}, err
	}
	preview := RemediationPreview{
		AffectedChecks: []CheckIdentity{
			{Code: CheckDigest, Role: proposed.Role, Impact: "direct"}, {Code: CheckCRS, Role: proposed.Role, Impact: "direct"},
			{Code: CheckResolution, Role: proposed.Role, Impact: "direct"}, {Code: CheckCoverage, Role: proposed.Role, Impact: "direct"},
			{Code: CheckRequiredRoles, Impact: "dependency"},
		},
		ReusedResults: []CheckResult{}, PredictedFailures: []PredictedCheck{}, ClosureBlockers: []ClosureBlocker{},
	}
	if len(s.ValidationRuns) > 0 {
		for _, result := range s.ValidationRuns[len(s.ValidationRuns)-1].Results {
			if result.Role != proposed.Role && result.Code != CheckRequiredRoles {
				preview.ReusedResults = append(preview.ReusedResults, result)
			}
		}
	}
	preview.PredictedFailures = append(preview.PredictedFailures, PredictedCheck{Code: CheckDigest, Role: proposed.Role, Status: "pending_content_verification", Message: "内容上传后才能核验字节数和 SHA-256"})
	if NormalizeCRS(proposed.CRS) != NormalizeCRS(s.RequiredCRS) {
		preview.PredictedFailures = append(preview.PredictedFailures, PredictedCheck{Code: CheckCRS, Role: proposed.Role, Status: "failed", Message: "坐标基准仍不一致"})
	}
	if proposed.GroundResolutionCM > s.MaxGroundResolutionCM {
		preview.PredictedFailures = append(preview.PredictedFailures, PredictedCheck{Code: CheckResolution, Role: proposed.Role, Status: "failed", Message: "地面分辨率仍超过阈值"})
	}
	if !proposed.CoverageBBox.Contains(s.AreaBBox) {
		preview.PredictedFailures = append(preview.PredictedFailures, PredictedCheck{Code: CheckCoverage, Role: proposed.Role, Status: "failed", Message: "覆盖范围仍未包含项目范围"})
	}
	for _, failure := range preview.PredictedFailures {
		preview.ClosureBlockers = append(preview.ClosureBlockers, ClosureBlocker{Code: failure.Code, Role: failure.Role, Message: failure.Message})
	}
	for _, other := range s.Discrepancies {
		if other.DiscrepancyID != discrepancyID && other.Status != DiscrepancyClosed {
			preview.ClosureBlockers = append(preview.ClosureBlockers, ClosureBlocker{Code: "other_open_discrepancy", Role: other.Role, DiscrepancyID: other.DiscrepancyID, Message: "另有未关闭差异"})
		}
	}
	sort.Slice(preview.ClosureBlockers, func(i, j int) bool {
		if preview.ClosureBlockers[i].Code == preview.ClosureBlockers[j].Code {
			return preview.ClosureBlockers[i].DiscrepancyID < preview.ClosureBlockers[j].DiscrepancyID
		}
		return preview.ClosureBlockers[i].Code < preview.ClosureBlockers[j].Code
	})
	return preview, nil
}
