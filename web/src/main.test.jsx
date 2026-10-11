import {LanguageSelector} from './language';
import React from 'react';
import {describe,it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,cleanup,waitFor,act} from '@testing-library/react';
import {Field,Resource,RoutingPanel,Queries,Settings} from './main';
import {request,setCSRF,selectBackend} from './client';
afterEach(()=>{cleanup();localStorage.removeItem('routepilot.locale');vi.unstubAllGlobals();delete window.webkit;delete window.routepilotNativePreferences;selectBackend('')});
it('distinguishes operational fallback from a DIRECT rule match',()=>{
 const rows=[{context:{client_ip:'192.0.2.10',received_at:'2026-10-08T00:00:00Z',question:{Name:'accounts.google.com.',Qtype:65}},result:{decision:'DIRECT',source:'state-fallback',confidence:'uncertain'},latency:0,answers:[]}];
 render(<Queries rows={rows}/>);
 expect(screen.getByRole('cell',{name:'回退至真实 DNS'})).toBeInTheDocument();
 fireEvent.click(screen.getByRole('cell',{name:'accounts.google.com.'}));
 expect(screen.getByText('回退至真实 DNS（未执行规则判断）')).toBeInTheDocument();
});
describe('management forms',()=>{
 it('uses non-echoing inputs for both credential fields',()=>{render(<><Field name="password" value="" onChange={()=>{}}/><Field name="api_token" value="" onChange={()=>{}}/></>);expect(document.querySelectorAll('input[type="password"]')).toHaveLength(2)});
 it('keeps the route default health binding independent',()=>{const change=vi.fn();render(<Field name="health_binding" value="none" onChange={change}/>);expect(screen.getByRole('combobox')).toHaveValue('none');fireEvent.change(screen.getByRole('combobox'),{target:{value:'surge'}});expect(change).toHaveBeenCalledWith('surge')});
 it('sends the session CSRF token on changes and surfaces API errors',async()=>{const fetch=vi.fn().mockResolvedValue({ok:false,status:422,text:async()=>JSON.stringify({error:'hard route limit exceeded'})});vi.stubGlobal('fetch',fetch);setCSRF('test-csrf');await expect(request('bgp/routes','POST',{prefix:'10.0.0.0/8'})).rejects.toThrow('hard route limit exceeded');expect(fetch.mock.calls[0][1].headers['X-CSRF-Token']).toBe('test-csrf')});
});

it('opens an existing local DNS entry for wildcard editing',()=>{
 render(<Resource page="Hosts" rows={[{id:'host-test',domain:'ui-acceptance.example',match:'exact',addresses:['192.0.2.53'],ttl:120,enabled:true,source:'manual'}]} refresh={()=>{}} notify={()=>{}}/>);
 fireEvent.click(screen.getByRole('cell',{name:'ui-acceptance.example'}));
 expect(screen.getByLabelText('匹配方式')).toHaveValue('exact');
 fireEvent.change(screen.getByLabelText('匹配方式'),{target:{value:'wildcard'}});
 expect(screen.getByLabelText('匹配方式')).toHaveValue('wildcard');
});

it('exposes static confirmations and independent hybrid backends',()=>{
 const settings={routing:{mode:'static',fake_ip_backend:'static',dns_backend:'static',static_fake_ip_configured:false,static_dns_interception:false,static_dns_configured:false}};
 render(<Settings settings={settings} refresh={()=>{}} notify={()=>{}}/>);fireEvent.click(screen.getByRole('button',{name:'路由',exact:true}));
 expect(screen.getByLabelText('Fake-IP 路由已在网关配置')).not.toBeChecked();
 fireEvent.click(screen.getByLabelText('启用静态 DNS 拦截'));
 expect(screen.getByLabelText('DNS 拦截路由已在网关配置')).not.toBeChecked();
 fireEvent.change(screen.getByLabelText('路由模式'),{target:{value:'hybrid'}});
 expect(screen.getByLabelText('Fake-IP 路由方式')).toHaveValue('static');
 fireEvent.change(screen.getByLabelText('DNS 拦截路由方式'),{target:{value:'bgp'}});
 expect(screen.getByLabelText('DNS 拦截路由方式')).toHaveValue('bgp');
});

it('defaults DIRECT conservatism on and saves an explicit opt-out',async()=>{
 const settings={general:{web_listen:'127.0.0.1:1055'},surge:{dns:'198.18.0.2:53'},security:{tls_cert:''}};
 const fetch=vi.fn().mockResolvedValue({ok:true,status:200,text:async()=>JSON.stringify({csrf:'new-csrf'})});
 vi.stubGlobal('fetch',fetch);
 render(<Settings settings={settings} refresh={()=>{}} notify={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'Surge'}));
 expect(screen.getByLabelText('DIRECT 保守判断')).toBeChecked();
 fireEvent.click(screen.getByLabelText('DIRECT 保守判断'));
 fireEvent.click(screen.getByRole('button',{name:'保存设置'}));
 await waitFor(()=>expect(fetch).toHaveBeenCalled());
 const saved=JSON.parse(fetch.mock.calls[0][1].body);
 expect(saved.surge.trust_direct).toBe(true);
 expect(saved.surge.dns).toBe('198.18.0.2:53');
 expect(screen.getByText(/未来连接仍可能命中不同策略/)).toBeInTheDocument();
});

it('keeps routing configuration in settings and preserves it when saving other categories',async()=>{
 const routing={mode:'hybrid',fake_ip_backend:'static',dns_backend:'bgp',static_fake_ip_configured:true};
 const bgp={enabled:true,local_asn:64512,router_id:'192.0.2.1',listen_port:179,route_limit:128,hold_seconds:90,keepalive_seconds:30};
 const settings={general:{web_listen:'127.0.0.1:1055',theme:'system'},routing,bgp,security:{tls_cert:''}};
 const fetch=vi.fn(async(url,options)=>({ok:true,status:200,json:async()=>({settings:JSON.parse(options.body)})}));
 vi.stubGlobal('fetch',fetch);
 render(<Settings settings={settings} refresh={()=>{}} notify={()=>{}}/>);
 expect(screen.getByRole('button',{name:'路由',exact:true})).toBeInTheDocument();
 expect(screen.queryByRole('button',{name:'BGP',exact:true})).not.toBeInTheDocument();
 expect(screen.queryByLabelText('路由模式')).not.toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'路由',exact:true}));
 expect(screen.getByRole('heading',{name:'BGP'})).toBeInTheDocument();
 fireEvent.change(screen.getByLabelText('本地 ASN'),{target:{value:'64513'}});
 fireEvent.click(screen.getByRole('button',{name:'通用',exact:true}));
 fireEvent.change(screen.getByLabelText('主题'),{target:{value:'dark'}});
 fireEvent.click(screen.getByRole('button',{name:'保存设置'}));
 await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(1));
 const saved=JSON.parse(fetch.mock.calls[0][1].body);
 expect(saved.routing).toEqual(routing);
 expect(saved.bgp.local_asn).toBe(64513);
 expect(saved.general.theme).toBe('dark');
});

it('shows configured DIRECT separately from confirmed connection context',()=>{
 render(<Queries rows={[{context:{client_ip:'192.0.2.10',received_at:'2026-10-08T00:00:00Z',question:{Name:'configured.test.',Qtype:1}},result:{decision:'DIRECT',source:'surge',confidence:'configured',reason:'按配置采用 Surge DIRECT 结果'},latency:0,answers:[]}]}/>);
 fireEvent.click(screen.getByRole('cell',{name:'configured.test.'}));
 expect(screen.getByText('按配置采用（上下文未确认）')).toBeInTheDocument();
});

it('combines configured neighbors and live BGP status into one editable table',()=>{
 render(<Resource page="Peers" rows={[{id:'peer',address:'192.0.2.1',asn:65001,port:179,enabled:true}]} peerStates={[{address:'192.0.2.1',state:'ESTABLISHED',uptime_seconds:3600,prefixes:2}]} refresh={()=>{}} notify={()=>{}}/>);
 expect(screen.getAllByRole('table')).toHaveLength(1);expect(screen.getByRole('cell',{name:'已建立'})).toBeInTheDocument();
 fireEvent.click(screen.getByRole('cell',{name:'192.0.2.1'}));expect(screen.getByLabelText('邻居地址')).toHaveValue('192.0.2.1');
});

it('exposes independent long-lived cache invalidation settings',()=>{
 render(<Settings settings={{cache:{expire_decisions:false,invalidate_on_read_failure:false,invalidate_direct_on_selection_change:true,invalidate_reject_on_selection_change:false,unknown_ttl_seconds:10,error_ttl_seconds:1}}} refresh={()=>{}} notify={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'缓存'}));
 expect(screen.getByLabelText('决策 TTL 过期')).not.toBeChecked();
 expect(screen.getByLabelText('配置读取失败时失效决策')).not.toBeChecked();
 expect(screen.getByLabelText('DIRECT 选择变化时失效')).toBeChecked();
 expect(screen.getByLabelText('REJECT 选择变化时失效')).not.toBeChecked();
});

it('keeps cache option labels concise and shows explanations on hover',()=>{
 render(<Settings settings={{cache:{optimistic:false,optimistic_responses:false,optimistic_idle_refresh:true,optimistic_response_idle_refresh:true,expire_decisions:false,invalidate_on_read_failure:false,invalidate_direct_on_selection_change:true,invalidate_reject_on_selection_change:false}}} refresh={()=>{}} notify={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'缓存'}));
 const decision=screen.getByLabelText('乐观决策缓存'),query=screen.getByLabelText('乐观查询缓存');
 expect(decision.closest('label')).toHaveAttribute('title','确认配置或资源更新后保留旧决策，并在命中或空闲时刷新。');
 expect(query.closest('label')).toHaveAttribute('title','乐观查询缓存已开启：过期响应仍可立即返回，后台刷新成功后替换；响应会持久化并在重启后恢复。');
 expect(screen.getByLabelText('决策缓存空闲刷新').closest('label')).toHaveAttribute('title',expect.stringContaining('关闭不影响命中时的按需刷新'));
 expect(screen.queryByText(/先响应旧决策|再在后台刷新|关闭后仍会在缓存命中时按需刷新/)).not.toBeInTheDocument();
});

it('translates all cache settings and shares language with the routing page',()=>{
 localStorage.setItem('routepilot.locale','en');
 render(<><Settings settings={{cache:{expire_decisions:false,invalidate_on_read_failure:false,invalidate_direct_on_selection_change:true,invalidate_reject_on_selection_change:true,capacity:10000,unknown_ttl_seconds:10,error_ttl_seconds:1}}} refresh={()=>{}} notify={()=>{}}/></>);fireEvent.click(screen.getByRole('button',{name:'Cache'}));
 expect(screen.getByText('Cache settings')).toBeInTheDocument();expect(screen.getByLabelText('Expire decisions by TTL')).not.toBeChecked();expect(screen.getByLabelText('Invalidate decisions on config read failure')).not.toBeChecked();expect(screen.getByLabelText('Invalidate DIRECT on selection change')).toBeChecked();expect(screen.getByLabelText('Invalidate REJECT on selection change')).toBeChecked();expect(screen.getByRole('button',{name:'Save settings'})).toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'General',exact:true}));fireEvent.change(screen.getByLabelText(/页面语言|Page language/),{target:{value:'zh-CN'}});fireEvent.click(screen.getByRole('button',{name:'缓存',exact:true}));expect(screen.getByLabelText('决策 TTL 过期')).toBeInTheDocument();
});

it('binds backend update automation to the connected backend settings',()=>{
 render(<Settings settings={{updates:{automatic_check:true,check_beta:false,automatic_install:false,check_interval_hours:12,update_time:'03:00'}}} refresh={()=>{}} notify={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'软件更新',exact:true}));
 expect(screen.getByLabelText('自动检查新版本')).toBeChecked();
 expect(screen.getByLabelText('自动检查 Beta 版本')).not.toBeChecked();
 expect(screen.getByLabelText('自动检查间隔（小时）')).toHaveValue(12);
 expect(screen.getByLabelText('自动更新时间（本地时间）')).toHaveValue('03:00');
 const install=screen.getByLabelText('自动安装新版本');
 expect(install).not.toBeChecked();
 expect(install).toBeEnabled();
 fireEvent.click(screen.getByLabelText('自动检查新版本'));
 expect(install).toBeDisabled();
});

it('disables automatic installation when automatic checks are turned off and saves both',async()=>{
 const settings={general:{web_listen:'127.0.0.1:1055'},security:{tls_cert:''},updates:{automatic_check:true,check_beta:false,automatic_install:true,check_interval_hours:12,update_time:'03:00'}};
 const fetch=vi.fn(async(url,options)=>({ok:true,status:200,json:async()=>({settings:JSON.parse(options.body)})}));
 vi.stubGlobal('fetch',fetch);
 render(<Settings settings={settings} refresh={()=>{}} notify={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'软件更新',exact:true}));
 fireEvent.click(screen.getByLabelText('自动检查新版本'));
 expect(screen.getByLabelText('自动安装新版本')).not.toBeChecked();
 expect(screen.getByLabelText('自动安装新版本')).toBeDisabled();
 fireEvent.click(screen.getByRole('button',{name:'保存设置'}));
 await waitFor(()=>expect(fetch).toHaveBeenCalled());
 expect(JSON.parse(fetch.mock.calls[0][1].body).updates).toMatchObject({automatic_check:false,automatic_install:false});
});

it('degrades update automation settings for older backends',()=>{
 render(<Settings settings={{general:{web_listen:'127.0.0.1:1055'}}} refresh={()=>{}} notify={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'软件更新',exact:true}));
 expect(screen.getByText('当前后端版本较旧，不支持自动更新设置；请先升级该后端。')).toBeInTheDocument();
});

const updateSettings={general:{web_listen:'127.0.0.1:1055'},security:{tls_cert:'',authentication:true},updates:{automatic_check:true,check_beta:false,automatic_install:false,check_interval_hours:12,update_time:'03:00'}};
it('combines local update controls and policy into one form, saving prompts with settings',async()=>{
 const postAction=vi.fn(),postPreferences=vi.fn(value=>queueMicrotask(()=>{Object.assign(window.routepilotNativePreferences,value);window.dispatchEvent(new CustomEvent('routepilot-preferences-saved',{detail:{requestID:value._requestID,ok:true}}))}));
 window.webkit={messageHandlers:{routepilotPreferences:{postMessage:postPreferences},routepilotUpdateAction:{postMessage:postAction}}};
 window.routepilotNativePreferences={updatePrompts:true,assistantReady:false};
 const fetch=vi.fn(async(url,options)=>({ok:true,status:200,json:async()=>({settings:JSON.parse(options.body)})}));vi.stubGlobal('fetch',fetch);
 const notify=vi.fn();render(<Settings settings={updateSettings} refresh={()=>{}} notify={notify}/>);
 fireEvent.click(screen.getByRole('button',{name:'软件更新',exact:true}));
 expect(screen.getAllByRole('heading',{name:'软件更新',exact:true})).toHaveLength(1);
 expect(document.querySelectorAll('form')).toHaveLength(1);
 expect(screen.queryByRole('heading',{name:'软件更新设置'})).not.toBeInTheDocument();
 expect(screen.queryByRole('button',{name:'启用更新助手'})).not.toBeInTheDocument();
 expect(screen.queryByRole('button',{name:'保存应用偏好'})).not.toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'立即检查更新'}));
 expect(postAction).toHaveBeenCalledWith('check');expect(fetch).not.toHaveBeenCalled();
 fireEvent.click(screen.getByLabelText('发现新版本时提示'));
 act(()=>{Object.assign(window.routepilotNativePreferences,{updateAvailable:true,updateVersion:'v9.0.0',desktopNotifications:true});window.dispatchEvent(new Event('routepilot-preferences-changed'))});
 expect(screen.getByLabelText('发现新版本时提示')).not.toBeChecked();
 expect(screen.getByText('已发现可用更新：v9.0.0')).toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'保存设置'}));
 await waitFor(()=>expect(notify).toHaveBeenCalledWith('设置已保存'));
 expect(fetch).toHaveBeenCalledTimes(1);
 expect(postPreferences).toHaveBeenCalledWith(expect.objectContaining({updatePrompts:false,desktopNotifications:true}));
 expect(JSON.parse(fetch.mock.calls[0][1].body)).not.toHaveProperty('updatePrompts');
});

it('checks and upgrades the remote connection without invoking the local App',async()=>{
 const postAction=vi.fn();window.webkit={messageHandlers:{routepilotPreferences:{postMessage:vi.fn()},routepilotUpdateAction:{postMessage:postAction}}};
 window.routepilotNativePreferences={updatePrompts:true,assistantReady:true};selectBackend('hermes');
 const result={current:'1.0.0',app_current:'1.0.0',latest:'v1.0.1',available:true,supported:true,phase:'idle',release_notes:'## Changes'};
 const fetch=vi.fn(async()=>({ok:true,status:200,json:async()=>result}));vi.stubGlobal('fetch',fetch);
 render(<Settings settings={updateSettings} backend={{id:'hermes',name:'Hermes'}} status={{online_update:true}} refresh={()=>{}} notify={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'软件更新',exact:true}));
 await screen.findByText('有可用的软件新版本');
 expect(screen.queryByLabelText('发现新版本时提示')).not.toBeInTheDocument();
 expect(screen.queryByRole('button',{name:'停用更新助手'})).not.toBeInTheDocument();
 expect(screen.getAllByRole('heading',{level:2})).toHaveLength(1);
 fireEvent.click(screen.getByRole('button',{name:'立即检查更新'}));
 await screen.findByRole('dialog');
 fireEvent.click(screen.getByRole('button',{name:'立即升级'}));
 await waitFor(()=>expect(fetch.mock.calls.some(([url,options])=>url==='/api/v1/management/relay/hermes/updates/install'&&options.method==='POST')).toBe(true));
 expect(fetch.mock.calls.filter(([url])=>url.endsWith('/updates/check'))).toHaveLength(2);
 expect(fetch.mock.calls.every(([url])=>url.startsWith('/api/v1/management/relay/hermes/'))).toBe(true);
 expect(postAction).not.toHaveBeenCalled();
});

it('checks the connected local Mac from a browser through the backend API',async()=>{
 const fetch=vi.fn(async()=>({ok:true,status:200,json:async()=>({available:false,supported:true,current:'1.0.1',phase:'idle'})}));vi.stubGlobal('fetch',fetch);
 render(<Settings settings={updateSettings} status={{online_update:true}} refresh={()=>{}} notify={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'软件更新',exact:true}));
 await screen.findByText('软件已是最新版本');
 fireEvent.click(screen.getByRole('button',{name:'立即检查更新'}));
 await waitFor(()=>expect(fetch.mock.calls.filter(([url])=>url==='/api/v1/updates/check')).toHaveLength(2));
});

it('shows the local GitHub token beside the update source only when GitHub is selected',async()=>{
 const tokenBridge=vi.fn(payload=>queueMicrotask(()=>window.dispatchEvent(new CustomEvent('routepilot-github-token-saved',{detail:{requestID:payload.requestID,ok:true}}))));
 window.webkit={messageHandlers:{routepilotPreferences:{postMessage:vi.fn()},routepilotGitHubToken:{postMessage:tokenBridge},routepilotUpdateAction:{postMessage:vi.fn()}}};
 window.routepilotNativePreferences={updatePrompts:true,githubAccessTokenConfigured:false};
 render(<Settings settings={{...updateSettings,updates:{...updateSettings.updates,update_source:'github'}}} refresh={()=>{}} notify={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'软件更新',exact:true}));
 const source=screen.getByLabelText('更新源'),token=screen.getByLabelText('GitHub Access token');
 expect(token).toHaveAttribute('type','password');
 expect(token.closest('.form-grid')).toBe(source.closest('.form-grid'));
 expect(token.closest('.github-access-token-controls')).not.toHaveClass('panel');
 fireEvent.change(source,{target:{value:'gitea'}});
 expect(screen.queryByLabelText('GitHub Access token')).not.toBeInTheDocument();
 fireEvent.change(source,{target:{value:'github'}});
 const restoredToken=screen.getByLabelText('GitHub Access token');
 expect(restoredToken).toBeInTheDocument();
 fireEvent.change(restoredToken,{target:{value:'  fixture-read-only-token  '}});
 fireEvent.click(screen.getByRole('button',{name:'保存 Access token'}));
 await waitFor(()=>expect(tokenBridge).toHaveBeenCalledWith(expect.objectContaining({action:'save',token:'fixture-read-only-token'})));
});

it('deletes a configured local GitHub token through the native bridge',async()=>{
 const tokenBridge=vi.fn(payload=>queueMicrotask(()=>window.dispatchEvent(new CustomEvent('routepilot-github-token-saved',{detail:{requestID:payload.requestID,ok:true}}))));
 window.webkit={messageHandlers:{routepilotPreferences:{postMessage:vi.fn()},routepilotGitHubToken:{postMessage:tokenBridge},routepilotUpdateAction:{postMessage:vi.fn()}}};
 window.routepilotNativePreferences={updatePrompts:true,githubAccessTokenConfigured:true};
 render(<Settings settings={{...updateSettings,updates:{...updateSettings.updates,update_source:'github'}}} refresh={()=>{}} notify={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'软件更新',exact:true}));
 fireEvent.click(screen.getByRole('button',{name:'删除已保存的 token'}));
 await waitFor(()=>expect(tokenBridge).toHaveBeenCalledWith(expect.objectContaining({action:'clear',token:''})));
});
