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
		if info.Size() != size {
			return "", fmt.Errorf("同摘要对象尺寸冲突")
		}
		return digest, nil
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
		if _, statErr := os.Stat(path); statErr == nil {
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
