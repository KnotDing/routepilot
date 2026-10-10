import {LanguageSelector} from './language';
import React from 'react';
import {describe,it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,cleanup,waitFor} from '@testing-library/react';
import {Field,Resource,RoutingPanel,Queries,Settings} from './main';
import {request,setCSRF} from './client';
afterEach(()=>{cleanup();localStorage.removeItem('routepilot.locale');vi.unstubAllGlobals()});
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
 const settings={general:{web_listen:'127.0.0.1:1055',name:'Controller',theme:'system'},routing,security:{tls_cert:''}};
 const fetch=vi.fn(async(url,options)=>({ok:true,status:200,json:async()=>({settings:JSON.parse(options.body)})}));
 vi.stubGlobal('fetch',fetch);
 render(<Settings settings={settings} refresh={()=>{}} notify={()=>{}}/>);
 expect(screen.getByRole('button',{name:'路由',exact:true})).toBeInTheDocument();
 expect(screen.queryByLabelText('路由模式')).not.toBeInTheDocument();
 fireEvent.change(screen.getByLabelText('主题'),{target:{value:'dark'}});
 fireEvent.click(screen.getByRole('button',{name:'保存设置'}));
 await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(1));
 const saved=JSON.parse(fetch.mock.calls[0][1].body);
 expect(saved.routing).toEqual(routing);
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
 expect(screen.getByLabelText('读取失败时失效')).not.toBeChecked();
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
 expect(screen.getByText('Cache settings')).toBeInTheDocument();expect(screen.getByLabelText('Expire decisions by TTL')).not.toBeChecked();expect(screen.getByLabelText('Invalidate on read failure')).not.toBeChecked();expect(screen.getByLabelText('Invalidate DIRECT on selection change')).toBeChecked();expect(screen.getByLabelText('Invalidate REJECT on selection change')).toBeChecked();expect(screen.getByRole('button',{name:'Save settings'})).toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'General',exact:true}));fireEvent.change(screen.getByLabelText(/页面语言|Page language/),{target:{value:'zh-CN'}});fireEvent.click(screen.getByRole('button',{name:'缓存',exact:true}));expect(screen.getByLabelText('决策 TTL 过期')).toBeInTheDocument();
});
