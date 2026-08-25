package repository

import (
	"context"
	"geopack/internal/domain"
	"testing"
	"time"
)

func TestFileStorePersistsSnapshotObjectAndIdempotency(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	sub, err := domain.NewSubmission("sub-1", "项目", "EPSG:4490", domain.BBox{MinX: 1, MinY: 1, MaxX: 2, MaxY: 2}, 10, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Create(ctx, sub); err != nil {
		t.Fatal(err)
	}
	content := []byte("object")
	digest := domain.DigestBytes(content)
	if _, err = store.PutObject(ctx, content, digest, int64(len(content))); err != nil {
		t.Fatal(err)
	}
	_, err = sub.RegisterArtifact(domain.ArtifactInput{Role: domain.RoleMetadata, Filename: "metadata.json", SizeBytes: int64(len(content)), SHA256: digest, CRS: "EPSG:4490", GroundResolutionCM: 1, CoverageBBox: domain.BBox{MinX: 0, MinY: 0, MaxX: 3, MaxY: 3}, Content: content}, digest, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Save(ctx, sub, 1); err != nil {
		t.Fatal(err)
	}
	if err = store.SaveIdempotency(ctx, "scope", "key", "request", []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Load(ctx, "sub-1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != 2 || len(loaded.Artifacts) != 1 {
		t.Fatal("快照恢复内容错误")
	}
	d, body, ok, err := reopened.LoadIdempotency(ctx, "scope", "key")
	if err != nil || !ok || d != "request" || len(body) == 0 {
		t.Fatal("幂等记录未恢复")
	}
}
func TestFileStoreRejectsStaleVersion(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sub, err := domain.NewSubmission("sub-1", "项目", "EPSG:4490", domain.BBox{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}, 10, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Create(context.Background(), sub); err != nil {
		t.Fatal(err)
	}
	sub.Version = 2
	if err = store.Save(context.Background(), sub, 99); err == nil {
		t.Fatal("陈旧 expectedVersion 应失败")
	}
}
