# Agent 工作规范

适用于 RoutePilot 仓库中的所有 AI agent。用户当前请求决定任务范围；先阅读相关代码和文档，再实施。这里记录工作约定与快速入口，具体实现以当前源码为准。

## 开始任务

1. 执行 `git status --short --branch`，阅读目标文件和已有差异，识别本次任务、前序未提交工作与无关文件。保留所有无关改动，不使用 `git add .` 混入发布。
2. 从 [README](README.md) 了解用户流程，按需阅读 [架构说明](docs/architecture.md) 与对应专题文档。先搜索已有实现，复用现有组件和接口。
3. 需求清楚时直接实施；会改变用户行为或安全边界的歧义才询问。已有授权在当前任务中持续有效。
4. 开始前简述处理方向；执行较长工作时说明实际进展、结果和阻碍。完成时区分“已实现”“已编译”“已测试”“已发布”，不要把其中一个当作其他项的证据。

## 修改与验证

- 改动应小而完整，同步直接相关的文档。新增依赖、接口或配置字段前检查现有模式；不要顺手重构无关代码。
- Go 使用 `gofmt`；React / JavaScript 沿用 `web/src` 和 `web/package.json` 的结构。新增用户文案提供中英文；更新 `localization/ui-en.json` 时保留已有内容，避免整文件重新序列化意外移除旧键。
- 配置需兼容旧文档；新增功能提供合理默认值。保持“校验并持久化完成即反馈，耗时应用在后台执行”，并显示应用进度、失败原因和重试入口。
- 错误清晰暴露；不以宽泛捕获、静默失败或成功形状的默认值掩盖错误。无数据应显示未知或不可用，不伪造正常状态。
- 测试的添加与执行遵循当前任务授权和上层指令。需要验证时，优先选择覆盖改动的最小检查；记录实际命令、结果与未覆盖范围。编译通过不能替代运行时验收。
- `web/dist` 与 `web/dist-native` 是构建产物，不手工编辑或提交。Go 嵌入管理页面前先执行 `npm --prefix web run build`。
- `make build`、`make verify`、`make verify-release` 和 `make package` 会通过 `clean-artifacts` 清理旧安装包。执行前检查 `artifacts/`、`bin/` 和构建缓存；需要保留的产物先备份。仅需源码编译时使用前端构建与 `go build ./...`。
- 不提交、不推送、不安装或重启实际部署，除非用户已授权相应动作。发布授权包含必要的版本文件、提交、标签和推送，不等于授权升级用户正在使用的环境。

## 产品与安全边界

### 本地、远程与管理认证

- 管理窗口、浏览器和 CLI 操作当前连接的后端。后端配置不得误写到本机；App 偏好与系统集成需明确属于本机还是目标 Mac。
- 本地 App 与远端可能运行不同版本。可选接口或字段缺失时局部降级并说明原因，不把不支持的功能误报成整个后台离线。
- 修改远程接口时一起检查 `internal/remote/remote.go` 的允许列表、请求和响应格式、旧版后端兼容及 API 错误处理。已有可读状态中新增可选字段时优先复用接口。
- 管理认证可通过明确配置的 IP 白名单豁免；写操作仍校验 CSRF。只信任配置中的代理来源，按选中的客户端 IP 请求头确认代理链；不使用任意转发头或代理自身 IP 给客户端免认证。
- 不读取、输出或提交密码、令牌、私钥、App 凭证与 CLI Cookie 文件。需要控制正在运行的服务时使用已安装的 `routepilot-control` skill 与授权的 CLI 登录方式；状态和完整诊断快照也必须隐藏密钥。

### DNS、缓存与统计

- 乐观决策和乐观响应是独立配置：先用允许保留的旧值响应，再异步刷新。响应缓存的乐观模式允许超过 TTL 的旧响应；关闭并应用成功时清理过期项，保留 TTL 内项与原始到期时间。
- 响应缓存仅在启用乐观响应时持久化，恢复时不延长原始 TTL。稳定决策可跨重启保留；UNKNOWN 和求值失败不持久化。
- 缓存失效、刷新与恢复必须维护 generation 和单次刷新约束，拒绝旧代任务写回。保留客户端、查询类型与策略上游隔离，不把空闲维护读取记作用户命中。
- 乐观可靠性按请求统计：旧值命中、已核对、不一致；不一致率分母为已核对请求。刷新失败、未确认结果、失效或淘汰前未完成核对的请求不得记为一致。
- 决策比较分流结果与最终策略；DNS 响应比较实际记录与响应语义，忽略 ID、TTL 倒计时、记录顺序和域名大小写等变化。不能忽略 TXT 等不透明内容的变化。
- 区分持久化的累计 DNS 统计与缓存实例内的命中/可靠性计数。后者在缓存实例重建或进程重启时重置；文案不得暗示它们已持久化。新增统计同时检查页面、状态 API 和 `/metrics`。

### BGP 与网络路径

- GoBGP 只发布配置所管理的前缀；接收的邻居路由被拒绝，不能变成中转或反射器。DNS 请求不生成单 IP BGP 路由。
- 邻居 ESTABLISHED、本机 RIB、Adj-RIB-Out、外部网关 RIB/FIB 和实际包路径分别表示不同事实。发布状态与详情不能宣称网关已安装路由。
- 邻居详情展示实时快照与保存配置，隐藏认证密钥。未运行、已禁用、旧后端和详情编码失败时保留基本管理能力，不将详情问题影响 BGP 健康判定。
- Static 路由由用户在网关配置，应用不自动安装或验证。保持 Surge / Controller / DNS 健康门禁、TTL / 宽限期和稳定等待。
- IPv4 透明入口已有 macOS PF 与 Docker macvlan 抓包证据，见 [部署验收](docs/phase-2-docker-evidence.md)。真实 IPv6 PF/WAN 路径尚未验收，IPv6 透明拦截默认关闭；不要把 IPv4 证据延伸成 IPv6 已验证。

## 构建入口

| 入口 | 实际范围 |
| --- | --- |
| `npm --prefix web run build` | 构建管理界面和离线更新说明，不清理安装包 |
| `go build ./...` | 编译 Go 源码，要求嵌入的前端资源已存在 |
| `make verify` | 前端测试、Go race 测试、vet、构建；会清理旧安装包 |
| `make native-test ui-test` | 原生 UI 检查、浏览器与响应式页面检查 |
| `make verify-release` | 基础验证、原生与浏览器检查、打包与包校验 |
| `make fuzz` | 版本解析 fuzz |
| `make package package-check` | 构建完整 App，验证版本、资源、签名、DMG/ZIP 和 SHA256SUMS |

完整升级包含 App、管理页面、CLI 和网络服务。发布不再附带独立后台安装包。手动拖放替换 App 不保证常驻后台同步更新；在线更新需继续遵守正在使用 App 时确认重启、仅后台运行时通知后自动重启的约定。

## 发布约定

### 共同要求

- 网页/API 域名为 `https://gitea.knotding.com`；Git SSH remote 可能是 `git.knotding.com`。以 `.gitea/workflows/` 和 [验收记录](docs/acceptance.md) 为门禁依据，不绕过失败。
- 发布前同步远端，确认 `main` 基线、工作区范围、正式/测试版基线和未用标签。只暂存明确属于本次发布的文件，保留无关的未提交工作。
- 所有提交使用 Conventional Commits，如 `feat:`、`fix:`、`docs:`。提交标题概括本次功能或修复，Gitea Actions 会将它作为运行标题；不使用泛化的 `Release RoutePilot vX.Y.Z`。
- 更新 `internal/buildinfo/release.json`、README 中确实存在的版本引用，并新增 `.gitea/release-notes/vX.Y.Z.md`。说明只包含 App 用户可见变化，不加入 agent 工作规范或开发流程。
- CI 从标签写入版本，以 Actions run number 作为制品 build。核验下载包中的 `Info.plist` 和后台版本身份，不要求 CI build 与发布前源码中的 build 相同。
- 不强制推送、不移动或复用已发布标签、不删除旧发布附件，除非用户另有明确要求。

### 测试版

1. 使用唯一的 `vX.Y.Z-beta.N`，同一目标版本的 N 严格递增。更新源码版本及对应说明，`scripts/install_skill.sh` 继续指向正式版。
2. 按已授权范围完成本地检查，创建 Conventional Commit，推送 `main` 与注释标签。
3. `main` 推送会触发 `test.yaml` 回归；Beta 标签触发 `release.yaml` 的精简打包与 `package-check`。Beta Release job 本身不跑完整回归/fuzz，不能因此宣称完整测试已通过。
4. 等待两个工作流结果；回归失败需要修复，不以已生成安装包代替通过。Beta 发布页必须 `prerelease=true`、`draft=false`。

### 正式版

1. 选择未用的无后缀 `vX.Y.Z`，更新源码版本、`scripts/install_skill.sh` 正式 tag/提示版本、实际版本引用和用户说明。
2. 按正式发布授权运行 `make verify-release` 与 `make fuzz`；失败先修复，不跳过门禁。
3. 提交并推送 `main` 与注释标签，等待回归与完整 Release workflow 成功。正式发布页必须 `prerelease=false`、`draft=false`。

### 对外报告前

- 确认发布标签指向本次提交，两个工作流均成功，发布类型正确。
- 确认仅有 `RoutePilot-macos-arm64.dmg`、`RoutePilot-macos-arm64.zip`、`SHA256SUMS` 三项发布附件，均可下载且非空。
- 下载附件校验 SHA256SUMS，检查 App 与捆绑后台的版本/build；包校验只说明制品完整，不等于实际安装、升级、联网或公证验收。
- 最后提供版本号、面向用户的主要变化和 Release 链接。尚未完成的门禁或现场验证应准确说明。

## 协作与交接

- 多 agent 委派遵循当前会话和上层指令；需要委派时先明确负责文件、依赖、禁止触碰范围和验收方式。共享工作区不得相互覆盖。
- 接收结果后检查实际差异与命令证据，不把口头“已完成”作为验证。
- 交接只写继续工作需要的信息；不要重复完整发布历史，不携带凭证。使用以下格式：

```text
## 任务交接
- 目标与验收条件：
- 当前状态：未开始 / 进行中 / 已完成 / 受阻
- 已完成与修改文件：
- 验证：实际命令与结果；未覆盖范围：
- 已发布：tag、commit、workflow、Release 与附件证据；未发布则明确说明：
- 保留的无关改动：
- 下一步与依赖：
```

## 最近已核验发布（2026-10-11）

- 正式版基线为 `v1.0.2`；最新已核验测试版为 `v1.0.3-beta.2`，提交 `7108de6`。Gitea 回归与 Release workflows 已成功，Release 为 prerelease，DMG/ZIP/校验和已下载核验。
- Beta.1 将 CLI 与 skill 设置改为管理当前连接的后端；Beta.2 增加管理 IP 免认证、可信代理、代理域名与请求来源处理。默认免认证白名单为空。真实反向代理部署尚未现场验收。
- 当前准备的 `v1.0.3-beta.3` 包含乐观缓存可靠性统计和 BGP 邻居详情；发布结果必须另行核验，不能把这里的候选信息当作已发布证据。
- 此节仅为带日期的快照。接手时以 `git status`、`internal/buildinfo/release.json`、远端标签与实际 Release/Actions 为准，不凭本文猜测最新版本。

## 快速入口

- **安装与操作：** [README](README.md)，关注第一次配置、本地/远程、缓存、软件更新与 CLI。
- **架构与验收：** [架构说明](docs/architecture.md)、[验收记录](docs/acceptance.md)。区分自动化、隔离网络与用户现场证据。
- **DNS 与缓存：** `internal/dnsserver/`、`internal/classifier/`、`internal/resolver/`、`internal/cache/`。
- **路由与状态：** `internal/bgp/`、`internal/routing/`、`internal/state/`；专题见 [路由配置](docs/routing-backends.md)。
- **管理与安全：** `internal/api/`、`internal/remote/`、`internal/security/`、`internal/controlclient/`；CLI 入口 `cmd/routepilot/`。
- **软件升级：** `internal/softwareupdate/`、`native/`、`web/src/backend-update.jsx`、`web/src/app-preferences.jsx`、`.gitea/workflows/`。
- **界面与系统集成：** `web/src/`、`localization/`、`native/`。沿用现有主题、响应式布局、语言与凭证恢复流程。
