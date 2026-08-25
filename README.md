# 航测成果接收核验服务

本项目是面向无人机航测成果接收场景的单流程 Go HTTP 服务。提交员可以建立批次、原子批量登记正射影像、点云、控制点报告和元数据修订，并查询候选清单的完整修订沿革；系统执行清单完整性、SHA-256、坐标基准、地面分辨率与覆盖范围核验。失败检查会形成可筛选、可批量认领的差异台账，补交前可只读预览影响范围，补交后仅重跑受影响检查。全部核验通过并关闭差异后，质量复核员可先执行冻结就绪预检，再冻结精确清单并签发带递增序号和摘要链的不可变接收凭据。

服务把数据保存在本地目录：聚合采用按版本分代的 JSON 快照，成果内容采用 SHA-256 内容寻址对象，审计事实采用带长度、校验和与前序摘要的只追加帧。启动恢复会验证快照 schema、对象摘要和审计链；幂等响应也会落盘，可在重启后安全重放。

## 构建

```sh
go build ./cmd/geopack
```

## 运行

默认只监听高位回环地址 `127.0.0.1:19081`，数据写入 `./data`：

```sh
go run ./cmd/geopack
```

可以显式指定监听地址和数据目录：

```sh
go run ./cmd/geopack -addr=127.0.0.1:19092 -data=./data
```

也可设置 `PORT` 为端口号，此时未显式传入 `-addr` 时绑定 `127.0.0.1:<PORT>`。地址必须同时包含主机和端口，服务不会默认绑定 `0.0.0.0`。

## 测试与自检

运行全部单元和集成测试：

```sh
go test ./...
```

运行有界的真实 HTTP 自检：

```sh
go run ./cmd/geopack -selfcheck -addr=127.0.0.1:19091
```

自检会在临时数据目录启动真实监听，走完创建批次、登记四类成果、生成差异、认领差异、补交修订、定向复验、冻结、签发和凭据重算校验，然后优雅关闭并自行退出。

## 主要 API

- `POST /v1/submissions`：创建批次，要求 `Idempotency-Key`。
- `GET /v1/submissions`：按 `status`、`projectName`、`createdFrom`、`createdTo` 组合筛选，并使用 `limit`、`cursor` 稳定分页；响应包含进度投影和筛选口径汇总。
- `GET /v1/submissions/{submissionID}`：返回批次及修订历史；可用 `role` 查看单角色历史，或同时提供 `role`、`fromRevision`、`toRevision` 比较两个修订。
- `POST /v1/submissions/{submissionID}/artifacts`：登记单个成果文件修订；正文提供 `items` 时原子登记二至四个不同角色。
- `POST /v1/submissions/{submissionID}/validation-runs`：执行核验。
- `GET /v1/submissions/{submissionID}/validation-runs`：筛选和分页查询核验历史；可用 `fromRunId`、`toRunId` 比较结果演变。
- `GET /v1/submissions/{submissionID}/discrepancies`：按状态、负责人、检查代码和成果角色查询差异台账。
- `PATCH /v1/submissions/{submissionID}/discrepancies`：使用 `items` 原子批量认领差异。
- `PATCH /v1/submissions/{submissionID}/discrepancies/{discrepancyID}`：登记差异负责人和原因。
- `POST /v1/submissions/{submissionID}/discrepancies/{discrepancyID}/revisions`：补交修订并复验；正文设置 `preview:true` 时仅预览影响检查、复用结果和关闭阻塞项。
- `POST /v1/submissions/{submissionID}/freeze`：批准冻结并签发凭据；正文设置 `preview:true` 时只读返回冻结阻塞项及就绪令牌，正式请求可携带 `preflightToken` 防止预检后清单漂移。
- `GET /v1/submissions/{submissionID}/receipt`：逐文件流式重算冻结对象大小与摘要，并验证清单、凭据、前序凭据引用和完整审计轨迹；可用 `role` 筛选明细，但总体结论始终基于完整冻结清单。

所有改变状态的写请求都要求 `Idempotency-Key`，除创建外还要求正文中的 `expectedVersion`；整改影响预览和冻结预检是只读操作，不写入成果、聚合、审计或凭据序号。JSON 解码拒绝未知字段，查询入口拒绝未知、重复和空白参数；版本冲突响应会包含当前聚合版本。
