# Surge DNS & BGP Controller

macOS 原生状态栏 APP 与 DNS/BGP 后台控制器。React 管理界面嵌入 Go 二进制，配置保存到 SQLite；
使用本机 Surge 判断策略并获取 Fake-IP，通过集成 GoBGP 发布固定的受管理路由。
Controller 不修改 Surge 配置、客户端 DNS 或网关配置。

IPv4 透明 UDP/TCP DNS 和回流防护已用 Docker macvlan、真实 macOS PF 与抓包验证。
[验证记录](docs/phase-2-docker-evidence.md)、[验收范围及十五项不变量](docs/acceptance.md)
明确区分自动测试、实际包路径证据和部署现场仍需验证的部分。

## APP 使用

解压 `artifacts/RoutePilot-macos-arm64.zip` 得到 **RoutePilot.app**，可拖到 Applications
后双击运行。APP 常驻状态栏，不显示 Dock 图标；管理窗口关闭后继续运行。
菜单显示 Controller/Surge/BGP、Fake-IP 门禁和查询速率，可切换 DNS 模式、清缓存、
导入 Surge Hosts、打开管理窗口以及选择“登录时启动”。管理认证开启后，可在原生
安全密码框登录；会话和内嵌 WebKit 数据仅保留在内存。

设置中的“关于”标签显示网页界面版本、当前后端版本与构建编号、运行平台、服务状态和运行时间，支持中英文。管理远程后端时分别显示界面与远端版本；旧后端未提供版本信息时明确标记为不可用。

界面语言统一在顶部“语言”菜单切换，选择保存在当前浏览器，并跨页面、本地服务和远程后端沿用。各页面不再单独提供语言选项。中文和英文覆盖导航、配置项、表格、查询详情、规则测试、FRR 导出、后端管理和应用诊断；域名、策略名称、配置标识及原始 DNS/Surge 记录保留原值。APP 管理窗口中的语言选择同时更新原生状态栏菜单和对话框，并保留到下次启动。应用、网页和状态栏使用统一的节点分流图标。DNS 设置按监听、网络出口和转发参数分组，IPv4/IPv6 成对排列；复选框与输入框分行显示。

设置 → DNS →“策略专用 DNS 上游”可为 Surge 的 `final_policy` 或 `policy` 名称指定 UDP、TCP、DoT、DoH 服务器，支持每个策略多个上游。按名称精确匹配，`final_policy` 优先；未匹配时仍使用 Surge，DIRECT、REJECT、always-real-ip 与本地 DNS 保持原有行为。命中后优先返回指定上游的真实解析结果。可为每条策略选择“查询失败后的备用选项”：其他 DNS 上游、Surge、全局真实 DNS 上游，或不回退（旧配置默认行为）。超时、连接错误、SERVFAIL 等失败会触发备用；NXDOMAIN 和正常空响应不会回退。主上游和备用各有独立的 DNS 超时预算，备用也失败时返回 SERVFAIL。Surge 备用沿用 Fake-IP 发行、路由与健康检查，HTTPS/SVCB 使用空响应继续 A/AAAA。全局真实 DNS 备用固定使用配置的上游池，忽略客户端原始解析器。查询详情记录主上游与备用的实际服务器、状态、耗时、响应缓存和失败原因。专用上游复用固定出口和回流防护，并独立缓存响应；策略配置随设置保存、备份和远程管理。

首次启动默认管理地址为 `http://127.0.0.1:1055`，配置和私有日志保存到
`~/Library/Application Support/RoutePilot`。已有本机服务会直接连接，避免重复启动。
管理监听地址变更后，本机恢复入口仍为 `http://127.0.0.1:1055`；状态栏 APP 可在离线时更新已安装的网络后台服务。
退出 APP 会停止自己启动的子进程并等待 DNS/BGP/PF 清理；独立网络服务继续运行。
后台子进程异常退出会有限重试，可用“重新连接”重试并查看日志。

需要透明 DNS/PF 时，在状态栏选择“安装网络后台服务”，通过 macOS 管理员授权界面
安装 LaunchDaemon；APP 将先完整停止子进程，通过仅当前用户可访问的临时目录迁移
数据库、密钥和安装文件，完成或取消后清理临时副本，避免管理员安装器直接访问受保护
的用户目录或外置卷。安装器拒绝覆盖
既有服务数据。服务随系统启动，APP 可独立设置登录启动。切勿同时运行旧用户数据库
副本和已迁移的服务实例。此发布包使用本机 ad-hoc 签名，未做 Developer ID 公证。

已安装网络服务时，状态栏菜单显示“更新网络后台服务”。更新只替换后台程序，
正常停止旧服务并释放自身 PF 规则后重新启动，保留已安装的数据库、密钥和配置。
更新失败时尝试恢复旧程序；更新过程会短暂中断 DNS。

“BGP 邻居”页面提供对端设备 FRR 配置预览和导出，详见
[FRR 配置导出](docs/frr-export.md)。查询详情单独显示 DNS 响应状态、完整资源记录、
TTL、权威段、附加段和失败原因；UNKNOWN 不等于直连，运行状态回退单独标注。

## BGP、Static 与 Hybrid 路由

在“路由后端”页面切换模式：BGP 自动发布 Fake-IP 与 DNS 拦截路由；Static 使用外部
静态路由；Hybrid 可为两类路由分别选择后端。BGP 可完全关闭。DNS 分类、Surge、
Real-IP 和 Fake-IP 解析独立运行，Fake-IP 发行只检查所选路由后端的就绪状态。

Static 模式先在 Surge 设置填下一跳，按页面给出的前缀及下一跳手动配置网关，再勾选
“Fake-IP 路由已在网关配置”。确认代表用户声明，安装状态始终
显示“未知（外部）”；Controller 不安装、撤销或自动验证这些静态路由。
修改模式、前缀或下一跳会清除旧确认，需先保存修改再重新勾选。
路由后端页只显示当前模式的简短限制、确认项及状态；路由页、缓存列表与缓存设置共用
中文／英文语言选择，
本机连通性检查默认折叠。

静态 DNS 拦截另有独立的持久拦截许可，默认关闭。若使用它，Controller 停止时路由
仍可能把 DNS 请求送往不可用的 Mac，必须安排外部故障切换或手动移除；BGP 方案则
依靠会话失效撤销发布。Surge 不可用时，所有模式都停止新 Fake-IP 并进入 DIRECT_FALLBACK；
外部静态路由保持原状。可选 TCP 探测只证明 Mac 对指定 Fake-IP/端口的连通性，不证明
外部网关 RIB/FIB、LAN 可达性或故障切换。探测不会改变静态路由确认。

路由配置单独更新，DNS 监听和解析核心继续运行；更换 BGP 后端前会等待已发行
Fake-IP 的 TTL 排空。详细字段、操作顺序与限制见 [路由后端说明](docs/routing-backends.md)。

## 规则求值 DNS

Surge 在遇到需要 IP 地址的规则时通常会自动完成解析。Controller 不再把“需要 DNS 查询”当成失败：只有解析尚未完成或失败时，才调用一次 `surge-cli dns lookup`，然后用相同的域名和客户端上下文重新求值。补查走 Surge 内部解析管线，使用它的 DNS 映射和缓存；不会查询 Fake-IP 响应端口来补充真实地址。全部步骤共用规则求值时限，最多补查一次。解析成功不解除其他连接上下文限制；补查失败仍按 UNKNOWN 处理。

查询详情分别显示客户端 DNS 响应与规则求值 DNS 状态，并保留 Surge 原始求值记录。

Controller 每 5 秒读取 Surge 生效配置中的 `always-real-ip`，支持顺序匹配、`*` / `?` 通配符与 `-` 排除项。命中后跳过 Surge 规则求值和 Fake-IP 查询，直接使用 Controller 配置的真实 DNS 上游，即使开启“保留客户端原始解析器”也不会查询原始目的地址。显式本地 DNS 条目仍优先。读取失败保留最后确认的列表，确认移除条目后才停止旁路。查询记录的来源标记为 `surge-always-real-ip`。“域名覆盖”表格同时显示自动同步的 Surge 条目、排除项和手动覆盖；Surge 条目标明来源并保持只读，应在 Surge 中修改。读取失败仍显示实际沿用的最后确认列表。

## 本地 DNS

“本地 DNS”页面支持手动增删改、启用/禁用、TTL、精确域名与 `*`/`?` 通配符，
一个条目可填写多个 IPv4/IPv6 地址。静态条目优先于全局模式和 Surge 分类，
不会产生单 IP BGP 路由；缺少查询地址族或 HTTPS/SVCB 时返回空成功响应。

“从 Surge 导入”读取当前有效配置的 `[Host]` 静态 IP 映射，保留原始匹配顺序。
导入是快照，之后修改 Surge 需要重新导入；重复导入替换旧 Surge 快照、保留手动条目。
手动条目始终优先，手动精确条目优先于手动通配符。`*.example.com` 不匹配根域名。
服务器指定、别名、脚本、远程规则集及其可能遮蔽的后续条目会跳过并列出原因；
不会下载资源或执行脚本，也不读取 `/etc/hosts`。可手动补充对应静态记录。
导入的条目在下次导入时会被替换；长期修改请另建手动条目。Fake-IP 网段映射拒绝导入。

## 构建与启动

源码构建需要 Go 1.26+、Node.js 22.12+ 和 npm。实际 PF/固定接口出口只支持 macOS。

```sh
go mod download
cd web
npm ci
cd ..
make verify
bin/surge-controller serve --database /ABSOLUTE/PRIVATE/DIRECTORY/controller.sqlite
```

打开 **http://127.0.0.1:1055**。数据库目录应属于运行用户；应用创建目录时使用 0700，
数据库、加密密钥使用 0600。不要把加密密钥放进 Git，也不要让两个实例共享数据库。
默认仅开放本机管理界面，BGP 和透明拦截关闭；先配置出口接口及源地址才能启动 DNS。
未配置的状态会显示真实告警，Fake-IP 发行保持关闭。

`make build` 编译前端并生成 `bin/surge-controller` 和 `bin/dns-probe`。
`make verify` 运行前端测试、Go race 测试、go vet 并构建；`make package` 生成带
SHA256SUMS 的 macOS 发布压缩包、原生 APP、APP ZIP 和 DMG 安装镜像。APP 构建另需 Apple Command Line Tools（Swift/AppKit/WebKit）；`make app` 可单独打包 APP。离线单元/集成测试不需要 Surge、真实路由器或外网。

### Gitea Release 与在线升级

`.gitea/workflows/release.yaml` 在推送 `vMAJOR.MINOR.PATCH` 标签时构建并发布 macOS Apple Silicon 版本，包括 DMG、ZIP、后台服务归档和 SHA256SUMS。Gitea 仓库需启用 Actions，并注册一个 macOS Apple Silicon Runner，标签为 `macos-arm64`；工作流使用仓库内置 `GITEA_TOKEN` 创建 Release 和上传附件。示例：

```sh
git tag v0.4.7
git push origin v0.4.7
```

状态栏 App 每 12 小时检查一次 Gitea 最新稳定 Release；有新版本时，菜单会显示“在线升级”。升级前会验证 Release 中的 SHA256SUMS，再经 macOS 管理员确认替换 App；已安装的 launchd 后台服务也会一并更新，配置数据库与密钥保留。远程管理模式下，此功能升级本机 App，不会升级远端主机上的服务。当前发布未使用 Developer ID 签名或 Apple 公证。

## 初始设置

1. 在设置 → DNS 填入实际物理出口接口、IPv4/IPv6 源地址和网关；配置真实上游池。
   上游地址必须是字面 IP:port；DoT 填证书名称，DoH 填 HTTPS URL。默认保留透明请求
   的原始解析器，直接向 Controller 查询时使用配置的池。源端口默认 20000–20999。
2. 核对 Surge CLI、Fake-IP DNS、Fake-IP 网段及各地址族的 Surge 下一跳。
   service 以 root 运行时，CLI 调用降权到当前桌面用户；也可指定 Surge 所属 macOS 用户。
   HTTP API 可选；在密码输入框输入凭据，禁止把密钥写进命令行或聊天记录。
3. 在网关手动设置高优先级源策略：Controller 的上游流量走不含拦截路由的 WAN 路径。
   单网口固定接口本身不能改变网关的路由决定。分别验证各地址族后再勾选绕行已验证。
4. 配置本地 ASN、Router ID、邻居、计时器和路由硬上限。邻居 Local ASN/Router ID 留空
   或使用全局值。Fake-IP 下一跳是 Surge，DNS 拦截下一跳是 Controller，不能互换。
5. 在 DNS 拦截页面添加解析器前缀；填写实际接收接口和具体透明监听地址（建议 1054），
   再启用透明拦截。必须以具备 PF 权限的服务运行，且 macOS 根规则可到达
   `com.apple/*` 锚点。只能管理 `com.apple/surge-controller`，不得替换全局规则。
6. 选择路由后端：使用 Static/Hybrid 的外部路由时按“路由后端”页面声明并确认，
   静态 DNS 拦截必须独立许可且配备外部故障切换。
7. 正式切换前关闭 Surge 的全量 `hijack-dns = *:53`，保留原有 `dns-server`；
   客户端 DNS 请求需先经过正常网关，不能先进入 Surge VM。仅修改上游 DNS 无法改变
   已被 Surge 抢先接收的包路径。开发验证期间先保留现有设置，Controller 不自动修改。
8. 使用 BGP 后端时，启用 BGP 后，等待邻居 ESTABLISHED、所需路由进入 Adj-RIB-Out 和稳定延时。
   再用 Docker 的非本地目标 DNS 测试及包路径核对验收；不能用普通容器解析成功代替。

AUTO 默认仅在策略证据充分时返回真实地址，复杂/动态 Surge 规则会保守归入 UNKNOWN/SURGE。
可在“设置 → Surge”关闭“DIRECT 保守判断”（配置 `surge.trust_direct=true`），
采用成功求值后的内置 DIRECT 结果并查询真实 DNS 上游。查询详情以
`confidence=configured` 和原因区分该配置选择与上下文已确认的 DIRECT；
未来连接仍可能匹配不同策略。求值失败、超时、非内置 DIRECT 和带底层链的结果仍不会因此直连。
新旧配置默认保持保守判断，保存开关会重建运行时并清除旧决策缓存。
规则测试明确区分指定的未来 TCP/UDP 连接与 DNS 时尚未知的连接参数。
FORCE_DIRECT 是显式覆盖，FORCE_SURGE 仍受健康与路由门禁控制。Surge 故障会停止新
Fake-IP、保留已发行地址的 TTL/宽限期并进入真实 DNS 回退；Controller 健康时保留 DNS
拦截路由。Controller/BGP 会话消失后，网关应删除这些路由、恢复原解析器路径。
本地 RIB/Adj-RIB-Out 无法证明网关已安装路由，现场需核对网关 RIB/FIB。

IPv6 代码、报文结构及 BGP NLRI 有自动测试覆盖；**IPv6 实际 PF/WAN 路径尚未验收，
默认不启用**。解析器前缀的 BGP 路由影响到该地址的全部流量，透明入口只处理 UDP/TCP
53；请在部署中考虑该解析器的 DoH/DoT 等其他流量。

## 安装、launchd 与移除

发布包解压后可直接运行 `bin/surge-controller`。安装常驻服务需显式执行：

```sh
sudo sh scripts/install_macos.sh
sudo launchctl print system/com.routepilot.surge-controller
```

二进制安装到 `/usr/local/libexec/surge-controller`，私有配置保存到
`/var/db/surge-controller`，结构化日志在 `/var/log/surge-controller/service.log`。
LaunchDaemon 开机启动、异常退出重启，使用 root 获取 PF/179/53 端口权限；仅 CLI 子进程
降权到 Surge 用户，管理界面默认只监听环回地址。日志级别及隐私容量可在 UI 调整，
服务日志由运维按本站策略轮转。安装器拒绝覆盖已安装实例。

```sh
sudo sh scripts/uninstall_macos.sh
```

移除会停止服务，释放记录且校验过的自身 PF 锚点/引用，删除自身 plist 和二进制。
SQLite、密钥与私有日志保留；需升级时可移除后安装新版本继续使用原配置。
若 PF 所有权已变化或旧进程仍存活，清理会拒绝操作，保留文件供排查；不要全局关闭 PF。
安装/移除脚本和 plist 已做语法检查，当前开发环境尚未安装 LaunchDaemon。

## 管理、安全和隐私

15 个页面提供状态、查询及缓存、拦截前缀、覆盖、规则测试、BGP 邻居/手动路由、
本地 DNS、独立健康探测、事件和全部设置。手动路由默认 Health Binding=None，可显式绑定
Controller、Surge 或 DNS（手动路由属于 BGP 资源）；没有从查询/回答生成 per-IP BGP 路由的路径。

“手动路由 → 从 URL 导入”支持公开 HTTP/HTTPS 文本列表（每行 IPv4/IPv6 地址或规范 CIDR，支持 `#` 注释）。例如 Telegram 的 `https://core.telegram.org/resources/cidr.txt`。先预览，再选择网段或地址族，分别设置 IPv4/IPv6 下一跳，以及统一的健康绑定、MED、本地优先级、Communities、说明和启用状态；默认下一跳取自 Surge 配置。普通地址转为 `/32` 或 `/128`，重复、已有和无效行在预览中列出。默认保存为可独立编辑的手动路由快照。也可启用定时更新（每小时、6/12 小时、每天或每周），并单独选择同步 IPv4、IPv6 或两者：服务端定期重新抓取 URL，同步该来源管理的路由；URL 中新增/移除的前缀会相应添加/撤下，停止更新时保留现有路由并转为手动路由。列表上限为 1 MiB、8192 行和 1024 个新目的地址。列表由当前选中的管理后端获取，远程模式也支持导入。

REST `/api/v1`、SSE `/api/v1/events/stream` 和 Prometheus `/metrics` 使用同一管理
网络边界。认证开启后需要管理会话；写操作始终要求 CSRF。远程管理必须配置认证和允许网段；HTTPS/TLS 可选但推荐，默认拒绝任意域名 Host/跨来源请求以防 DNS rebinding。密码使用 bcrypt；
Surge API token 使用 AES-GCM 保存，导出不含 token 或密码哈希。导入空凭据保留现有密钥。

查询历史只在有界内存中，开关、域名记录及容量可配置；不会把逐条 DNS 查询写入数据库
或服务日志。SQLite 只持久配置与有界重要事件。显式诊断 journal 会记录域名/客户端，
使用 0600，测试后应按需删除。备份导出用于配置恢复，迁移到新机器需重新输入凭据。

## 开发与排错

独立诊断入口（区别于正常 `serve`）不启动 BGP，始终关闭 Fake-IP 门禁：

```sh
bin/surge-controller --mode=force-direct --development-system-egress \
  --listen=127.0.0.1:1053 --upstream=udp://1.1.1.1:53
dig @127.0.0.1 -p 1053 example.com A
dig +tcp @127.0.0.1 -p 1053 example.com A
```

`--development-system-egress` 仅允许非透明开发诊断。透明测试步骤和清理命令见
[Phase 2 指南](docs/phase-2-acceptance.md)。Docker 离线回归：

```sh
sh scripts/build_docker_tests.sh
docker run --rm --network=none --user=1000:1000 \
  --mount type=bind,src=/ABSOLUTE/PATH/bin/docker-tests,dst=/tests,readonly \
  --mount type=bind,src=/ABSOLUTE/PATH/scripts/run_docker_tests.sh,dst=/run-tests.sh,readonly \
  node:22-bookworm /bin/sh /run-tests.sh
python3 -m unittest discover -s scripts -p 'test_*.py'
```

- DNS 未启动：检查界面告警、出口接口/已分配源地址、端口占用及服务权限。
- 透明 DNS 超时：同时核对 PF 原始目的查找、实际接口/桥、网关源策略与物理出口抓包。
  PF NATLOOK 在本机使用 Darwin 私有 84 字节 ABI，UDP `proto_variant=1`、TCP 为 0。
- Surge CLI 不可达：检查指定桌面用户、Surge 是否运行和本地 IPC；沙箱失败不代表 Surge 已停止。
- Fake-IP 未启用：检查 CLI/API/Fake-IP DNS 各探测、所选路由后端、静态确认或 BGP 发布，以及稳定等待。
- PF 清理被拒绝：保留记录，确认无旧 Controller，检查自身锚点；不要 flush 全局规则或状态。
- 设置核心 DNS/BGP 参数会受控重建服务，可能短暂中断；已发行 Fake-IP 会先按 TTL 排空。

[架构说明](docs/architecture.md)记录保守分类、缓存一致性和真实网络验证边界。
原始需求完整保留于 [工程规格](docs/engineering-specification.md)。


决策缓存默认长期保留 DIRECT、SURGE、REJECT，直到确认 Surge 有效配置、临时规则、
代理组结构或外部资源版本更新。UNKNOWN 和求值错误仍按短 TTL 重试。外部资源元数据
每 5 秒核对一次，下载进行中不视为版本更新；确认更新后清理全部决策并拒绝旧代在途写入。
“设置 → 缓存”可独立控制读取失败时是否失效旧决策（默认保留），以及策略组选择变化时
是否失效 DIRECT、REJECT（默认均开启）。读取失败保留上次成功确认的版本，未变化的
配置恢复不会重复清理；启用失败失效后，连续失败只清理一次。
可开启“按 TTL 定时过期稳定决策”恢复原有时限。长期条目仍受缓存容量限制；服务重启、
手动清理或相关本地分流配置修改也会清理，不影响 DNS 响应自身的 TTL 或健康门禁。
缓存页面搜索全部有效条目，每页 50 条，显示长期保留标识和最近失效原因／时间。
缓存 API 支持 `offset`、`limit`、`domain`、`decision` 查询参数。

Web 顶部“本地服务 · 切换”可添加、编辑、移除和切换远程 Controller。
远程页面明确显示当前连接，登录、查询、配置、缓存和 FRR 导出均发送至选中的后端。
详见 [远程后端管理](docs/remote-management.md)。BGP 概览仅保留引擎、会话汇总和 BGP 路由；
邻居配置、实时会话详情和 FRR 导出集中在“BGP 邻居”。

Web 界面统一使用 42px 输入框／下拉框／操作按钮和明确的标题层级，
支持明暗主题、键盘焦点、桌面和移动布局。查询弹窗中的结果卡片独立排版。
