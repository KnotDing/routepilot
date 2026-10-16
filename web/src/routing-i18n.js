const messages={
 'zh-CN':{
  autoNextHopChanged:'Surge VM IPv4 地址已变化，请更新网关静态路由后重新勾选确认并保存。',
  language:'页面语言',configuration:'路由配置',mode:'路由模式',fakeBackend:'Fake-IP 路由方式',dnsBackend:'DNS 拦截路由方式',
  bgpLimit:'自动发布与撤销路由；需要可用的 BGP 邻居。',staticLimit:'网关路由由外部管理，停止服务后仍保留。',hybridLimit:'两类路由分别选择后端；静态路由由外部维护。',
  fakeConfirmed:'Fake-IP 路由已在网关配置',staticDNS:'启用静态 DNS 拦截',dnsConfirmAction:'确认网关路由已配置',dnsConfirmed:'✓ 网关路由已确认',staticRisk:'静态 DNS 路由不会随服务停止撤销。',
  dnsConfirmationHint:'点击按钮后立即保存确认；此项仅代表用户确认，不会自动配置或验证网关路由。',dnsConfirmationSaved:'DNS 拦截路由确认已保存',dnsConfigurationChanged:'DNS 拦截路由配置已变更，请刷新后重新确认。',
  save:'保存路由设置',saved:'路由设置已保存',confirmReset:'路由变更后，请重新确认网关配置。',status:'路由状态',fakeRoute:'Fake-IP 路由',dnsRoute:'DNS 拦截路由',engine:'BGP 引擎',running:'运行中',disabled:'未启用',ready:'就绪',notReady:'未就绪',confirmed:'用户已确认',pending:'待确认',unused:'未启用',
  prefix:'网段',nextHop:'下一跳',kind:'用途',backend:'管理方式',state:'声明 / 发布状态',installed:'安装状态',connectivity:'本机探测',static:'外部静态',bgp:'BGP',externalUnknown:'未知（外部）',rib:'已加入本地 RIB',noRib:'未加入本地 RIB',checkedAt:'探测时间',success:'TCP 可达',failure:'TCP 不可达',empty:'暂无路由',
  probe:'本机连通性检查',probeScope:'仅检查本机 TCP，不验证网关路由安装。',fakePrefix:'Fake-IP 前缀',choose:'请选择',target:'Surge 分配的 Fake-IP',port:'TCP 端口',check:'检查连接',dnsKind:'DNS 拦截',manual:'手动路由',
  'external-unconfirmed':'待外部配置','external-confirmed':'用户已声明配置','external-missing-next-hop':'缺少下一跳','inactive':'未发布','announced':'已发布','ribState':'本地 RIB','withdrawn':'已撤销'
 },
 en:{
  autoNextHopChanged:'The Surge VM IPv4 address changed. Update the static routes on the gateway, then confirm again and save.',
  language:'Page language',configuration:'Routing configuration',mode:'Routing mode',fakeBackend:'Fake-IP routing method',dnsBackend:'DNS interception method',
  bgpLimit:'Routes are announced and withdrawn automatically; an available BGP neighbor is required.',staticLimit:'Gateway routes are managed externally and remain after the service stops.',hybridLimit:'Choose a backend for each route type; static routes are maintained externally.',
  fakeConfirmed:'Fake-IP routes configured on the gateway',staticDNS:'Enable static DNS interception',dnsConfirmAction:'Confirm gateway routes are configured',dnsConfirmed:'✓ Gateway routes confirmed',staticRisk:'Static DNS routes remain when the service stops.',
  dnsConfirmationHint:'Click the button to save confirmation immediately. This is a user declaration; gateway routes are not automatically configured or verified.',dnsConfirmationSaved:'DNS interception route confirmation saved',dnsConfigurationChanged:'DNS interception routing configuration changed. Refresh and confirm again.',
  save:'Save routing settings',saved:'Routing settings saved',confirmReset:'Confirm the gateway configuration again after route changes.',status:'Route status',fakeRoute:'Fake-IP routing',dnsRoute:'DNS interception',engine:'BGP engine',running:'Running',disabled:'Disabled',ready:'Ready',notReady:'Not ready',confirmed:'User confirmed',pending:'Pending confirmation',unused:'Disabled',
  prefix:'Prefix',nextHop:'Next hop',kind:'Purpose',backend:'Managed by',state:'Declaration / announcement',installed:'Installation',connectivity:'Local probe',static:'External static',bgp:'BGP',externalUnknown:'Unknown (external)',rib:'In local RIB',noRib:'Not in local RIB',checkedAt:'Checked at',success:'TCP reachable',failure:'TCP unreachable',empty:'No routes',
  probe:'Local connectivity check',probeScope:'Checks local TCP connectivity only, without verifying gateway route installation.',fakePrefix:'Fake-IP prefix',choose:'Choose a prefix',target:'Fake-IP assigned by Surge',port:'TCP port',check:'Check connection',dnsKind:'DNS interception',manual:'Manual route',
  'external-unconfirmed':'Awaiting external configuration','external-confirmed':'User declared configuration','external-missing-next-hop':'Missing next hop','inactive':'Not announced','announced':'Announced','ribState':'Local RIB','withdrawn':'Withdrawn'
 }
};
export const routingText=locale=>key=>messages[locale]?.[key]||messages['zh-CN'][key]||key;
