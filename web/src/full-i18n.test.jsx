import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,cleanup,waitFor,within} from '@testing-library/react';
import {App} from './main';
import {selectBackend} from './client';
import {message,translate} from './i18n';
const settings={general:{web_listen:'127.0.0.1:1055',mode:'auto',theme:'system'},dns:{listeners:['127.0.0.1:1053'],transparent_listeners:['192.0.2.2:1054'],interception_enabled:true,ingress_interfaces:['en0'],egress_interface:'en0',source4:'192.0.2.2',source6:'',gateway4:'192.0.2.1',gateway6:'',bypass_verified:true,preserve_original:false,timeout_ms:2000,source_port_first:50000,source_port_last:51000,upstreams:[{name:'Resolver',protocol:'udp',address:'223.6.6.6:53',tls_name:'',url:'',enabled:true}],policy_upstreams:[{field:'final_policy',policy:'Proxy',enabled:true,fallback:'upstreams',fallback_upstreams:[{name:'Backup',protocol:'dot',address:'1.1.1.1:853',tls_name:'cloudflare-dns.com',enabled:true}],upstreams:[{name:'Custom',protocol:'tcp',address:'1.1.1.1:53',enabled:true}]}]},surge:{user_name:'',cli_path:'/Applications/Surge.app/Contents/Applications/surge-cli',api_url:'',api_token:'',dns:'198.18.0.2:53',fake_prefixes:['198.18.0.0/16'],next_hop4:'192.0.2.2',next_hop6:'',trust_direct:false},routing:{mode:'hybrid',fake_ip_backend:'static',dns_backend:'bgp',static_fake_ip_configured:false,static_dns_interception:false,static_dns_configured:false},bgp:{enabled:true,local_asn:65002,router_id:'192.0.2.2',listen_port:179,route_limit:128,hold_seconds:30,keepalive_seconds:10},cache:{optimistic:false,optimistic_responses:false,expire_decisions:false,invalidate_on_read_failure:false,invalidate_direct_on_selection_change:true,invalidate_reject_on_selection_change:true,capacity:10000,response_capacity:10000,workers:8,queue:256,direct_ttl_seconds:60,surge_ttl_seconds:30,unknown_ttl_seconds:10,error_ttl_seconds:1},health:{interval_ms:3000,failures:3,successes:2,grace_seconds:30,stabilize_seconds:5,probe_domain:'example.com'},logging:{level:'info',query_history:true,history_capacity:1000,log_domains:true,event_retention:10000},security:{authentication:true,username:'admin',session_minutes:60,allowed_networks:['127.0.0.0/8'],tls_cert:'',tls_key:''}};
const record={context:{client_ip:'192.0.2.3',client_port:53000,original_destination_ip:'223.5.5.5',original_destination_port:53,transport:'udp',question:{Name:'example.com.',Qtype:1,Qclass:1},intercepted:true,received_at:'2026-10-08T00:00:00Z'},result:{domain:'example.com.',decision:'SURGE',source:'surge',confidence:'certain',policy:'Proxy',final_policy:'Proxy',matched_rule:'DOMAIN-SUFFIX,example.com,Proxy',reason:'Surge evaluation reported context or resolution limitations',created_at:'2026-10-08T00:00:00Z',expires_at:'2026-10-08T00:00:30Z',rule_dns:{state:'completed',lookup_performed:true,re_evaluated:true,resolver:'223.6.6.6:53',addresses:['192.0.2.10'],duration_ms:10},policy_dns:{field:'final_policy',policy:'Proxy',fallback:'upstreams',fallback_used:true,attempts:[{source:'primary',resolver:'udp://223.6.6.6:53',error:'context deadline exceeded',latency_ms:2000},{source:'upstreams',resolver:'dot://1.1.1.1:853',rcode:0,latency_ms:15,cache_hit:true}]}},classification_ms:20,resolver_ms:10,resolver:'Custom',rcode:0,latency:30000000,response:{id:1234,status:'NOERROR',answers:['example.com. 60 IN A 192.0.2.10'],authority:[],additional:[]}};
const peer={id:'p',address:'192.0.2.1',asn:65001,local_asn:0,router_id:'',hold_seconds:30,keepalive_seconds:10,port:179,enabled:true,description:''};
function fixture(path){
 if(path==='session')return {authenticated:true,csrf:'test'};
 if(path==='status')return {state:'HEALTHY',mode:'auto',fake_ip_enabled:true,queries_per_minute:1,counters:{Queries:1,Direct:0,Surge:1,Reject:0,Unknown:0,Errors:0,Intercepted:1,Addressed:0},cache:{decision:{entries:1,hits:1,misses:0},response:{entries:2,stale_entries:1,hits:7,misses:3,evictions:1,capacity:10000}},software:{name:'RoutePilot',version:'0.4.5',build:'14',os:'darwin',arch:'arm64',go_version:'go1.26.0'}};
 if(path==='settings')return settings;
 if(path==='health')return [{name:'Controller',state:'UP',latency_ms:1.49,last_failure:'0001-01-01T00:00:00Z',last_error:''},{name:'Transparent DNS',state:'UP',latency_ms:1.5,last_failure:'2026-10-08T00:00:00Z',last_error:''}];
 if(path==='bgp/status')return {enabled:true,mode:'hybrid',advertised:1,limit:128,peers:[{address:'192.0.2.1',state:'ESTABLISHED',uptime_seconds:100,prefixes:1}],routes:[{prefix:'8.8.8.8/32',kind:'dns-interception',backend:'bgp',next_hop:'192.0.2.2',installed:true,announced:true,route_state:'announced'}]};
 if(path==='routing/status')return {mode:'hybrid',routes:[{prefix:'198.18.0.0/16',kind:'fake-ip',backend:'static',next_hop:'192.0.2.2',route_state:'external-unconfirmed',installed:null,verified:false}]};
 if(path==='bgp/peers')return [peer];
 if(path==='dns/queries')return [record];
 if(path==='dns/clients')return {clients:[],stats:{clients:0,queries:0,direct:0,surge:0,reject:0,unknown:0,errors:0},capacity:4096,evicted_clients:0};
 if(path.startsWith('dns/cache'))return {entries:[],response_stats:{entries:2,stale_entries:1,hits:7,misses:3,evictions:1,capacity:10000},stats:{entries:1,hits:1,misses:0},total:0};
 if(path==='surge/status')return {snapshot:{profile:'Default',mode:'rule',always_real_ip:['*.example.net'],always_real_ip_count:1,always_real_ip_observed_at:'2026-10-08T00:00:00Z'}};
 if(path==='test/domain')return {match:{rule:'DOMAIN,example.com,Proxy',final:'Proxy'},decision:record.result};
 if(path==='events')return [{at:'2026-10-08T00:00:00Z',severity:'info',message:'Surge 有效配置或外部资源已确认更新'}];
 if(path==='bgp/export-frr')return {filename:'routepilot-frr.conf',content:'router bgp 65001\n neighbor 192.0.2.2 remote-as 65002',warnings:['请填写 Mac 的可达地址和 对端 的 IPv4 Router ID']};
 if(path==='dns/hosts/import-surge')return {imported:1,skipped:[{line:1,reason:'不支持'}]};
 return [];
}
function noUntranslatedText(){const texts=[];const walker=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);let node;while(node=walker.nextNode()){if(!node.parentElement.closest('pre,script,.language-control select')&&/[\u4e00-\u9fff]/.test(node.textContent))texts.push(node.textContent)}for(const e of document.querySelectorAll('[placeholder],[aria-label]'))for(const attr of ['placeholder','aria-label']){const text=e.getAttribute(attr);if(text&&/[\u4e00-\u9fff]/.test(text))texts.push(text)}expect(texts).toEqual([])}
afterEach(()=>{cleanup();localStorage.clear();sessionStorage.clear();selectBackend('');vi.unstubAllGlobals();document.documentElement.lang='zh-CN'});
it('localizes every page, settings category, editor and query detail through the global preference',async()=>{
 localStorage.setItem('routepilot.locale','en');vi.stubGlobal('EventSource',class{addEventListener(){}close(){}});vi.stubGlobal('fetch',vi.fn(async url=>({ok:true,status:200,json:async()=>fixture(String(url).replace('/api/v1/',''))})));render(<App/>);await screen.findByRole('heading',{level:1,name:'Dashboard'});await screen.findByText('DNS and routing');
 const pages=['DNS overview','Query history','Decision cache','Client','DNS interception','Local DNS','Domain overrides','Rule test','Routing','BGP overview','BGP neighbors','Manual routes','Health status','Events','Settings'];
 for(const [index,page] of pages.entries()){
  fireEvent.click(screen.getByRole('navigation').querySelectorAll('button')[index+1]);await screen.findByRole('heading',{level:1,name:page});await waitFor(noUntranslatedText);
  if(['DNS interception','Local DNS','Domain overrides','BGP neighbors','Manual routes'].includes(page)){fireEvent.click(screen.getByRole('button',{name:'＋ Add'}));noUntranslatedText();fireEvent.click(screen.getByRole('button',{name:'Close'}))}
  if(page==='Query history'){for(const label of ['Valid entries','DNS response cache capacity','Cache hits','Cache misses','Capacity evictions','Responses awaiting refresh'])expect(screen.getByText(label)).toBeInTheDocument();expect(screen.getByText('7')).toBeInTheDocument();fireEvent.click(screen.getByRole('cell',{name:'example.com.'}));noUntranslatedText();fireEvent.click(screen.getByRole('button',{name:'Close'}))}
  if(page==='Rule test'){fireEvent.click(screen.getByRole('button',{name:'Test rules'}));await screen.findByText('Raw evaluation result');noUntranslatedText()}
  if(page==='Local DNS'){fireEvent.click(screen.getByRole('button',{name:'Import from Surge'}));await screen.findByText('Imported 1 / skipped 1');noUntranslatedText()}
  if(page==='BGP neighbors'){fireEvent.click(screen.getByRole('button',{name:'Preview FRR configuration'}));await screen.findByText(/Mac's reachable address/);noUntranslatedText()}
  if(page==='Settings')for(let category=1;category<=document.querySelectorAll('.tabs button').length;category++){fireEvent.click(document.querySelector('.tabs button:nth-child('+category+')'));noUntranslatedText()}
 }
 fireEvent.click(screen.getByRole('button',{name:'Local service · Switch'}));await screen.findByRole('dialog',{name:'Manage backends'});noUntranslatedText();fireEvent.click(screen.getByRole('button',{name:'Add remote backend'}));noUntranslatedText();expect(screen.queryAllByLabelText(/页面语言|Page language/)).toHaveLength(0);
});
it('polls page-specific data only and pauses polling while the management window is hidden',async()=>{
 const fetch=vi.fn(async url=>({ok:true,status:200,json:async()=>fixture(String(url).replace('/api/v1/',''))}));
 vi.stubGlobal('EventSource',class{addEventListener(){}close(){}});
 vi.stubGlobal('fetch',fetch);
 render(<App/>);
 await screen.findByRole('heading',{level:1,name:'仪表盘'});
 await waitFor(()=>expect(fetch).toHaveBeenCalled());
 const dashboardRequests=fetch.mock.calls.map(([url])=>String(url));
 expect(dashboardRequests.some(url=>url.endsWith('/dns/queries'))).toBe(false);
 expect(dashboardRequests.some(url=>url.endsWith('/dns/clients'))).toBe(false);
 expect(dashboardRequests.some(url=>url.includes('/dns/cache'))).toBe(false);
 fireEvent.click(screen.getByRole('navigation').querySelector('button.active').parentElement.querySelectorAll('button')[4]);
 await screen.findByRole('heading',{level:1,name:'客户端'});
 await waitFor(()=>expect(fetch.mock.calls.some(([url])=>String(url).endsWith('/dns/clients'))).toBe(true));
 expect(fetch.mock.calls.some(([url])=>String(url).endsWith('/dns/queries'))).toBe(false);
 const requestsBeforeHide=fetch.mock.calls.length;
 window.dispatchEvent(new CustomEvent('routepilot-window-visibility',{detail:{visible:false}}));
 await new Promise(resolve=>setTimeout(resolve,3200));
 expect(fetch).toHaveBeenCalledTimes(requestsBeforeHide);
 window.dispatchEvent(new CustomEvent('routepilot-window-visibility',{detail:{visible:true}}));
 await waitFor(()=>expect(fetch.mock.calls.length).toBeGreaterThan(requestsBeforeHide));
});
it('formats diagnostics without changing domain names, policy values, or DNS records',()=>{expect(translate('第 {line} 行：{reason}',{line:12,reason:'example.com'},'en')).toBe('Line 12: example.com');expect(message('Surge 有效配置或外部资源已确认更新','en')).toMatch(/changed/);expect(message('Surge evaluation reported context or resolution limitations','zh-CN')).toBe('Surge 求值存在连接上下文或解析限制');expect(message('example.com. 60 IN A 192.0.2.10','zh-CN')).toBe('example.com. 60 IN A 192.0.2.10')});
it('uses localized diagnostic templates for nested resolver failures',()=>{
 expect(message('real resolver failed: context deadline exceeded','zh-CN')).toBe('真实 DNS 解析失败：操作超时');
 expect(message('real resolver failed: upstream DNS returned SERVFAIL','zh-CN')).toBe('真实 DNS 解析失败：上游 DNS 返回 SERVFAIL');
});

it('shows never for unset health times and rounds monitoring latency to integer milliseconds',async()=>{
 vi.stubGlobal('EventSource',class{addEventListener(){}close(){}});
 vi.stubGlobal('fetch',vi.fn(async url=>({ok:true,status:200,json:async()=>fixture(String(url).replace('/api/v1/',''))})));
 render(<App/>);
 await screen.findByRole('heading',{level:1,name:'仪表盘'});
 const dashboard=await screen.findByRole('row',{name:'控制器 正常 1'});
 expect(within(dashboard).getByRole('cell',{name:'1',exact:true})).toBeInTheDocument();
 fireEvent.click(screen.getByRole('navigation').querySelectorAll('button')[13]);
 await screen.findByRole('heading',{level:1,name:'健康状态'});
 const rows=screen.getAllByRole('row');
 expect(within(rows[1]).getAllByRole('cell',{name:'从不'})).toHaveLength(2);
 expect(within(rows[2]).getByRole('cell',{name:'2026-10-08T00:00:00Z'})).toBeInTheDocument();
 expect(within(rows[2]).getByRole('cell',{name:'2',exact:true})).toBeInTheDocument();
 expect(screen.queryByText('0001-01-01T00:00:00Z')).not.toBeInTheDocument();
 expect(screen.queryByText('1.49')).not.toBeInTheDocument();
});


it('keeps mode selection in routing settings and connection actions in the sidebar',async()=>{
 let mode='auto';
 const fetch=vi.fn(async(url,options)=>{
  const path=String(url).replace('/api/v1/','');
  if(path==='settings'&&options.method==='PUT'){const submitted=JSON.parse(options.body);mode=submitted.general.mode;return {ok:true,status:200,json:async()=>({settings:submitted})};}
  const value=path==='status'?{...fixture(path),mode}:path==='settings'?{...settings,general:{...settings.general,mode}}:fixture(path);
  return {ok:true,status:200,json:async()=>value};
 });
 vi.stubGlobal('fetch',fetch);vi.stubGlobal('EventSource',class{addEventListener(){}close(){}});
 render(<App/>);await screen.findByRole('heading',{level:1,name:'仪表盘'});await screen.findByText('DNS 与路由');
 const sidebar=document.querySelector('.desktop-sidebar'),topbar=document.querySelector('.topbar');
 expect(topbar.querySelectorAll('select,button')).toHaveLength(0);
 expect(screen.queryAllByLabelText(/页面语言|Page language/)).toHaveLength(0);
 expect(document.querySelector('.connection-banner')).toBeNull();
 expect(within(sidebar.querySelector('.aside-foot')).getByRole('button',{name:'退出',exact:true})).toBeInTheDocument();
 expect(within(sidebar.querySelector('.aside-foot')).getByRole('button',{name:'本地服务 · 切换',exact:true})).toBeInTheDocument();
 expect(within(sidebar).queryByRole('combobox')).not.toBeInTheDocument();
 expect(within(sidebar).getByLabelText('当前分流模式')).toHaveTextContent('自动分流');
 fireEvent.click(within(sidebar).getByRole('button',{name:'设置',exact:true}));
 expect(screen.getAllByLabelText(/页面语言|Page language/)).toHaveLength(1);
 expect(screen.queryByRole('combobox',{name:'全局模式'})).not.toBeInTheDocument();
 fireEvent.click(within(document.querySelector('.tabs')).getByRole('button',{name:'路由',exact:true}));
 const selection=screen.getByRole('combobox',{name:'分流模式'});
 fireEvent.change(selection,{target:{value:'force-direct'}});
 expect(selection).toHaveValue('force-direct');
 fireEvent.click(screen.getByRole('button',{name:'保存设置'}));
 await waitFor(()=>expect(fetch.mock.calls.some(([url,options])=>String(url).endsWith('/settings')&&options.method==='PUT'&&JSON.parse(options.body).general.mode==='force-direct')).toBe(true));
 await waitFor(()=>expect(within(sidebar).getByLabelText('当前分流模式')).toHaveTextContent('强制直连'));
 fireEvent.click(screen.getByRole('button',{name:'DNS',exact:true}));
 expect(screen.queryAllByLabelText(/页面语言|Page language/)).toHaveLength(0);
});
