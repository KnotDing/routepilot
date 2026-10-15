const messages={
 'zh-CN':{
  surgeUnsupported:'当前后端未声明支持 Surge 响应缓存，请升级后端后再设置 Surge。',
  optimistic_idle_refresh:'乐观决策缓存空闲自动刷新',
  optimistic_response_idle_refresh:'乐观查询缓存空闲自动刷新',
  optimistic_idle_refresh_hint:'关闭后仍会在缓存命中时按需刷新，只停止空闲时自动刷新。',
  optimistic_response_idle_refresh_hint:'关闭后仍会在缓存命中时按需查询上游，只停止空闲时自动刷新。',
  optimisticHelpSummary:'乐观缓存已开启：确认配置变化后保留旧决策；命中时立即返回并按需刷新。空闲自动刷新由独立开关控制，开启时会在至少 1 秒无 DNS 请求且求值队列空闲后逐条刷新；失败或 UNKNOWN 时保留旧值并重试。',
  language:'页面语言',settingsTitle:'缓存设置',optimistic:'乐观决策缓存',optimistic_tooltip:'确认配置或资源更新后保留旧决策，并在命中或空闲时刷新。',optimistic_responses:'乐观查询缓存',optimistic_responses_tooltip:'TTL 到期后仍保留查询响应；命中时立即返回旧响应并按需刷新。',optimistic_idle_refresh:'决策缓存空闲刷新',optimistic_idle_refresh_tooltip:'启用后，在 DNS 空闲且求值队列空闲时自动刷新待更新决策；关闭不影响命中时的按需刷新。',optimistic_response_idle_refresh:'查询缓存空闲刷新',optimistic_response_idle_refresh_tooltip:'启用后，在 DNS 空闲时自动刷新待更新响应；关闭不影响命中时的按需查询上游。',optimistic_idle_refresh_hint:'关闭后仍会在缓存命中时按需刷新，只停止空闲时自动刷新。',optimistic_response_idle_refresh_hint:'关闭后仍会在缓存命中时按需查询上游，只停止空闲时自动刷新。',optimisticHelpSummary:'乐观缓存已开启：确认配置变化后保留旧决策；命中时立即返回并按需刷新。空闲自动刷新由独立开关控制，开启时会在至少 1 秒无 DNS 请求且求值队列空闲后逐条刷新；失败或 UNKNOWN 时保留旧值并重试。',
  optimistic_responses_tooltip:'乐观查询缓存已开启：过期响应仍可立即返回，后台刷新成功后替换；响应会持久化并在重启后恢复。',
  language:'页面语言',settingsTitle:'缓存设置',optimisticResponsesHelp:'乐观响应缓存已开启：TTL 到期后仍保留上游响应，命中时先返回旧响应，再在后台查询上游；成功后替换，失败时保留旧响应并稍后重试。确认 Surge 配置或外部资源更新后也会按需刷新。响应缓存每 5 秒保存，正常退出前保存最终值；重启后恢复包括已过期的响应，命中时后台刷新。过期响应返回 30 秒 TTL，不修改缓存原始过期时间；手动清理、容量淘汰或查询配置变化仍会移除缓存。关闭并应用此选项时，仅清理已过期响应，保留 TTL 内的响应和原始过期时间；关闭后不做响应缓存持久化。',refreshPending:'待刷新决策',responsePending:'待刷新响应',optimisticHelp:'乐观缓存已开启：确认 Surge 配置或外部资源更新后保留旧决策；下次命中时立即响应并在后台刷新；空闲时也会自动刷新待刷新决策，成功后替换。至少 1 秒没有 DNS 请求且求值队列空闲时，每次刷新一条、每秒最多启动 4 条；新请求优先，取消空闲刷新。刷新失败或无法确认决策时保留旧值并稍后重试。',expire_decisions:'决策 TTL 过期',expire_decisions_tooltip:'启用后，稳定决策按配置的 TTL 自动过期；关闭时直到确认配置更新才失效。',invalidate_on_read_failure:'配置读取失败时失效决策',invalidate_on_read_failure_tooltip:'Surge 配置读取失败时移除已有决策；关闭则保留旧决策。',invalidate_direct_on_selection_change:'DIRECT 选择变化时失效',invalidate_direct_on_selection_change_tooltip:'策略组选择变化时失效 DIRECT 决策。',invalidate_reject_on_selection_change:'REJECT 选择变化时失效',invalidate_reject_on_selection_change_tooltip:'策略组选择变化时失效 REJECT 决策。',capacity:'决策缓存容量',response_capacity:'响应缓存容量',workers:'规则求值工作线程',queue:'求值队列容量',timeout_ms:'超时（毫秒）',direct_ttl_seconds:'DIRECT 缓存 TTL（秒）',surge_ttl_seconds:'SURGE / REJECT 缓存 TTL（秒）',unknown_ttl_seconds:'UNKNOWN TTL（秒）',error_ttl_seconds:'错误 TTL（秒）',
  help:'默认长期缓存 DIRECT / SURGE / REJECT，确认 Surge 配置或外部资源更新时失效。开启乐观缓存后改为先用旧决策响应，命中时或空闲时后台刷新；成功后替换，失败或 UNKNOWN 时保留旧值。空闲刷新逐条限速，实时查询优先。决策缓存的待刷新标记可跨重启保留。响应缓存可独立选择乐观刷新，开启后每 5 秒及正常退出时保存到 SQLite，重启后恢复包括过期响应并按需后台刷新，保留原始过期时间。定时过期启用后才使用其 TTL；UNKNOWN 和求值错误始终按短 TTL 重试。读取失败及策略组选择变化的失效行为可独立控制。稳定决策每 5 秒保存到 SQLite，正常停止时保存最终值；重启后确认 Surge 配置再恢复，定时条目保留原过期时间。UNKNOWN 和求值错误不跨重启保留；手动清理、配置变化和容量限制仍然生效。',save:'保存设置',saved:'设置已保存',export:'导出备份',import:'导入 / 恢复',
  entries:'有效条目',cacheCapacity:'缓存容量',hits:'命中次数',misses:'未命中次数',evictions:'容量淘汰',lifecycle:'缓存生命周期',persistent:'DIRECT / SURGE / REJECT：长期保留，直到确认配置更新',timed:'DIRECT / SURGE / REJECT TTL',unknown:'UNKNOWN',errors:'求值错误',seconds:'秒',lastReason:'最近失效原因',lastAt:'最近失效时间',invalidations:'本次运行失效次数',noInvalidation:'本次运行尚未清理',confirmed:'确认 Surge 有效配置或外部资源更新后清理决策。',readFailure:'读取失败时',keep:'保留旧决策',drop:'失效旧决策',selection:'策略组选择变化时',none:'均保留',invalidated:'失效',capacityNote:'稳定决策每 5 秒保存，正常停止时保存最终值；重启后确认配置再恢复。强制结束可能丢失最近尚未保存的条目。手动清理、分流配置修改和容量限制仍然生效。',restorePending:'已读取持久化缓存，正在等待确认 Surge 配置；确认前不使用旧决策。',
  search:'搜索缓存域名',searchPlaceholder:'搜索全部有效缓存的域名',decisionFilter:'缓存决策',all:'全部决策',flushDomain:'清理此域名缓存',flushAll:'清理全部缓存',domainFlushed:'该域名缓存已清理',allFlushed:'全部决策缓存已清理',domain:'域名',type:'类型',decision:'决策',confidence:'依据',client:'客户端',remaining:'剩余时间',reason:'原因',certain:'已确认',configured:'按配置采用',uncertain:'尚未确认',untilUpdate:'直到配置更新',noExpiry:'不按时间过期',empty:'没有符合条件的有效缓存',loading:'正在加载缓存…',total:'共',items:'条',page:'页',pageSize:'每页',previous:'上一页',next:'下一页'
 },
 en:{
  surgeUnsupported:'This backend has not declared Surge response cache support. Upgrade the backend to configure Surge.',
  optimistic:'Optimistic decision cache',
  optimistic_tooltip:'Retain the previous decision after a confirmed configuration or resource update; refresh on cache hits or while idle.',
  optimistic_responses:'Optimistic query cache',
  optimistic_responses_tooltip:'Retain query responses past TTL expiry; return them immediately on hits and refresh on demand.',
  optimistic_idle_refresh:'Decision cache idle refresh',
  optimistic_idle_refresh_tooltip:'Automatically refresh pending decisions while DNS and evaluation are idle. Disabling this does not affect on-demand refresh on hits.',
  optimistic_response_idle_refresh:'Query cache idle refresh',
  optimistic_response_idle_refresh_tooltip:'Automatically refresh pending responses while DNS is idle. Disabling this does not affect on-demand upstream queries on hits.',
  expire_decisions:'Expire decisions by TTL',
  expire_decisions_tooltip:'When enabled, stable decisions expire according to their configured TTL; otherwise they remain until a confirmed configuration update.',
  invalidate_on_read_failure:'Invalidate decisions on config read failure',
  invalidate_on_read_failure_tooltip:'Remove cached decisions when reading Surge configuration fails. When disabled, retain the previous decisions.',
  invalidate_direct_on_selection_change:'Invalidate DIRECT on selection change',
  invalidate_direct_on_selection_change_tooltip:'Invalidate DIRECT decisions when a policy-group selection changes.',
  invalidate_reject_on_selection_change:'Invalidate REJECT on selection change',
  invalidate_reject_on_selection_change_tooltip:'Invalidate REJECT decisions when a policy-group selection changes.',
  optimistic_idle_refresh_hint:'On-demand refresh on cache hits remains enabled; only automatic idle refresh is stopped.',
  optimistic_response_idle_refresh_hint:'On-demand upstream refresh on cache hits remains enabled; only automatic idle refresh is stopped.',
  optimisticHelpSummary:'Optimistic caching is enabled: after a confirmed configuration update, the previous decision is retained. A cache hit returns it immediately and refreshes on demand. Idle auto-refresh is controlled separately; when enabled, stale entries refresh one at a time after at least one second without DNS requests and with evaluation workers idle. Failures or UNKNOWN retain the old value for retry.',
  language:'Page language',settingsTitle:'Cache settings',optimistic:'Optimistic decision cache: respond first, refresh in the background',optimistic_responses:'Optimistic response cache: respond first, refresh in the background',optimisticResponsesHelp:'Optimistic response caching retains upstream responses past TTL expiry. A hit returns the previous response immediately and refreshes upstream in the background; success replaces it, while failure retains it for a later retry. Confirmed Surge configuration or resource updates also trigger refresh on demand. Responses are saved every 5 seconds and on normal shutdown; restart restores expired responses too. Expired responses use a 30-second client TTL without changing their original cache deadline. Manual clearing, capacity eviction and query configuration changes still remove entries. Disabling and applying this option removes only expired responses, retaining unexpired responses with their original deadlines. Response persistence is disabled while the option is off.',refreshPending:'Decisions awaiting refresh',responsePending:'Responses awaiting refresh',optimisticHelp:'Optimistic caching is enabled: confirmed Surge configuration or resource updates retain previous decisions. The next hit responds immediately and refreshes in the background. Pending decisions also refresh automatically after at least one second without DNS requests and when evaluation workers are idle: one at a time, at most four starts per second. Incoming queries cancel idle refreshes and take priority. Successful evaluations replace the cached decision; failures or uncertain results retain it for a later retry.',expire_decisions:'Expire stable decisions by TTL',invalidate_on_read_failure:'Invalidate on config read failure',invalidate_direct_on_selection_change:'Invalidate DIRECT on selection change',invalidate_reject_on_selection_change:'Invalidate REJECT on selection change',capacity:'Decision cache capacity',response_capacity:'DNS response cache capacity',workers:'Rule evaluation workers',queue:'Evaluation queue capacity',timeout_ms:'Timeout (ms)',direct_ttl_seconds:'DIRECT TTL (seconds)',surge_ttl_seconds:'SURGE / REJECT TTL (seconds)',unknown_ttl_seconds:'UNKNOWN TTL (seconds)',error_ttl_seconds:'Error TTL (seconds)',
  help:'By default, DIRECT, SURGE and REJECT persist until a confirmed Surge configuration or resource update. Optimistic caching responds with the previous decision and refreshes on demand or during idle time. Idle refreshes are rate-limited and yield to incoming queries. Successful evaluations replace it; failures or UNKNOWN retain it. Pending decision refresh markers survive restart. Optimistic response caching is saved to SQLite every 5 seconds and on normal shutdown. After restart, responses including expired entries are restored for refresh on demand, preserving their original deadlines. Decision TTLs apply only when timed expiry is enabled. UNKNOWN and evaluation errors always use short retry TTLs. Invalidation on failed reads and policy-group selection changes can be controlled independently. Stable decisions are saved to SQLite every 5 seconds and on normal shutdown. After restart, they are restored once the Surge configuration is confirmed, with original expiry deadlines. UNKNOWN and evaluation errors are not restored. Manual clearing, configuration changes and capacity limits still apply.',save:'Save settings',saved:'Settings saved',export:'Export backup',import:'Import / restore',
  entries:'Valid entries',cacheCapacity:'Cache capacity',hits:'Cache hits',misses:'Cache misses',evictions:'Capacity evictions',lifecycle:'Cache lifecycle',persistent:'DIRECT / SURGE / REJECT: retained until a confirmed configuration update',timed:'DIRECT / SURGE / REJECT TTL',unknown:'UNKNOWN',errors:'Evaluation errors',seconds:'seconds',lastReason:'Last invalidation reason',lastAt:'Last invalidation time',invalidations:'Invalidations this run',noInvalidation:'No invalidations this run',confirmed:'Confirmed Surge configuration or resource updates clear decisions.',readFailure:'Failed configuration reads',keep:'Keep cached decisions',drop:'Invalidate cached decisions',selection:'Policy-group selection changes',none:'Keep both',invalidated:'Invalidated',capacityNote:'Stable decisions are saved every 5 seconds and on normal shutdown, then restored after configuration confirmation. Forced termination may lose unsaved entries. Manual clearing, policy changes and capacity limits still apply.',restorePending:'Saved cache loaded; waiting to confirm the Surge configuration before reusing decisions.',
  search:'Search cached domains',searchPlaceholder:'Search all valid cached domains',decisionFilter:'Cached decision',all:'All decisions',flushDomain:'Clear domain cache',flushAll:'Clear all decisions',domainFlushed:'Domain cache cleared',allFlushed:'Decision cache cleared',domain:'Domain',type:'Type',decision:'Decision',confidence:'Confidence',client:'Client',remaining:'Remaining lifetime',reason:'Reason',certain:'Confirmed',configured:'Configured',uncertain:'Uncertain',untilUpdate:'Until configuration update',noExpiry:'No timed expiry',empty:'No matching valid entries',loading:'Loading cache…',total:'Total',items:'entries',page:'Page',pageSize:'Per page',previous:'Previous',next:'Next'
 }
};
Object.assign(messages['zh-CN'],{
 decisionReliability:'乐观决策缓存可靠性',responseReliability:'乐观响应缓存可靠性',optimisticHits:'过期 / 待刷新命中请求',optimisticCompared:'已核对请求',optimisticMismatches:'不一致请求',optimisticMismatchRate:'不一致率',optimisticUnverified:'未核对请求',
 decisionComparisonHelp:'比较分流决策（DIRECT / SURGE / REJECT）和最终策略；忽略规则说明、时间和缓存来源等元数据。',
 responseComparisonHelp:'比较 DNS 返回码及各区段记录内容；忽略请求 ID、TTL、记录排列、域名大小写和 EDNS 传输信息。',
 optimisticStatisticsHelp:'按实际命中旧缓存的请求数计数，一次刷新可以核对多次命中。不一致率 = 不一致请求 / 已核对请求；无样本时显示 —。刷新失败、UNKNOWN、取消或清理 / 淘汰前尚未成功刷新的请求列为未核对，不算一致。空闲刷新未命中的条目不增加样本。统计会跨服务重启和自动缓存重建保留，直到手动清理缓存。',
 reliabilityHelpLabel:'可靠性统计说明',
 optimistic:'乐观决策缓存',
 optimistic_responses:'乐观查询缓存',
 optimistic_idle_refresh:'决策缓存空闲刷新',
 optimistic_response_idle_refresh:'查询缓存空闲刷新',
 expire_decisions:'决策 TTL 过期',
 invalidate_on_read_failure:'配置读取失败时失效决策',
 invalidate_direct_on_selection_change:'DIRECT 选择变化时失效',
 invalidate_reject_on_selection_change:'REJECT 选择变化时失效'
});
Object.assign(messages.en,{
 decisionReliability:'Optimistic decision cache reliability',responseReliability:'Optimistic response cache reliability',optimisticHits:'Stale requests served',optimisticCompared:'Verified requests',optimisticMismatches:'Mismatched requests',optimisticMismatchRate:'Mismatch rate',optimisticUnverified:'Unverified requests',
 decisionComparisonHelp:'Compares the routing decision (DIRECT / SURGE / REJECT) and final policy. Rule explanations, timestamps and cache source metadata are ignored.',
 responseComparisonHelp:'Compares DNS response codes and record contents in each section. IDs, TTLs, record order, domain-name case and EDNS transport metadata are ignored.',
 optimisticStatisticsHelp:'Counts requests served from stale cache, not refresh jobs. One refresh can verify multiple hits. Mismatch rate = mismatched / verified requests; no samples shows —. Failed, UNKNOWN or cancelled refreshes and requests whose entries were cleared or evicted before verification remain unverified, not matching. Idle refreshes without stale hits add no samples. Counters persist across service restarts and automatic cache recreation until the cache is manually cleared.',
 reliabilityHelpLabel:'Reliability statistics help',
 optimistic:'Optimistic decision cache',
 optimistic_responses:'Optimistic query cache',
 optimistic_idle_refresh:'Decision cache idle refresh',
 optimistic_response_idle_refresh:'Query cache idle refresh',
 expire_decisions:'Expire decisions by TTL',
 invalidate_on_read_failure:'Invalidate decisions on config read failure',
 invalidate_direct_on_selection_change:'Invalidate DIRECT on selection change',
 invalidate_reject_on_selection_change:'Invalidate REJECT on selection change'
});
Object.assign(messages['zh-CN'],{
 groupStatisticsHelp:'条目为当前缓存快照，普通命中、未命中与淘汰随当前缓存实例计数；分项未命中只统计已获得结果的前台查询，不能归属的失败仅保留在总计。乐观可靠性按实际返回的旧值归属，跨重启保留；升级前缺失的分项不会倒填。超过 1024 个上游统计身份后合并为其他上游。',
 decisionMaster:'乐观决策缓存',responseMaster:'乐观响应缓存',
 decisionPolicies:'按决策设置缓存',upstreamPolicies:'按上游设置缓存',optimisticEnabled:'允许乐观缓存',
 selectiveHelp:'每项开关只控制乐观缓存，保存并应用后生效。关闭后仍保留有效的普通缓存，配置失效或 TTL 到期后等待重新查询。UNKNOWN 沿用短期普通缓存，不提供乐观缓存开关。',
 selectiveUnsupported:'当前后端不支持按决策和上游设置缓存，请先升级该后端。',
 originalUpstreamHelp:'上游按协议、地址及 DoH URL / TLS 名称识别；全局、策略主上游和备用中相同的上游共用开关。保留原始目标时，已观察到的上游也可单独设置；未设置的上游沿用总开关。',
 decisionBreakdown:'各决策缓存统计',upstreamBreakdown:'各上游缓存统计',upstream:'上游',noGroupSamples:'暂无分项统计',otherUpstreams:'其他上游（统计容量分组）'
});
Object.assign(messages.en,{
 groupStatisticsHelp:'Entries are a current snapshot; ordinary hits, misses and evictions belong to the current cache instance. Per-item misses cover completed foreground lookups with an identifiable result; unattributed failures remain aggregate-only. Optimistic reliability is attributed to the old value served and persists across restarts. Earlier totals without item identities are not backfilled. Beyond 1024 upstream identities, statistics are grouped as other upstreams.',
 decisionMaster:'Optimistic decision cache',responseMaster:'Optimistic response cache',
 decisionPolicies:'Cache by decision',upstreamPolicies:'Cache by upstream',optimisticEnabled:'Allow optimistic caching',
 selectiveHelp:'Each item switch controls only optimistic caching and takes effect after saving and applying. Disabling it preserves valid ordinary cache entries and waits for a new lookup after invalidation or TTL expiry. UNKNOWN keeps short-lived ordinary caching and has no optimistic switch.',
 selectiveUnsupported:'This backend does not support cache policies by decision and upstream. Upgrade this backend first.',
 originalUpstreamHelp:'Upstreams are identified by protocol, address and DoH URL / TLS name. The same upstream shares its switches across global, policy and backup pools. Observed original-destination upstreams can also be configured; unconfigured upstreams inherit the global switch.',
 decisionBreakdown:'Cache statistics by decision',upstreamBreakdown:'Cache statistics by upstream',upstream:'Upstream',noGroupSamples:'No per-item samples yet',otherUpstreams:'Other upstreams (statistics capacity group)'
});
Object.assign(messages['zh-CN'],{
 decisionCacheMaster:'决策缓存',responseCacheMaster:'响应缓存',cacheEnabled:'缓存此项',
 decisions_enabled_tooltip:'缓存规则求值结果；关闭后每次请求重新求值，并停用乐观决策缓存。',
 responses_enabled_tooltip:'按 DNS TTL 缓存上游响应；关闭后每次请求查询上游，并停用乐观响应缓存。',
 enableCacheFirst:'请先启用对应的缓存。',
 cacheControlsUnsupported:'当前后端不支持关闭普通缓存，请升级后端后设置决策缓存与响应缓存。',
 cacheControlsHelp:'普通缓存与乐观缓存分别设置，保存并应用后生效。关闭某类缓存或某个分项时，对应乐观缓存同时关闭且不可启用；该项不再复用、写入或恢复缓存。仅关闭乐观缓存时仍保留有效普通缓存。UNKNOWN 可关闭普通缓存，不提供乐观缓存。'
});
Object.assign(messages.en,{
 decisionCacheMaster:'Decision cache',responseCacheMaster:'Response cache',cacheEnabled:'Cache this item',
 decisions_enabled_tooltip:'Cache rule evaluation results. Disabling this evaluates every request and disables optimistic decision caching.',
 responses_enabled_tooltip:'Cache upstream responses within their DNS TTL. Disabling this queries upstreams on every request and disables optimistic response caching.',
 enableCacheFirst:'Enable the corresponding cache first.',
 cacheControlsUnsupported:'This backend cannot disable ordinary caching. Upgrade it to configure decision and response caching.',
 cacheControlsHelp:'Configure ordinary and optimistic caching separately, then save and apply. Disabling a cache or item also disables its optimistic cache and prevents enabling it. Disabled items are not reused, stored or restored. Disabling only optimistic caching retains valid ordinary entries. UNKNOWN supports ordinary caching only.'
});
export const cacheText=locale=>key=>messages[locale]?.[key==='optimisticHelp'?'optimisticHelpSummary':key]||messages['zh-CN'][key==='optimisticHelp'?'optimisticHelpSummary':key]||key;

export function cacheInvalidationReason(locale,reason){
 if(locale!=="en"||!reason)return reason;
 const translated={
  'Surge 配置已首次确认':'Surge configuration confirmed for the first time',
  'Surge 有效配置或外部资源已确认更新':'Surge configuration or resource update confirmed',
  'Surge 配置读取失败；按配置失效旧决策':'Surge configuration read failed; cached decisions invalidated by configuration',
  '全局模式或域名覆盖已更新':'Global mode or domain override updated',
  '用户手动清理全部决策缓存':'All decisions cleared by the user',
  '按域名清理缓存':'Domain cache cleared',
  '缓存已清理':'Cache cleared',
  '本地 DNS 条目已更新':'Local DNS entries updated'
 };
 if(translated[reason])return translated[reason];
 if(reason.includes('；乐观'))return reason.split('；').map(part=>translated[part]||({'乐观缓存保留旧决策，按需后台刷新':'Optimistic decisions retained for background refresh on demand','乐观响应缓存保留旧响应，按需后台刷新':'Optimistic responses retained for background refresh on demand'}[part]||part)).join('; ');
 const selection=reason.match(/^Surge 策略组选择已变化；按配置仅失效 (.+)$/);
 return selection?'Surge policy-group selection changed; invalidate '+selection[1]:reason;
}
