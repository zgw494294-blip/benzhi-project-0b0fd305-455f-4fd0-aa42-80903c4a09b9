package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func (s *FileStore) PutObject(ctx context.Context, content []byte, expected string, size int64) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if int64(len(content)) != size {
		return "", fmt.Errorf("对象字节数不匹配")
	}
	h := sha256.New()
	if _, err := io.Copy(h, bytes.NewReader(content)); err != nil {
		return "", err
	}
	digest := hex.EncodeToString(h.Sum(nil))
	if digest != expected {
		return "", fmt.Errorf("对象摘要不匹配")
	}
	path := s.layout.object(digest)
	if info, err := os.Stat(path); err == nil {
		if info.Size() == size && verifyObjectDigest(path, digest) {
			return digest, nil
		}
		// 同路径对象尺寸或摘要不匹配（例如同尺寸损坏内容）：不当作缓存命中，
		// 落到下方原子写入用正确内容覆盖，避免新修订继续引用损坏对象。
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".object-*.tmp")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(name)
		}
	}()
	if _, err = tmp.Write(content); err != nil {
		return "", err
	}
	if err = tmp.Sync(); err != nil {
		return "", err
	}
	if err = tmp.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(name, path); err != nil {
		// 并发上传可能在同一时刻原子写入同一摘要对象；只有当既有对象尺寸与摘要
		// 都正确时才算成功，否则继续报错，避免把损坏对象当作缓存命中。
		if info, statErr := os.Stat(path); statErr == nil && info.Size() == size && verifyObjectDigest(path, digest) {
			ok = true
			return digest, nil
		}
		return "", err
	}
	if err = syncDir(filepath.Dir(path)); err != nil {
		return "", err
	}
	ok = true
	return digest, nil
}

func (s *FileStore) VerifyObject(ctx context.Context, key string, expectedSize int64, expectedDigest string) (ObjectIntegrity, error) {
	check := ObjectIntegrity{ObjectKey: key, ExpectedSize: expectedSize, ExpectedSHA256: expectedDigest}
	f, err := os.Open(s.layout.object(key))
	if errors.Is(err, os.ErrNotExist) {
		return check, nil
	}
	if err != nil {
		return check, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, &contextReader{ctx: ctx, reader: f})
	if err != nil {
		return check, err
	}
	check.Exists = true
	check.ActualSize = n
	check.ActualSHA256 = hex.EncodeToString(h.Sum(nil))
	check.SizeMatches = n == expectedSize
	check.DigestMatches = check.ActualSHA256 == expectedDigest
	return check, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// verifyObjectDigest 重算磁盘上既有对象的 SHA-256，仅在字节数和摘要都匹配时返回 true。
// 这用于内容寻址对象的缓存命中判断，避免同尺寸损坏内容被错误复用。
func verifyObjectDigest(path, expectedDigest string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == expectedDigest
}
