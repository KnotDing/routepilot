const messages={
 'zh-CN':{
  autoNextHopChanged:'Surge VM IPv4 地址已变化，请更新网关静态路由后重新勾选确认并保存。',
  language:'页面语言',configuration:'路由配置',mode:'路由模式',fakeBackend:'Fake-IP 路由方式',dnsBackend:'DNS 拦截路由方式',
  bgpLimit:'自动发布与撤销路由；需要可用的 BGP 邻居。',staticLimit:'网关路由由外部管理，停止服务后仍保留。',hybridLimit:'两类路由分别选择后端；静态路由由外部维护。',apiLimit:'通过路由器管理 API 自动发布与撤销路由；需要路由器可达且凭据有效。',
  fakeConfirmed:'Fake-IP 路由已在网关配置',staticDNS:'启用静态 DNS 拦截',dnsConfirmAction:'确认网关路由已配置',dnsConfirmed:'✓ 网关路由已确认',staticRisk:'静态 DNS 路由不会随服务停止撤销。',
  dnsConfirmationHint:'点击按钮后立即保存确认；此项仅代表用户确认，不会自动配置或验证网关路由。',dnsConfirmationSaved:'DNS 拦截路由确认已保存',dnsConfigurationChanged:'DNS 拦截路由配置已变更，请刷新后重新确认。',
  save:'保存路由设置',saved:'路由设置已保存',confirmReset:'路由变更后，请重新确认网关配置。',status:'路由状态',fakeRoute:'Fake-IP 路由',dnsRoute:'DNS 拦截路由',engine:'BGP 引擎',apiEngine:'路由器 API',running:'运行中',disabled:'未启用',ready:'就绪',notReady:'未就绪',confirmed:'用户已确认',pending:'待确认',unused:'未启用',apiRunning:'已连接',apiError:'异常',
  prefix:'网段',nextHop:'下一跳',kind:'用途',backend:'管理方式',state:'声明 / 发布状态',installed:'安装状态',connectivity:'本机探测',static:'外部静态',bgp:'BGP',api:'API',externalUnknown:'未知（外部）',rib:'已加入本地 RIB',noRib:'未加入本地 RIB',checkedAt:'探测时间',success:'TCP 可达',failure:'TCP 不可达',empty:'暂无路由',
  probe:'本机连通性检查',probeScope:'仅检查本机 TCP，不验证网关路由安装。',fakePrefix:'Fake-IP 前缀',choose:'请选择',target:'Surge 分配的 Fake-IP',port:'TCP 端口',check:'检查连接',dnsKind:'DNS 拦截',manual:'手动路由',
  'external-unconfirmed':'待外部配置','external-confirmed':'用户已声明配置','external-missing-next-hop':'缺少下一跳','inactive':'未发布','announced':'已发布','ribState':'本地 RIB','withdrawn':'已撤销','api-published':'已发布到路由器','api-pending':'待发布',
  apiSectionTitle:'路由器 API',preset:'API 预设',customPreset:'自定义',baseURL:'路由器地址',baseURLHint:'路由器管理接口的 HTTP(S) 地址。',authMode:'认证方式',authNone:'无认证',authHeader:'请求头密钥',authUbus:'OpenWrt ubus 会话',
  username:'用户名',password:'密码（留空保留）',secretConfigured:'已保存；留空保留，勾选下方选项可清除。',headerName:'认证请求头名称',headerValue:'API 密钥（留空保留）',verifyTLS:'验证 HTTPS 证书（自签证书可关闭）',clearPassword:'清除已保存的密码',clearHeader:'清除已保存的 API 密钥',
  openwrtHint:'通过 OpenWrt 的 ubus JSON-RPC 接口写入 network 配置的命名路由段，提交并重载生效；需要路由器管理员账号。适配 OpenWrt 18.06 及以上。',
  unifiHint:'使用 UniFi OS 的 Integration API（/proxy/network/integration/v1/routes）配合 API 密钥；仅适用于提供该接口的 UniFi OS 网关，密钥在网关的系统设置中生成。',
  customHint:'自定义模板按顺序渲染变量后发送请求；添加请求负责发布路由，删除请求按列表结果中的 ID 撤销，列表请求用于对比路由器现状。路由名称必须使用 {{name}}，否则残留路由无法识别。',
  variables:'可用变量',addTemplate:'添加路由请求',removeTemplate:'删除路由请求',listTemplate:'列表请求（用于对比与恢复）',method:'请求方法',path:'请求路径',headersText:'请求头（JSON，值支持变量）',headersInvalid:'请求头不是有效的 JSON 对象。',bodyText:'请求体（JSON 模板，支持变量）',bodyInvalid:'请求体模板替换变量后不是有效的 JSON。',
  itemsPath:'列表数组路径',itemsPathHint:'留空表示响应本身就是数组；例如 data.routes。',idField:'ID 字段',nameField:'名称字段',prefixField:'网段字段',nextHopField:'下一跳字段',
  apiBackendNote:'API 模式发布状态仅代表路由器接受了配置，不代表转发平面已生效。'
 },
 en:{
  autoNextHopChanged:'The Surge VM IPv4 address changed. Update the static routes on the gateway, then confirm again and save.',
  language:'Page language',configuration:'Routing configuration',mode:'Routing mode',fakeBackend:'Fake-IP routing method',dnsBackend:'DNS interception method',
  bgpLimit:'Routes are announced and withdrawn automatically; an available BGP neighbor is required.',staticLimit:'Gateway routes are managed externally and remain after the service stops.',hybridLimit:'Choose a backend for each route type; static routes are maintained externally.',apiLimit:'Routes are published and withdrawn automatically through the router management API; the router must be reachable with valid credentials.',
  fakeConfirmed:'Fake-IP routes configured on the gateway',staticDNS:'Enable static DNS interception',dnsConfirmAction:'Confirm gateway routes are configured',dnsConfirmed:'✓ Gateway routes confirmed',staticRisk:'Static DNS routes remain when the service stops.',
  dnsConfirmationHint:'Click the button to save confirmation immediately. This is a user declaration; gateway routes are not automatically configured or verified.',dnsConfirmationSaved:'DNS interception route confirmation saved',dnsConfigurationChanged:'DNS interception routing configuration changed. Refresh and confirm again.',
  save:'Save routing settings',saved:'Routing settings saved',confirmReset:'Confirm the gateway configuration again after route changes.',status:'Route status',fakeRoute:'Fake-IP routing',dnsRoute:'DNS interception',engine:'BGP engine',apiEngine:'Router API',running:'Running',disabled:'Disabled',ready:'Ready',notReady:'Not ready',confirmed:'User confirmed',pending:'Pending confirmation',unused:'Disabled',apiRunning:'Connected',apiError:'Error',
  prefix:'Prefix',nextHop:'Next hop',kind:'Purpose',backend:'Managed by',state:'Declaration / announcement',installed:'Installation',connectivity:'Local probe',static:'External static',bgp:'BGP',api:'API',externalUnknown:'Unknown (external)',rib:'In local RIB',noRib:'Not in local RIB',checkedAt:'Checked at',success:'TCP reachable',failure:'TCP unreachable',empty:'No routes',
  probe:'Local connectivity check',probeScope:'Checks local TCP connectivity only, without verifying gateway route installation.',fakePrefix:'Fake-IP prefix',choose:'Choose a prefix',target:'Fake-IP assigned by Surge',port:'TCP port',check:'Check connection',dnsKind:'DNS interception',manual:'Manual route',
  'external-unconfirmed':'Awaiting external configuration','external-confirmed':'User declared configuration','external-missing-next-hop':'Missing next hop','inactive':'Not announced','announced':'Announced','ribState':'Local RIB','withdrawn':'Withdrawn','api-published':'Published to router','api-pending':'Pending publication',
  apiSectionTitle:'Router API',preset:'API preset',customPreset:'Custom',baseURL:'Router address',baseURLHint:'HTTP(S) origin of the router management interface.',authMode:'Authentication',authNone:'None',authHeader:'Header key',authUbus:'OpenWrt ubus session',
  username:'Username',password:'Password (leave blank to keep)',secretConfigured:'Saved; leave blank to keep, or tick the option below to remove it.',headerName:'Authentication header name',headerValue:'API key (leave blank to keep)',verifyTLS:'Verify HTTPS certificate (disable for self-signed)',clearPassword:'Remove the saved password',clearHeader:'Remove the saved API key',
  openwrtHint:'Writes named route sections into the network configuration through the OpenWrt ubus JSON-RPC endpoint, then commits and reloads; a router admin account is required. Targets OpenWrt 18.06 and newer.',
  unifiHint:'Uses the UniFi OS Integration API (/proxy/network/integration/v1/routes) with an API key; only UniFi OS gateways exposing that endpoint are supported. Generate the key in the gateway system settings.',
  customHint:'Custom templates render variables before each request; the add request publishes a route, the remove request withdraws it by the ID from the list response, and the list request reconciles router state. Route names must use {{name}} so leftovers stay recognizable.',
  variables:'Available variables',addTemplate:'Add route request',removeTemplate:'Remove route request',listTemplate:'List request (reconciliation)',method:'Method',path:'Path',headersText:'Headers (JSON, values support variables)',headersInvalid:'Headers are not a valid JSON object.',bodyText:'Body (JSON template, supports variables)',bodyInvalid:'The body is not valid JSON after variable substitution.',
  itemsPath:'Items path',itemsPathHint:'Leave empty when the response itself is the array; for example data.routes.',idField:'ID field',nameField:'Name field',prefixField:'Prefix field',nextHopField:'Next-hop field',
  apiBackendNote:'API publication only confirms the router accepted the configuration, not that the forwarding plane applied it.'
 }
};
export const routingText=locale=>key=>messages[locale]?.[key]||messages['zh-CN'][key]||key;
