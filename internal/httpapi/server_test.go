package httpapi

import (
	"bytes"
	"encoding/json"
	"geopack/internal/application"
	"geopack/internal/audit"
	"geopack/internal/domain"
	"geopack/internal/repository"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	log, err := audit.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(application.NewService(repository.NewMemoryStore(), log))
}
func TestCreateIdempotencyAndStrictJSON(t *testing.T) {
	server := testServer(t)
	body := []byte(`{"projectName":"项目","areaBBox":{"minX":1,"minY":1,"maxX":2,"maxY":2},"requiredCrs":"EPSG:4490","maxGroundResolutionCm":10}`)
	var first string
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/submissions", bytes.NewReader(body))
		req.Header.Set("Idempotency-Key", "same")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("创建返回 %d: %s", rec.Code, rec.Body.String())
		}
		var response struct {
			Submission struct {
				ID string `json:"submissionId"`
			} `json:"submission"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = response.Submission.ID
		} else if first != response.Submission.ID {
			t.Fatal("幂等重放创建了新批次")
		}
	}
	bad := append(body[:len(body)-1], []byte(`,"unknown":true}`)...)
	req := httptest.NewRequest(http.MethodPost, "/v1/submissions", bytes.NewReader(bad))
	req.Header.Set("Idempotency-Key", "bad")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("未知字段应拒绝，得到 %d", rec.Code)
	}
}
func TestRouteNotFoundIncludesRequestID(t *testing.T) {
	server := testServer(t)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if rec.Code != http.StatusNotFound || rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("404 缺少请求标识")
	}
}

func TestBatchRegistrationRouteAndStrictListQuery(t *testing.T) {
	server := testServer(t)
	createBody := []byte(`{"projectName":"批量项目","areaBBox":{"minX":1,"minY":1,"maxX":2,"maxY":2},"requiredCrs":"EPSG:4490","maxGroundResolutionCm":10}`)
	createRequest := httptest.NewRequest(http.MethodPost, "/v1/submissions", bytes.NewReader(createBody))
	createRequest.Header.Set("Idempotency-Key", "create-batch-route")
	createResponse := httptest.NewRecorder()
	server.ServeHTTP(createResponse, createRequest)
	var created struct {
		Submission struct {
			ID      string `json:"submissionId"`
			Version uint64 `json:"version"`
		} `json:"submission"`
	}
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	artifact := func(role, name, content string) map[string]any {
		data := []byte(content)
		return map[string]any{"role": role, "filename": name, "sizeBytes": len(data), "sha256": domain.DigestBytes(data), "crs": "EPSG:4490", "groundResolutionCm": 8, "coverageBBox": map[string]any{"minX": 0, "minY": 0, "maxX": 3, "maxY": 3}, "content": data}
	}
	body, err := json.Marshal(map[string]any{"expectedVersion": created.Submission.Version, "actor": "submitter", "items": []any{artifact("orthophoto", "ortho.tif", "ortho"), artifact("point_cloud", "cloud.laz", "cloud")}})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/submissions/"+created.Submission.ID+"/artifacts", bytes.NewReader(body))
	request.Header.Set("Idempotency-Key", "batch-route")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("批量登记返回 %d: %s", response.Code, response.Body.String())
	}
	var registered struct {
		Submission struct {
			Version uint64 `json:"version"`
		} `json:"submission"`
		Artifacts []domain.ArtifactRevision `json:"artifacts"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &registered); err != nil {
		t.Fatal(err)
	}
	if registered.Submission.Version != created.Submission.Version+1 || len(registered.Artifacts) != 2 {
		t.Fatal("批量登记未通过公开入口一次提交")
	}
	invalidList := httptest.NewRecorder()
	server.ServeHTTP(invalidList, httptest.NewRequest(http.MethodGet, "/v1/submissions?projectName=", nil))
	if invalidList.Code != http.StatusUnprocessableEntity {
		t.Fatalf("空白项目关键字应拒绝，得到 %d", invalidList.Code)
	}
}
