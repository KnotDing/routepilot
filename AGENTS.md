# Agent 工作规范

适用于 RoutePilot 仓库中的所有 AI agent。用户当前请求决定任务范围；先阅读相关代码和文档，再实施。这里记录工作约定与快速入口，具体实现以当前源码为准。

## 开始任务

1. 执行 `git status --short --branch`，阅读目标文件和已有差异，识别本次任务、前序未提交工作与无关文件。保留所有无关改动，不使用 `git add .` 混入发布。
2. 从 [README](README.md) 了解用户流程，按需阅读 [架构说明](docs/architecture.md) 与对应专题文档。先搜索已有实现，复用现有组件和接口。
3. 需求清楚时直接实施；会改变用户行为或安全边界的歧义才询问。已有授权在当前任务中持续有效。
4. 开始前简述处理方向；执行较长工作时说明实际进展、结果和阻碍。完成时区分“已实现”“已编译”“已测试”“已发布”，不要把其中一个当作其他项的证据。

## 双仓库同步

- RoutePilot 同时维护 Gitea 与 GitHub：Gitea 为 `git@git.knotding.com:knot/routepilot.git`，GitHub 为 `git@github.com:KnotDing/routepilot.git`。
- 每次提交后的分支和标签推送都必须同时到达两个仓库。配置 `origin` 的多个 `pushurl`（保留 Gitea 为 fetch URL），这样 `git push origin <分支>` 会推送到两个地址；推送后核对两个远端的提交或标签一致。不要只推其中一个。
- 新克隆或远端配置未完成时，先检查并配置两个推送地址，再继续推送。任一仓库推送失败时明确报告部分成功，并在恢复后补齐；不要强推或改写远端历史。
- 对外 README 链接优先使用 GitHub（Release 与 `raw.githubusercontent.com/KnotDing/routepilot/main`）；Gitea 链接作为清楚标注的备用入口。
- `.github/workflows/test.yaml` 是唯一回归测试门禁；Gitea 不运行本地测试。`.gitea/workflows/release.yaml` 的 Ubuntu runner 先等待 2 分钟，再每分钟轮询 GitHub Release，最多 15 次；可从 Gitea Actions Secret `RELEASE_SOURCE_TOKEN` 读取 GitHub 只读 token，以认证 GitHub API 与附件下载。下载 DMG、ZIP 与 SHA256SUMS 并校验成功后发布到 Gitea。获取任务失败或制品不可用时，再由 macOS runner 本地打包回退，不在 Gitea 执行测试。获取作业允许失败，只有三附件上传完成才输出 `published=true`；否则执行必须成功的回退作业。任一路径完整发布成功即为整体成功，两条路径均失败仍失败。GitHub Release 仍由 `.github/workflows/release.yaml` 构建并发布。

## 修改与验证

- 改动应小而完整，同步直接相关的文档。新增依赖、接口或配置字段前检查现有模式；不要顺手重构无关代码。
- Go 使用 `gofmt`；React / JavaScript 沿用 `web/src` 和 `web/package.json` 的结构。新增用户文案提供中英文；更新 `localization/ui-en.json` 时保留已有内容，避免整文件重新序列化意外移除旧键。
- Swift 中从非隔离回调进入主 actor 时，在 `Task { @MainActor ... }` 上显式捕获 `[weak self]`；AppDelegate 对接旧式 WebKit delegate 时，对 `WebKit` 使用 `@preconcurrency import` 兼容并发标注。
- 配置需兼容旧文档；新增功能提供合理默认值。保持“校验并持久化完成即反馈，耗时应用在后台执行”，并显示应用进度、失败原因和重试入口。
- 错误清晰暴露；不以宽泛捕获、静默失败或成功形状的默认值掩盖错误。无数据应显示未知或不可用，不伪造正常状态。
- 测试的添加与执行遵循当前任务授权和上层指令。需要验证时，优先选择覆盖改动的最小检查；记录实际命令、结果与未覆盖范围。编译通过不能替代运行时验收。
- `web/dist` 与 `web/dist-native` 是构建产物，不手工编辑或提交。Go 嵌入管理页面前先执行 `npm --prefix web run build`。
- `make build`、`make verify`、`make verify-release` 和 `make package` 会通过 `clean-artifacts` 清理旧安装包。执行前检查 `artifacts/`、`bin/` 和构建缓存；需要保留的产物先备份。仅需源码编译时使用前端构建与 `go build ./...`。
- 不提交、不推送、不安装或重启实际部署，除非用户已授权相应动作。发布授权包含必要的版本文件、提交、标签和推送，不等于授权升级用户正在使用的环境。

## 性能与资源占用

- RoutePilot 面向局域网应用，核心要求是空闲和日常负载下的低 CPU、低内存占用，同时保持响应及时、运行稳定。峰值 QPS 和极高并发主要用于定位瓶颈，不作为默认优化目标；不要为了提高压测数字增加默认 worker、队列或缓存容量，也不要直接把某台机器的测量值设为通用硬门槛。
- 优先消除重复计算、字符串格式化、正则编译、临时对象分配、无关数据复制与排序。高频采样只读取所需计数，避免为累计指标遍历全部缓存；功能关闭时跳过其记录和格式化工作。参数标记、静态翻译等可复用资源不要反复构造。
- 关注原生 App、Go 后台和 WebKit / 管理页面各自的成本。定时器、状态栏动画、菜单重建、状态轮询、统计快照和持久化检查点都需避免无变化时的重复工作；优化刷新或持久化时保留健康状态、通知及时性和已有恢复保证。
- 优化不得牺牲 DNS 语义、客户端 / 上游隔离、缓存 generation 与 TTL、统计精确性、告警核对分母及路由安全门禁。先用剖析确认热点，再做小范围改动；分片、对象池、复杂淘汰结构等需有常见负载下的实际收益支撑。
- 性能验证优先覆盖空闲、局域网常见请求量及管理窗口打开 / 关闭场景，并记录缓存规模和命中状态。对照测量使用非 race 构建，保持测试条件一致，记录机器、工具链、请求率、持续时间与背景负载；race 用于正确性检查，不用于容量或资源结论。
- 报告明确区分单核 CPU 时间占比、整机 CPU、进程 RSS、Go 存活堆和累计分配速率，分别说明 App / 后台 / WebKit 的覆盖范围。短测或微基准不能证明长期无泄漏，也不能直接换算为完整 App 的资源降幅；实现、编译、测试和现用环境验证分别报告。
- 压测使用隔离实例、临时数据库和测试上游，不向现用后台施加压力。现用进程可做只读资源观察；安装、替换 App 或重启服务仍遵循已有授权要求。原始日志与剖析放在 `artifacts/`，不提交构建产物；基准入口和测量边界见 [CPU 与内存检查](docs/performance-2026-10-13.md)，其中数值仅为带日期的样本。

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
- 区分持久化的累计 DNS 统计与缓存实例内的命中计数。乐观可靠性计数需跨缓存实例重建和进程重启持久化，直到用户手动清理缓存；新增统计同时检查页面、状态 API 和 `/metrics`。

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

完整升级包含 App、管理页面、CLI 和网络服务。发布不再附带独立后台安装包。手动拖放替换 App 不保证常驻后台同步更新；在线更新默认遵守正在使用 App 时确认重启、仅后台运行时通知后自动重启的约定；远程升级完成后，允许管理端通过单独二次确认的“强制重启远程 App”覆盖远端等待，不再次重启网络后台。

## 发布约定

### 共同要求

- 网页/API 域名为 `https://gitea.knotding.com`；Git SSH remote 可能是 `git.knotding.com`。回归以 `.github/workflows/test.yaml` 为唯一门禁；GitHub Release 制品由 `.gitea/workflows/release.yaml` 优先下载复用，失败时才本地构建。README 对外链接遵循 GitHub 优先、Gitea 备用。发布门禁同时检查 GitHub 与 Gitea 对应 workflow 和 [验收记录](docs/acceptance.md)，不绕过失败。
- 发布前同步远端，确认 `main` 基线、工作区范围、正式/测试版基线和未用标签。只暂存明确属于本次发布的文件，保留无关的未提交工作。
- 所有提交使用 Conventional Commits，如 `feat:`、`fix:`、`docs:`。提交标题概括本次功能或修复，Gitea Actions 会将它作为运行标题；不使用泛化的 `Release RoutePilot vX.Y.Z`。
- 更新 `internal/buildinfo/release.json`、README 中确实存在的版本引用，并新增 `.gitea/release-notes/vX.Y.Z.md`。说明只包含 App 用户可见变化，不加入 agent 工作规范或开发流程。
- CI 从标签写入版本，以 Actions run number 作为制品 build。核验下载包中的 `Info.plist` 和后台版本身份，不要求 CI build 与发布前源码中的 build 相同。
- 不强制推送、不移动或复用已发布标签、不删除旧发布附件，除非用户另有明确要求。

### 测试版

1. 使用唯一的 `vX.Y.Z-beta.N`，同一目标版本的 N 严格递增。更新源码版本及对应说明，`scripts/install_skill.sh` 继续指向正式版。
2. 按已授权范围完成本地检查，创建 Conventional Commit，推送 `main` 与注释标签。
3. `main` 推送由 GitHub `test.yaml` 执行回归；Beta 标签由 GitHub Release workflow 执行精简打包，同时 Gitea Release workflow 等待并复用 GitHub 制品，等待超时或获取失败时才在 Gitea macOS runner 回退构建。Beta 标签本身不运行完整回归/fuzz，不能因此宣称完整测试已通过。
4. 确认 GitHub 回归成功及两个 Release workflow 完成；回归失败需要修复，不以已生成安装包代替通过。Beta 发布页必须 `prerelease=true`、`draft=false`。
5. 每次发布测试版都必须更新本文件，做好随时交接给其他 agent 的准备。发布前按下方交接格式记录版本、任务范围、已有验证和待办；发布后补齐最终提交 / 标签、双仓库 workflow 与 Release / 附件证据，更新「最近已核验发布」和本轮交接。未完成或受阻时准确记录当前状态、失败原因和下一步，不将进行中的发布写为已核验。交接更新提交后同样推送双仓库，不移动已发布标签。

### 正式版

1. 选择未用的无后缀 `vX.Y.Z`，更新源码版本、`scripts/install_skill.sh` 正式 tag/提示版本、实际版本引用和用户说明。
2. 按正式发布授权运行 `make verify-release` 与 `make fuzz`；失败先修复，不跳过门禁。
3. 提交并推送 `main` 与注释标签，等待 GitHub 回归和完整 GitHub Release workflow 成功，再确认 Gitea 已下载并发布同一 GitHub 制品；只有获取失败时才接受 Gitea 本地构建回退。正式发布页必须 `prerelease=false`、`draft=false`。

### 对外报告前

- 确认发布标签指向本次提交，两个工作流均成功，发布类型正确。
- 确认仅有 `RoutePilot-macos-arm64.dmg`、`RoutePilot-macos-arm64.zip`、`SHA256SUMS` 三项发布附件，均可下载且非空。
- 下载附件校验 SHA256SUMS，检查 App 与捆绑后台的版本/build；包校验只说明制品完整，不等于实际安装、升级、联网或公证验收。
- 最后提供版本号、面向用户的主要变化和 Release 链接。尚未完成的门禁或现场验证应准确说明。

## 协作与交接

- 多 agent 委派遵循当前会话和上层指令；需要委派时先明确负责文件、依赖、禁止触碰范围和验收方式。共享工作区不得相互覆盖。
- 接收结果后检查实际差异与命令证据，不把口头“已完成”作为验证。
- 交接只写继续工作需要的信息；不要重复完整发布历史，不携带凭证。使用以下格式：
- 每次测试版发布后，后续任务随时可能由其他 agent 接手；本文件必须能独立说明最新任务与发布状态。不得仅在聊天回复或未提交的本地日志中留下交接信息；发布过程中已有的交接记录也应随进度更新，保留未覆盖验收、无关改动及继续工作所需依赖。

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

## 任务交接（2026-10-30，v1.0.5-beta.8）

- 目标与验收条件：发布 Dock App 图标尺寸调整；双仓库分支和标签、GitHub 回归、两个 Release workflow 与三项附件均核验通过。
- 当前状态：进行中；发布前双仓库 main 均为 `b3d71c23e3143817d8105ed94a8be8f5cedf144e`，最新测试版为 beta.7，beta.8 标签尚不存在。
- 已完成与修改文件：`native/icon.swift` 对 App 图标绘制整体缩小 10% 并留出透明边距；`internal/buildinfo/release.json` 更新为 `1.0.5-beta.8 / build 120`；README 和 `.gitea/release-notes/v1.0.5-beta.8.md` 更新。正式 skill 安装来源保持 v1.0.4。
- 验证：独立编译图标生成器并查看生成图像，图案居中且四周留白；原版与新版状态栏图标 PNG 逐字节一致。`env GOCACHE="$PWD/.cache/go-build" GOMODCACHE="$PWD/.cache/go-mod" make package package-check` 在沙箱外通过（5 项包检查测试、App / 后台版本与签名、ZIP / DMG / SHA256 校验；源码及本地包 build 120）；沙箱内首次打包的 hdiutil 创建设备失败。未覆盖实际 Dock 显示与现用环境升级。
- 已发布：尚未发布；需完成提交和注释标签双推、工作流及附件核验。
- 保留的无关改动：无。原安装包、bin 和图标生成缓存备份至 `artifacts/pre-beta8-backup-20261030/`。
- 下一步与依赖：确认发布提交与两个远端标签，等待 GitHub 回归和两个 Release workflow，下载并核验六项附件后更新本交接并双推。GitHub 未认证 API 当前速率受限；本地构建日志在 `artifacts/releases/v1.0.5-beta.8/`。

## 最近已核验发布（更新于 2026-10-19）

- 最新完整核验正式版为 `v1.0.4`，标签指向 `6d336a770028a18690914eeaf2733cdd42e86ed3`，注释标签对象为 `cd460ac01058ff470afdd5fb9762a6f56e208219`。最新已核验测试版为 `v1.0.5-beta.7`（`910d63f`，双仓库 App / 后台 build 45；源码 / 本地包 build 119），详见下方交接。`v1.0.4-beta.29` 为误基准历史发布，不移动标签。后续交接文档提交不移动发布标签。
- 正式发布提交的 GitHub 回归 [run 38844597375](https://github.com/KnotDing/routepilot/actions/runs/38844597375) 与完整 [Release run 38844625516](https://github.com/KnotDing/routepilot/actions/runs/38844625516) 成功；[Gitea run 2109](https://gitea.knotding.com/knot/routepilot/actions/runs/2109) 成功复用 GitHub 制品，macOS 回退任务跳过。发布提交及注释标签对象已核对双仓库一致。
- [GitHub Release](https://github.com/KnotDing/routepilot/releases/tag/v1.0.4) 与 [Gitea 备用 Release](https://gitea.knotding.com/knot/routepilot/releases/tag/v1.0.4) 均为 `prerelease=false`、`draft=false`，只有 DMG、ZIP、SHA256SUMS 三项非空附件。六项附件均下载，双仓库 SHA256 完全一致；ZIP、签名、DMG 完整性 / 布局及 DMG 与 ZIP 的 App / CLI / 后台二进制一致性通过。App 与后台均为 `1.0.4 / build 37`（CI run number）；源码及本地包 build 为 111，不混为一谈。GitHub latest 已指向 v1.0.4。
- v1.0.4 归并上次正式版 v1.0.3 之后 beta.1–28 的最终变更：实时 / 七天历史监控和六维筛选、客户端备注 / 强制策略 / 主机名、分项普通 / 乐观缓存、模块化配置应用、Surge IPv4 下一跳自动探测、多后端窗口隔离、更新进度 / 远端 App 重启、通知 / 导航 / 资源优化及历史恢复修复。完整说明见 `.gitea/release-notes/v1.0.4.md`；README 含摘要与入口，`scripts/install_skill.sh` 正式 tag / 提示版本已同步。
- 本地 `make verify-release` 与 `make fuzz` 通过：前端 36 文件 / 193 项、Go 全包 race / vet、原生核心 / 真实 WKWebView / 多窗口、17 页面 / 14 分类、9 项缓存 UI、完整包校验，fuzz 217,336 次。GitHub 发布提交回归与正式 Release 均执行各自要求的完整验证 / fuzz。正式 skill 源文件已只读下载并与 tag 内容核对。证据位于 `artifacts/releases/v1.0.4/` 的 `local-verification.log`、`local-fuzz.log`、`local-package-verification.json`、`publication-verification.json`、`skill-source/verification.json` 及工作流 / 引用记录。
- 本轮未安装、升级或重启现用 App / 后台；未新增验收真实 DHCP / 远程 CLI 用户上下文、网关 RIB / FIB、PF / LAN / NAT / IPv6 路径或长期资源占用。既有 Swift 捕获 / WebKit 并发、测试夹具重复 React key、前端包体大小与 hdiutil 弃用提示仍存在；包检查不代表公证或现场升级验收。
- 此节为带日期快照。接手时以 `git status`、源码版本、远端标签及实际 Release / Actions 为准，下方旧版交接仅为历史记录。

- 2026-10-18 误将新测试版沿用为 `v1.0.4-beta.29`，该版已发布，不能移动或复用标签。用户指出应以稳定版 `1.0.4` 为基准启动 `1.0.5` 测试系列；正确序列从 `v1.0.5-beta.1` 开始。将 beta.29 视为误发历史记录，不作为当前源码基线或后续 beta 序号依据。

## 任务交接（2026-10-18，误发 v1.0.4-beta.29）

- 目标与验收条件：发布 PF 锚点意外清空后可自动恢复的修复版测试版；GitHub 回归及两个 Release workflow 成功，GitHub / Gitea 各发布 DMG、ZIP、SHA256SUMS 并核验版本与哈希。
- 当前状态：已意外按旧基准发布，现标记为误发且保留不可移动的历史标签。功能提交 `d945c03beb9e6c6417e06c3d42b96def60e8a75b` 已同步双仓库；GitHub 回归 #58、Release #38 和 Gitea Release #132 成功，版本为 `1.0.4-beta.29 / build 112`。正确系列已改为 `v1.0.5-beta.1`，发布待完成。
- 已完成与修改文件：修复代码在 `internal/dnsserver/pf_control.go`、`internal/app/app.go`，说明在 `docs/architecture.md`；状态栏「本机后台」新增管理员授权重启服务的入口，代码在 `native/main.swift`、双语文案在 `localization/ui-en.json`。版本与双语发行说明在 `internal/buildinfo/release.json`、`.gitea/release-notes/v1.0.4-beta.29.md`。安装 skill 继续指向正式版。现有安装包和三个 `bin/` 二进制备份于 `artifacts/pre-beta29-backup-20261018/`。
- 验证：获授权 loopback 网络后 `GOCACHE="$PWD/.cache/go-build" GOMODCACHE="$PWD/.cache/go-mod" make verify-release` 通过，前端 36 文件 / 193 项、Go 全包 race / vet、原生检查、17 页面 / 14 分类 UI、9 项缓存 UI、包校验均通过；`make fuzz` 通过 242,535 次执行。包内 App / 后台身份均为 `1.0.4-beta.29 / build 112`，DMG SHA256 `d37a742c662b58defe9881587c5d6b49a4ec5274cf2ea9f4ec15340b768efaab`，ZIP SHA256 `45464938b699d8c13b408f09b34c28e58caf741f3783dcd24548da3cbaaba5b0`。PF 恢复尚未在真实 macOS PF 上现场验收。服务状态检查为 HEALTHY；直接 `launchctl kickstart -k system/com.routepilot.surge-controller` 被系统拒绝，当前服务未重启。
- 已发布：误发标签 `v1.0.4-beta.29` 指向 `d945c03beb9e6c6417e06c3d42b96def60e8a75b`，GitHub: https://github.com/KnotDing/routepilot/releases/tag/v1.0.4-beta.29；Gitea: https://gitea.knotding.com/knot/routepilot/releases/tag/v1.0.4-beta.29。GitHub 回归 `39125524746`、Release `39125524816`、Gitea Release #132 均成功；Gitea Release API 核对为非草稿测试版及三项非空附件。该标签不删除、不移动、不复用。
- 保留的无关改动：无。
- 下一步与依赖：正确版本及发布工作记录见下节；运行对应本地验证并完成 `v1.0.5-beta.1` 双仓库发布核验。

## 任务交接（2026-10-18，v1.0.5-beta.1）

- 目标与验收条件：以正式版 `v1.0.4` 为基准发布 `v1.0.5-beta.1`，包含 PF 空锚点恢复和状态栏本机后台重启入口；双仓库分支 / 标签、GitHub 回归、两个 Release workflow 及三项附件均须核验。
- 当前状态：已完成。`v1.0.5-beta.1` 已在 GitHub 与 Gitea 发布并核验；源码 / 本地包为 `1.0.5-beta.1 / build 113`，GitHub 制品 build 39，Gitea 回退制品 build 133。
- 已完成与修改文件：PF 恢复及原生重启入口在前节所列文件；本修正新增 `.gitea/release-notes/v1.0.5-beta.1.md` 并更新版本。beta.29 既有版本 / 发行说明保留为已发布记录。安装 skill 继续指向正式版。
- 验证：`env GOCACHE="$PWD/.cache/go-build" GOMODCACHE="$PWD/.cache/go-mod" make verify-release` 通过：前端 36 文件 / 193 项、Go 全包 race / vet、原生 / WKWebView、17 页面 / 14 分类 UI、9 项缓存 UI、完整包校验。`make fuzz` 通过 238,291 次执行。包内 App / 后台为 `1.0.5-beta.1 / build 113`；本地 DMG SHA256 `b9d030b3643b958cc094b1a683fdd850c47c2896555ca3cc21ee25c35c9197c5`，ZIP SHA256 `28bb60e50b943bcc830502ecdf5909c8151cc35a88c5ea7e7681de3e40787c0f`。GitHub / Gitea 发布附件均下载，SHA256SUMS 校验通过；两边 App 签名、DMG 完整性通过，App / 后台身份均正确。GitHub build 39 / Gitea build 133 不同，因为 Gitea run #133 在附件复用失败后按规则回退构建；故双边 DMG / ZIP 哈希不同。没有真实 PF 清空恢复或非对称回程验收。请求重启本机后台时 `launchctl kickstart` 被系统拒绝；新菜单项已编译 / 测试但未安装 App。
- PF 非对称回程排查：生成规则在 `internal/dnsserver/pf_rules.go`。只有后台保留端口范围的出站 DNS / DoT / DoH 请求使用 `route-to (egress gateway)`，且为 `no state`；规则没有 `reply-to`，不会将所有回包强制绑定到原接口。客户端 DNS 的 `rdr` 则依赖 PF state 进行反向地址转换，所以回包必须再次经过这台 Mac 的 PF；若回程绕过该 Mac，RoutePilot 无法还原原始 DNS 地址。上游响应由 `pass in` 放行，但规则只限定在配置的 ingress 接口集；若非对称回程抵达未列入的接口，本锚点没有明确放行规则，结果还取决于系统根规则。当前尝试读取 PF 根规则、RoutePilot anchor 与 state policy 时，`pfctl` 均报 `/dev/pf: Permission denied`，所以未能确认当前实际加载内容 / ingress 接口集合。已有 `docs/phase-2-docker-evidence.md` 证明单一已验收路径，不覆盖非对称回程。
- 已发布：提交 `0909f022c2153246b07c74caa0f9fe4a4edb7ca9`，注释标签对象 `3c3e2a7cc359ce91aa13a0918316407012fc7e60`，双仓库 `main` / tag 指向一致。GitHub 回归 [run 39128186922](https://github.com/KnotDing/routepilot/actions/runs/39128186922) 与 Release [run 39128187014](https://github.com/KnotDing/routepilot/actions/runs/39128187014) 成功；Gitea Release #133 成功并按回退构建。GitHub: https://github.com/KnotDing/routepilot/releases/tag/v1.0.5-beta.1；Gitea: https://gitea.knotding.com/knot/routepilot/releases/tag/v1.0.5-beta.1。两边均 `draft=false`、`prerelease=true`，各有 DMG、ZIP、SHA256SUMS 三项非空附件。GitHub 附件 SHA256：DMG `36c14e44ad386d0de9467b561a04277cc5c755def45afcfa9f0222e24f221b91`，ZIP `fab1a51686469fc8d6d999d2488cf71e646883f26a4b5583631e169b236ed6b3`；Gitea：DMG `e753944645ade3389d225cb9afda170e76bab57cf15478aafe83acb7e3392a34`，ZIP `eb557f876d6dfd1700d2551b94617c7bad8f324a871ad5663ab8a240107df55e`。两边包内 App / 后台均为 `1.0.5-beta.1`，分别 build 39 与 133。
- 保留的无关改动：无。原有安装包及 `bin/` 备份在 `artifacts/pre-beta29-backup-20261018/`。
- 下一步与依赖：发布已完成；本机 PF 的当前根规则 / anchor 因系统拒绝访问未能现场核验。若要确认具体非对称回程接口行为，需要在授权的 root 终端只读读取 `pfctl -sr/-sn/-a com.apple/surge-controller -sr/-sn/-s info` 并比对已配置的 ingress / egress；要验证流量则需独立、可回滚的双接口测试路径。未安装升级本机 App；任务栏的新重启按钮需安装新 App 后才能实际操作。

## 任务交接（2026-10-11）

- 目标与验收条件：检查 v1.0.2 之后新增功能的测试覆盖；发布 v1.0.3 正式版；更新本文件记录工作流、版本与测试边界。
- 当前状态：已完成。正式版已在 GitHub 和 Gitea 发布，双仓库 `main` 已同步。
- 已完成与修改文件：测试补充见本次提交历史及 `docs/acceptance.md`；发行说明为 `.gitea/release-notes/v1.0.3.md`；`README.md`、`scripts/install_skill.sh` 已使用 GitHub 优先链接/版本；`AGENTS.md` 已更新发布和测试交接状态。发布工作流稳定版手动重跑修复位于 `.github/workflows/release.yaml`。
- 验证：`make verify-release`、`env GOCACHE=/tmp/routepilot-go-build-v1.0.3 make fuzz` 均通过；fuzz 执行 230,438 次。GitHub 回归 run #17、#18 和发布 run #8 成功；Gitea Release run #102 成功复用 GitHub 制品。DMG、ZIP 和 SHA256SUMS 校验通过。发布门禁不代表真实安装升级、现场网络运行或钥匙串写入验收。
- 已发布：`v1.0.3`，tag 指向 `5ca0fd97496776d9949dbe3218d57f982dc0e04f`。GitHub: https://github.com/KnotDing/routepilot/releases/tag/v1.0.3；Gitea: https://gitea.knotding.com/knot/routepilot/releases/tag/v1.0.3。两边各有 DMG、ZIP、SHA256SUMS 三个发布附件；GitHub Release run #8（`38141218351`），Gitea Release run #102（`2071`）。
- 保留的无关改动：无。
- 下一步与依赖：无。后续提交仍须同时推送 Gitea 与 GitHub。

## 任务交接（2026-10-12）

- 目标与验收条件：完成监控与客户端持久化、列表内编辑、更新按钮位置、完整备注 / IP 切换、三层客户端策略流向、请求来源占比排行，以及侧边栏展开 / 图标 / 隐藏 / 悬浮；发布测试版并更新交接。
- 当前状态：已完成。`v1.0.4-beta.15` 已核验发布（tag 指向 `63b1487`，App 与捆绑后台 `1.0.4-beta.15 / build 23`）；两张流向图点击高亮流带并固定显示精确请求数已随该版本发布。
- 已完成与修改文件：客户端配置与列表内编辑见 `internal/config/clients.go`、`internal/dnsserver/client_policy.go`、`web/src/clients.jsx`、`client-settings.jsx`、`docs/client-settings.md`；累计客户端持久化见 `internal/app/statistics.go`、`internal/dnsserver/dispatcher.go`。监控真实三元组在 `internal/dnsserver/source_policy_flows.go`，历史在 `internal/database/monitoring.go`，图形 / 完整名称 / 比例条在 `web/src/monitoring*`；说明见 `docs/monitoring.md`。更新操作在 `main.jsx`、`app-preferences.jsx`、`backend-update.jsx` 使用现有状态与 portal；导航及持久化偏好在 `navigation.jsx`、`style.css`。双语、自动化检查与发行说明已同步。 大屏卡片 / 展示脱敏在 `web/src/monitoring-preferences.jsx`，App 持久化在 `native/monitoring_preferences.swift` 与 `notifications.swift`；未知最终策略的图表范围过滤在 `monitoring-data.js`。容量分组保留未记录类型在 `internal/dnsserver/source_policy_flows.go`；流带点击高亮与精确计数在 `web/src/monitoring.jsx`（`FlowRibbon` / `FlowPickedHint`）、`monitoring.css`、`monitoring.test.jsx`。Gitea 获取 GitHub 制品的初始等待由 5 分钟缩短为 2 分钟（`.gitea/workflows/release.yaml`）。
- 验证：beta.12 前端 146 测试、原生偏好 / UI、完整打包与双仓库附件核验通过，GitHub 回归 #33、Release #20、Gitea #114 成功。beta.13 本地前端 30 文件 / 147 测试、生产构建、17 页 / 14 分类响应式检查、未知最终策略排除及三层比例尺 / 中间节点验证、`make package package-check` 通过（源码 build 95）。beta.14 前端 147 测试、`go vet` 与 dnsserver race、`make package package-check`（源码 build 96）通过并完成发布附件核验。beta.15 前端 149 测试、生产构建、17 页 / 14 分类响应式检查、`make package package-check`（源码 build 97）通过，发布附件核验完成（App 与后台 `1.0.4-beta.15 / build 23`）。未安装升级现用环境，未验收真实 LAN / NAT / IPv6 流量。
- 已发布：`v1.0.4-beta.15`，tag 指向 `63b1487191cc4a694f601177740fd1358ed977f9`，实际 App 与捆绑后台 `1.0.4-beta.15 / build 23`。GitHub: https://github.com/KnotDing/routepilot/releases/tag/v1.0.4-beta.15；Gitea: https://gitea.knotding.com/knot/routepilot/releases/tag/v1.0.4-beta.15。两边均为非草稿测试版，各有 DMG、ZIP、SHA256SUMS 三附件且哈希相同；证据见 `artifacts/releases/`。beta.14 同日已核验发布（tag `1e35279`，`build 22`）；beta.13 已发布但未做附件核验，不记为最终核验版本。
- DNS 三层图评估：用户允许不合适时不做。本轮保留「来源 → 实际处理去向」两层；现有持久化数据可添加直接请求 / 透明拦截的接入方式，但会减弱来源到去向的直观关联。更有价值的具体上游地址尚未纳入累计 / 历史计数，不能用有限查询历史或配置补造；缓存命中未访问上游，未来扩展需明确此边界。
- 发布故障记录：beta.7 GitHub 回归 #26 / Release #15 成功，Gitea run 2080 的 Ubuntu runner 拉取 Docker 镜像时遇到 unexpected EOF，备用任务成功但总体门禁失败，因此不记为完整核验发布。beta.8 新运行的 Ubuntu 镜像拉取恢复，成功镜像 GitHub 制品；未移动标签或删除旧附件。
- 保留的无关改动：无。
- 下一步与依赖：无。未授权安装升级现用环境；具体上游三层图尚未实现。后续提交仍须同时推送 Gitea 与 GitHub。


## 任务交接（2026-10-18，v1.0.5-beta.2）

- 目标与验收条件：发布 DHCPv4 服务及启用前其他服务器 / 静态地址检查；逐设备覆盖所有 DHCP 选项，双仓库工作流及发布附件核验通过。
- 当前状态：已完成。`v1.0.5-beta.2` 双仓库发布及附件核验通过，源码 build 114 / CI build 40。
- 已完成与修改文件：`internal/config/dhcp.go`、`internal/dhcpserver/`、`internal/database/dhcp.go` 实现配置、协议、网口、探测与租约；app / API / remote / CLI / 备份集成；`web/src/dhcp*` 与导航、双语；相关测试、`scripts/ui_forms_check.cjs`、`docs/dhcp.md`、README 与发行说明。
- 验证：`env GOCACHE=/private/tmp/routepilot-dhcp-gocache make verify-release` 通过：前端 37 文件 / 197 项、Go 全包 race / vet、原生 / WKWebView / 多窗口、18 页面 / 14 分类三宽度 UI 和完整包校验；`make fuzz` 207,725 次执行通过。本地 App / 后台均为 `1.0.5-beta.2 / build 114`。未覆盖现用安装升级、真实 DHCP 广播收发、iOS 租约及路由采纳。探测无响应不证明不存在其他服务器。
- 已发布：`v1.0.5-beta.2`，提交 `c1f7b31a2cd3efd8f3b02e67699ce7b4b6b2deab`，注释标签对象 `78fe293c4deaf0f63fa265f0c2180d98817bcdf8`，双仓库已核对一致。GitHub 回归 `39135288272` / Release `39135304875`、Gitea `2118`（#134）均成功；Gitea 镜像 GitHub 附件。两个发布页均为非草稿测试版，只有 DMG / ZIP / SHA256SUMS 三附件，六附件已下载、哈希一致、签名 / ZIP / DMG / 版本身份通过，App / 后台均为 `1.0.5-beta.2 / build 40`。链接：https://github.com/KnotDing/routepilot/releases/tag/v1.0.5-beta.2 ；https://gitea.knotding.com/knot/routepilot/releases/tag/v1.0.5-beta.2 。证据为 `artifacts/releases/v1.0.5-beta.2/publication-verification.json`。
- 保留的无关改动：无。旧安装包和 bin 备份至 `artifacts/pre-beta2-backup-20261018/`；本轮不安装或重启现用环境。
- 下一步与依赖：beta.2 无发布遗留；用户随后指出标题重复、间距及设备编辑入口，修订发布见 beta.3 交接。此最终记录随下一提交双推，不移动 beta.2 标签。

## 任务交接（2026-10-18，v1.0.5-beta.3）

- 目标与验收条件：修复 DHCP 页标题重复、保存区贴近租约卡片；将设备属性编辑收敛到租约列表，保留无租约配置设备；发布新测试版。
- 当前状态：已完成。源码 / 本地包 `1.0.5-beta.3 / build 115`，双仓库制品 build 41。
- 已完成与修改文件：`web/src/dhcp.jsx`、`dhcp.css` 改为列表内设备属性弹窗，独立设备接口保存且不提交全局草稿；新增 API / 远程中继 GET/PUT `dhcp/devices`，DHCP 状态 `device_settings` 可选能力；前端 / API / remote 测试、三宽度明暗截图和双语同步。
- 验证：前端 37 文件 / 199 项，API / remote / dhcpserver race 与 vet，18 页 / 14 分类三宽度响应式、DHCP 明暗主题 / 弹窗 / 间距 / 单标题 / 无额外分组边框 / 无障碍名称、9 项缓存 UI，以及 `make package package-check` 均通过。源码 / 本地包身份 `1.0.5-beta.3 / build 115`；beta.2 全包回归 / 原生 / fuzz 证据保留，本版未重复执行该完整本地组合。未安装 / 重启现用环境，未开启 DHCP 或验收真实客户端。
- 已发布：`v1.0.5-beta.3` 指向 `64ff6051435a2e13c63eb20fe23dee9ba5c0c1fb`，注释标签对象 `65511db4126ee25091862d7491b2188415285458`，双仓库对象一致。GitHub Release `39136416256`、回归 `39136395753`、Gitea `2119`（#135）均成功。双发布页均为非草稿测试版，只有 DMG / ZIP / SHA256SUMS 三附件；六附件下载校验通过且哈希一致，App / 后台均为 `1.0.5-beta.3 / build 41`，签名 / ZIP / DMG 校验通过。Gitea 复用 GitHub 制品。链接：https://github.com/KnotDing/routepilot/releases/tag/v1.0.5-beta.3 ；https://gitea.knotding.com/knot/routepilot/releases/tag/v1.0.5-beta.3 。证据：`artifacts/releases/v1.0.5-beta.3/publication-verification.json`。
- 保留的无关改动：无。beta.2 标签不移动，包及 bin 备份至 `artifacts/pre-beta3-backup-20261018/`。
- 下一步与依赖：beta.3 发布无遗留；用户随后要求移除设备表单底部分割线，见 beta.4。此最终交接随下一提交双推，不移动标签。

## 任务交接（2026-10-18，v1.0.5-beta.4）

- 目标与验收条件：移除 DHCP 设备属性表单底部的多余分割线，保留设置页输入风格、列表内编辑与间距修复，发布新测试版。
- 当前状态：已完成。源码 / 本地包 `1.0.5-beta.4 / build 116`，双仓库 App / 后台制品 build 42。
- 已完成与修改文件：`web/src/dhcp.css` 仅对设备窗口 footer 清除上边框；`scripts/ui_forms_check.cjs` 新增真实浏览器检查；版本、说明及交接同步。
- 验证：`npm --prefix web run build`、`make ui-test` 与 `env GOCACHE=/private/tmp/routepilot-dhcp-gocache make package package-check` 通过；18 页 / 14 分类 / 三宽度、明暗主题、设备表单底部无分割线及 9 项缓存 UI 均通过，本地 App / 后台 `1.0.5-beta.4 / build 116`。此前 beta.3 的 199 前端测试、相关 race / vet 与发布提交完整 CI 均通过；本轮 CSS 修复未重复该完整本地测试组合。本轮未安装、重启或开启 DHCP。
- 已发布：`v1.0.5-beta.4` 指向 `d3f479b89f53b3e6d205accf1fe306e3a3f1a545`，注释标签对象 `f93fd9687900e0988172ccf2e5cd653ecfa946c1`，双仓库已核对一致。GitHub 回归 `39137089104` / Release `39137101493`、Gitea `2120`（#136）均成功；Gitea 镜像 GitHub 附件。两个页面均为非草稿测试版，只有 DMG / ZIP / SHA256SUMS 三附件，六附件下载、哈希、签名、ZIP / DMG 完整性及包内 App / 后台身份均通过且双仓库哈希一致，身份为 `1.0.5-beta.4 / build 42`。额外确认捆绑后台中已嵌入移除分割线的最新 CSS。链接：https://github.com/KnotDing/routepilot/releases/tag/v1.0.5-beta.4 ；https://gitea.knotding.com/knot/routepilot/releases/tag/v1.0.5-beta.4 。证据：`artifacts/releases/v1.0.5-beta.4/publication-verification.json`。
- 保留的无关改动：无。beta.3 旧包及 bin 备份至 `artifacts/pre-beta4-backup-20261018/`。
- 下一步与依赖：无发布遗留。最终交接作为后续文档提交双推，不移动发布标签；未安装或重启现用环境，真实 DHCP 客户端与路由采纳仍待现场验收。

## 任务交接（2026-10-18，v1.0.5-beta.5）

- 目标与验收条件：统计数值空间不足时缩小字号，保持完整单行，扩宽后恢复；覆盖 DNS / 缓存 / 客户端 / 路由 / 监控的共用统计卡片，发布新测试版。
- 当前状态：已完成。源码 / 本地包 `1.0.5-beta.5 / build 117`，双仓库制品 build 43。
- 已完成与修改文件：`web/src/metric-value.jsx` 共用自适应组件，单一共享 ResizeObserver，仅数值 / 尺寸 / 字体变化时测量，无定时器；`style.css` 保持单行；main / clients / cache / routing / monitoring 接入；新增单元测试和真实浏览器长值 / 扩宽恢复检查。
- 验证：前端 38 文件 / 201 项、18 页 / 14 分类 / 三宽度和 9 项缓存 UI 通过；浏览器确认 16 位数值不折行、不溢出，扩宽恢复。`env GOCACHE=/private/tmp/routepilot-dhcp-gocache make package package-check` 通过，本地 App / 后台 `1.0.5-beta.5 / build 117`。本版仅前端显示改动，未重复本地 Go / 原生测试；发布提交 CI 仍需完整回归。未安装、重启或开启 DHCP。
- 已发布：`v1.0.5-beta.5` 指向 `cc8f34df2e53488d0b3309570dace9d407b9d03d`，注释标签对象 `070c409f38c61e62a1b9ce44da57efad1d37c878`，双仓库引用一致。GitHub 回归 `39137981766` / Release `39137997174`、Gitea `2121`（#137）均成功；Gitea 复用 GitHub 附件。双发布页均为非草稿测试版，只有 DMG / ZIP / SHA256SUMS 三附件，六附件下载、哈希、签名、ZIP / DMG 和 App / 后台版本身份均校验通过且双仓库哈希一致；App / 后台为 `1.0.5-beta.5 / build 43`。另核对捆绑后台中已嵌入最新统计适配和无分割线 CSS。链接：https://github.com/KnotDing/routepilot/releases/tag/v1.0.5-beta.5 ；https://gitea.knotding.com/knot/routepilot/releases/tag/v1.0.5-beta.5 。证据：`artifacts/releases/v1.0.5-beta.5/publication-verification.json`。
- 保留的无关改动：无。beta.4 旧包与 bin 备份至 `artifacts/pre-beta5-backup-20261018/`。
- 下一步与依赖：无发布遗留。最终交接作为后续文档提交双推，不移动已发布标签。未安装或重启现用环境，真实 DHCP 客户端 / 路由采纳及长期资源占用仍待现场验收。

## 任务交接（2026-10-19，v1.0.5-beta.6）

- 目标与验收条件：发布路由器 API 模式（OpenWrt / UniFi 预设与自定义请求模板）测试版；双仓库提交 / 标签、GitHub 回归、两个 Release workflow 及三附件核验通过。
- 当前状态：已完成。`v1.0.5-beta.6` 双仓库发布及附件核验通过，源码 / 本地包 build 118 / CI 制品 build 44。
- 已完成与修改文件：`internal/routingapi/`（`Manager` 对账与状态、`openwrt.go` ubus 会话 + uci 命名段、`unifi.go` Integration API、`custom.go` 模板渲染）、`internal/config/routing_api.go`（配置与校验，凭据加密存储、导出剥离）、`internal/routing/manager.go`（`DesiredAPI` / `ReconcileAPI` / `ReplaceAPI` 热替换，不重建 BGP）、`internal/app/configuration_impact.go`、`internal/api/`（状态与备份）、`internal/database/store.go`；UI 在 `web/src/routing-api-settings.jsx`、`routing.jsx`、`form-fields.jsx`、i18n；文档 `docs/routing-backends.md`、`docs/architecture.md`、README 与代码地图。版本 `internal/buildinfo/release.json` `1.0.5-beta.6 / build 118`，发行说明 `.gitea/release-notes/v1.0.5-beta.6.md`。安装 skill 继续指向正式版。
- 验证：`env GOCACHE=/private/tmp/routepilot-beta6-go-build make verify-release` 通过（前端测试、Go 全包 race/vet、原生检查、18 页 / 14 分类 / 三宽度与 9 项缓存 UI、完整包校验 `PASS package`）；本地包 App / 后台 / CLI 均为 `1.0.5-beta.6 / build 118`。证据 `artifacts/releases/v1.0.5-beta.6/local-verification.md` 与 `publication-verification.json`。未覆盖：真实 OpenWrt / UniFi / 自定义路由器的实际 API 对账、路由器故障重试、现用环境安装升级与非对称回程。
- 已发布：`v1.0.5-beta.6` 指向 `59d9109126d30f1423029ec9d95f7ca87a3446b9`，注释标签对象 `f4e9bdc21a3949389011777683addef148e47fb6`，双仓库已核对一致。GitHub 回归 `39186724528` / Release `39186744950`、Gitea Release run #138（actions/runs/2124）均成功；Gitea 复用 GitHub 附件，无回退构建。双发布页均为非草稿测试版（`draft=false`、`prerelease=true`），只有 DMG / ZIP / SHA256SUMS 三附件；六附件下载并经 SHA256SUMS 校验，双仓库 DMG / ZIP 哈希一致（DMG `9cc2ce711479e76dc564fcfebbe46929f89cbd906db2261f5d72580907ea7aff`、ZIP `74abe6d03e7fb7b7b370ff65c92ab219e3bde4a3bec741e8ad4c3535c0d53f1a`），ZIP 完整性、strict codesign、DMG verify 通过，App 与捆绑后台均为 `1.0.5-beta.6 / build 44`。链接：https://github.com/KnotDing/routepilot/releases/tag/v1.0.5-beta.6 ；https://gitea.knotding.com/knot/routepilot/releases/tag/v1.0.5-beta.6 。证据：`artifacts/releases/v1.0.5-beta.6/publication-verification.json`。
- 保留的无关改动：无。beta.5 旧包与 bin 备份至 `artifacts/pre-beta6-backup-20261019/`。
- 下一步与依赖：无发布遗留。最终交接作为发布后的文档提交双推，不移动已发布标签。已知事项：GitHub SSH 22 端口再次关闭，推送沿用 `artifacts/releases/v1.0.4/github-ssh443.sh` 的 `ssh.github.com:443` 通道（`git -c core.sshCommand=<绝对路径> push origin ...`），未修改全局配置；`web/src/session-recovery.test.jsx` 存在偶发时序脆弱性（上一 main 提交 6198ef8 的回归 39138529934 因此失败，本地与本次发布提交回归均通过），后续任务可跟进修复。未安装或重启现用环境。

## 任务交接（2026-10-19，v1.0.5-beta.7）

- 目标与验收条件：发布 Hybrid 手动路由分发方式选择（BGP / Static / API）测试版；双仓库提交 / 标签、GitHub 回归、两个 Release workflow 及三附件核验通过。
- 当前状态：已完成。`v1.0.5-beta.7` 双仓库发布及附件核验通过，源码 / 本地包 build 119 / CI 制品 build 45。
- 已完成与修改文件：`internal/config/routing.go`（`ManualBackend` 字段、`Effective` 旧配置默认 BGP、`UsesBGP` / `UsesAPI` 纳入手动选择）、`internal/config/config.go` 默认值、`internal/routing/manager.go`（`Desired` / `DesiredAPI` / `Reconcile` 门禁、`Status.ManualBackend`、状态行按选择显示、静态确认与 DNS 就绪分母修正）、前端 `web/src/routing.jsx`（Hybrid 选择器）、`main.jsx`（标签 / choices / 提交归一 / API 区块显示条件）、`routing-i18n.js` 与 `localization/ui-en.json` 双语、`native/main.swift`（状态栏详情与 usesBGP 判定、smoke 测试）；文档 `docs/routing-backends.md`（模式表新增手动路由列）、`docs/architecture.md`、`docs/configuration-application.md`、README 与代码地图。版本 `internal/buildinfo/release.json` `1.0.5-beta.7 / build 119`，发行说明 `.gitea/release-notes/v1.0.5-beta.7.md`。功能提交 `732ffc5` 已双推且回归成功。安装 skill 继续指向正式版。
- 验证：`env GOCACHE=/private/tmp/routepilot-beta6-go-build make verify-release` 通过（前端测试、Go 全包 race/vet、原生检查、18 页 / 14 分类 / 三宽度与 9 项缓存 UI、完整包校验 `PASS package`）；本地包 App / 后台 / CLI 均为 `1.0.5-beta.7 / build 119`；本地 DMG SHA256 `340e41ba07a65af13014a27b223df4530e1c0a8e578f3661f4d7fc13b60dcacc`、ZIP `1197d5420edf9507c9b4f5227e3e2de8ccc37933653330e4adcb18f0352292bf4`。证据 `artifacts/releases/v1.0.5-beta.7/local-verification.md`。未覆盖：真实路由器上手动路由的 API 发布 / 撤销、现用环境安装升级与非对称回程。
- 已发布：`v1.0.5-beta.7` 指向 `910d63f04f7b02025dc1dfd6009fd35e866f1a34`，注释标签对象 `19a877f8d7758afefc2e317f7b39ac3dfdc0fbcb`，双仓库已核对一致。GitHub 回归 `39235916205` / Release `39235924736`、Gitea Release run #139（actions/runs/2125）均成功；Gitea 复用 GitHub 附件，无回退构建。双发布页均为非草稿测试版（`draft=false`、`prerelease=true`），只有 DMG / ZIP / SHA256SUMS 三附件；六附件下载并经 SHA256SUMS 校验，双仓库 DMG / ZIP 哈希一致（DMG `c7f1c9e7aa41de168ff987ad31be31a3ba3e221ba1a3f9d017d4bfb8f75eceb4`、ZIP `462635b6b346407af0a051cded710b36d68948f5c78929a3500145bbe2660eb7`），ZIP 完整性、strict codesign、DMG verify 通过，App 与捆绑后台均为 `1.0.5-beta.7 / build 45`。链接：https://github.com/KnotDing/routepilot/releases/tag/v1.0.5-beta.7 ；https://gitea.knotding.com/knot/routepilot/releases/tag/v1.0.5-beta.7 。证据：`artifacts/releases/v1.0.5-beta.7/publication-verification.json`。
- 保留的无关改动：无。beta.6 旧包与 bin 备份至 `artifacts/pre-beta7-backup-20261019/`。
- 下一步与依赖：无发布遗留。最终交接作为发布后的文档提交双推，不移动已发布标签。未安装或重启现用环境，真实路由器上手动路由的 API 发布 / 撤销仍待现场验收。

## 代码地图：按任务定位

先查下表，通常从“首读”开始，再跟随调用读取“关联”。函数名比行号稳定；模块位置变化时同步更新这里。不要预先通读 `app.go`、`api.go`、`main.jsx` 或 `main.swift` 的全部内容。

### 入口与装配

| 入口 | 职责与定位点 |
| --- | --- |
| `cmd/controller/main.go` | 后台命令分发：`serve`、版本、更新 worker/agent；无 `serve` 的旧参数进入独立诊断工具，不是集成服务 |
| `internal/app/app.go` | `Open` / `build` 装配 Runtime；`monitor` 驱动探测与路由；`View` / `CacheView` / `Metrics` 输出只读视图；`Serve` / `Close` 管理生命周期 |
| `internal/api/api.go` | `Handler` / `serve` 分派 `/api/v1/`，CRUD、导入、健康与更新接口；具体逻辑优先找同目录专题文件 |
| `cmd/routepilot/main.go` | CLI 参数、子命令及 JSON 输出；HTTP、认证与异步等待在 `internal/controlclient/client.go` |
| `native/main.swift` | `AppDelegate`：状态栏菜单、管理 WKWebView、后台连接与启动、JS bridge、更新提示；独立行为优先找拆出的 Swift 文件 |
| `web/src/main.jsx` | `App` / `Workspace`：会话、后端切换与按页面轮询；`Resource`：通用表项 CRUD；`Settings`：设置分组与提交；专用页面组件已拆分 |
| `web/embed.go` | 嵌入 `web/dist`，后台提供管理页面；离线更新说明由 `web/src/native-notes.jsx` 另行构建并随 App 打包 |

### 常见任务首读清单

| 任务 | 首读文件 / 关键点 | 关联文件 |
| --- | --- | --- |
| 配置字段、默认值、校验 | `internal/config/config.go`（`Document` / `Settings`）；路由、安全、通知、Surge 重启和更新源校验在同目录专题文件 | `internal/database/store.go`；`web/src/form-fields.jsx`；`web/src/main.jsx` 的 `Settings` |
| 保存慢、配置应用失败、监听变更 | `internal/app/configuration.go`（`SaveConfiguration` / `runConfigurationWorker`）；`configuration_impact.go`（DNS / BGP / 邻居 / 路由依赖与安全等待）；`app.go` 的 `activateConfiguration` / `Serve` | `internal/api/configuration_apply.go`；`web/src/configuration.js`、`configuration-status.jsx`；`docs/submission-responsiveness.md`、`docs/configuration-application.md` |
| 耗时操作与进度 | `internal/api/operations.go`（`startOperation`）；检查 `Prefer: respond-async` | `web/src/client.js`、`operation-status.jsx`；`internal/controlclient/client.go` 的 `Request` |
| DNS 分流、AAAA 回落、Fake-IP 门禁 | `internal/dnsserver/dispatcher.go`（`Dispatch` / `resolveSurge`）；`internal/model/model.go` 定义查询上下文与结果 | `internal/localdns/local.go`；`internal/app/app.go` 的 `policy.Classify`；`internal/state/machine.go` |
| Surge 策略求值、always-real-ip、Host/IPv6 同步 | `internal/surge/adapter.go`；`api.go` / `cli.go`；`real_ip.go` / `hosts.go` / `ipv6.go` | `internal/classifier/classifier.go`（`Classify` / `evaluate`）；`internal/app/decision_revision.go` |
| Surge IPv4 下一跳自动探测 | `internal/surge/vmnet.go`（`ReadVMNextHop4`，只读 VMNET `self-ip`）；`internal/routing/next_hop.go`（自动 / 显式地址、30 秒探测节流、安全校验、静态确认地址绑定） | `internal/app/app.go` 的 `monitor`（旧 Fake-IP TTL / 宽限期排空后切换）；`configuration.go`（确认时绑定自动地址）；`web/src/surge-next-hop.jsx`、`form-fields.jsx`；`docs/routing-backends.md` |
| 决策缓存、失效与空闲刷新 | `internal/classifier/classifier.go`、`idle_refresh.go`；`internal/cache/cache.go` / `policy.go`（分项开关、按旧值归属的有界统计） | `internal/app/decision_cache.go`、`decision_revision.go`；`web/src/cache.jsx`、`cache-i18n.js`、`cache-policies.jsx`（决策 / 上游独立开关与统计）；`internal/config/cache_policies.go`；`docs/cache-policies.md` |
| 响应缓存、TTL、持久化与可靠性比较 | `internal/resolver/real.go`（`Resolve` / `exchange` / `refresh`）；`internal/app/surge_response_cache.go`（Surge 分项缓存、发行门禁、响应范围验证）；`response_cache.go`、`response_comparison.go`、`idle_refresh.go` | `internal/app/response_cache.go`、`app.go` 的 `responseCacheStats` / `Metrics`；`web/src/main.jsx` 的 Queries 页 |
| 上游、出口绑定、策略专用 DNS | `internal/resolver/upstream_transport.go`、`egress*.go`；`internal/dnsserver/policy_dns.go` | `web/src/policy-dns.jsx`、`dns-settings.jsx`；`internal/config/config.go` |
| DHCPv4、租约与设备覆盖 | `internal/config/dhcp.go`；`internal/dhcpserver/engine.go` / `packet.go`（分配、状态转换、Option 121）；`server.go` / `check.go` / `address_darwin.go`（接口绑定、DHCPINFORM 探测、静态地址检查） | `internal/database/dhcp.go`；`internal/app/dhcp.go`；`internal/api/dhcp.go`；`web/src/dhcp.jsx`；`docs/dhcp.md`；独立配置接口 `dhcp/configuration`、状态 `dhcp/status`，不通过通用 settings PUT 覆盖 DHCP |
| 透明 DNS / PF | `internal/dnsserver/transparent.go`、`pf_rules.go`、`pf_control.go`、`pf_darwin.go`；普通 UDP/TCP 监听在 `server.go` | `web/src/dns-interception.jsx`（DNS 拦截页的静态网关配置确认，复用 settings 保存）；`docs/routing-backends.md`、`phase-2-acceptance.md`、`phase-2-docker-evidence.md`；`scripts/*phase2*` |
| BGP 邻居、详情与路由发布 | `internal/bgp/gobgp.go`（`peerConfig` / `SyncPeers` / `Peers` / `Exported`）；`manager.go`（`Reconcile` / `Refresh`） | `internal/routing/manager.go`；`web/src/bgp-peer-details.jsx`、`routing.jsx`、`main.jsx` 的 `Resource`；`docs/routing-backends.md` |
| 路由器 API 模式（OpenWrt / UniFi / 自定义模板） | `internal/routingapi/`（`Manager` 对账与状态；`openwrt.go` ubus+uci 命名段；`unifi.go` Integration API；`custom.go` 模板渲染）；配置与校验在 `internal/config/routing_api.go`，凭据加密存储、导出剥离 | `internal/routing/manager.go` 的 `DesiredAPI` / `ReconcileAPI` / `ReplaceAPI`（热替换不重建 BGP）；`internal/app/configuration_impact.go` 的 `API` 影响；`web/src/routing-api-settings.jsx`、`routing.jsx` 的模式选择与 API 状态卡；`docs/routing-backends.md`。Hybrid 下手动路由分发方式 `manual_backend` 与 Fake-IP / DNS 拦截独立选择（`internal/config/routing.go` 的 `Effective` / `UsesBGP` / `UsesAPI`，旧配置默认 BGP），Static 时为外部声明不安装 |
| URL 路由导入与定时同步 | `internal/routeimport/import.go`；`internal/api/api.go` 的 `RunRouteFeedScheduler` | `web/src/route-import.jsx`；配置中的 route feeds；导入与后台任务状态 |
| 健康检查、异常通知、Surge 重启 | `internal/app/app.go` 的 `monitor`；`surge_recovery.go`；`notification_anomalies.go`（每秒统计告警采样）；`internal/state/machine.go` | `internal/surge/recovery.go`；`internal/notifications/notifications.go`、`anomalies.go`（有界核对窗口与持续计时，不驱动路由门禁）；`internal/config/notification_anomalies.go`；`native/notifications.swift`；`web/src/surge-restart.jsx`、`notification-errors.jsx`、`notification-anomalies.jsx`；`docs/notification-anomalies.md` |
| 大屏监控、策略命中与 DNS 流向 | `internal/dnsserver/monitoring.go`（`Begin` / `FinishQuerySource` / `Snapshot`）；`monitoring_dimensions.go`（有界真实请求关联与组合筛选）；`monitoring_domains.go`（独立域名待写批次、保存唤醒与确认）；`source_policy_flows.go`（有界真实来源 / 命中策略 / 最终策略三元组）；`dispatcher.go` 的实际解析路径标记；`internal/app/statistics.go`；`internal/database/monitoring.go`（事务检查点与分钟增量桶）、`monitoring_domains.go`（磁盘累计 / 分钟域名排行与批次去重） | `internal/api/monitoring.go`（实时源详情 / 历史 GET）、`internal/remote/remote.go`；`app.go` 的 `status.dns_monitoring` / `Metrics`；`web/src/monitoring.jsx`、`monitoring-filters.jsx`（六维选择 / 域名占比）、`monitoring-flow-grid.jsx`（并列图自然高度同步 / 上下排列恢复）、`monitoring-source-label.jsx`（完整备注换行 / 点击 IP 切换）、`monitoring-history.jsx`、`monitoring-data.js`（三层共享比例尺）、`monitoring.css`；`docs/monitoring.md` |
| DNS 累计统计、客户端与查询历史 | `internal/dnsserver/dispatcher.go` 的 `Counters` / `ClientStatistics` / `History`；`internal/app/statistics.go`（客户端共享累计实例与五秒检查点）；`internal/dnsserver/client_statistics.go`（恢复校验）；`internal/app/client_hostnames.go`（日志与隐私可选的后台 PTR 探测与有界临时名称缓存） | `web/src/dns-overview.jsx`、`clients.jsx`、`client-settings.jsx`（列表编辑窗口）、`dns-result.jsx`；客户端配置校验在 `internal/config/clients.go`，分流在 `internal/dnsserver/client_policy.go`；查询历史不要误当成持久化累计统计 |
| 登录、管理 IP 白名单、反向代理 | `internal/security/security.go`（`Wrap`）；`client_ip.go`；`internal/config/management_security.go` | `internal/controlclient/access.go`；`web/src/credentials.js`、`backends.jsx`；`native/credentials.swift`、`remote_credentials.swift`；`docs/remote-management.md` |
| 远程连接与接口兼容 | `internal/remote/remote.go` 的接口允许列表与中继；`web/src/client.js` 的 `request` / `requestOptional` / `selectBackend` | `web/src/backends.jsx`、`version-notice.jsx`；`Workspace` 的页面请求与登录恢复 |
| 检查版本、更新源、自动下载/安装与跳过版本 | `internal/config/config.go` 的 `Updates.UpdateSource`；`internal/softwareupdate/update.go`（`Check` / `StartForChannel` / 版本比较）；`internal/softwareupdate/worker.go` 保留已选来源；`internal/api/update_scheduler.go` | `web/src/main.jsx` 的 `Settings`、`app-preferences.jsx`、`backend-update.jsx`（持久挂载更新器 / 检查按钮 portal）、`form-fields.jsx`；`native/release_updater.swift`；`native/credentials.swift` 的 `GitHubAccessTokenVault` 保存本机 App 的 GitHub token；`update_dialog.swift`、`update_assistant.swift` |
| 完整升级、回滚、远端 App 重启 | `internal/softwareupdate/application.go`、`worker.go`、`app_restart.go`（`ForceRestartApp` / 取消旧助手与 GUI 交接）；`agent.go`（更新助手）、`download_progress.go`（实际下载字节采样） | `native/remote_update_restart.swift`、`installer.swift`；`internal/security/update_sessions.go`；凭证与管理窗口恢复 |
| CLI / skill 开关、安装目标与冲突 | `internal/api/automation.go`；`internal/appautomation/automation.go`（目标后端的桌面用户 helper） | `native/automation.swift`；`web/src/automation-settings.jsx`；`scripts/install_cli.sh` / `install_skill.sh`；`skills/routepilot-control/` |
| 状态栏菜单、动画与指标 | `native/main.swift` 的 `rebuildMenu` / `refreshFromBackend`；`status_icon.swift`、`status_preferences.swift`；`management_backends.swift`（打开管理窗口的后端子菜单）；`remote_credentials.swift` 的 `openManagementBackend`（按后端新建 / 唤起独立窗口） | `web/src/status-bar-settings.jsx`；`internal/app/surge_recovery.go` 的请求速率采样 |
| 多管理窗口与会话隔离 | `native/management_windows.swift`（每个窗口的后端、存储、生命周期、可见性与入口迁移）；`main.swift` 的 `userContentController` 按 `message.webView` 回复来源；`remote_credentials.swift` 按来源恢复登录 | `web/src/client.js`（逐页面请求状态与目标地址绑定）、`main.jsx` 的后端选择；`internal/remote/remote.go`（地址变更返回 409，保留旧调用兼容）；`native/management_windows_test.swift` / `management_windows_fixture.py` 使用真实 WKWebView 与请求客户端隔离验证；`docs/remote-management.md` |
| 窗口尺寸、重开页面与 JS bridge | `native/main.swift` 的管理窗口与 `userContentController`；`window_frame.swift`；`monitoring_preferences.swift`（大屏显示偏好） | `web/src/credentials.js`、`app-preferences.jsx`、`window-visibility.js` |
| 页面布局、主题与双语 | `web/src/style.css`、对应专用 JSX；导航在 `navigation.jsx`（完整 / 图标 / 隐藏悬浮状态，当前界面 localStorage 偏好） | `web/src/monitoring-preferences.jsx`（卡片显示 / 展示脱敏，App 偏好与浏览器保存）；`web/src/i18n.js`、`locale.js`；`localization/ui-en.json` / `errors-zh.json`；缓存与路由另有专用 i18n；帮助气泡的触发器和定位在 `.inline-help-control` / `.inline-help-popover` |
| 数据库与重启恢复 | `internal/database/store.go`（迁移 / `Save` / `Load`）；`active_configuration.go` 区分已保存与已生效配置 | `internal/app/decision_cache.go`、`response_cache.go`、`statistics.go`；不要改变现有 secret 加密边界 |
| 版本与安装包 | `internal/buildinfo/release.json` / `info.go`；`scripts/build_app.sh` / `check_package.py`；DMG 矢量 PDF 背景图 `native/dmg_background.swift`、Finder 布局 `scripts/dmg_layout.sh` / `scripts/dmg/DS_Store` | `Makefile`；`.gitea/workflows/release.yaml`；`.github/workflows/test.yaml` / `release.yaml`；`.gitea/release-notes/`；`native/Info.plist` |

### 核心调用关系

- **服务启动：** `cmd/controller serve → app.Run / Open → build Runtime → Serve`；Runtime 将监听器、分流器、解析器、路由、健康状态和持久化装配在一起。
- **DNS 请求：** `dnsserver.Server → Dispatcher.Dispatch → 本地/AAAA/always-real-ip 判断 → app.policy / classifier.Engine → resolver.Real 或 Surge Fake-IP`；实际优先级以 `Dispatch` 为准，修改时先读相关分支。
- **配置提交：** `Settings / Resource → client.request → api.saveConfiguration → app.SaveConfiguration → SQLite`；后台 worker 再调用 `activateConfiguration`，页面通过 `configuration/status` 看生效结果。
- **路由与详情：** `app.monitor → routing.Manager → bgp.Manager / GoBGP`；`bgp/status → app.View → Workspace → Resource / BGPPeerDetails`。
- **远程管理：** `client.request → 本机 remote 中继 → 远端 /api/v1`；直接浏览器/CLI 访问远端时走远端 security/API。不要混淆代理来源 IP、Controller 出口 IP 与浏览器 IP。
- **软件升级：** 页面或 CLI 调用 `updates/* → API/update scheduler → softwareupdate.Manager → application/worker → 服务就绪 + App 重启协调`；原生更新提示/助手另有 Swift 入口，排查时先确定更新发起方式。

### 减少读取与避免误改

- 按问题选择上表一行，先用 `rg -n '函数名|字段名|接口路径' 文件` 定位，再读目标函数和直接调用者；不要默认递归打印整个仓库。
- 前端新增选项一般同时涉及：`Settings` 或专用组件、`form-fields.jsx` 的必填提示、语言映射、后端 config 默认/校验和保存链路。原生 App 偏好还需检查 JS bridge，不要直接加进后台配置。
- 新增状态字段一般检查：运行模块 → `app.View` / `Metrics` → 远程允许列表 → 页面轮询/旧版降级。新增可选字段不一定需要新接口。
- 测试文件通常在目标 Go 包内的 `*_test.go`、`web/src/*.test.*`；原生检查入口 `scripts/test_native.sh`，浏览器入口 `scripts/ui_forms_check.cjs`。测试执行遵循当前任务授权与上层指令。
- 默认排除 `.git/`、`.cache/`、`web/node_modules/`、`web/dist*/`、`bin/`、`artifacts/` 和发布历史目录；需要版本追溯时才定向读取标签与说明。
- 文件职责或关键调用改变后同步更新本表；新增模块只记录职责、入口和关联，不在 AGENTS.md 复制实现代码。

## 任务交接（2026-10-15，v1.0.4-beta.25）

- 目标与验收条件：发布主机名探测及状态栏后端选择测试版；每次测试版发布必须更新本文件并准备交接，核对双仓库提交 / 标签、回归、发布 workflow 与三附件。
- 当前状态：已完成。`v1.0.4-beta.25` 已在 GitHub 和 Gitea 完整核验发布；本轮交接更新作为发布后的文档提交同步双仓库，不移动标签。
- 已完成与修改文件：PTR 后台探测见 `internal/app/client_hostnames.go`、`app.go`、`internal/config/config.go`、`internal/dnsserver/dispatcher.go`；显示与开关见 `web/src/clients.jsx`、`main.jsx` 及双语；状态栏后端菜单见 `native/management_backends.swift`、`main.swift`、`remote_credentials.swift` 与构建 / 原生测试入口；说明见 `docs/client-settings.md`、`docs/remote-management.md`、`docs/acceptance.md`、`.gitea/release-notes/v1.0.4-beta.25.md`。本文件已明确要求每次测试版发布前后更新交接，受阻时记录真实状态和下一步。
- 验证：`env GOCACHE=/Volumes/Code/RoutePilot/.cache/go-build make verify-release` 全部通过，前端 35 文件 / 189 项、Go 全包 race / vet、原生核心与真实 WKWebView、17 页面 / 14 设置分类响应式检查、9 项缓存 UI 检查和完整 App / DMG / ZIP / SHA256 包校验。GitHub 回归含 fuzz 通过。双仓库下载包 App / 后台均为 `1.0.4-beta.25 / build 33`，本地源码 / 包为 build 107；六附件哈希、ZIP / 签名 / DMG 完整性已核对。未验收范围见上方最近发布说明，不把包校验当成安装或联网验收。
- 已发布：tag `v1.0.4-beta.25`，发布提交 `352a8f82c43038957325910ff68acc2e468650a6`；GitHub 回归 `38690630180`、Release `38690630151`、Gitea `2103` 成功，Gitea 获取路径成功 / 回退跳过。GitHub: https://github.com/KnotDing/routepilot/releases/tag/v1.0.4-beta.25；Gitea: https://gitea.knotding.com/knot/routepilot/releases/tag/v1.0.4-beta.25。两个页面均为非草稿测试版，各有相同 DMG / ZIP / SHA256SUMS 三附件。
- 保留的无关改动：无。构建产物 / 原始日志保持在忽略目录 `artifacts/`，不提交；未安装升级或重启现用环境。
- 下一步与依赖：无未完成的功能或发布事项；后续新任务可随时交接，先核对当前 main 与工作区再实施。GitHub SSH 22 / 443 本次间歇关闭，推送使用按命令的 GitHub URL → HTTPS 改写及已有 gh 凭证助手，保留 origin 的 Gitea / GitHub 两个 pushurl，不修改持久远端配置或输出凭证。继续推送仍须双仓库同步。发布证据目录 `artifacts/releases/v1.0.4-beta.25/`，旧产物备份 `artifacts/pre-beta25-backup-20261015/`；这些本地证据若不在交接机器上，以以上远端 workflow / Release 为核验入口。

## 任务交接（2026-10-15，v1.0.4-beta.26）

- 目标与验收条件：DNS 设置变更保留未改变的 BGP 会话；BGP 全局设置不重建 DNS；隐私与客户端设置热更新；排查其余设置项并记录必要的跨模块依赖；新增可选的探测主机名域部分截断并一并发布测试版。
- 当前状态：已完成。`v1.0.4-beta.26` 已在 GitHub 与 Gitea 完整核验发布；本轮最终交接文档作为发布后的提交同步双仓库，不移动发布标签。
- 已完成与修改文件：`internal/app/configuration_impact.go` 统一计算 DNS / BGP / 邻居 / 路由依赖，`app.go` 分离生命周期、保持安全排空与失败恢复；`client_hostnames.go` 单独启停 PTR，并依据 `settings.logging.hostname_device_only` 在视图中截断名称，保留完整缓存；`internal/dnsserver/dispatcher.go` 与 `client_policy.go` 原地调整记录和客户端策略；`internal/routing/manager.go` 保护共享配置读取。截断开关在 `internal/config/config.go`、`web/src/main.jsx` 和 `localization/ui-en.json`，默认关闭、随日志分区备份，仅影响探测名称。相关 app / dnsserver / config / 前端测试覆盖热更新、名称恢复、真实隔离 BGP 会话与恢复。分项设置表及边界见 `docs/configuration-application.md`，README、架构、客户端、保存响应和验收说明同步。
- 验证：`env GOCACHE=/Volumes/Code/RoutePilot/.cache/go-build make verify-release` 最终通过，前端 35 文件 / 189 测试、Go 全包 race / vet、原生核心与真实 WKWebView、17 页面 / 14 分类响应式检查、9 项缓存 UI 和完整 App / DMG / ZIP 包校验（本地 App 与后台 `1.0.4-beta.26 / build 108`）。日志及本地包报告见 `artifacts/releases/v1.0.4-beta.26/`。隔离双 GoBGP 邻居确认 DNS / 缓存修改不改变会话建立时间，独立路由保留；隐私 / 客户端不替换运行实例；监听失败恢复与重试保持 BGP；截断开关不重启探测任务且可从完整缓存恢复名称。未覆盖现用安装升级、真实网关 / PF / LAN / IPv6 及长期运行。
- 已发布：`v1.0.4-beta.26`，发布提交 `70ddfedf3108ed421be3e408593e0ae9e9d7b233`，注释标签对象 `3cb6c5b165ba9d01c23afc36c10469ef20183086` 已双仓库核对。GitHub 回归 `38696844640`（含 fuzz）与 Release `38696844858` 成功；Gitea `2104` 成功复用 GitHub 制品 / 回退跳过。两边均为非草稿测试版，只有 DMG / ZIP / SHA256SUMS 三附件，六附件非空且哈希一致，App 与后台均为 `1.0.4-beta.26 / build 34`；ZIP、代码签名、DMG 校验通过。GitHub: https://github.com/KnotDing/routepilot/releases/tag/v1.0.4-beta.26；Gitea: https://gitea.knotding.com/knot/routepilot/releases/tag/v1.0.4-beta.26。正式 skill 安装版本保持 `v1.0.3`。
- 保留的无关改动：无；本轮开始时仅有本次待发布改动。旧包 / bin 备份后执行发布构建，历史证据与缓存保留；未安装或重启现用环境。
- 下一步与依赖：无未完成的实现或发布事项；后续可随时交接。先核对工作区与远端，继续提交仍须双仓库同步；GitHub SSH 不稳定时沿用前节按命令的 HTTPS 改写和已有 gh 凭证助手，不输出凭证。证据目录 `artifacts/releases/v1.0.4-beta.26/`，旧包及 bin 已备份至 `artifacts/pre-beta26-backup-20261015/`。GoBGP 的路由选择选项为进程级全局变量，隔离测试在重建本地全局 BGP 参数前先关闭远端模拟 speaker，避免同一测试进程的上游竞争；生产后台仍只有一个 speaker。不要为继续此任务重启现用环境。

## 任务交接（2026-10-15，v1.0.4-beta.27）

- 目标与验收条件：同时打开多个管理页面分别控制本地 / 不同远程后端；选择、写操作、登录回调和窗口生命周期相互隔离，保存的连接地址被修改时阻止旧页面请求转向新目标。
- 当前状态：已完成。`v1.0.4-beta.27` 已在 GitHub 和 Gitea 完整核验发布；最终交接更新作为发布后的文档提交同步双仓库，不移动发布标签。
- 已完成与修改文件：`native/management_windows.swift` 管理窗口集合、逐页后端与存储、可见性、关闭和监听迁移；`main.swift` bridge 按来源分派并逐页管理后台通知，`remote_credentials.swift` 按来源登录并打开 / 唤起指定后端；`notifications.swift` 的后台请求绑定选中地址。`web/src/client.js` / `main.jsx` 记录每页目标地址，`internal/remote/remote.go` 核对所有带绑定头的请求；新错误中英映射在 `localization/errors-zh.json`。构建 / 原生测试脚本已纳入新模块，README、远程说明和验收同步。
- 验证：`sh scripts/test_native.sh` 最终全部通过（含新真实 AppDelegate / WKWebView / JS 请求客户端隔离验收）；前端 35 文件 / 190 项与生产构建通过；`env GOCACHE=/Volumes/Code/RoutePilot/.cache/go-build go test -race ./internal/remote ./internal/api -count=1 -timeout 90s`、相关 vet 与 `go build ./...` 通过；完整 Swift 源码以 macOS 13 目标编译 / 链接通过。证据目录 `artifacts/multiple-management-windows-2026-10-15/`。未覆盖真实远程 / 实际钥匙串登录、安装升级、长期资源测量与现场网络；既有 WebKit 并发警告保留。
- 发布前验证：`env GOCACHE=/Volumes/Code/RoutePilot/.cache/go-build make verify-release` 全部通过，前端 35 文件 / 190 项、Go 全包 race / vet、原生核心 / 真实 WKWebView / 多窗口隔离、17 页面 / 14 分类响应式检查、9 项缓存 UI 和完整 App / 后台 / CLI / DMG / ZIP 校验。实际本地包 App 与后台均为 `1.0.4-beta.27 / build 109`；证据见 `artifacts/releases/v1.0.4-beta.27/local-verification.log` 与 `local-package-verification.json`。
- 已发布：`v1.0.4-beta.27`，发布提交 `7c36b3aae8a7bbe590f8dce11c1fac13b371e781`，注释标签对象 `7100a6414613abc168a7ea34b571bbf228cbfc6b` 已双仓库核对。GitHub 回归 `38721553427`（含 fuzz）、Release `38721553616` 成功，Gitea `2105` 成功镜像 GitHub 制品 / 回退跳过。两个页面均为非草稿测试版，各有相同 DMG / ZIP / SHA256SUMS 三附件；六附件下载非空、哈希一致，包内 App 与后台均为 `1.0.4-beta.27 / build 35`，ZIP / 签名 / DMG 完整性通过。GitHub: https://github.com/KnotDing/routepilot/releases/tag/v1.0.4-beta.27；Gitea: https://gitea.knotding.com/knot/routepilot/releases/tag/v1.0.4-beta.27。正式 skill 安装版本保持 `v1.0.3`。
- 保留的无关改动：无；本轮开始时只有本次待发布改动。旧包 / bin 备份至 `artifacts/pre-beta27-backup-20261015/`，历史证据保留；不安装或重启现用环境。
- 下一步与依赖：无未完成的实现或发布事项；后续可随时交接，先核对工作区和远端。继续提交仍须双仓库同步；GitHub SSH 不稳定时沿用此前按命令的 HTTPS 改写及已有 gh 凭证助手，不输出凭证。证据目录 `artifacts/releases/v1.0.4-beta.27/`。源地址绑定头对旧客户端保持可选兼容；新页面的全部读取、写入和任务轮询绑定需本机中继后台同步升级。新窗口不复制状态栏本地会话 Cookie，通过已存凭证独立登录；未保存凭证时新窗口需登录。原生测试通过 `ROUTEPILOT_NATIVE_TEST` 排除实际 App 启动和凭证种子，使用随机 loopback 端口及现有请求客户端；不能运行测试构建作为生产 App。

## 任务交接（2026-10-16，beta.28）

- 目标与验收条件：发布 Surge IPv4 下一跳选填 / 自动探测和 Fake-IP 网段单行逗号输入；双仓库分支 / 标签、发布提交 GitHub 回归、两个 Release workflow 和三项安装附件均核验通过。
- 当前状态：已完成。`v1.0.4-beta.28` 已在 GitHub / Gitea 核验发布；App 与后台为 `1.0.4-beta.28 / build 36`，源码 / 本地包为 build 110。
- 已完成与修改文件：`internal/surge/vmnet.go`（两秒 CLI 探测，仅读 VM `self-ip`）；`internal/routing/next_hop.go`（显式优先、30 秒节流、路由安全校验、有效 / 待应用地址）；`internal/app/app.go` / `configuration.go`（旧 Fake-IP TTL 排空、静态确认绑定）；配置、路由就绪检查和相关测试已同步。UI 在 `web/src/surge-next-hop.jsx`、`form-fields.jsx`、`main.jsx`、`routing.jsx`；网段支持中英文逗号，保存仍为数组。双语、README、代码地图、路由 / 配置应用 / 验收文档和 beta.28 发行说明已更新。
- 验证：本地 `env GOCACHE=/private/tmp/routepilot-next-hop-go-build make verify-release` 全部通过（36 文件 / 193 前端测试、Go 全包 race / vet、原生 / 浏览器与包检查）；发布提交 GitHub 回归包含 fuzz 并成功。六项下载附件哈希一致、版本 / build / ZIP / 签名 / DMG 校验通过。仅只读实测 VM 地址；未安装 / 重启现用环境、未验收真实 DHCP / 网关 / LAN / IPv6 路径。
- 已发布：`v1.0.4-beta.28`，commit `bfb4501848212838f819dc1158b193e6ca859bea`，tag 对象 `2e9345739823347c8091e83ba9f57b5d844e0ca2`；GitHub 回归 `38834865752`、Release `38834901914` 与 Gitea `2108` 均成功，Gitea 下载复用、回退跳过。Release 链接见上方最近核验节；完整证据为 `artifacts/releases/v1.0.4-beta.28/publication-verification.json`。本轮最终交接为后续文档提交，不移动标签。
- 保留的无关改动：无。旧安装包和 `bin/` 备份于 `artifacts/pre-beta28-backup-20261016/`。
- 下一步与依赖：无发布遗留。后续仍需双仓库推送；本轮 GitHub SSH 22 端口关闭且 HTTPS 凭据写入返回 403，已通过官方 `ssh.github.com:443` 和既有 SSH 身份完成推送，未修改全局配置。临时 wrapper 为 `artifacts/releases/v1.0.4-beta.28/github-ssh443.sh`，可配合 `git -c core.sshCommand=<绝对路径> push origin <分支或标签>`；必要时复用以保留 origin 双推约定。

## 任务交接（2026-10-16，v1.0.4 正式版）

- 目标与验收条件：整理 v1.0.3 以来的全部最终变更并发布 v1.0.4 正式版；本地完整验证 / fuzz、发布提交 CI 回归 / 完整 Release、Gitea 镜像及六项附件均核验通过。
- 当前状态：已完成。GitHub / Gitea 正式版均为非草稿、非预发布；App / 后台为 `1.0.4 / build 37`，源码 / 本地包为 build 111。
- 已完成与修改文件：`.gitea/release-notes/v1.0.4.md` 归并 beta.1–28 最终功能、修复与升级边界，含英文摘要和完整提交差异链接；README 新增正式版摘要 / 完整记录入口并更新 skill 版本；`internal/buildinfo/release.json` 和 `scripts/install_skill.sh` 正式 tag / 提示为 1.0.4。验收记录与本文件已补最终证据；本轮没有修改功能代码。
- 验证：本地 `env GOCACHE=/private/tmp/routepilot-next-hop-go-build make verify-release` 及 `make fuzz` 均通过（193 前端测试、Go 全包 race / vet、原生 / 浏览器、完整包；fuzz 217,336 次）。CI 回归和正式 Release 均成功；双仓库三附件分别下载且哈希一致，ZIP / 签名 / DMG / App 和后台身份通过。正式 skill 三源文件可下载且与 tag 一致，未执行安装。真实网关 / LAN / IPv6、长期运行和现用升级未验收。
- 已发布：`v1.0.4`，commit `6d336a770028a18690914eeaf2733cdd42e86ed3`，tag 对象 `cd460ac01058ff470afdd5fb9762a6f56e208219`；GitHub 回归 `38844597375`、完整 Release `38844625516`、Gitea `2109` 均成功，Gitea 复用、回退跳过。双发布页链接见上方；证据为 `artifacts/releases/v1.0.4/publication-verification.json`。后续交接文档提交不移动标签。
- 保留的无关改动：无。此前 beta.28 安装包与 bin 备份于 `artifacts/pre-v1.0.4-backup-20261016/`。
- 下一步与依赖：无发布遗留；未授权升级现用环境。后续仍需 origin 双推；本轮沿用已验证的官方 SSH 443 通道，临时 wrapper 在 `artifacts/releases/v1.0.4/github-ssh443.sh`，可使用 `git -c core.sshCommand=<绝对路径> push origin <分支或标签>`，未更改全局配置。
