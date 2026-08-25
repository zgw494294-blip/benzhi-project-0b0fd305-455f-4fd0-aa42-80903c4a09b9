package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type checkClient struct {
	base    string
	client  *http.Client
	counter int
}

func (c *checkClient) call(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method == http.MethodPost || method == http.MethodPatch {
		c.counter++
		req.Header.Set("Idempotency-Key", fmt.Sprintf("selfcheck-%03d", c.counter))
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s 返回 %d: %s", method, path, resp.StatusCode, data)
	}
	if out != nil && len(data) > 0 {
		if err = json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("解析响应失败: %w; body=%s", err, data)
		}
	}
	return nil
}

type checkMutation struct {
	Submission struct {
		SubmissionID  string `json:"submissionId"`
		Version       uint64 `json:"version"`
		Discrepancies []struct {
			DiscrepancyID string `json:"discrepancyId"`
		} `json:"discrepancies"`
	} `json:"submission"`
}

func digestText(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}
func artifactPayload(role, filename, content string, resolution float64, version uint64) map[string]any {
	return map[string]any{"expectedVersion": version, "actor": "selfcheck-submitter", "role": role, "filename": filename, "sizeBytes": len(content), "sha256": digestText(content), "crs": "EPSG:4490", "groundResolutionCm": resolution, "coverageBBox": map[string]any{"minX": 99.0, "minY": 19.0, "maxX": 111.0, "maxY": 31.0}, "content": []byte(content)}
}

func runSelfcheck(ctx context.Context, base string) error {
	client := &checkClient{base: base, client: &http.Client{Timeout: 5 * time.Second}}
	deadline := time.NewTicker(25 * time.Millisecond)
	defer deadline.Stop()
	ready := false
	for !ready {
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待 HTTP 就绪超时: %w", ctx.Err())
		case <-deadline.C:
			err := client.call(ctx, http.MethodGet, "/healthz", nil, nil)
			ready = err == nil
		}
	}
	var state checkMutation
	create := map[string]any{"projectName": "自检航测项目", "areaBBox": map[string]any{"minX": 100.0, "minY": 20.0, "maxX": 110.0, "maxY": 30.0}, "requiredCrs": "EPSG:4490", "maxGroundResolutionCm": 10.0}
	if err := client.call(ctx, http.MethodPost, "/v1/submissions", create, &state); err != nil {
		return err
	}
	id := state.Submission.SubmissionID
	version := state.Submission.Version
	artifacts := []map[string]any{artifactPayload("orthophoto", "ortho.tif", "orthophoto-v1", 20, version), artifactPayload("point_cloud", "cloud.laz", "point-cloud-v1", 8, 0), artifactPayload("control_point_report", "control.pdf", "control-report-v1", 8, 0), artifactPayload("metadata", "metadata.json", "metadata-v1", 8, 0)}
	for i, payload := range artifacts {
		payload["expectedVersion"] = version
		if err := client.call(ctx, http.MethodPost, "/v1/submissions/"+id+"/artifacts", payload, &state); err != nil {
			return err
		}
		version = state.Submission.Version
		_ = i
	}
	if err := client.call(ctx, http.MethodPost, "/v1/submissions/"+id+"/validation-runs", map[string]any{"expectedVersion": version, "actor": "selfcheck-reviewer"}, &state); err != nil {
		return err
	}
	version = state.Submission.Version
	if len(state.Submission.Discrepancies) == 0 {
		return fmt.Errorf("自检未生成预期差异")
	}
	disc := state.Submission.Discrepancies[0].DiscrepancyID
	ack := map[string]any{"expectedVersion": version, "assignee": "selfcheck-submitter", "reason": "首版正射影像分辨率超限", "actor": "selfcheck-reviewer"}
	if err := client.call(ctx, http.MethodPatch, "/v1/submissions/"+id+"/discrepancies/"+disc, ack, &state); err != nil {
		return err
	}
	version = state.Submission.Version
	revision := artifactPayload("orthophoto", "ortho-v2.tif", "orthophoto-v2-compliant", 8, 0)
	remedy := map[string]any{"expectedVersion": version, "remediationNote": "重新处理正射影像并达到阈值", "evidenceDigest": digestText("selfcheck-evidence"), "actor": "selfcheck-submitter", "artifact": revision}
	if err := client.call(ctx, http.MethodPost, "/v1/submissions/"+id+"/discrepancies/"+disc+"/revisions", remedy, &state); err != nil {
		return err
	}
	version = state.Submission.Version
	var preflight struct {
		Ready          bool   `json:"ready"`
		PreflightToken string `json:"preflightToken"`
	}
	freezePreview := map[string]any{"preview": true, "expectedVersion": version, "approvedBy": "selfcheck-quality-reviewer"}
	if err := client.call(ctx, http.MethodPost, "/v1/submissions/"+id+"/freeze", freezePreview, &preflight); err != nil {
		return err
	}
	if !preflight.Ready || preflight.PreflightToken == "" {
		return fmt.Errorf("冻结预检未就绪")
	}
	freeze := map[string]any{"expectedVersion": version, "approvedBy": "selfcheck-quality-reviewer", "preflightToken": preflight.PreflightToken}
	if err := client.call(ctx, http.MethodPost, "/v1/submissions/"+id+"/freeze", freeze, &state); err != nil {
		return err
	}
	var receipt map[string]any
	if err := client.call(ctx, http.MethodGet, "/v1/submissions/"+id+"/receipt", nil, &receipt); err != nil {
		return err
	}
	verification, ok := receipt["verification"].(map[string]any)
	checks, checksOK := verification["artifactChecks"].([]any)
	if !ok || verification["manifestVerified"] != true || verification["receiptVerified"] != true || verification["overallVerified"] != true || !checksOK || len(checks) != 4 {
		return fmt.Errorf("凭据重算校验未通过")
	}
	return nil
}
