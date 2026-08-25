package domain

import (
	"strings"
	"time"
)

type DiscrepancyStatus string

const (
	DiscrepancyOpen         DiscrepancyStatus = "open"
	DiscrepancyAcknowledged DiscrepancyStatus = "acknowledged"
	DiscrepancyRemediated   DiscrepancyStatus = "remediated"
	DiscrepancyClosed       DiscrepancyStatus = "closed"
)

type Discrepancy struct {
	DiscrepancyID      string            `json:"discrepancyId"`
	CheckCode          string            `json:"checkCode"`
	Role               ArtifactRole      `json:"role,omitempty"`
	Status             DiscrepancyStatus `json:"status"`
	Assignee           string            `json:"assignee,omitempty"`
	Reason             string            `json:"reason,omitempty"`
	RemediationNote    string            `json:"remediationNote,omitempty"`
	EvidenceDigest     string            `json:"evidenceDigest,omitempty"`
	ResolvedByRevision string            `json:"resolvedByRevision,omitempty"`
	OpenedAt           time.Time         `json:"openedAt"`
	ClosedAt           *time.Time        `json:"closedAt,omitempty"`
}

func (s DiscrepancyStatus) Valid() bool {
	switch s {
	case DiscrepancyOpen, DiscrepancyAcknowledged, DiscrepancyRemediated, DiscrepancyClosed:
		return true
	}
	return false
}

func (d *Discrepancy) Acknowledge(assignee, reason string) error {
	if d.Status != DiscrepancyOpen && d.Status != DiscrepancyAcknowledged {
		return NewError("invalid_discrepancy_transition", "仅待处理差异可登记责任")
	}
	if strings.TrimSpace(assignee) == "" || strings.TrimSpace(reason) == "" {
		return NewError("invalid_remediation", "负责人和原因不能为空")
	}
	d.Assignee = strings.TrimSpace(assignee)
	d.Reason = strings.TrimSpace(reason)
	d.Status = DiscrepancyAcknowledged
	return nil
}

type DiscrepancyAssignment struct {
	DiscrepancyID string `json:"discrepancyId"`
	Assignee      string `json:"assignee"`
	Reason        string `json:"reason"`
}

func (s *Submission) AcknowledgeBatch(assignments []DiscrepancyAssignment, now time.Time) ([]Discrepancy, error) {
	if len(assignments) == 0 {
		return nil, NewError("empty_discrepancy_batch", "认领项不能为空")
	}
	if len(assignments) > 50 {
		return nil, NewError("discrepancy_batch_too_large", "单次最多认领 50 个差异")
	}
	seen := map[string]bool{}
	indices := make([]int, len(assignments))
	for index, assignment := range assignments {
		if strings.TrimSpace(assignment.DiscrepancyID) == "" {
			return nil, NewError("invalid_discrepancy_id", "items[%d].discrepancyId 不能为空", index)
		}
		if seen[assignment.DiscrepancyID] {
			return nil, NewError("duplicate_discrepancy_id", "差异 %s 重复", assignment.DiscrepancyID)
		}
		seen[assignment.DiscrepancyID] = true
		if strings.TrimSpace(assignment.Assignee) == "" {
			return nil, NewError("invalid_assignee", "差异 %s 的 assignee 不能为空", assignment.DiscrepancyID)
		}
		if strings.TrimSpace(assignment.Reason) == "" {
			return nil, NewError("invalid_reason", "差异 %s 的 reason 不能为空", assignment.DiscrepancyID)
		}
		found := -1
		for i := range s.Discrepancies {
			if s.Discrepancies[i].DiscrepancyID == assignment.DiscrepancyID {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, NewError("discrepancy_not_found", "差异项不存在: %s", assignment.DiscrepancyID)
		}
		if s.Discrepancies[found].Status != DiscrepancyOpen {
			return nil, NewError("discrepancy_not_claimable", "差异 %s 当前状态 %s 不可认领", assignment.DiscrepancyID, s.Discrepancies[found].Status)
		}
		indices[index] = found
	}
	updated := make([]Discrepancy, 0, len(assignments))
	for index, assignment := range assignments {
		d := &s.Discrepancies[indices[index]]
		_ = d.Acknowledge(assignment.Assignee, assignment.Reason)
		updated = append(updated, *d)
	}
	s.Version++
	s.UpdatedAt = now.UTC()
	return updated, nil
}

func (d *Discrepancy) Remediate(note, evidence, revision string) error {
	if d.Status != DiscrepancyAcknowledged && d.Status != DiscrepancyRemediated {
		return NewError("invalid_discrepancy_transition", "差异需先登记责任")
	}
	if note == "" || ValidateSHA256(evidence) != nil || revision == "" {
		return NewError("invalid_remediation", "整改说明、证据摘要和修订引用均为必填")
	}
	d.RemediationNote = note
	d.EvidenceDigest = evidence
	d.ResolvedByRevision = revision
	d.Status = DiscrepancyRemediated
	return nil
}

func (d *Discrepancy) Close(now time.Time) {
	d.Status = DiscrepancyClosed
	t := now.UTC()
	d.ClosedAt = &t
}
