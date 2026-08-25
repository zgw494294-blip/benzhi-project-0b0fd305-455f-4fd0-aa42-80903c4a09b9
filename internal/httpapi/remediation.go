package httpapi

import (
	"geopack/internal/application"
	"geopack/internal/domain"
	"net/http"
)

type remediationRequest struct {
	Preview         bool            `json:"preview,omitempty"`
	ExpectedVersion uint64          `json:"expectedVersion"`
	RemediationNote string          `json:"remediationNote"`
	EvidenceDigest  string          `json:"evidenceDigest"`
	Actor           string          `json:"actor"`
	Artifact        artifactRequest `json:"artifact"`
}

func (s *Server) SubmitRemediation(w http.ResponseWriter, r *http.Request, id, discrepancyID string) {
	var body remediationRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, r, application.NewError("invalid_json", err.Error()))
		return
	}
	if body.Artifact.Items != nil {
		writeError(w, r, application.NewError("invalid_remediation_artifact", "整改修订的 artifact 必须是单项成果"))
		return
	}
	if body.Preview {
		preview, err := s.service.PreviewRemediation(r.Context(), id, discrepancyID, body.ExpectedVersion, domain.ProposedArtifactMetadata{Role: body.Artifact.Role, CRS: body.Artifact.CRS, GroundResolutionCM: body.Artifact.GroundResolutionCM, CoverageBBox: body.Artifact.CoverageBBox})
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"affectedChecks": preview.AffectedChecks, "reusedResults": preview.ReusedResults, "predictedFailures": preview.PredictedFailures, "closureBlockers": preview.ClosureBlockers, "requestId": requestID(r)})
		return
	}
	result, err := s.service.Remediate(r.Context(), application.SubmitRemediation{SubmissionID: id, DiscrepancyID: discrepancyID, ExpectedVersion: body.ExpectedVersion, IdempotencyKey: r.Header.Get("Idempotency-Key"), Artifact: body.Artifact.input(), RemediationNote: body.RemediationNote, EvidenceDigest: body.EvidenceDigest, Actor: body.Actor})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("ETag", versionETag(result.Submission.Version))
	writeJSON(w, http.StatusCreated, result)
}

type freezeRequest struct {
	Preview         bool   `json:"preview,omitempty"`
	ExpectedVersion uint64 `json:"expectedVersion"`
	ApprovedBy      string `json:"approvedBy"`
	PreflightToken  string `json:"preflightToken,omitempty"`
}

func (s *Server) FreezeSubmission(w http.ResponseWriter, r *http.Request, id string) {
	var body freezeRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, r, application.NewError("invalid_json", err.Error()))
		return
	}
	if body.Preview {
		preview, err := s.service.FreezePreview(r.Context(), id, body.ExpectedVersion, body.ApprovedBy)
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ready": preview.Ready, "blockers": preview.Blockers, "preflightToken": preview.PreflightToken, "submissionId": preview.SubmissionID, "expectedVersion": preview.ExpectedVersion, "manifestDigest": preview.ManifestDigest, "rulesetVersion": preview.RulesetVersion, "requestId": requestID(r)})
		return
	}
	result, err := s.service.Freeze(r.Context(), application.FreezeSubmission{SubmissionID: id, ExpectedVersion: body.ExpectedVersion, IdempotencyKey: r.Header.Get("Idempotency-Key"), ApprovedBy: body.ApprovedBy, PreflightToken: body.PreflightToken})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("ETag", versionETag(result.Submission.Version))
	writeJSON(w, http.StatusCreated, result)
}
func (s *Server) GetReceipt(w http.ResponseWriter, r *http.Request, id string) {
	values, err := strictQuery(r, "role")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var role *domain.ArtifactRole
	if value, present, parseErr := requiredQueryValue(values, "role"); parseErr != nil {
		writeError(w, r, parseErr)
		return
	} else if present {
		parsed := domain.ArtifactRole(value)
		role = &parsed
	}
	view, err := s.service.Receipt(r.Context(), id, role)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"verification": view, "requestId": requestID(r)})
}
