package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "geopack:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	cleanup := func() {}
	if cfg.Selfcheck && cfg.DataDir == "./data" {
		cfg.DataDir, err = os.MkdirTemp("", "geopack-selfcheck-")
		if err != nil {
			return err
		}
		cleanup = func() { _ = os.RemoveAll(cfg.DataDir) }
	}
	defer cleanup()
	rt, err := buildRuntime(cfg)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", cfg.Addr, err)
	}
	serveErr := make(chan error, 1)
	go func() {
		err := rt.Server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()
	if cfg.Selfcheck {
		return runAndStopSelfcheck(rt.Server, listener.Addr().String(), serveErr)
	}
	fmt.Printf("航测成果接收核验服务监听 %s\n", listener.Addr())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case sig := <-signals:
		fmt.Printf("收到 %s，准备停止\n", sig)
	case err := <-serveErr:
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return rt.Server.Shutdown(ctx)
}

func runAndStopSelfcheck(server *http.Server, addr string, serveErr <-chan error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	flowErr := runSelfcheck(ctx, "http://"+addr)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	serverErr := <-serveErr
	if flowErr != nil {
		return flowErr
	}
	if shutdownErr != nil {
		return shutdownErr
	}
	if serverErr != nil {
		return serverErr
	}
	fmt.Println("selfcheck: 完整整改、冻结和凭据校验流程通过")
	return nil
}
