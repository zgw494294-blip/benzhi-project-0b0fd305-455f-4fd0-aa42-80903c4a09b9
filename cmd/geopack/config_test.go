package main

import "testing"

func TestParseConfigDefaultsAndValidation(t *testing.T) {
	t.Setenv("PORT", "")
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:19081" {
		t.Fatalf("默认地址错误: %s", cfg.Addr)
	}
	if _, err = parseConfig([]string{"-addr=19091"}); err == nil {
		t.Fatal("缺少主机的地址应拒绝")
	}
}
func TestParseConfigPortEnvironment(t *testing.T) {
	t.Setenv("PORT", "19123")
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:19123" {
		t.Fatalf("PORT 未生效: %s", cfg.Addr)
	}
}
