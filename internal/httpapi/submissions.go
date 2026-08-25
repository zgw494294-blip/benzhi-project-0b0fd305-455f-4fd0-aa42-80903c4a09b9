package httpapi

import (
	"geopack/internal/application"
	"geopack/internal/domain"
	"net/http"
	"strings"
)

type createRequest struct {
	ProjectName           string      `json:"projectName"`
	AreaBBox              domain.BBox `json:"areaBBox"`
	RequiredCRS           string      `json:"requiredCrs"`
	MaxGroundResolutionCM float64     `json:"maxGroundResolutionCm"`
}

func (s *Server) CreateSubmission(w http.ResponseWriter, r *http.Request) {
	var body createRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, r, application.NewError("invalid_json", err.Error()))
		return
	}
	result, err := s.service.Create(r.Context(), application.CreateSubmission{IdempotencyKey: r.Header.Get("Idempotency-Key"), ProjectName: body.ProjectName, AreaBBox: body.AreaBBox, RequiredCRS: body.RequiredCRS, MaxGroundResolutionCM: body.MaxGroundResolutionCM})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("ETag", versionETag(result.Submission.Version))
	writeJSON(w, http.StatusCreated, result)
}
func (s *Server) GetSubmission(w http.ResponseWriter, r *http.Request, id string) {
	values, err := strictQuery(r, "role", "fromRevision", "toRevision")
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
	from, err := parseRevision(values, "fromRevision")
	if err != nil {
		writeError(w, r, err)
		return
	}
	to, err := parseRevision(values, "toRevision")
	if err != nil {
		writeError(w, r, err)
		return
	}
	if (from == nil) != (to == nil) || (from != nil && role == nil) {
		writeError(w, r, application.NewError("invalid_comparison_parameters", "比较修订必须同时提供 role、fromRevision 和 toRevision"))
		return
	}
	view, err := s.service.SubmissionDetail(r.Context(), id, role, from, to)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("ETag", versionETag(view.Submission.Version))
	writeJSON(w, http.StatusOK, map[string]any{"submission": view.Submission, "history": view.History, "comparison": view.Comparison, "requestId": requestID(r)})
}
func (s *Server) ListSubmissions(w http.ResponseWriter, r *http.Request) {
	values, err := strictQuery(r, "status", "projectName", "createdFrom", "createdTo", "limit", "cursor")
	if err != nil {
		writeError(w, r, err)
		return
	}
	query := application.ListSubmissionsQuery{}
	if value, present, parseErr := requiredQueryValue(values, "status"); parseErr != nil {
		writeError(w, r, parseErr)
		return
	} else if present {
		status := domain.SubmissionStatus(value)
		query.Status = &status
	}
	if value, present := values["projectName"]; present {
		if strings.TrimSpace(value) == "" {
			writeError(w, r, application.NewError("blank_project_name", "projectName 不能为空"))
			return
		}
		query.ProjectName = strings.TrimSpace(value)
	}
	query.CreatedFrom, err = parseRFC3339Query(values, "createdFrom")
	if err != nil {
		writeError(w, r, err)
		return
	}
	query.CreatedTo, err = parseRFC3339Query(values, "createdTo")
	if err != nil {
		writeError(w, r, err)
		return
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
	items, err := s.service.ListSubmissions(r.Context(), query)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items.Items, "nextCursor": items.NextCursor, "count": items.Count, "summary": items.Summary, "requestId": requestID(r)})
}
func versionETag(version uint64) string { return `"v` + fmtUint(version) + `"` }
