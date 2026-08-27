package receipt_object_verification_cache_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"geopack/internal/application"
	"geopack/internal/audit"
	"geopack/internal/domain"
	"geopack/internal/repository"
)

func artifact(role domain.ArtifactRole, content string) domain.ArtifactInput {
	data := []byte(content)
	return domain.ArtifactInput{
		Role:               role,
		Filename:           string(role) + ".dat",
		SizeBytes:          int64(len(data)),
		SHA256:             domain.DigestBytes(data),
		CRS:                "EPSG:4490",
		GroundResolutionCM: 8,
		CoverageBBox:       domain.BBox{MinX: 99, MinY: 19, MaxX: 111, MaxY: 31},
		Content:            data,
	}
}

func TestReceiptRevalidatesObjectAfterReplacement(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := repository.Open(filepath.Join(root, "data"))
	if err != nil {
		t.Fatal(err)
	}
	log, err := audit.Open(filepath.Join(root, "audit"))
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(store, log)

	created, err := service.Create(ctx, application.CreateSubmission{
		IdempotencyKey:        "create-cache-repro",
		ProjectName:           "凭据对象失效复现",
		RequiredCRS:           "EPSG:4490",
		AreaBBox:              domain.BBox{MinX: 100, MinY: 20, MaxX: 110, MaxY: 30},
		MaxGroundResolutionCM: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	inputs := []domain.ArtifactInput{
		artifact(domain.RoleOrthophoto, "orthophoto-content"),
		artifact(domain.RolePointCloud, "point-cloud-content"),
		artifact(domain.RoleControlReport, "control-report-content"),
		artifact(domain.RoleMetadata, "metadata-content"),
	}
	registered, err := service.RegisterBatch(ctx, application.RegisterArtifactBatch{
		SubmissionID: created.Submission.SubmissionID, ExpectedVersion: created.Submission.Version,
		IdempotencyKey: "register-cache-repro", Artifacts: inputs, Actor: "submitter",
	})
	if err != nil {
		t.Fatal(err)
	}
	validated, err := service.Validate(ctx, application.StartValidation{
		SubmissionID: created.Submission.SubmissionID, ExpectedVersion: registered.Submission.Version,
		IdempotencyKey: "validate-cache-repro", Actor: "reviewer",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Freeze(ctx, application.FreezeSubmission{
		SubmissionID: created.Submission.SubmissionID, ExpectedVersion: validated.Submission.Version,
		IdempotencyKey: "freeze-cache-repro", ApprovedBy: "reviewer",
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.Receipt(ctx, created.Submission.SubmissionID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !first.OverallVerified {
		t.Fatal("初次凭据完整性查询应验证通过")
	}

	target := inputs[0]
	objectPath := filepath.Join(root, "data", "objects", target.SHA256[:2], target.SHA256)
	corrupt := bytes.Repeat([]byte{'x'}, len(target.Content))
	if bytes.Equal(corrupt, target.Content) {
		t.Fatal("测试损坏内容必须不同于原对象")
	}
	if err = os.WriteFile(objectPath, corrupt, 0640); err != nil {
		t.Fatal(err)
	}

	second, err := service.Receipt(ctx, created.Submission.SubmissionID, nil)
	if err != nil {
		t.Fatal(err)
	}
	targetDigestMatches := true
	for _, check := range second.ArtifactChecks {
		if check.ExpectedSHA256 == target.SHA256 {
			targetDigestMatches = check.DigestMatches
		}
	}
	if second.OverallVerified || targetDigestMatches {
		t.Fatalf("对象被同尺寸替换后仍复用了旧校验结果: overall=%v digestMatches=%v", second.OverallVerified, targetDigestMatches)
	}
}
