package application

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"geopack/internal/domain"
)

type ListSubmissionsQuery struct {
	Status      *domain.SubmissionStatus
	ProjectName string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Limit       int
	Cursor      string
}

type SubmissionListItem struct {
	SubmissionID string                    `json:"submissionId"`
	ProjectName  string                    `json:"projectName"`
	Status       domain.SubmissionStatus   `json:"status"`
	Version      uint64                    `json:"version"`
	CreatedAt    time.Time                 `json:"createdAt"`
	UpdatedAt    time.Time                 `json:"updatedAt"`
	Progress     domain.SubmissionProgress `json:"progress"`
}

type SubmissionSummary struct {
	StatusCounts        map[domain.SubmissionStatus]int `json:"statusCounts"`
	PendingRegistration int                             `json:"pendingRegistration"`
	PendingValidation   int                             `json:"pendingValidation"`
	Remediation         int                             `json:"remediation"`
	Approvable          int                             `json:"approvable"`
}

type ListSubmissionsResult struct {
	Items      []SubmissionListItem `json:"items"`
	NextCursor string               `json:"nextCursor,omitempty"`
	Count      int                  `json:"count"`
	Summary    SubmissionSummary    `json:"summary"`
}

type tupleCursor struct {
	QueryDigest string `json:"queryDigest"`
	Timestamp   int64  `json:"timestamp"`
	ID          string `json:"id"`
}

func encodeCursor(cursor tupleCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeCursor(value, digest string) (tupleCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return tupleCursor{}, NewError("invalid_cursor", "cursor 格式无效")
	}
	var cursor tupleCursor
	if err = json.Unmarshal(data, &cursor); err != nil || cursor.ID == "" || cursor.QueryDigest != digest {
		return tupleCursor{}, NewError("invalid_cursor", "cursor 与当前查询条件不匹配")
	}
	return cursor, nil
}

func (s *Service) ListSubmissions(ctx context.Context, query ListSubmissionsQuery) (ListSubmissionsResult, error) {
	if query.Status != nil && !query.Status.Valid() {
		return ListSubmissionsResult{}, NewError("invalid_status", "未知批次状态 %q", *query.Status)
	}
	if query.Limit < 1 || query.Limit > 100 {
		return ListSubmissionsResult{}, NewError("invalid_limit", "limit 必须在 1 至 100 之间")
	}
	if query.CreatedFrom != nil && query.CreatedTo != nil && query.CreatedFrom.After(*query.CreatedTo) {
		return ListSubmissionsResult{}, NewError("invalid_time_range", "createdFrom 不能晚于 createdTo")
	}
	if query.ProjectName != "" && strings.TrimSpace(query.ProjectName) == "" {
		return ListSubmissionsResult{}, NewError("blank_project_name", "projectName 不能为空")
	}
	query.ProjectName = strings.TrimSpace(query.ProjectName)
	all, err := s.store.List(ctx)
	if err != nil {
		return ListSubmissionsResult{}, err
	}
	filtered := make([]*domain.Submission, 0, len(all))
	keyword := strings.ToLower(query.ProjectName)
	for _, sub := range all {
		if query.Status != nil && sub.Status != *query.Status {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(sub.ProjectName), keyword) {
			continue
		}
		if query.CreatedFrom != nil && sub.CreatedAt.Before(*query.CreatedFrom) {
			continue
		}
		if query.CreatedTo != nil && sub.CreatedAt.After(*query.CreatedTo) {
			continue
		}
		filtered = append(filtered, sub)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
			return filtered[i].SubmissionID > filtered[j].SubmissionID
		}
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	summary := SubmissionSummary{StatusCounts: map[domain.SubmissionStatus]int{
		domain.StatusDraft: 0, domain.StatusPendingValidation: 0, domain.StatusRemediation: 0,
		domain.StatusApprovable: 0, domain.StatusFrozen: 0, domain.StatusIssued: 0,
	}}
	for _, sub := range filtered {
		progress := domain.ProjectProgress(sub)
		summary.StatusCounts[sub.Status]++
		if progress.RegisteredRequired < len(domain.RequiredRoles) {
			summary.PendingRegistration++
		}
		if progress.RegisteredRequired == len(domain.RequiredRoles) && (len(sub.ValidationRuns) == 0 || sub.ValidationRuns[len(sub.ValidationRuns)-1].ManifestDigest != sub.ManifestDigest()) {
			summary.PendingValidation++
		}
		if sub.Status == domain.StatusRemediation {
			summary.Remediation++
		}
		if progress.FreezePrerequisitesMet {
			summary.Approvable++
		}
	}
	digest := requestDigest(struct {
		Status   *domain.SubmissionStatus
		Project  string
		From, To *time.Time
	}{query.Status, query.ProjectName, query.CreatedFrom, query.CreatedTo})
	start := 0
	if query.Cursor != "" {
		cursor, err := decodeCursor(query.Cursor, digest)
		if err != nil {
			return ListSubmissionsResult{}, err
		}
		start = len(filtered)
		for index, sub := range filtered {
			stamp := sub.CreatedAt.UnixNano()
			if stamp < cursor.Timestamp || (stamp == cursor.Timestamp && sub.SubmissionID < cursor.ID) {
				start = index
				break
			}
		}
	}
	end := start + query.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	result := ListSubmissionsResult{Items: []SubmissionListItem{}, Count: end - start, Summary: summary}
	for _, sub := range filtered[start:end] {
		result.Items = append(result.Items, SubmissionListItem{SubmissionID: sub.SubmissionID, ProjectName: sub.ProjectName, Status: sub.Status, Version: sub.Version, CreatedAt: sub.CreatedAt, UpdatedAt: sub.UpdatedAt, Progress: domain.ProjectProgress(sub)})
	}
	if end < len(filtered) && end > start {
		last := filtered[end-1]
		result.NextCursor = encodeCursor(tupleCursor{QueryDigest: digest, Timestamp: last.CreatedAt.UnixNano(), ID: last.SubmissionID})
	}
	return result, nil
}

type SubmissionDetail struct {
	Submission *domain.Submission           `json:"submission"`
	History    []domain.RevisionHistoryItem `json:"history"`
	Comparison *domain.RevisionComparison   `json:"comparison,omitempty"`
}

func (s *Service) SubmissionDetail(ctx context.Context, id string, role *domain.ArtifactRole, fromRevision, toRevision *uint32) (SubmissionDetail, error) {
	sub, err := s.Get(ctx, id)
	if err != nil {
		return SubmissionDetail{}, err
	}
	history, err := domain.RevisionHistory(sub, role)
	if err != nil {
		return SubmissionDetail{}, err
	}
	view := SubmissionDetail{Submission: sub, History: history}
	if fromRevision != nil && toRevision != nil && role != nil {
		comparison, err := domain.CompareRevisions(sub, *role, *fromRevision, *toRevision)
		if err != nil {
			return SubmissionDetail{}, err
		}
		view.Comparison = &comparison
	}
	return view, nil
}

type ValidationRunsQuery struct {
	Outcome        *domain.ValidationOutcome
	RulesetVersion string
	CheckCode      string
	Role           *domain.ArtifactRole
	Limit          int
	Cursor         string
	FromRunID      string
	ToRunID        string
}

type ValidationRunView struct {
	domain.ValidationRun
	Integrity domain.RunIntegrity `json:"manifestIntegrity"`
}

type ValidationComparison struct {
	FromRunID string                   `json:"fromRunId"`
	ToRunID   string                   `json:"toRunId"`
	Results   []domain.ResultEvolution `json:"results"`
}

type ValidationRunsResult struct {
	Runs       []ValidationRunView   `json:"runs"`
	NextCursor string                `json:"nextCursor,omitempty"`
	Comparison *ValidationComparison `json:"comparison,omitempty"`
}

func cloneValidationRunsResult(result ValidationRunsResult) ValidationRunsResult {
	data, _ := json.Marshal(result)
	var cloned ValidationRunsResult
	_ = json.Unmarshal(data, &cloned)
	return cloned
}

func validCheckCode(code string) bool {
	switch code {
	case domain.CheckRequiredRoles, domain.CheckDigest, domain.CheckCRS, domain.CheckResolution, domain.CheckCoverage:
		return true
	}
	return false
}

func (s *Service) ValidationRuns(ctx context.Context, id string, query ValidationRunsQuery) (ValidationRunsResult, error) {
	if query.Outcome != nil && *query.Outcome != domain.OutcomePassed && *query.Outcome != domain.OutcomeFailed {
		return ValidationRunsResult{}, NewError("invalid_validation_outcome", "未知核验结论 %q", *query.Outcome)
	}
	if query.CheckCode != "" && !validCheckCode(query.CheckCode) {
		return ValidationRunsResult{}, NewError("invalid_check_code", "未知检查代码 %q", query.CheckCode)
	}
	if query.Role != nil && !query.Role.Valid() {
		return ValidationRunsResult{}, NewError("invalid_artifact_role", "未知成果角色 %q", *query.Role)
	}
	if query.Limit < 1 || query.Limit > 100 {
		return ValidationRunsResult{}, NewError("invalid_limit", "limit 必须在 1 至 100 之间")
	}
	sub, err := s.Get(ctx, id)
	if err != nil {
		return ValidationRunsResult{}, err
	}
	cacheKey := id + "\x00" + strconv.FormatUint(sub.Version, 10) + "\x00" + requestDigest(query)
	s.validationMu.RLock()
	cached, ok := s.validationQueries[cacheKey]
	s.validationMu.RUnlock()
	if ok {
		return cloneValidationRunsResult(cached), nil
	}
	filtered := make([]domain.ValidationRun, 0, len(sub.ValidationRuns))
	for _, run := range sub.ValidationRuns {
		if query.Outcome != nil && run.Outcome != *query.Outcome {
			continue
		}
		if query.RulesetVersion != "" && run.RulesetVersion != query.RulesetVersion {
			continue
		}
		if query.CheckCode != "" || query.Role != nil {
			matched := false
			for _, check := range run.Results {
				if query.CheckCode != "" && check.Code != query.CheckCode {
					continue
				}
				if query.Role != nil && check.Role != *query.Role {
					continue
				}
				matched = true
				break
			}
			if !matched {
				continue
			}
		}
		filtered = append(filtered, run)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].CompletedAt.Equal(filtered[j].CompletedAt) {
			return filtered[i].RunID > filtered[j].RunID
		}
		return filtered[i].CompletedAt.After(filtered[j].CompletedAt)
	})
	digest := requestDigest(struct {
		Outcome     *domain.ValidationOutcome
		Rules, Code string
		Role        *domain.ArtifactRole
	}{query.Outcome, query.RulesetVersion, query.CheckCode, query.Role})
	start := 0
	if query.Cursor != "" {
		cursor, err := decodeCursor(query.Cursor, digest)
		if err != nil {
			return ValidationRunsResult{}, err
		}
		start = len(filtered)
		for index, run := range filtered {
			stamp := run.CompletedAt.UnixNano()
			if stamp < cursor.Timestamp || (stamp == cursor.Timestamp && run.RunID < cursor.ID) {
				start = index
				break
			}
		}
	}
	end := start + query.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	result := ValidationRunsResult{Runs: []ValidationRunView{}}
	for _, run := range filtered[start:end] {
		result.Runs = append(result.Runs, ValidationRunView{ValidationRun: run, Integrity: domain.ValidationRunIntegrity(sub, run)})
	}
	if end < len(filtered) && end > start {
		last := filtered[end-1]
		result.NextCursor = encodeCursor(tupleCursor{QueryDigest: digest, Timestamp: last.CompletedAt.UnixNano(), ID: last.RunID})
	}
	if query.FromRunID != "" && query.ToRunID != "" {
		var from, to *domain.ValidationRun
		for index := range sub.ValidationRuns {
			run := &sub.ValidationRuns[index]
			if run.RunID == query.FromRunID {
				from = run
			}
			if run.RunID == query.ToRunID {
				to = run
			}
		}
		if from == nil || to == nil {
			return ValidationRunsResult{}, NewError("validation_run_not_found", "指定核验运行不存在")
		}
		if from.RunID == to.RunID {
			return ValidationRunsResult{}, NewError("duplicate_run_comparison", "不能比较同一核验运行")
		}
		result.Comparison = &ValidationComparison{FromRunID: from.RunID, ToRunID: to.RunID, Results: domain.CompareValidationRuns(*from, *to)}
	}
	s.validationMu.Lock()
	s.validationQueries[cacheKey] = cloneValidationRunsResult(result)
	s.validationMu.Unlock()
	return result, nil
}

type DiscrepancyQuery struct {
	Status              *domain.DiscrepancyStatus
	Assignee, CheckCode string
	Role                *domain.ArtifactRole
}
type DiscrepancyLedger struct {
	Items   []domain.Discrepancy             `json:"items"`
	Count   int                              `json:"count"`
	Summary map[domain.DiscrepancyStatus]int `json:"summary"`
}

func discrepancySummary(items []domain.Discrepancy) map[domain.DiscrepancyStatus]int {
	summary := map[domain.DiscrepancyStatus]int{domain.DiscrepancyOpen: 0, domain.DiscrepancyAcknowledged: 0, domain.DiscrepancyRemediated: 0, domain.DiscrepancyClosed: 0}
	for _, item := range items {
		summary[item.Status]++
	}
	return summary
}

func (s *Service) Discrepancies(ctx context.Context, id string, query DiscrepancyQuery) (DiscrepancyLedger, error) {
	if query.Status != nil && !query.Status.Valid() {
		return DiscrepancyLedger{}, NewError("invalid_discrepancy_status", "未知差异状态 %q", *query.Status)
	}
	if query.CheckCode != "" && !validCheckCode(query.CheckCode) {
		return DiscrepancyLedger{}, NewError("invalid_check_code", "未知检查代码 %q", query.CheckCode)
	}
	if query.Role != nil && !query.Role.Valid() {
		return DiscrepancyLedger{}, NewError("invalid_artifact_role", "未知成果角色 %q", *query.Role)
	}
	sub, err := s.Get(ctx, id)
	if err != nil {
		return DiscrepancyLedger{}, err
	}
	items := []domain.Discrepancy{}
	for _, item := range sub.Discrepancies {
		if query.Status != nil && item.Status != *query.Status {
			continue
		}
		if query.Assignee != "" && item.Assignee != query.Assignee {
			continue
		}
		if query.CheckCode != "" && item.CheckCode != query.CheckCode {
			continue
		}
		if query.Role != nil && item.Role != *query.Role {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].OpenedAt.Equal(items[j].OpenedAt) {
			return items[i].DiscrepancyID < items[j].DiscrepancyID
		}
		return items[i].OpenedAt.Before(items[j].OpenedAt)
	})
	return DiscrepancyLedger{Items: items, Count: len(items), Summary: discrepancySummary(items)}, nil
}
