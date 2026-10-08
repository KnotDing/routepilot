const messages={
 'zh-CN':{
  language:'页面语言',settingsTitle:'缓存设置',expire_decisions:'按 TTL 定时过期稳定决策',invalidate_on_read_failure:'Surge 配置读取失败时失效旧决策',invalidate_direct_on_selection_change:'策略组选择变化时失效 DIRECT',invalidate_reject_on_selection_change:'策略组选择变化时失效 REJECT',capacity:'决策缓存容量',response_capacity:'响应缓存容量',workers:'规则求值工作线程',queue:'求值队列容量',timeout_ms:'超时（毫秒）',direct_ttl_seconds:'DIRECT 缓存 TTL（秒）',surge_ttl_seconds:'SURGE / REJECT 缓存 TTL（秒）',unknown_ttl_seconds:'UNKNOWN TTL（秒）',error_ttl_seconds:'错误 TTL（秒）',
  help:'默认长期缓存 DIRECT / SURGE / REJECT，直到确认 Surge 配置或外部资源更新。定时过期启用后才使用其 TTL；UNKNOWN 和求值错误始终按短 TTL 重试。读取失败及策略组选择变化的失效行为可独立控制。缓存仍受容量限制，服务重启、手动清理或分流配置修改会清理缓存。',save:'保存设置',saved:'设置已保存',export:'导出备份',import:'导入 / 恢复',
  entries:'有效条目',cacheCapacity:'缓存容量',hits:'命中次数',misses:'未命中次数',evictions:'容量淘汰',lifecycle:'缓存生命周期',persistent:'DIRECT / SURGE / REJECT：长期保留，直到确认配置更新',timed:'DIRECT / SURGE / REJECT TTL',unknown:'UNKNOWN',errors:'求值错误',seconds:'秒',lastReason:'最近失效原因',lastAt:'最近失效时间',invalidations:'本次运行失效次数',noInvalidation:'本次运行尚未清理',confirmed:'确认 Surge 有效配置或外部资源更新后清理决策。',readFailure:'读取失败时',keep:'保留旧决策',drop:'失效旧决策',selection:'策略组选择变化时',none:'均保留',invalidated:'失效',capacityNote:'长期缓存仍受容量限制，服务重启、手动清理或分流配置修改也会清理。',
  search:'搜索缓存域名',searchPlaceholder:'搜索全部有效缓存的域名',decisionFilter:'缓存决策',all:'全部决策',flushDomain:'清理此域名缓存',flushAll:'清理全部缓存',domainFlushed:'该域名缓存已清理',allFlushed:'全部决策缓存已清理',domain:'域名',type:'类型',decision:'决策',confidence:'依据',client:'客户端',remaining:'剩余时间',reason:'原因',certain:'已确认',configured:'按配置采用',uncertain:'尚未确认',untilUpdate:'直到配置更新',noExpiry:'不按时间过期',empty:'没有符合条件的有效缓存',loading:'正在加载缓存…',total:'共',items:'条',page:'页',pageSize:'每页',previous:'上一页',next:'下一页'
 },
 en:{
  language:'Page language',settingsTitle:'Cache settings',expire_decisions:'Expire stable decisions by TTL',invalidate_on_read_failure:'Invalidate on config read failure',invalidate_direct_on_selection_change:'Invalidate DIRECT on selection change',invalidate_reject_on_selection_change:'Invalidate REJECT on selection change',capacity:'Decision cache capacity',response_capacity:'DNS response cache capacity',workers:'Rule evaluation workers',queue:'Evaluation queue capacity',timeout_ms:'Timeout (ms)',direct_ttl_seconds:'DIRECT TTL (seconds)',surge_ttl_seconds:'SURGE / REJECT TTL (seconds)',unknown_ttl_seconds:'UNKNOWN TTL (seconds)',error_ttl_seconds:'Error TTL (seconds)',
  help:'DIRECT, SURGE and REJECT persist until a confirmed Surge configuration or resource update by default. Their TTLs apply only when timed expiry is enabled. UNKNOWN and evaluation errors always use short retry TTLs. Invalidation on failed reads and policy-group selection changes can be controlled independently. Capacity eviction, service restarts, manual clearing and local policy changes may also remove entries.',save:'Save settings',saved:'Settings saved',export:'Export backup',import:'Import / restore',
  entries:'Valid entries',cacheCapacity:'Cache capacity',hits:'Cache hits',misses:'Cache misses',evictions:'Capacity evictions',lifecycle:'Cache lifecycle',persistent:'DIRECT / SURGE / REJECT: retained until a confirmed configuration update',timed:'DIRECT / SURGE / REJECT TTL',unknown:'UNKNOWN',errors:'Evaluation errors',seconds:'seconds',lastReason:'Last invalidation reason',lastAt:'Last invalidation time',invalidations:'Invalidations this run',noInvalidation:'No invalidations this run',confirmed:'Confirmed Surge configuration or resource updates clear decisions.',readFailure:'Failed configuration reads',keep:'Keep cached decisions',drop:'Invalidate cached decisions',selection:'Policy-group selection changes',none:'Keep both',invalidated:'Invalidated',capacityNote:'Capacity eviction, service restarts, manual clearing and local policy changes may also remove persistent entries.',
  search:'Search cached domains',searchPlaceholder:'Search all valid cached domains',decisionFilter:'Cached decision',all:'All decisions',flushDomain:'Clear domain cache',flushAll:'Clear all decisions',domainFlushed:'Domain cache cleared',allFlushed:'Decision cache cleared',domain:'Domain',type:'Type',decision:'Decision',confidence:'Confidence',client:'Client',remaining:'Remaining lifetime',reason:'Reason',certain:'Confirmed',configured:'Configured',uncertain:'Uncertain',untilUpdate:'Until configuration update',noExpiry:'No timed expiry',empty:'No matching valid entries',loading:'Loading cache…',total:'Total',items:'entries',page:'Page',pageSize:'Per page',previous:'Previous',next:'Next'
 }
};
export const cacheText=locale=>key=>messages[locale]?.[key]||messages['zh-CN'][key]||key;

export function cacheInvalidationReason(locale,reason){
 if(locale!=="en"||!reason)return reason;
 const translated={
  'Surge 配置已首次确认':'Surge configuration confirmed for the first time',
  'Surge 有效配置或外部资源已确认更新':'Surge configuration or resource update confirmed',
  'Surge 配置读取失败；按配置失效旧决策':'Surge configuration read failed; cached decisions invalidated by configuration',
  '全局模式或域名覆盖已更新':'Global mode or domain override updated',
  '用户手动清理全部决策缓存':'All decisions cleared by the user',
  '按域名清理缓存':'Domain cache cleared',
  '缓存已清理':'Cache cleared'
 };
 if(translated[reason])return translated[reason];
 const selection=reason.match(/^Surge 策略组选择已变化；按配置仅失效 (.+)$/);
 return selection?'Surge policy-group selection changed; invalidate '+selection[1]:reason;
}
