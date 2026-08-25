package httpapi

import (
	"errors"
	"geopack/internal/application"
	"geopack/internal/domain"
	"net/http"
)

type errorResponse struct {
	Error struct {
		Code          string `json:"code"`
		Message       string `json:"message"`
		RequestID     string `json:"requestId"`
		LatestVersion uint64 `json:"latestVersion,omitempty"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	message := "服务内部错误"
	version := uint64(0)
	var ae *application.Error
	var de *domain.Error
	if errors.As(err, &ae) {
		code = ae.Code
		message = ae.Message
		version = ae.Version
		switch code {
		case "submission_not_found", "receipt_not_found", "validation_run_not_found":
			status = http.StatusNotFound
		case "version_conflict", "idempotency_conflict", "preflight_expired":
			status = http.StatusConflict
		case "idempotency_key_required", "expected_version_required":
			status = http.StatusBadRequest
		default:
			status = http.StatusUnprocessableEntity
		}
	}
	if errors.As(err, &de) {
		code = de.Code
		message = de.Message
		status = http.StatusUnprocessableEntity
	}
	var response errorResponse
	response.Error.Code = code
	response.Error.Message = message
	response.Error.RequestID = requestID(r)
	response.Error.LatestVersion = version
	writeJSON(w, status, response)
}
