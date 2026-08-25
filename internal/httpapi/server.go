package httpapi

import (
	"context"
	"fmt"
	"geopack/internal/application"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type Server struct {
	service  *application.Service
	sequence atomic.Uint64
	started  time.Time
}

func New(service *application.Service) *Server {
	return &Server{service: service, started: time.Now().UTC()}
}

type contextKey string

const requestIDKey contextKey = "request-id"

func requestID(r *http.Request) string {
	value, _ := r.Context().Value(requestIDKey).(string)
	return value
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := fmt.Sprintf("req-%012d", s.sequence.Add(1))
	r = r.WithContext(context.WithValue(r.Context(), requestIDKey, id))
	w.Header().Set("X-Request-ID", id)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
		s.Health(w, r)
		return
	}
	if r.URL.Path == "/v1/submissions" {
		if r.Method == http.MethodPost {
			s.CreateSubmission(w, r)
			return
		}
		if r.Method == http.MethodGet {
			s.ListSubmissions(w, r)
			return
		}
	}
	parts := splitPath(r.URL.Path)
	if len(parts) >= 3 && parts[0] == "v1" && parts[1] == "submissions" {
		submissionID := parts[2]
		switch {
		case len(parts) == 3 && r.Method == http.MethodGet:
			s.GetSubmission(w, r, submissionID)
			return
		case len(parts) == 4 && parts[3] == "artifacts" && r.Method == http.MethodPost:
			s.RegisterArtifact(w, r, submissionID)
			return
		case len(parts) == 4 && parts[3] == "validation-runs" && r.Method == http.MethodPost:
			s.StartValidation(w, r, submissionID)
			return
		case len(parts) == 4 && parts[3] == "validation-runs" && r.Method == http.MethodGet:
			s.GetValidationRuns(w, r, submissionID)
			return
		case len(parts) == 4 && parts[3] == "discrepancies" && r.Method == http.MethodGet:
			s.GetDiscrepancies(w, r, submissionID)
			return
		case len(parts) == 4 && parts[3] == "discrepancies" && r.Method == http.MethodPatch:
			s.BatchAcknowledgeDiscrepancies(w, r, submissionID)
			return
		case len(parts) == 4 && parts[3] == "freeze" && r.Method == http.MethodPost:
			s.FreezeSubmission(w, r, submissionID)
			return
		case len(parts) == 4 && parts[3] == "receipt" && r.Method == http.MethodGet:
			s.GetReceipt(w, r, submissionID)
			return
		case len(parts) == 5 && parts[3] == "discrepancies" && r.Method == http.MethodPatch:
			s.UpdateDiscrepancy(w, r, submissionID, parts[4])
			return
		case len(parts) == 6 && parts[3] == "discrepancies" && parts[5] == "revisions" && r.Method == http.MethodPost:
			s.SubmitRemediation(w, r, submissionID, parts[4])
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"code": "route_not_found", "message": "路由不存在", "requestId": id}})
}
func splitPath(path string) []string {
	raw := strings.Split(strings.Trim(path, "/"), "/")
	out := []string{}
	for _, p := range raw {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "geopack", "startedAt": s.started, "requestId": requestID(r)})
}
