# RoutePilot CLI 与 AI agent skill

CLI 通过同一套管理 API 控制运行中的 RoutePilot，支持本机和远程后台。默认输出 JSON，失败输出 JSON 到 stderr，适合脚本和 AI agent。无需管理员权限，不直接操作数据库或启动服务。

## 在 App 设置中管理

进入 **设置 → CLI 与 skill**，启用 CLI 后安装到 `~/.local/bin/routepilot`；请把这个目录加入 PATH。关闭 CLI 后移除 App 管理的命令行入口，并拒绝同一用户运行 CLI 发起的控制请求，包括通过绝对路径启动。帮助和版本查询仍可使用；独立源码 CLI 在没有 App 管理开关时正常工作。这个开关管理本机 CLI，不改变后台 HTTP API 的认证设置。

勾选 **自动安装 AI agent skill**，选择 Codex、Claude Code 或两者。Codex 使用 `$CODEX_HOME/skills`（未设置时 `~/.codex/skills`），Claude Code 使用 `~/.claude/skills`。安装的是指向 App 内资源的链接，完整 App 更新后 CLI 和 skill 会同步使用新版本。取消自动安装会移除本 App 管理的 skill。

已有同名文件、目录或其他链接时，设置会列出冲突并提供 **覆盖 / 跳过**，默认选择覆盖。覆盖前将原内容完整移到 CLI 配置目录下的 `backups`，禁用对应集成时恢复原内容；跳过不会覆盖已有内容，设置中显示跳过状态。重新保存可再次处理被跳过的冲突。自动启动时不会未经选择覆盖新的冲突，可在设置中处理。

这些操作由本机 App 执行；普通浏览器或切换远程后台不会在目标远程 Mac 安装 CLI 或修改 agent 目录。打开浏览器时该设置页说明如何在 App 中管理。

## 手动安装

源码构建：

```sh
make cli
./bin/routepilot help
sh scripts/install_cli.sh
```

完整 App 自带 CLI 与 skill。App 放到 `/Applications` 后可执行：

```sh
sh /Applications/RoutePilot.app/Contents/Resources/Service/scripts/install_cli.sh \
  --skill-dir "$HOME/.codex/skills"
```

如果此前已在 App 中禁用 CLI，手动安装链接不会绕过该开关，请先在 App 设置中启用。

默认在 `~/.local/bin/routepilot` 建立符号链接；将 `~/.local/bin` 加入 PATH，或使用二进制绝对路径。该链接随完整 App 升级使用新 CLI。`--bin-dir` 可指定其他绝对目录；`--skill-dir` 可指定 agent 的 skills 目录。不覆盖已有其他命令或已有 skill。不传 `--skill-dir` 时仅安装 CLI，不修改 agent 设置。

skill 源码位于 [`skills/routepilot-control`](../skills/routepilot-control/SKILL.md)，普通支持 SKILL.md 的 agent 也可使用；Codex 安装后可通过 `$routepilot-control` 调用。无需配置 MCP 服务。图形 App 的状态栏偏好、开机启动等仍在 App 中设置。

## 连接与登录

```sh
routepilot version
routepilot help --json
routepilot --url http://192.0.2.10:1055 session
routepilot --url http://192.0.2.10:1055 login --username admin --password-file /absolute/private/password-file
routepilot --url http://192.0.2.10:1055 status
```

也可通过 `ROUTEPILOT_URL` 设置当前目标。默认仅连接 `http://127.0.0.1:1055`；Web 中选中的远程连接不会改变 CLI 的目标。地址仅支持 IP 或 localhost、HTTP/HTTPS，可使用 IPv6，例如 `http://[2001:db8::10]:1055`。默认不使用环境 HTTP 代理。

密码通过 `--password-stdin` 或权限为 0600 的 `--password-file` 读取，不接受密码参数，不保存密码。会话按管理地址分别保存，只保存会话 Cookie，每次操作重新取得 CSRF。macOS 默认目录为 `~/Library/Application Support/RoutePilot/CLI`，Linux 为用户配置目录下 `RoutePilot/CLI`；目录 0700，文件 0600。`--session-dir` / `ROUTEPILOT_SESSION_DIR` 可改位置。会话过期后重新登录；`logout` 撤销会话，`logout --local-only` 在后台不可达时清除本地会话。

认证、管理网段白名单和 CSRF 检查与页面一致。HTTP 可用于受保护网络；HTTPS 默认验证证书，`--ca-file` 可增加自有 CA，不提供跳过验证的选项，也不会跟随重定向。

## 常用操作

```sh
routepilot health
routepilot bgp status
routepilot routing status
routepilot mode set auto
routepilot dns test --domain example.com --client-ip 192.0.2.20
routepilot cache list --domain example.com
routepilot cache flush --domain example.com
routepilot routes list
routepilot routes schema
routepilot routes add --file route.json
routepilot routes update ACTUAL_ID --file revised-route.json
routepilot routes delete ACTUAL_ID
routepilot config wait
routepilot updates check
routepilot updates install --version CHECKED_VERSION --yes
routepilot updates status
```

资源支持 list/schema/add/update/delete，hosts/interception/overrides/peers/routes 还支持 bulk-delete。settings set 和资源 update 接受完整对象；先读取当前配置再修改，不要将部分字段或 schema 示例直接覆盖现有配置。settings schema 返回当前 CLI 版本的默认字段模板，不代表已部署后台。完整备份恢复使用 `config import --file backup.json --yes`。

保存成功表示配置已持久化；服务应用在后台继续进行，使用 `config status` 或 `config wait` 确认结果。查询、导入及清理等长任务默认等待任务结果；全局 `--no-wait` 可立即得到任务 ID，再用 `operation status ID` 查询。请求超时后先查询当前状态，不要重复提交修改。

`updates install` 更新目标 Mac 的**完整 App、网页和网络服务**，提交后用 updates status 跟踪。后台需要支持在线升级、启用认证并标准安装；App 在使用中时仍由目标用户确认重启。命令成功提交不代表升级或 App 重启已完成。

全局选项放在命令之前。JSON 输入 `--file -` 从 stdin 读取；`--pretty` 格式化输出；`metrics` 输出 Prometheus 文本。成功退出 0，请求或客户端错误退出 1，用法错误退出 2。`api METHOD RELATIVE_PATH --file FILE` 覆盖未提供专用命令的管理 API，接受同源相对路径，不支持任意 URL。完整示例见 skill 的 [操作说明](../skills/routepilot-control/references/recipes.md)。
