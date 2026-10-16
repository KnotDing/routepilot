# 路由配置

| 模式 | Fake-IP 路径 | DNS 拦截路径 | BGP 是否必需 |
|---|---|---|---|
| BGP | BGP 发布至 Surge 下一跳 | BGP 发布至 Controller 下一跳 | 仅使用此后端时启用 |
| Static | 用户手动配置外部路由 | 可选；需独立持久拦截许可 | 不启动 BGP |
| Hybrid | 独立选择 BGP / Static | 独立选择 BGP / Static | 仅为选用 BGP 的路径启用 |

DNS 分类、Hosts、Surge CLI/Fake-IP DNS 和 Real-IP 转发与 BGP 生命周期独立。
路由门禁决定是否允许向客户端发行新的 Fake-IP，不决定这些解析器是否可运行。

## Surge IPv4 下一跳自动探测

“设置 → Surge”中的“Surge IPv4 下一跳”选填：填写时使用指定地址，留空时由当前连接
的后端调用 `surge-cli --raw vmnet status`，使用正在运行的 VMNET 的 `self-ip`。
它是 Surge VM 地址，不是 Mac 主接口地址，也不是 `router-ip` 上游网关。
IPv6 下一跳仍按原有规则填写，不自动推算。
同页“Fake-IP 网段”使用单行文本框，多个网段用逗号分隔，例如
`198.18.0.0/15, fd00:1234::/64`；保存格式仍为网段数组。

首次启动 / 切换为自动模式时探测，随后复用健康循环，每 30 秒最多探测一次，单次最多
2 秒。填写手动 IPv4 地址后不执行该探测。自动地址不会写回 `surge.next_hop4`，保存、
重启后仍保持自动模式。设置字段旁显示探测结果、等待应用状态或错误；路由页面显示
当前路由使用的地址。旧后端没有该状态时显示不可用。

探测失败、VMNET 未运行、CLI 不支持或探测地址未通过路由安全校验时，暂停新的
Fake-IP 响应，沿用真实 DNS 回退。旧 BGP Fake-IP 路径在已发放响应的 TTL / 宽限期内
保留，之后撤回；发现新地址也先排空旧响应，再差量更新路径并等待正常路由稳定。
不因此重建 DNS 或 BGP speaker，不影响未绑定 Surge 健康的手动路由。

静态路由仍由用户在外部网关配置。确认会在 `routing.static_fake_ip_next_hop4` 保存
当时的自动地址；地址改变后旧确认失效，需按新地址修改网关路由，再在“设置 → 路由”
重新勾选并保存确认。其他设置的保存不会自动确认新地址，重启也不会绕过此要求。

## 静态操作顺序

1. 配置 DNS 固定出口和网关绕行策略。单网口不等于不会回流。
2. 在 Surge 设置填写 IPv4 下一跳或留空等待探测；使用 IPv6 时填写其下一跳，选择 Static 或 Hybrid/Fake-IP=Static。
3. 按页面的 Fake-IP 前缀 → Surge 下一跳提示，在外部网关手动配置路由。
4. 保存配置后，明确勾选已配置全部 Fake-IP 路由；这只代表操作者确认。
5. 可选择对已经由 Surge 分配的 Fake-IP/TCP 服务端口做本机连通性探测。
   返回 `success` 和 `checked_at` 仅表示这次 Mac 连接结果，不证明网关路由表或 LAN 路径。
6. DNS 拦截可继续用 BGP，也可不开启。若选 Static，必须先独立允许持久拦截，再手动
   配置解析器前缀 → Controller 源地址路由，然后在“DNS 拦截”页的“添加”按钮前点击“确认网关路由已配置”按钮。
   点击后立即保存确认，已确认时显示“✓ 网关路由已确认”并置灰禁用；启用透明 DNS 还需完整 PF/出口配置。

更改模式、前缀或下一跳会清除旧静态确认，避免旧确认被用于新路由。
在“设置 → 路由”选择路由模式、各类路由的后端、静态 DNS 拦截许可和 Fake-IP 网关配置确认，使用“保存设置”提交。
DNS 拦截路由的网关配置确认在“DNS 拦截”页完成，无需返回设置页。

“路由”页面显示所有声明的前缀、下一跳、后端、发布状态和独立探测记录。
BGP 的 `installed=true` 仅表示本地 RIB；`announced=true` 表示本地 Adj-RIB-Out，
两者都不能代替远端网关的安装确认。

## 静态声明的含义

外部静态路由始终 `managed=false`、`installed=null`、`verified=false`。
`declared` 是配置声明，`operator_confirmed` 是用户确认；不会自动推断安装成功。
未确认的 Fake-IP 静态路径不会打开发行门禁。确认后的路径仍按 Surge/DNS 健康和稳定
延迟控制新 Fake-IP。探测成功也不会自动勾选确认或改变安装状态。

RoutePilot 不调用外部路由器写 API，不执行路由器命令，不安装或删除系统静态路由。
静态声明从配置移除或换成 BGP，也不会删除已在外部网关配置的静态路由；须自行核对。

## 故障和切换限制

Surge 不可用时立即关闭新 Fake-IP 门禁，并进入 DIRECT_FALLBACK。BGP Fake-IP 发布
按已经发行地址的 TTL/宽限排空；外部静态路由保持原状。Controller 停止时，静态 DNS
拦截路由可能持续把请求发送到不可用的 Mac，导致 DNS 中断。必须安排网关健康探测、
外部撤路机制或手动应急移除；RoutePilot 不声称已验证这些机制。

路由配置单独替换，DNS 服务继续监听。替换原来承载 Fake-IP 的 BGP 后端时，先暂停新
Fake-IP、排空响应写入并等待已经发行的 TTL；期间真实 DNS 和本地 DNS 仍可运行。
仅更换拦截路径的 BGP，不应使已确认的静态 Fake-IP 路径依赖 BGP 就绪。

REST：`GET /api/v1/routing/status`；`POST /api/v1/routing/verify` 接受
`{"prefix":"198.18.0.0/15","target":"<实际 Fake-IP>","port":443}`。
探测仅允许已声明的静态 Fake-IP 前缀内字面地址、有限超时与有限并发；不处理外部
DNS 拦截“验证”，避免把普通解析成功错误当作透明拦截证明。
