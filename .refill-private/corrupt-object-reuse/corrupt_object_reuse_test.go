package bugtest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"geopack/internal/domain"
	"geopack/internal/repository"
)

func TestPutObjectDoesNotReuseCorruptSameSizeObject(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := repository.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("correct-content")
	corrupt := []byte("broken-content!")
	if len(content) != len(corrupt) {
		t.Fatal("测试数据必须等长")
	}
	digest := domain.DigestBytes(content)
	if _, err = store.PutObject(ctx, content, digest, int64(len(content))); err != nil {
		t.Fatal(err)
	}
	objectPath := filepath.Join(root, "objects", digest[:2], digest)
	if err = os.WriteFile(objectPath, corrupt, 0640); err != nil {
		t.Fatal(err)
	}

	_, putErr := store.PutObject(ctx, content, digest, int64(len(content)))
	if putErr != nil {
		return
	}
	check, err := store.VerifyObject(ctx, digest, int64(len(content)), digest)
	if err != nil {
		t.Fatal(err)
	}
	if !check.DigestMatches {
		t.Fatalf("PutObject 静默复用了同尺寸但摘要损坏的对象: %#v", check)
	}
}
