package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"geopack/internal/domain"
)

type FileStore struct {
	mu     sync.RWMutex
	layout layout
}

func Open(root string) (*FileStore, error) {
	if root == "" {
		return nil, fmt.Errorf("数据目录不能为空")
	}
	l := layout{root: root}
	if err := os.MkdirAll(l.objectsDir(), 0750); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(l.submissionsDir(), 0750); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(l.idempotencyDir(), 0750); err != nil {
		return nil, err
	}
	s := &FileStore{layout: l}
	if err := s.verifyAll(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *FileStore) Create(ctx context.Context, sub *domain.Submission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Stat(s.layout.index(sub.SubmissionID)); err == nil {
		return fmt.Errorf("批次已存在")
	}
	return s.writeLocked(sub)
}

func (s *FileStore) Load(ctx context.Context, id string) (*domain.Submission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.loadLocked(id)
}

func (s *FileStore) loadLocked(id string) (*domain.Submission, error) {
	raw, err := os.ReadFile(s.layout.index(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, &NotFoundError{ID: id}
	}
	if err != nil {
		return nil, err
	}
	version, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("CURRENT 索引损坏: %w", err)
	}
	data, err := os.ReadFile(s.layout.generation(id, version))
	if err != nil {
		return nil, err
	}
	var sub domain.Submission
	if err = json.Unmarshal(data, &sub); err != nil {
		return nil, fmt.Errorf("快照 JSON 损坏: %w", err)
	}
	if sub.SchemaVersion != 1 {
		return nil, fmt.Errorf("不支持的 schemaVersion %d", sub.SchemaVersion)
	}
	if sub.Version != version {
		return nil, fmt.Errorf("索引与快照版本不一致")
	}
	return &sub, nil
}

func (s *FileStore) Save(ctx context.Context, sub *domain.Submission, expected uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := s.loadLocked(sub.SubmissionID)
	if err != nil {
		return err
	}
	if current.Version != expected {
		return &ConflictError{Expected: expected, Actual: current.Version}
	}
	if current.Status == domain.StatusIssued && sub.Version != current.Version {
		return fmt.Errorf("已签发批次不可修改")
	}
	return s.writeLocked(sub)
}

func (s *FileStore) writeLocked(sub *domain.Submission) error {
	dir := s.layout.submissionDir(sub.SubmissionID)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sub, "", "  ")
	if err != nil {
		return err
	}
	generation := s.layout.generation(sub.SubmissionID, sub.Version)
	if err := atomicWrite(generation, data); err != nil {
		return err
	}
	if err := atomicWrite(s.layout.index(sub.SubmissionID), []byte(strconv.FormatUint(sub.Version, 10)+"\n")); err != nil {
		return err
	}
	return syncDir(dir)
}

func (s *FileStore) List(ctx context.Context) ([]*domain.Submission, error) {
	entries, err := os.ReadDir(s.layout.submissionsDir())
	if err != nil {
		return nil, err
	}
	result := []*domain.Submission{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sub, err := s.Load(ctx, entry.Name())
		if err != nil {
			return nil, err
		}
		result = append(result, sub)
	}
	return result, nil
}

func (s *FileStore) verifyAll() error {
	entries, err := os.ReadDir(s.layout.submissionsDir())
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sub, err := s.Load(context.Background(), entry.Name())
		if err != nil {
			return err
		}
		for _, a := range sub.Artifacts {
			path := s.layout.object(a.ObjectKey)
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("成果对象 %s 缺失: %w", a.ObjectKey, err)
			}
			if int64(len(data)) != a.SizeBytes || domain.DigestBytes(data) != a.SHA256 {
				return fmt.Errorf("成果对象 %s 校验失败", a.ObjectKey)
			}
		}
	}
	return nil
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".snapshot-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(name)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	return syncDir(dir)
}
func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
