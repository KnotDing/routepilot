# DHCPv4 服务

RoutePilot 后台可在明确指定的一个局域网接口、一个 IPv4 子网上提供 DHCP 服务。默认关闭，旧配置升级后保持关闭。配置属于管理窗口当前连接的后台；远程页面的检查、保存及启停均在目标 Mac 执行。

## 配置与启用

在「DHCP 服务」填写接收接口、服务器 IPv4、规范子网 CIDR 和动态地址池起止地址。服务器地址须已配置在该接口上，且接口子网必须与填写值一致；服务器和各类网关必须位于动态地址池之外。支持 1–65,536 个动态地址、最多 4,096 项设备配置。

启用前会进行两项检查：

- 在所选网口以当前已配置的 IPv4 和随机客户端标识发送 DHCPINFORM，等待匹配的 DHCPACK。只查询配置，不申请、不获取或更改本机租约；已经运行的 RoutePilot 自身回复会被排除。页面列出响应的服务器地址。
- 在 macOS 读取该网口对应网络服务的 IPv4 配置方式，区分手动静态地址、DHCP 和未知。不会修改网口配置。

发现其他服务器、网口使用 DHCP 或检查失败时展示具体提示，页面允许取消或明确选择继续。探测窗口内未收到回复仅表示「未观察到」，不能保证网络内不存在其他服务器。UDP 68 被占用、权限不足或平台不支持地址方式检查时，结果为未知/检查失败。后台启用或重启时再次检查，结果保留在 DHCP 状态和警告事件中；CLI 使用者应查看 `dhcp status`。

在同一子网正式启用前停用原有 DHCP 服务器，并采用静态服务器地址。现用客户端须重新获取租约才会更新选项。服务需要管理员权限及独占 UDP 67；不修改 PF、系统网关、接口地址、UCG 设置或 BGP 路由。

## 全局选项与设备覆盖

| 页面字段 | DHCP 行为 |
| --- | --- |
| 网关 | Option 3；存在自定义路由时也加入 Option 121 的默认路由 |
| DNS 服务器 | Option 6，多个 IPv4 |
| 自定义路由 | Option 121，最多 16 条规范 IPv4 CIDR / 下一跳；默认路由通过网关字段配置 |
| NTP 服务器 | Option 42，多个 IPv4 |
| 网络启动文件或 URL | Option 67；长度允许时同时填入 BOOTP file 字段 |
| 启动服务器 IPv4 | BOOTP siaddr / next-server |
| TFTP 服务器名称或地址 | Option 66 |
| 租期 | Option 51；同时提供 T1=租期一半、T2=租期 7/8，120 秒至 365 天 |
| 域名 | Option 15，DNS 格式的域名 |

设备按 MAC 匹配，在「DHCP 租约」列表点击编辑即可设置名称、固定 IPv4 和各项选项。已配置但没有有效租约的设备仍保留在列表；列表上方可手动添加设备。设备属性独立保存，不提交上方尚未保存的全局配置。固定地址允许在池内或池外，但须为同一子网内的唯一主机地址，不能占用服务器或任何配置的网关地址；其他客户端不会获得保留地址。

上述九项选项都可逐项「继承全局」或单独覆盖。选中继承时配置字段缺省；取消继承后清空 DNS/NTP/路由列表或文本值表示不发送该选项。租期必须填写有效数值。设备路由覆盖替换整个全局路由列表。iOS 私有 Wi-Fi 地址需使用该网络中实际出现的 MAC。

例如，可为指定设备配置 `91.104.0.0/13 → 192.168.5.2`，网关仍填 `192.168.5.1`。Option 121 会同时包含两条路由；客户端是否采纳需通过实际流量验证。这里的子网示例不是默认部署值。

## 租约与协议边界

支持 DISCOVER/OFFER、REQUEST/ACK/NAK、续租/重绑定、RELEASE、DECLINE 和 INFORM。地址分配/续租在 SQLite 持久化成功后才发送 ACK；失败显示原因并拒绝成功确认。重新应用配置时先停止旧收包循环，再迁移最新租约，避免迁移期间漏掉已确认的分配。正常重启恢复原到期时间，不延长租期。

未确认的 OFFER 只短暂保留在内存。DECLINE 的地址隔离十分钟，并持久化隔离状态。服务不主动对地址池做 ARP/ICMP 冲突扫描，地址池应避免与其他静态设备重叠；客户端报告的冲突会隔离。租约页展示有效分配与冲突隔离，并可从租约创建设备专属配置。

当前范围为本地以太网广播域的 DHCPv4；不支持 DHCP relay、DHCPv6、BOOTP-only 或 option overload 输入。网络启动/TFTP 字段提供已有服务器的信息，RoutePilot 不托管启动文件或运行 TFTP 服务。初始分配在选定接口广播回复，续租按 ciaddr 单播。DHCP 是局域网配置分发协议，设备的 MAC/客户端标识不作为可信管理认证。

## API、CLI 与备份

- `GET /api/v1/dhcp/configuration` / `PUT /api/v1/dhcp/configuration`：独立读取/保存 DHCP 配置。保存立即确认持久化，启停在现有配置应用队列运行。`configuration/status` 展示失败原因，可通过现有重试入口重新应用。
- `POST /api/v1/dhcp/check`：接收同结构配置，执行只读网口方式检查及 DHCP 探测；支持异步操作协议。它不启用服务器。
- `GET /api/v1/dhcp/status`：状态、收发计数、有效租约、错误及最近启用检查。
- `GET /api/v1/dhcp/devices` / `PUT /api/v1/dhcp/devices`：独立读取/替换设备配置列表，保留服务器及全局选项。状态中的可选 `device_settings` 标识该能力；旧后端仍可查看租约，但列表编辑入口禁用并显示升级提示。
- `routepilot --url http://目标:1055 dhcp get|status|schema`；`dhcp check --file dhcp.json`；`dhcp set --file dhcp.json`；`config wait`。

管理认证、CSRF 和远程中继沿用现有机制。配置备份新增可独立选择的 `dhcp` 分区，包括选项及设备配置；租约是运行状态，不包含在配置备份中。旧版通用设置页面保存不会覆盖 DHCP 模块，DHCP 修改需通过专用页面/接口/CLI。

协议依据：[RFC 2131](https://www.rfc-editor.org/rfc/rfc2131)、[RFC 2132](https://www.rfc-editor.org/rfc/rfc2132)、[RFC 3442](https://www.rfc-editor.org/rfc/rfc3442)。

## 实现验证（2026-10-18）

- `npm --prefix web test`：37 个文件、197 项通过，包含设备逐项继承、启用前检查/取消、远程保存及草稿保护。
- `go test -race ./internal/dhcpserver ./internal/config ./internal/database ./internal/api ./internal/remote ./internal/app ./cmd/routepilot`：通过。覆盖租约分配/续租/释放/重启恢复、设备选项与 Option 121、持久化失败、冲突隔离、保留地址、报文边界、检查提示、独立保存/备份和远程允许列表；后续补充测试也单独通过。
- `npm --prefix web run build`、`go build ./...` 及上述 Go 包的 `go vet`：通过。前端仍有大于 500 kB 的分块提示。
- 未在现用局域网启用 DHCP，也未完成真实 UDP 67/68 收发、iOS 租约及路由采纳、与其他 DHCP 服务共存的现场验收。启用检查使用可注入的模拟探测验证分支，不能代替实际广播域验证。本节为实现阶段记录；后续 UI 检查及发布结果见 [验收记录](acceptance.md)。

English: The optional DHCPv4 server serves one explicitly selected local IPv4 subnet. Gateway, DNS, classless routes, NTP, network boot, next-server, TFTP, lease duration and domain support independent per-MAC overrides. Preflight discovery and macOS address-method checks report competing servers, DHCP-configured interfaces and inconclusive checks before enabling. Leases are durably committed before ACK, restored without extending expiry, and kept independent of DNS/BGP restarts. Relay, DHCPv6 and boot-file/TFTP hosting are outside this implementation.
