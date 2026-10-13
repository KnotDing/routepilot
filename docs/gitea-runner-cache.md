# Gitea runner 镜像缓存

2026-10-13 的 beta.17 / beta.18 发布运行在执行工作流步骤前，均因访问 Docker Hub 的 TLS 握手超时而失败。日志显示 `forcePull=true`。这属于 runner 镜像准备阶段；GitHub 回归和发布成功、Gitea 回退包生成，都不能替代 Gitea 工作流成功。

当前日志标识为 act_runner v0.2.11。该版本将 `container.force_pull` 传给 act v0.261.2；为 true 时直接拉取，跳过本地镜像存在性检查。设为 false 后先检查本地镜像，缺少对应镜像 / 平台时才拉取。固定镜像 tag 或摘要不能覆盖该版本的强制拉取设置。

在 runner **现有配置文件**中修改以下字段，保留其他内容：

```yaml
container:
  force_pull: false
```

这是 runner 的运行配置，不能通过仓库工作流的环境变量覆盖。需要在已确认的 runner 部署位置应用，并让 runner 重新读取配置；应等正在运行的任务完成，只重载该 runner，不重启 Docker 或 RoutePilot。用户已修改 runner 设置；本轮发布将通过新任务日志核对设置是否已加载。

在 runner 使用的 Docker 环境中，可只读检查缓存：

```sh
docker image inspect gitea/runner-images:ubuntu-latest --format '{{.Id}}'
```

下一轮日志应显示 `forcePull=false`、`Image exists? true`，并直接进入工作流步骤。缓存缺失时仍会拉取；需要更新镜像时显式拉取，再继续复用本地版本。

源码依据：[runner 配置示例](https://gitea.com/gitea/act_runner/src/tag/v0.2.11/internal/pkg/config/config.example.yaml)、[runner 配置传递](https://gitea.com/gitea/act_runner/src/tag/v0.2.11/internal/app/run/runner.go)、[实际镜像拉取实现](https://gitea.com/gitea/act/src/tag/v0.261.2/pkg/container/docker_pull.go)。

## 工作流成功条件

Gitea 28 支持作业级 `continue-on-error`。GitHub 获取任务作为可恢复路径；只有制品发布和三附件上传均完成才输出 `published=true`。否则执行 macOS 回退构建发布任务，且该任务没有容错标记。两条路径中任一发布成功即使整体成功；两条均失败则由回退任务保留失败结果。获取超时、校验或上传失败仍在任务日志中清楚记录。
