# FRR对端配置导出

在“设置 → BGP”、“BGP 概览”或“BGP 邻居”页面选择已配置的对端邻居，
核对对端ASN、对端Router ID 和 Mac 地址，点击“一键导出 FRR 配置文件”。
尚未添加邻居时可填写对端ASN 生成规划文件，但需要另外在 RoutePilot 添加对端邻居。
Mac 地址必须与对应 IPv4/IPv6 出口源地址一致。APP 弹出原生保存窗口；浏览器执行普通下载。

导出文件是网关视角：`router bgp` 使用对端 ASN，`neighbor … remote-as` 使用
RoutePilot 本地 ASN，邻居地址为 RoutePilot。不会把对端地址当作网关自己的邻居。
对端会接受此 RoutePilot 邻居发布的全部 IPv4/IPv6 前缀，按 `maximum-prefix` 限制数量，
并拒绝向 RoutePilot 发布路由。新添加的路由不需要更新对端入站白名单。
不改变接收路由的下一跳；Fake-IP 仍使用 Surge 下一跳，DNS 拦截使用 Controller 下一跳。
请只连接可信的 RoutePilot 邻居，并检查对端现有的其他邻居和出站策略，避免路由被转发给不相关邻居。
外部静态路由不由 RoutePilot 发布，导出不宣称安装、验证或撤销任何外部路由。

Static 模式的文件可建立邻居，但拒收全部路由。BGP/Hybrid 下前缀或下一跳变化会随 BGP
自动更新，无需重新导出；更改邻居地址、ASN 或运行模式后应重新核对并应用配置。
导出不启用本地 BGP；需另行配置 BGP/Hybrid、启用 BGP 和对端邻居。

route-map 放在 `router bgp` 节退出之后。
导出文件名为 `routepilot-frr.conf`。请按对端设备支持的方式手动应用 FRR 配置，
合并保留既有邻居和策略，并在对端核对实际邻居状态、RIB 和 FIB。

此导出功能不会自动登录设备、上传配置或宣称路由已安装。
