# CPU 与内存检查（2026-10-13）

本轮以局域网应用的低资源占用为目标。高并发仅用于定位热点，不提高默认 worker、队列或缓存容量。测试基于 `v1.0.4-beta.16` 之后的当前工作区，包括尚未发布的通知功能。未安装、升级、重启现用环境。

## 主要结论与已实现优化

1. **原生中文菜单翻译存在重复工作。** 现用 App 的线程采样同时出现状态栏重绘、菜单刷新和 ICU 正则编译。`NativeText.translate` 对已知中文 UI 文案仍构造反向字典、遍历条目，并对普通文本反复编译参数标记正则。现在已知中文直接保留，参数标记正则复用，普通文本跳过模板正则。独立优化构建的“设置”翻译由 **4,684 µs/次降到 0.98 µs/次**；英文由 1.94 降到 0.58 µs/次。未命中中文文本仍需模板搜索，由 4,311 降到 2,373 µs/次，可继续优化。参数替换、英文回译中文、错误翻译与原始值回归通过。
2. **DNS 查询历史重复格式化答案。** 原来 `Answers` 和响应详情分别调用 `RR.String()`；关闭历史后仍生成字符串再丢弃。现在每条记录格式化一次，关闭历史直接跳过。两个对外数组仍独立，响应内容保持不变。含监控、客户端统计和历史的单条缓存请求分配由 **1,096 B / 37 次降到约 920 B / 30 次**，减少约 **16%**；关闭历史由 1,048 B / 34 次降到 664 B / 18 次，减少约 37%。这是累计分配的减少，不等于常驻内存减少相同比例。
3. **告警采样不再扫描缓存条目。** 每秒采样只读取现有 `OptimisticCounters()` 累计计数，并合并重建后携带的历史计数及策略专用上游计数。原 `Stats()` 在持锁时扫描全部缓存：1 万条约 0.103 ms，10 万条约 1.21 ms，100 万条约 45.7 ms；累计计数读取约 4 ns，零分配，与缓存条目数无关。多个上游的总成本仍随上游池数增长。已验证不改变命中统计和核对分母。
4. **无关接口不再复制、排序客户端列表。** `App.View` 只在 `status` 和 `dns/clients` 需要时取列表。配置应用状态接口在 100 客户端下由 9,736 B/次降到 96 B/次；4,096 客户端下由 360,712 B/次降到 96 B/次。完整状态和客户端接口保持原有字段。

这些变化在当时已通过本地验证，本轮将经完整发布检查后纳入 beta.19；没有替换现用 App。原生翻译微基准的收益不能直接换算为整个 App 的 CPU 降幅。

## 现用进程的只读观察

在当时的使用状态下连续采集 15 秒 CPU 时间增量；没有给现用后台发送压测流量，也没有读取其私有配置。App 线程采样身份为 `1.0.4-beta.16 / build 24`。

| 进程 | 单核 CPU 时间占比 | RSS 起始 → 结束 |
| --- | ---: | ---: |
| RoutePilot 原生 App | 35.4% | 57.6 → 57.7 MiB |
| surge-controller 后台 | 5.1% | 75.3 → 77.2 MiB |

CPU 以一个逻辑核的时间为 100%，不是整机 CPU 百分比。RSS 是进程驻留内存，未包含 WebKit 独立进程。此处是当前使用状态的短样本，不能视为空闲基线或长期内存趋势。线程采样提示状态栏图像重绘、菜单重建和翻译是下一轮原生 A/B 验证的重点；本轮没有改变动画帧率。

## 隔离的局域网负载资源对照

Apple M4 / Mac16,10，10 个逻辑 CPU，16 GiB 内存；Go 1.27.2 darwin/arm64。资源测量使用非 race 构建，每个档位 10 秒；本机其他 App 继续运行，未控制背景负载或热状态。后台上游和策略求值均为内存测试夹具；预热 1 万条决策与响应缓存、100 个客户端、1,000 条查询历史。

测试包含每秒一次管理状态快照、每秒告警采样、每五秒监控与客户端 SQLite 检查点。每秒状态快照比当前界面默认三秒轮询更频繁。CPU 是整个测试进程的 CPU 时间；堆包含测试输入、SQLite 和 Go 测试框架。未覆盖真实 Surge CLI/外部上游、PF、BGP、全部缓存持久化、HTTP JSON 输出、原生 App 或 WebKit，因此不是完整后台的资源预算。

| 目标请求率 | 优化前 CPU | 优化后 CPU | 分配速率前 → 后 | 优化后 GC 后存活堆 |
| --- | ---: | ---: | ---: | ---: |
| 空闲，状态快照开启 | 0.82% | 0.55% | 229 → 229 KiB/s | 18.56 MiB |
| 50 QPS | 1.88% | 1.74% | 304 → 296 KiB/s | 18.58 MiB |
| 200 QPS | 3.59% | 2.70% | 455 → 426 KiB/s | 18.57 MiB |

实际达到约 49.9 和 197.6–198.6 QPS，定时器在系统负载下有少量错过节拍。短测 CPU 对系统调度敏感，上表仅表示本次前后样本；分配字节的变化更可靠。各档位没有查询错误，10 秒内未触发自动 GC，GC 后存活堆没有明显增长。

优化后关闭管理状态快照、保留告警采样和 SQLite 检查点的空闲场景：约 **0.063% 单核 CPU、61 KiB/s 分配、18.55 MiB 存活堆**。这说明管理状态的快照生成仍比告警判定本身更值得继续减少重复工作。

## 高并发定位与候选优化

优化前完整缓存组件通过真实 UDP 回环 socket 连续压测三个档位，每档 10 秒，总计 3,191,767 个请求，零失败、服务端零丢弃；预热后没有外部规则求值或上游交换。

| 并发客户端连接 | QPS | P50 | P99 |
| --- | ---: | ---: | ---: |
| 16 | 122,551 | 0.121 ms | 0.353 ms |
| 64 | 102,344 | 0.318 ms | 4.515 ms |
| 256 | 94,257 | 2.348 ms | 7.629 ms |

回环连接来自同一客户端 IP；独立分流基准另预热 256 个不同 IP，保持客户端决策隔离。高并发锁剖析中约 88.5% 的累计等待落在 `Monitoring.BeginSource` / `FinishSource`；这是多个协程等待时间之和，不是 CPU 占比。CPU 剖析含大量运行时等待/调度帧，不能把这些帧简单解释为业务计算热点。

按局域网场景价值排序，后续候选为：

- **先测原生状态栏重绘与菜单重建。** 翻译已消除一处明确浪费；下一轮应在隔离原生环境对照测图像更新、菜单打开/关闭和闲时唤醒，再决定是否降频。保持健康状态与通知及时更新。
- **复用未变化的监控视图与静态翻译模板。** 密集夹具（256 来源 × 64 策略）一次监控 View 约分配 2.84 MB、耗时 2.05 ms；完整 checkpoint 约 3.03 MB、3.4–3.7 ms。应优先避免不必要的重复生成，在保持真实来源/策略关系和准确分母的前提下缩短锁内复制。实际 100 来源 × 单策略场景远小于此密集夹具。
- **检查全部缓存持久化的检查点成本。** 本轮低速资源对照没有覆盖决策缓存全量保存和开启乐观响应后的响应缓存全量保存。实际后台还需对此单独剖析，才能判断是否值得增加 dirty/revision 跟踪。
- **客户端容量淘汰、监控分片优先级较低。** 4,096 客户端容量满后，每个新 IP 的淘汰扫描约 79–84 µs；既有 IP 记录约 45–46 ns。持续大量新身份时才值得改为有序淘汰结构。监控锁分片会增加维护复杂度，当前常见负载没有证据要求它。

告警判定微基准：默认窗口约 1.0–1.15 µs/次，约 1.7 KiB 分配；一小时窗口约 1.0–1.15 µs/次、约 2.0 KiB 分配，每秒执行一次。窗口长度没有使每次判定成本线性增长。

## 验证与复现

本地 Go race 回归覆盖 `internal/dnsserver`、`internal/notifications`、`internal/app`、`tests/integration`；原生菜单、参数模板、错误和真实 WKWebView UI 检查通过；Go vet、源码构建和完整原生源文件类型检查通过，实际结果见本地日志。原有 WebKit 并发标注警告仍存在。第一次低速资源测试因测试 SQLite 目录权限被拒绝而提前终止，修正为私有目录后前后对照均通过，失败日志保留。当时没有执行发布、安装或现场网络验收；本轮发布结果见 [验收记录](acceptance.md)。

基准文件为 `tests/integration/performance_test.go`、`internal/{cache,dnsserver,notifications,app}/performance_test.go` 和 `tests/performance/localization_benchmark.swift`。持续压测默认跳过，需要显式设置环境变量。旧扫描对照通过 `ROUTEPILOT_SAMPLE_FULL_SCAN=1` 开启；关闭状态快照用 `ROUTEPILOT_RESOURCE_NO_STATUS=1`。

```sh
GOCACHE=/tmp/routepilot-go-build-perf go test ./tests/integration -run '^$' -bench '^BenchmarkDNSProductionPaths$' -benchmem -cpu=1,4,10 -count=2
ROUTEPILOT_STRESS=1 GOCACHE=/tmp/routepilot-go-build-perf go test ./tests/integration -run '^TestDNS(ProductionStress|LANResourceUsage)$' -count=1 -v
GOCACHE=/tmp/routepilot-go-build-perf go test ./internal/cache ./internal/dnsserver ./internal/notifications ./internal/app -run '^$' -bench '^(BenchmarkCacheStatisticsCost|BenchmarkClientStatisticsCapacity|BenchmarkMonitoringSnapshot|BenchmarkAnomalySampling|BenchmarkConfigurationStatusView)$' -benchmem
xcrun swiftc -O -swift-version 5 -module-cache-path .cache/swift-module native/localization.swift tests/performance/localization_benchmark.swift -o /tmp/routepilot-localization-bench
/tmp/routepilot-localization-bench localization/ui-en.json localization/errors-zh.json
```

本地原始日志、结构化结果、CPU / 分配 / 锁剖析和现用进程只读采样保存在 `artifacts/performance/2026-10-13/`，不提交构建产物。该 Go 安装未提供 `go tool pprof` 可执行入口，本轮通过随工具链提供的 `go run cmd/pprof` 读取剖析文件。

本节性能数值来自当时 beta.16 之后工作区的样本，不是包含 Surge 分项缓存与全部告警后的 beta.19 整体资源结论；本轮只确认源码兼容、回归与发布包，不将微基准换算为完整 App 的资源降幅。
