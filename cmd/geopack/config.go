package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
)

type config struct {
	Addr      string
	DataDir   string
	Selfcheck bool
}

func parseConfig(args []string) (config, error) {
	defaultAddr := "127.0.0.1:19081"
	if port := os.Getenv("PORT"); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return config{}, fmt.Errorf("PORT 必须是 1 至 65535 的端口号")
		}
		defaultAddr = net.JoinHostPort("127.0.0.1", port)
	}
	fs := flag.NewFlagSet("geopack", flag.ContinueOnError)
	cfg := config{}
	fs.StringVar(&cfg.Addr, "addr", defaultAddr, "HTTP 监听地址")
	fs.StringVar(&cfg.DataDir, "data", "./data", "持久化数据目录")
	fs.BoolVar(&cfg.Selfcheck, "selfcheck", false, "运行完整 HTTP 自检后退出")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("不接受位置参数")
	}
	host, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil || host == "" || port == "" {
		return config{}, fmt.Errorf("-addr 必须同时包含主机和端口")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return config{}, fmt.Errorf("-addr 端口无效")
	}
	return cfg, nil
}
