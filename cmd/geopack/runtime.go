package main

import (
	"fmt"
	"geopack/internal/application"
	"geopack/internal/audit"
	"geopack/internal/httpapi"
	"geopack/internal/repository"
	"net/http"
	"os"
	"path/filepath"
)

type runtime struct {
	Server *http.Server
	Audit  *audit.Log
}

func buildRuntime(cfg config) (runtime, error) {
	if err := os.MkdirAll(cfg.DataDir, 0750); err != nil {
		return runtime{}, err
	}
	store, err := repository.Open(filepath.Join(cfg.DataDir, "repository"))
	if err != nil {
		return runtime{}, fmt.Errorf("恢复仓库失败: %w", err)
	}
	log, err := audit.Open(filepath.Join(cfg.DataDir, "audit"))
	if err != nil {
		return runtime{}, fmt.Errorf("恢复审计日志失败: %w", err)
	}
	service := application.NewService(store, log)
	handler := httpapi.New(service)
	server := &http.Server{Addr: cfg.Addr, Handler: handler, ReadHeaderTimeout: 5e9, ReadTimeout: 35e9, WriteTimeout: 35e9, IdleTimeout: 60e9}
	return runtime{Server: server, Audit: log}, nil
}
