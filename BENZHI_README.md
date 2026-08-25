# BENZHI_README

基于 Go 实现的航测成果接收核验服务 HTTP API 项目，一款后端服务，已完整实现航测成果接收核验服务：支持批次幂等创建、四类成果修订登记、技术核验、差异整改与定向复验、并发版本控制、冻结清单、不可变凭据、审计链校验及真实 HTTP 自检。

## 项目说明
- 项目：benzhi-project-0b0fd305-455f-4fd0-aa42-80903c4a09b9
- 项目用途：已完整实现航测成果接收核验服务：支持批次幂等创建、四类成果修订登记、技术核验、差异整改与定向复验、并发版本控制、冻结清单、不可变凭据、审计链校验及真实 HTTP 自检。
- Go 工具链：`golang:1.22`
- 前端工具链：无

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/geopack -selfcheck -addr=127.0.0.1:19091
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-0b0fd305-455f-4fd0-aa42-80903c4a09b9-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-0b0fd305-455f-4fd0-aa42-80903c4a09b9-arm64 linux/arm64
docker run -it benzhi-project-0b0fd305-455f-4fd0-aa42-80903c4a09b9-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/geopack -selfcheck -addr=127.0.0.1:19091`
