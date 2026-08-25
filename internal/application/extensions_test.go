package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"geopack/internal/audit"
	"geopack/internal/domain"
	"geopack/internal/repository"
)

func extensionService(t *testing.T) *Service {
	t.Helper()
	log, err := audit.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(repository.NewMemoryStore(), log)
	service.clock = func() time.Time { return time.Unix(1000, 0).UTC() }
	return service
}

func createForExtension(t *testing.T, service *Service, key, project string) *domain.Submission {
	t.Helper()
	result, err := service.Create(context.Background(), CreateSubmission{IdempotencyKey: key, ProjectName: project, RequiredCRS: "EPSG:4490", AreaBBox: domain.BBox{MinX: 100, MinY: 20, MaxX: 110, MaxY: 30}, MaxGroundResolutionCM: 10})
	if err != nil {
		t.Fatal(err)
	}
	return result.Submission
}

func extensionArtifact(role domain.ArtifactRole, content string) domain.ArtifactInput {
	data := []byte(content)
	return domain.ArtifactInput{Role: role, Filename: string(role) + ".dat", SizeBytes: int64(len(data)), SHA256: domain.DigestBytes(data), CRS: "EPSG:4490", GroundResolutionCM: 8, CoverageBBox: domain.BBox{MinX: 99, MinY: 19, MaxX: 111, MaxY: 31}, Content: data}
}

func TestBatchRegistrationIsAtomicAndIdempotent(t *testing.T) {
	service := extensionService(t)
	sub := createForExtension(t, service, "create-batch", "批量项目")
	valid := extensionArtifact(domain.RoleOrthophoto, "ortho")
	invalid := extensionArtifact(domain.RolePointCloud, "cloud")
	invalid.SHA256 = domain.DigestBytes([]byte("different"))
	command := RegisterArtifactBatch{SubmissionID: sub.SubmissionID, ExpectedVersion: sub.Version, IdempotencyKey: "bad-batch", Artifacts: []domain.ArtifactInput{valid, invalid}, Actor: "submitter"}
	if _, err := service.RegisterBatch(context.Background(), command); err == nil {
		t.Fatal("摘要不一致的批量登记应失败")
	}
	afterFailure, err := service.Get(context.Background(), sub.SubmissionID)
	if err != nil {
		t.Fatal(err)
	}
	if afterFailure.Version != sub.Version || len(afterFailure.Artifacts) != 0 {
		t.Fatal("失败批量登记改变了聚合")
	}
	command.IdempotencyKey = "good-batch"
	command.Artifacts[1] = extensionArtifact(domain.RolePointCloud, "cloud")
	first, err := service.RegisterBatch(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.RegisterBatch(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if first.Submission.Version != sub.Version+1 || second.Submission.Version != first.Submission.Version || len(first.Artifacts) != 2 {
		t.Fatal("批量登记未一次提交或幂等重放不一致")
	}
}

func TestStableCursorForEqualCreationTimes(t *testing.T) {
	service := extensionService(t)
	for index, name := range []string{"同名项目甲", "同名项目乙", "同名项目丙"} {
		createForExtension(t, service, string(rune('a'+index)), name)
	}
	query := ListSubmissionsQuery{ProjectName: "同名项目", Limit: 1}
	seen := map[string]bool{}
	for {
		page, err := service.ListSubmissions(context.Background(), query)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 1 || seen[page.Items[0].SubmissionID] {
			t.Fatal("游标分页发生遗漏或重复")
		}
		seen[page.Items[0].SubmissionID] = true
		if page.NextCursor == "" {
			break
		}
		query.Cursor = page.NextCursor
	}
	if len(seen) != 3 {
		t.Fatalf("期望取得 3 个批次，实际 %d", len(seen))
	}
}

func TestFreezePreflightExpiresAfterManifestChange(t *testing.T) {
	service := extensionService(t)
	sub := createForExtension(t, service, "create-preflight", "预检项目")
	items := []domain.ArtifactInput{
		extensionArtifact(domain.RoleOrthophoto, "ortho-v1"), extensionArtifact(domain.RolePointCloud, "cloud-v1"),
		extensionArtifact(domain.RoleControlReport, "control-v1"), extensionArtifact(domain.RoleMetadata, "metadata-v1"),
	}
	registered, err := service.RegisterBatch(context.Background(), RegisterArtifactBatch{SubmissionID: sub.SubmissionID, ExpectedVersion: sub.Version, IdempotencyKey: "preflight-batch", Artifacts: items, Actor: "submitter"})
	if err != nil {
		t.Fatal(err)
	}
	validated, err := service.Validate(context.Background(), StartValidation{SubmissionID: sub.SubmissionID, ExpectedVersion: registered.Submission.Version, IdempotencyKey: "preflight-validate", Actor: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	preflight, err := service.FreezePreview(context.Background(), sub.SubmissionID, validated.Submission.Version, "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if !preflight.Ready || preflight.PreflightToken == "" {
		t.Fatal("合格批次冻结预检应就绪")
	}
	changed, err := service.Register(context.Background(), RegisterArtifact{SubmissionID: sub.SubmissionID, ExpectedVersion: validated.Submission.Version, IdempotencyKey: "manifest-change", Artifact: extensionArtifact(domain.RoleOrthophoto, "ortho-v2"), Actor: "submitter"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Freeze(context.Background(), FreezeSubmission{SubmissionID: sub.SubmissionID, ExpectedVersion: validated.Submission.Version, IdempotencyKey: "stale-preflight", ApprovedBy: "reviewer", PreflightToken: preflight.PreflightToken})
	var appError *Error
	if !errors.As(err, &appError) || appError.Code != "preflight_expired" {
		t.Fatalf("清单变化后应拒绝旧预检令牌，实际 %v", err)
	}
	current, err := service.Get(context.Background(), sub.SubmissionID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Version != changed.Submission.Version || current.FrozenManifest != nil {
		t.Fatal("过期预检请求改变了批次")
	}
}
