package domain

import (
	"fmt"
	"sort"
	"time"
)

const CurrentRuleset = "geopack-rules/1.0"

type ValidationOutcome string

const (
	OutcomePassed ValidationOutcome = "passed"
	OutcomeFailed ValidationOutcome = "failed"
)

type CheckStatus string

const (
	CheckPassed CheckStatus = "passed"
	CheckFailed CheckStatus = "failed"
)

const (
	CheckRequiredRoles = "manifest.required_roles"
	CheckDigest        = "artifact.digest"
	CheckCRS           = "artifact.crs"
	CheckResolution    = "artifact.ground_resolution"
	CheckCoverage      = "artifact.coverage"
)

type CheckResult struct {
	Code        string       `json:"code"`
	Role        ArtifactRole `json:"role,omitempty"`
	Status      CheckStatus  `json:"status"`
	Message     string       `json:"message"`
	SourceRunID string       `json:"sourceRunId,omitempty"`
	CheckedAt   time.Time    `json:"checkedAt"`
}

type ValidationRun struct {
	RunID          string              `json:"runId"`
	SubmissionID   string              `json:"submissionId"`
	ManifestDigest string              `json:"manifestDigest"`
	RulesetVersion string              `json:"rulesetVersion"`
	StartedAt      time.Time           `json:"startedAt"`
	CompletedAt    time.Time           `json:"completedAt"`
	Outcome        ValidationOutcome   `json:"outcome"`
	ManifestRefs   []ArtifactReference `json:"manifestRefs,omitempty"`
	Results        []CheckResult       `json:"results"`
}

func RunValidation(s *Submission, now time.Time, affected map[string]bool) ValidationRun {
	runID := NewID("run", now, fmt.Sprintf("%s-%d", s.SubmissionID, s.Version))
	results := make([]CheckResult, 0, 20)
	add := func(code string, role ArtifactRole, ok bool, msg string) {
		if affected != nil && !affected[checkIdentity(code, role)] && !affected[code] {
			return
		}
		status := CheckPassed
		if !ok {
			status = CheckFailed
		}
		results = append(results, CheckResult{Code: code, Role: role, Status: status, Message: msg, SourceRunID: runID, CheckedAt: now.UTC()})
	}
	missing := []ArtifactRole{}
	for _, role := range RequiredRoles {
		if _, ok := s.CurrentRevision(role); !ok {
			missing = append(missing, role)
		}
	}
	add(CheckRequiredRoles, "", len(missing) == 0, fmt.Sprintf("必需角色缺失: %v", missing))
	for _, role := range RequiredRoles {
		a, ok := s.CurrentRevision(role)
		if !ok {
			continue
		}
		add(CheckDigest, role, ValidateSHA256(a.SHA256) == nil, "文件摘要格式有效")
		add(CheckCRS, role, NormalizeCRS(a.CRS) == NormalizeCRS(s.RequiredCRS), fmt.Sprintf("坐标基准应为 %s，实际为 %s", s.RequiredCRS, a.CRS))
		add(CheckResolution, role, a.GroundResolutionCM <= s.MaxGroundResolutionCM, fmt.Sprintf("分辨率 %.3f cm，阈值 %.3f cm", a.GroundResolutionCM, s.MaxGroundResolutionCM))
		add(CheckCoverage, role, a.CoverageBBox.Validate() == nil && a.CoverageBBox.Contains(s.AreaBBox), "成果覆盖范围应包含项目范围")
	}
	if affected != nil && len(s.ValidationRuns) > 0 {
		previous := s.ValidationRuns[len(s.ValidationRuns)-1]
		for _, old := range previous.Results {
			if !affected[old.Code] && !affected[checkIdentity(old.Code, old.Role)] {
				if old.SourceRunID == "" {
					old.SourceRunID = previous.RunID
				}
				results = append(results, old)
			}
		}
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Code == results[j].Code {
			return results[i].Role < results[j].Role
		}
		return results[i].Code < results[j].Code
	})
	outcome := OutcomePassed
	for _, r := range results {
		if r.Status == CheckFailed {
			outcome = OutcomeFailed
			break
		}
	}
	refs := make([]ArtifactReference, 0, len(s.ManifestArtifacts()))
	for _, artifact := range s.ManifestArtifacts() {
		refs = append(refs, artifact.Reference())
	}
	return ValidationRun{RunID: runID, SubmissionID: s.SubmissionID, ManifestDigest: s.ManifestDigest(), RulesetVersion: CurrentRuleset, StartedAt: now.UTC(), CompletedAt: now.UTC(), Outcome: outcome, ManifestRefs: refs, Results: results}
}

func AffectedChecks(role ArtifactRole) map[string]bool {
	return map[string]bool{
		CheckRequiredRoles:                   true,
		checkIdentity(CheckDigest, role):     true,
		checkIdentity(CheckCRS, role):        true,
		checkIdentity(CheckResolution, role): true,
		checkIdentity(CheckCoverage, role):   true,
	}
}

func checkIdentity(code string, role ArtifactRole) string { return code + "\x00" + string(role) }
