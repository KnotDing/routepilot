# Agent 工作规范

本文件适用于在 RoutePilot 仓库中工作的所有 AI agent。开始任务前先阅读相关代码和文档；以用户当前请求为准，并遵守仓库现有约定。

## 工作方式

- 先确认任务目标、范围和验收条件。需求明确时直接实施；会显著影响行为或设计的歧义，先向用户澄清。
- 修改前查看 `git status` 和目标文件，识别并保留已有工作区改动。不得覆盖、撤销或顺手整理与任务无关的修改。
- 优先做小而完整的改动，复用现有模式；同步更新直接相关的文档和测试。
- 错误应清晰暴露；不要用宽泛捕获、静默失败或成功形状的默认值掩盖问题。
- 不提交代码、不推送、不执行破坏性操作，除非用户明确要求。
- 不把仓库代码、凭证或用户数据发送到第三方服务；不要将密钥写入代码、测试输出或交接内容。

## RoutePilot 项目约定

- RoutePilot 是 macOS 状态栏应用；Go 后端负责 DNS、分流、缓存、配置与路由，React 管理界面随 Go 后台打包，配置和持久化状态使用 SQLite。
- 先查看 [架构说明](docs/architecture.md) 与 [README](README.md) 中与任务相关的章节；不要在本文件重复整份项目文档。
- Go 源码使用 `gofmt`；前端沿用 `web/src` 的 React / JavaScript 结构和 `web/package.json` 脚本。
- 前端资源由 Vite 构建并嵌入 Go 应用；`web/dist` 是生成产物，不提交源码，也不要手工编辑。`make test`、`make cli` 和 `make build` 会先构建这些资源。
- 验证优先采用覆盖改动的最小测试；完整验证入口为 `make verify`（前端测试、Go race 测试、`go vet` 和构建）。`make verify-release` 还会执行原生、浏览器及打包检查，按需运行。
- 对运行时配置、网络行为、缓存和安全边界的变更，应验证默认值、旧配置兼容性及失败路径。
- 远程管理必须考虑本地 App 与远程后端可能运行不同版本。新增或修改远程功能时，确保中继允许列表、请求与响应处理及 UI 降级行为都兼容旧版后端；旧版缺少可选接口或字段时应清晰、局部地降级，不得误报为连接故障或导致其他管理功能不可用。为远程 API 变更补充旧版后端兼容测试。

## 发布版本

- Gitea 网页与 API 使用 `https://gitea.knotding.com`；Git SSH remote 可能使用 `git.knotding.com`，不要把 SSH 主机名误作网页/API 域名。
- 发布遵循 [实际验收记录](docs/acceptance.md)、`.gitea/workflows/release.yaml` 和下列发布约定，不绕过自动化门禁。
- 发布前确认 `main` 基线、工作区改动范围、下一个未使用的语义化版本标签及远端状态；不得覆盖已有标签，不得把无关或他人未提交改动混入发布。
- Gitea Actions 列表中每次运行的主标题取自触发提交标题，工作流名称显示在其下方。所有提交（包括发布提交）都遵循 Conventional Commits（如 `feat:`、`fix:`、`docs:`）；发布提交标题应简洁概括本次面向用户的功能或修复，不要写成 `Release RoutePilot vX.Y.Z`。不要为了改标题重写或移动已推送的提交与标签。
- 更新 `internal/buildinfo/release.json` 的版本、`scripts/install_skill.sh` 中的官方 tag 和提示版本、README 中的当前版本引用，并新增 `.gitea/release-notes/vX.Y.Z.md`。Release Notes 只写 App 用户可见的功能、修复和行为变化；排除 agent/仓库工作规范、开发流程等与 App 无关的内容。资产由工作流生成。
- 正式版发布前运行 `make verify-release` 和 `make fuzz`；任何门禁失败都先修复，不得跳过。正式版 Release 必须不是 prerelease，并通过 `release.yaml` 的完整测试与包验证。
- Beta 版本使用 `vX.Y.Z-beta.N` 标签（例如 `v1.0.1-beta.1`、`v1.0.1-beta.2`），每个 tag 唯一且 N 对同一个目标版本严格递增；不要覆盖或复用已推送的 Beta tag。为每个 Beta 新增对应 `.gitea/release-notes/vX.Y.Z-beta.N.md`，只写用户可见变化，并在 `internal/buildinfo/release.json` 保持同一版本值，供本地构建一致使用（CI 也会从 tag 设置版本及构建号）。
- Beta tag 推送到 `main` 后由同一个 `release.yaml` 自动识别为 prerelease；Beta CI 精简为安装锁定依赖、构建 App/后台及前端、校验 DMG/ZIP/校验和和包内容（`make package package-check`），不运行 `make verify-release`、完整 Go/UI 回归及 fuzz。正式版仍执行完整门禁。Beta 发布前至少确认代码已推送到 `main`，目标 tag 不存在，且 Beta 说明文件已加入该提交。
- 建议的 Beta 发布步骤：确认工作区干净且 `main` 已同步；选择当前正式版基础上的下一个稳定版本号及未用的 Beta 序号；更新 `internal/buildinfo/release.json`、README 中与本次行为有关的说明及对应 Beta Release Notes；提交 Conventional Commit；创建注释 tag（如 `git tag -a v1.0.1-beta.1 -m "RoutePilot v1.0.1-beta.1"`）并推送 `main` 与 tag。等待 Release workflow 成功后检查 Gitea Release 标记为 prerelease，三项资产齐全且可下载，再向测试者提供链接。下一轮继续递增 Beta 序号；正式候选版本则按下列稳定版流程发布正式版本。Beta 发布不要把 `scripts/install_skill.sh` 的官方稳定版 tag 改指到 Beta。
- 正式版发布步骤：选择尚未使用的稳定 tag `vX.Y.Z`（不带后缀），更新 `internal/buildinfo/release.json`、`scripts/install_skill.sh` 官方 tag/提示版本、README 当前版本及 `.gitea/release-notes/vX.Y.Z.md`；确认变更和工作区仅包含本次发布内容，运行 `make verify-release` 与 `make fuzz`。通过后按 Conventional Commits 提交、创建注释 tag 并推送 `main` 与 tag。确认 Gitea 回归和 Release workflows 均成功，Release 不是 prerelease，DMG、ZIP、SHA256SUMS 均可下载后才报告完成。
- `make verify-release` 和 `make package` 会重建 `web/dist` 并替换 `artifacts/` 内的生成安装包；执行前检查这些路径中的现有文件。
- `web/dist` 由发布工作流在构建时生成，不应加入版本提交；只提交 Web 源码和必要的构建配置。
- 不得使用强制推送或移动已发布标签。未确认工作流、Release 类型与三项资产前不得宣称版本已发布。

## 多 agent 协作与交接

- 仅在工作能清楚拆分且有实际收益时并行委派。委派前写明目标、负责范围、禁止触碰的区域、依赖和验收方式；避免两个 agent 同时编辑同一文件或重复调查同一问题。
- 委派者负责合并与最终验证。接收结果后检查实际文件和工作区状态，不把“已完成”的口头报告视作验证。
- 交接时使用以下格式；未知或未验证的信息明确标为“未知”或“未验证”，不要猜测。

### 通用交接模板

```text
## 任务交接
- 目标与验收条件：
- 当前状态：未开始 / 进行中 / 已完成 / 受阻
- 已完成：
- 修改文件及目的：
- 验证：命令与结果；未运行的检查及原因
- 未解决问题、风险或重要约束：
- 下一步行动与依赖：
```

- 交接时只提供完成后续工作所需的信息；引用仓库文件使用相对路径，避免包含凭证、个人信息或无关对话。

### 当前项目交接（2026-10-10）

```text
## 项目交接
- 目标与验收条件：维护 RoutePilot 正式版；保持 IPv6 透明 DNS“未验证”的明确声明与默认关闭，远程 API 变更需兼容不同版本后端。
- 当前状态：v1.0.0 已正式发布；版本提交为 `578c51dc460b0a022da3b12ae2228167060cceec`，标签 `v1.0.0` 已推送，Gitea Release 已提供 DMG、ZIP、SHA256SUMS。
- 已完成：v0.6.10（commit 6d33ec2742c2d3f012f0550bfc26371f21bd797d）与 v1.0.0 发布；v1.0.0 本地门禁、Gitea 回归工作流及 Release 工作流通过，三个下载资产均可访问。用户确认本地实例实际使用 BGP，本机 App 和远程后台升级已多次实测。IPv4 透明 DNS/PF 的隔离 macvlan 验收见 `docs/phase-2-docker-evidence.md`。
- 修改文件及目的：更新 `internal/buildinfo/release.json`、`scripts/install_skill.sh` 至 v1.0.0；在 `README.md` 和 `docs/acceptance.md` 明确 IPv6 透明 DNS未验证；新增 `.gitea/release-notes/v1.0.0.md`；保留此前用户要求的验收文档更新、两份旧 readiness 文档删除及 v0.5.3 文档链接修正。
- 验证：v1.0.0 `make verify-release`、`make fuzz`、`git diff --check` 通过；前端 79 项测试、Go race tests、vet、原生 UI 检查通过。`artifacts/package-verification.json` 报告版本 1.0.0 和包完整性检查通过。DMG、ZIP、SHA256SUMS 下载均返回 HTTP 200。
- 未解决问题、风险或重要约束：真实 IPv6 PF/WAN 路径未验证，IPv6 透明拦截默认关闭；App 使用 ad-hoc 签名而非 Developer ID/公证。用户报告的 BGP 与升级实测无详细矩阵归档，不要将主流程描述为未实测，也不要推断所有边缘场景均覆盖。Surge 策略与缓存使用有界轮询同步。
- 下一步行动与依赖：没有未完成的 v1.0.0 发布步骤。后续若准备启用 IPv6 透明 DNS，先完成真实 PF 入口与网关 WAN/回程路径隔离验收并归档证据。
```

### 新接手者快速入口

- **产品、安装和用户操作：** 从 [README](README.md) 开始，按需读“第一次配置”“本地、远程，一个管理窗口”“数据与凭证”。
- **架构与安全边界：** [架构说明](docs/architecture.md) 描述 DNS 请求路径、保守 DIRECT、Fake-IP 发放、缓存一致性、PF、路由与安全；[验收记录](docs/acceptance.md) 区分自动化、隔离网络证据与现场验证。不要把历史隔离测试推断为整个用户部署均已验收。
- **主要代码：** `cmd/` 为程序入口；`internal/dnsserver/` 和 `internal/classifier/` 为 DNS 分发与决策；`internal/resolver/` 为真实 DNS 上游；`internal/bgp/`、`internal/routing/`、`internal/state/` 管理路由与运行状态；`internal/api/`、`internal/remote/` 管理 API 与远程中继；`native/` 是 macOS 状态栏 App；`web/src/` 是管理界面。Go 包边界与数据流以架构文档为准。
- **构建与测试：** `make verify` 跑前端测试、全 Go race 测试、vet 和构建；`make verify-release` 另外做原生/浏览器验收与 DMG/ZIP 打包校验；`make fuzz` 单独测版本解析。发布门禁会重建 `web/dist` 并替换 `artifacts/` 生成包，运行前检查并确认这些产物可重建。不要直接编辑 `web/dist`。
- **运行及发布现实边界：** IPv4 Docker macvlan/PF 证据见 `docs/phase-2-docker-evidence.md`；IPv6 真实入口和 WAN 回程仍未验证。服务端路由状态、BGP Adj-RIB-Out 与外部网关 RIB/FIB 不是同一确认信号；Static 路由由外部管理，不要描述为应用已安装或验证。远程 API 变更必须测试旧后端缺接口/字段的局部降级。
- **发布基线：** 当前已发布基线 `v1.0.0`（版本提交 `578c51d`）。发布流程及禁止移动标签等约束见本文件“发布版本”和 `.gitea/workflows/`。每次接手代码先执行 `git status --short --branch`，再决定是否运行会覆盖构建产物的门禁；不得覆盖或丢弃已有工作区内容。
