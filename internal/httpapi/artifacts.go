package httpapi

import (
	"geopack/internal/application"
	"geopack/internal/domain"
	"net/http"
	"strconv"
)

type artifactRequest struct {
	ExpectedVersion    uint64              `json:"expectedVersion"`
	Actor              string              `json:"actor"`
	Role               domain.ArtifactRole `json:"role"`
	Filename           string              `json:"filename"`
	SizeBytes          int64               `json:"sizeBytes"`
	SHA256             string              `json:"sha256"`
	CRS                string              `json:"crs"`
	GroundResolutionCM float64             `json:"groundResolutionCm"`
	CoverageBBox       domain.BBox         `json:"coverageBBox"`
	Content            []byte              `json:"content"`
	Items              *[]artifactItem     `json:"items,omitempty"`
}

type artifactItem struct {
	Role               domain.ArtifactRole `json:"role"`
	Filename           string              `json:"filename"`
	SizeBytes          int64               `json:"sizeBytes"`
	SHA256             string              `json:"sha256"`
	CRS                string              `json:"crs"`
	GroundResolutionCM float64             `json:"groundResolutionCm"`
	CoverageBBox       domain.BBox         `json:"coverageBBox"`
	Content            []byte              `json:"content"`
}

func (a artifactItem) input() domain.ArtifactInput {
	return domain.ArtifactInput{Role: a.Role, Filename: a.Filename, SizeBytes: a.SizeBytes, SHA256: a.SHA256, CRS: a.CRS, GroundResolutionCM: a.GroundResolutionCM, CoverageBBox: a.CoverageBBox, Content: a.Content}
}

func (a artifactRequest) input() domain.ArtifactInput {
	return domain.ArtifactInput{Role: a.Role, Filename: a.Filename, SizeBytes: a.SizeBytes, SHA256: a.SHA256, CRS: a.CRS, GroundResolutionCM: a.GroundResolutionCM, CoverageBBox: a.CoverageBBox, Content: a.Content}
}
func (s *Server) RegisterArtifact(w http.ResponseWriter, r *http.Request, id string) {
	var body artifactRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, r, application.NewError("invalid_json", err.Error()))
		return
	}
	if body.Items != nil {
		if body.Role != "" || body.Filename != "" || body.SizeBytes != 0 || body.SHA256 != "" || body.CRS != "" || body.GroundResolutionCM != 0 || len(body.Content) != 0 {
			writeError(w, r, application.NewError("mixed_artifact_payload", "批量 items 不能与单项成果字段同时出现"))
			return
		}
		items := make([]domain.ArtifactInput, 0, len(*body.Items))
		for _, item := range *body.Items {
			items = append(items, item.input())
		}
		result, err := s.service.RegisterBatch(r.Context(), application.RegisterArtifactBatch{SubmissionID: id, ExpectedVersion: body.ExpectedVersion, IdempotencyKey: r.Header.Get("Idempotency-Key"), Artifacts: items, Actor: body.Actor})
		if err != nil {
			writeError(w, r, err)
			return
		}
		w.Header().Set("ETag", versionETag(result.Submission.Version))
		writeJSON(w, http.StatusCreated, result)
		return
	}
	result, err := s.service.Register(r.Context(), application.RegisterArtifact{SubmissionID: id, ExpectedVersion: body.ExpectedVersion, IdempotencyKey: r.Header.Get("Idempotency-Key"), Artifact: body.input(), Actor: body.Actor})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("ETag", versionETag(result.Submission.Version))
	writeJSON(w, http.StatusCreated, result)
}
func fmtUint(value uint64) string { return strconv.FormatUint(value, 10) }
