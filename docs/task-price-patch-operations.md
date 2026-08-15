# 任务视频模型按秒/按次计费切换指南

本文用于维护 New API 异步任务模型（视频模型等）的按秒/按次计费方式，重点说明 `TASK_PRICE_PATCH` 的源码行为、生产环境修改流程、验证方法与回滚要求。

## 1. 结论速查

- 模型名出现在 `TASK_PRICE_PATCH` 中：跳过任务适配器生成的全部 `OtherRatios`，按基础模型价格计费，通常表现为“按次计费”。
- 模型名不在 `TASK_PRICE_PATCH` 中：应用时长、分辨率等 `OtherRatios`，通常表现为“基础价格 × 秒数 × 其他倍率”。
- 多个模型使用英文逗号分隔，程序会清理每项两侧空格。
- 模型名采用精确匹配，区分实际请求使用的模型别名；不支持 `veo-*` 等通配符或前缀匹配。
- 该变量仅在 New API 启动时读取。修改 Compose 文件后必须重新创建 `new-api` 服务；单纯执行 `docker restart` 不会加载新的 Compose 环境变量。
- 该补丁会跳过全部 `OtherRatios`，不只是秒数倍率。如果需要“忽略秒数但保留分辨率倍率”，不能使用此变量，需要单独修改计费代码。

## 2. 源码依据

相关实现位于：

- `common/init.go`：读取 `TASK_PRICE_PATCH`，按逗号拆分并清理空格。
- `common/str.go`：`StringsContains` 使用 `s == str` 精确匹配模型名。
- `relay/relay_task.go`：模型命中补丁列表时，不调用 `ApplyOtherRatiosToFloat`，因此时长、分辨率等倍率不会乘入额度。
- `controller/relay.go`：将命中补丁或固定价格的任务记录为 `PerCallBilling`，供异步任务后续结算使用。
- `service/task_billing.go`：命中补丁的任务日志标记为“按次计费”。

简化后的计费关系：

```text
未命中 TASK_PRICE_PATCH：基础模型价格 × 分组倍率 × 时长倍率 × 分辨率倍率 × 其他倍率
命中 TASK_PRICE_PATCH：  基础模型价格 × 分组倍率
```

因此，启用按次计费前必须确认“模型定价”中已经为该模型设置了正确的固定价格。分组倍率仍然有效。

## 3. 配置示例

只让 `seedance2-720p-15s` 按次计费：

```yaml
services:
  new-api:
    environment:
      - TASK_PRICE_PATCH=seedance2-720p-15s
```

让多个模型按次计费：

```yaml
- TASK_PRICE_PATCH=veo-3.1,veo-3.1-fast,seedance2-720p-15s
```

让 `veo-3.1` 和 `veo-3.1-fast` 恢复按秒计费、仅保留 Seedance 按次计费：

```yaml
- TASK_PRICE_PATCH=seedance2-720p-15s
```

让所有模型恢复正常倍率计费：删除 `TASK_PRICE_PATCH` 这一行，或将其设置为空，然后重新创建 `new-api` 服务。

## 4. 当前 88API 生产部署约定

截至 2026-08-09，生产部署的 Compose 源文件为：

```text
/root/new-api/docker-compose.yml
```

该项目创建的容器包括：

- `new-api`
- `new-api-mysql`
- `new-api-redis`

宝塔 Docker 的“容器”页面能看到这些容器，但“容器编排”页面没有对应项目记录。因此：

- 宝塔现有功能无法直接编辑或导入这个 Compose 项目。执行文件编辑和终端命令前，必须向用户说明宝塔无法完成的原因、拟修改文件、影响、备份及回滚方案，并取得明确的底层操作授权；不能把用户最初的修改请求自动理解为该授权。
- 不要只在宝塔容器创建页面临时填写环境变量；下次 Compose 更新会丢失。
- 必须修改 `/root/new-api/docker-compose.yml`，把 Compose 文件作为唯一配置来源。
- 通过宝塔文件管理器编辑文件，通过宝塔终端执行 Compose 校验和重建。
- 不得修改或重启 MySQL、Redis，除非任务明确要求。
- 不得硬编码宝塔会话 URL、密码、数据库连接串或其他凭据到本文档、脚本或日志中。

## 5. 标准操作流程

### 5.1 修改前确认

1. 在宝塔 Docker → 容器中确认 `new-api`、`new-api-mysql`、`new-api-redis` 正在运行。
2. 在 New API 后台“计费与支付 → 模型定价”确认目标模型的固定价格正确。
3. 确认请求使用的模型名与 `TASK_PRICE_PATCH` 中的名称完全一致。
4. 明确本次最终列表。不要只在原列表末尾追加而忘记移除需要恢复按秒计费的模型。

### 5.2 创建规范备份

备份必须位于：

```text
/www/wwwroot/work/backups/new-api/<时间戳>/root/new-api/docker-compose.yml
```

在宝塔终端执行：

```bash
mkdir -p /www/wwwroot/work/backups/new-api/<时间戳>/root/new-api
cp -a /root/new-api/docker-compose.yml /www/wwwroot/work/backups/new-api/<时间戳>/root/new-api/docker-compose.yml
sha256sum /root/new-api/docker-compose.yml /www/wwwroot/work/backups/new-api/<时间戳>/root/new-api/docker-compose.yml
```

两个 SHA-256 必须一致，才能继续修改。

### 5.3 修改 Compose 文件

使用宝塔文件管理器打开：

```text
/root/new-api/docker-compose.yml
```

在 `services.new-api.environment` 中修改 `TASK_PRICE_PATCH`。只保留最终需要按次计费的模型列表。

保存后不要立刻重建，先执行语法校验。

### 5.4 校验 Compose 配置

```bash
docker compose -f /root/new-api/docker-compose.yml config --quiet
echo $?
```

只有退出码为 `0` 时才能继续。校验失败时不要重启或重建容器，应先修复 YAML 或恢复备份。

### 5.5 仅重建 New API 服务

```bash
docker compose -f /root/new-api/docker-compose.yml up -d new-api
```

正常输出应显示 MySQL、Redis 保持 `Running`，只有 `new-api` 被 `Recreate`/`Started`。

## 6. 无费用验证

依次执行：

```bash
docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' new-api | grep '^TASK_PRICE_PATCH='
docker compose -f /root/new-api/docker-compose.yml ps new-api
curl -fsS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3000/
```

必须确认：

- 输出的 `TASK_PRICE_PATCH` 与最终目标列表完全一致。
- 已恢复按秒计费的模型名不再出现在该变量中。
- `new-api` 状态为 `Up`/“运行中”。
- 本地 HTTP 状态码为 `200`。
- 宝塔 Docker → 容器 → `new-api` → 容器详情中能看到相同变量。

实际提交视频任务会产生费用。除非用户明确授权，不要用 Veo、Seedance 等真实生成任务做验证。

配置只影响重建后的新请求。不要假设修改会追溯改变已经提交、正在执行或已经结算的任务。

## 7. 回滚

如果配置错误、容器启动失败或站点异常：

```bash
cp -a /www/wwwroot/work/backups/new-api/<时间戳>/root/new-api/docker-compose.yml /root/new-api/docker-compose.yml
docker compose -f /root/new-api/docker-compose.yml config --quiet
docker compose -f /root/new-api/docker-compose.yml up -d new-api
```

回滚后再次检查环境变量、容器状态和 HTTP `200`。

## 8. 更新镜像时的持久性

只要变量保存在 `/root/new-api/docker-compose.yml` 中，执行以下更新不会丢失配置：

```bash
docker compose -f /root/new-api/docker-compose.yml pull new-api
docker compose -f /root/new-api/docker-compose.yml up -d new-api
```

以下做法可能导致配置丢失：

- 在宝塔“容器”页面删除后手工创建，却漏填环境变量。
- 使用 `docker run` 绕开 Compose 创建容器。
- 更新脚本覆盖整个 `docker-compose.yml`。
- 只修改当前容器而不修改 Compose 源文件。

## 9. 最终回报清单

完成操作后必须报告：

- 实际服务器标识和主机名。
- 修改前后的按次模型列表。
- 修改文件路径。
- 规范备份路径。
- Compose 校验结果。
- 哪些容器被重建，哪些依赖容器保持运行。
- 容器环境变量、运行状态和 HTTP 验证结果。
- 是否执行了会产生费用的真实模型请求。
- 是否涉及站点级或全局 Nginx 配置。
