package httpapi

import (
	"geopack/internal/application"
	"geopack/internal/domain"
	"net/http"
)

type validationRequest struct {
	ExpectedVersion uint64 `json:"expectedVersion"`
	Actor           string `json:"actor"`
}

func (s *Server) StartValidation(w http.ResponseWriter, r *http.Request, id string) {
	var body validationRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, r, application.NewError("invalid_json", err.Error()))
		return
	}
	result, err := s.service.Validate(r.Context(), application.StartValidation{SubmissionID: id, ExpectedVersion: body.ExpectedVersion, IdempotencyKey: r.Header.Get("Idempotency-Key"), Actor: body.Actor})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("ETag", versionETag(result.Submission.Version))
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) GetValidationRuns(w http.ResponseWriter, r *http.Request, id string) {
	values, err := strictQuery(r, "outcome", "rulesetVersion", "checkCode", "role", "limit", "cursor", "fromRunId", "toRunId")
	if err != nil {
		writeError(w, r, err)
		return
	}
	query := application.ValidationRunsQuery{}
	if value, present, parseErr := requiredQueryValue(values, "outcome"); parseErr != nil {
		writeError(w, r, parseErr)
		return
	} else if present {
		outcome := domain.ValidationOutcome(value)
		query.Outcome = &outcome
	}
	if value, present, parseErr := requiredQueryValue(values, "rulesetVersion"); parseErr != nil {
		writeError(w, r, parseErr)
		return
	} else if present {
		query.RulesetVersion = value
	}
	if value, present, parseErr := requiredQueryValue(values, "checkCode"); parseErr != nil {
		writeError(w, r, parseErr)
		return
	} else if present {
		query.CheckCode = value
	}
	if value, present, parseErr := requiredQueryValue(values, "role"); parseErr != nil {
		writeError(w, r, parseErr)
		return
	} else if present {
		role := domain.ArtifactRole(value)
		query.Role = &role
	}
	query.Limit, err = parseLimit(values, 20)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if value, present, parseErr := requiredQueryValue(values, "cursor"); parseErr != nil {
		writeError(w, r, parseErr)
		return
	} else if present {
		query.Cursor = value
	}
	if value, present, parseErr := requiredQueryValue(values, "fromRunId"); parseErr != nil {
		writeError(w, r, parseErr)
		return
	} else if present {
		query.FromRunID = value
	}
	if value, present, parseErr := requiredQueryValue(values, "toRunId"); parseErr != nil {
		writeError(w, r, parseErr)
		return
	} else if present {
		query.ToRunID = value
	}
	if (query.FromRunID == "") != (query.ToRunID == "") {
		writeError(w, r, application.NewError("invalid_comparison_parameters", "比较核验运行必须同时提供 fromRunId 和 toRunId"))
		return
	}
	result, err := s.service.ValidationRuns(r.Context(), id, query)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": result.Runs, "nextCursor": result.NextCursor, "comparison": result.Comparison, "requestId": requestID(r)})
}

type discrepancyRequest struct {
	ExpectedVersion uint64 `json:"expectedVersion"`
	Assignee        string `json:"assignee"`
	Reason          string `json:"reason"`
	RemediationNote string `json:"remediationNote,omitempty"`
	EvidenceDigest  string `json:"evidenceDigest,omitempty"`
	Actor           string `json:"actor"`
}

func (s *Server) UpdateDiscrepancy(w http.ResponseWriter, r *http.Request, id, discrepancyID string) {
	var body discrepancyRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, r, application.NewError("invalid_json", err.Error()))
		return
	}
	result, err := s.service.UpdateDiscrepancy(r.Context(), application.UpdateDiscrepancy{SubmissionID: id, DiscrepancyID: discrepancyID, ExpectedVersion: body.ExpectedVersion, IdempotencyKey: r.Header.Get("Idempotency-Key"), Assignee: body.Assignee, Reason: body.Reason, RemediationNote: body.RemediationNote, EvidenceDigest: body.EvidenceDigest, Actor: body.Actor})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("ETag", versionETag(result.Submission.Version))
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) GetDiscrepancies(w http.ResponseWriter, r *http.Request, id string) {
	values, err := strictQuery(r, "status", "assignee", "checkCode", "role")
	if err != nil {
		writeError(w, r, err)
		return
	}
	query := application.DiscrepancyQuery{}
	if value, present, parseErr := requiredQueryValue(values, "status"); parseErr != nil {
		writeError(w, r, parseErr)
		return
	} else if present {
		status := domain.DiscrepancyStatus(value)
		query.Status = &status
	}
	if value, present, parseErr := requiredQueryValue(values, "assignee"); parseErr != nil {
		writeError(w, r, parseErr)
		return
	} else if present {
		query.Assignee = value
	}
	if value, present, parseErr := requiredQueryValue(values, "checkCode"); parseErr != nil {
		writeError(w, r, parseErr)
		return
	} else if present {
		query.CheckCode = value
	}
	if value, present, parseErr := requiredQueryValue(values, "role"); parseErr != nil {
		writeError(w, r, parseErr)
		return
	} else if present {
		role := domain.ArtifactRole(value)
		query.Role = &role
	}
	result, err := s.service.Discrepancies(r.Context(), id, query)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result.Items, "count": result.Count, "summary": result.Summary, "requestId": requestID(r)})
}

type batchDiscrepancyRequest struct {
	ExpectedVersion uint64                         `json:"expectedVersion"`
	Items           []domain.DiscrepancyAssignment `json:"items"`
	Actor           string                         `json:"actor"`
}

func (s *Server) BatchAcknowledgeDiscrepancies(w http.ResponseWriter, r *http.Request, id string) {
	var body batchDiscrepancyRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, r, application.NewError("invalid_json", err.Error()))
		return
	}
	result, err := s.service.BatchAcknowledge(r.Context(), application.BatchAcknowledgeDiscrepancies{SubmissionID: id, ExpectedVersion: body.ExpectedVersion, IdempotencyKey: r.Header.Get("Idempotency-Key"), Items: body.Items, Actor: body.Actor})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("ETag", versionETag(result.Submission.Version))
	writeJSON(w, http.StatusOK, result)
}
