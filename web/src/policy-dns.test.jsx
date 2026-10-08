import React,{useState} from 'react';
import {it,expect,afterEach,vi} from 'vitest';
import {render,screen,fireEvent,cleanup,waitFor,within} from '@testing-library/react';
import {PolicyDNSEditor,PolicyDNSResult} from './policy-dns';
import {Settings,Queries} from './main';
afterEach(()=>{cleanup();localStorage.clear();vi.unstubAllGlobals()});

it('adds, edits, disables and removes policy DNS rules without changing the normal pool',async()=>{
 const fetch=vi.fn().mockResolvedValue({ok:true,status:200,text:async()=>JSON.stringify({csrf:'new-csrf'})});vi.stubGlobal('fetch',fetch);
 const settings={general:{web_listen:'127.0.0.1:1055'},security:{tls_cert:''},dns:{upstreams:[{protocol:'udp',address:'192.0.2.1:53',enabled:true}],preserve_original:true}};
 render(<Settings settings={settings} refresh={()=>{}} notify={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'DNS'}));fireEvent.click(screen.getByRole('button',{name:'添加策略上游'}));
 const card=within(screen.getByRole('region',{name:'策略专用 DNS 上游 1'}));
 fireEvent.change(card.getByLabelText('匹配字段'),{target:{value:'policy'}});fireEvent.change(card.getByLabelText('策略名称'),{target:{value:'代理组'}});fireEvent.change(card.getByLabelText('服务器 IP:端口'),{target:{value:'223.6.6.6:53'}});
 fireEvent.click(screen.getByRole('button',{name:'保存设置'}));await waitFor(()=>expect(fetch).toHaveBeenCalled());
 const saved=JSON.parse(fetch.mock.calls[0][1].body);
 expect(saved.dns.policy_upstreams).toEqual([{field:'policy',policy:'代理组',enabled:true,upstreams:[{protocol:'udp',address:'223.6.6.6:53',enabled:true,name:''}]}]);expect(saved.dns.upstreams).toEqual(settings.dns.upstreams);expect(settings.dns.policy_upstreams).toBeUndefined();
 fireEvent.click(card.getByLabelText('启用策略上游'));expect(card.getByLabelText('启用策略上游')).not.toBeChecked();
 fireEvent.click(card.getByRole('button',{name:'删除策略'}));expect(screen.queryByLabelText('策略名称')).not.toBeInTheDocument();
});
it('supports multiple servers and encrypted transports with English labels',()=>{
 localStorage.setItem('routepilot.locale','en');
 function Editor(){const [rows,setRows]=useState([]);return <PolicyDNSEditor value={rows} onChange={setRows}/>}
 render(<Editor/>);fireEvent.click(screen.getByRole('button',{name:'Add policy upstream'}));
 fireEvent.change(screen.getByLabelText('Protocol'),{target:{value:'dot'}});expect(screen.getByLabelText('TLS server name')).toBeRequired();
 fireEvent.change(screen.getByLabelText('Protocol'),{target:{value:'doh'}});expect(screen.getByLabelText('DoH HTTPS URL')).toBeRequired();
 fireEvent.click(screen.getByRole('button',{name:'Add server'}));expect(screen.getAllByLabelText('Server IP:port')).toHaveLength(2);
 fireEvent.click(screen.getAllByRole('button',{name:'Remove server'})[1]);expect(screen.getAllByLabelText('Server IP:port')).toHaveLength(1);
 expect(screen.getByText(/final_policy takes priority over policy/)).toBeInTheDocument();
});
it('shows the matched field and policy alongside the actual resolver in query details',()=>{
 render(<Queries rows={[{context:{received_at:'2026-10-08T00:00:00Z',question:{Name:'test.example.',Qtype:1}},result:{decision:'SURGE',source:'cache',policy:'Proxy',final_policy:'Node A',policy_dns:{field:'final_policy',policy:'Node A'}},resolver:'udp://223.6.6.6:53',rcode:0,latency:1e6,answers:[]}]}/>);
 fireEvent.click(screen.getByRole('cell',{name:'test.example.'}));expect(screen.getByText('策略 DNS 覆盖')).toBeInTheDocument();expect(screen.getByText('udp://223.6.6.6:53')).toBeInTheDocument();expect(screen.getByText(/本次查询使用此策略/)).toBeInTheDocument();
});

it('saves custom backups and switches to Surge or the global real pool without changing the primary',async()=>{
 const fetch=vi.fn().mockResolvedValue({ok:true,status:200,json:async()=>({csrf:'test'})});vi.stubGlobal('fetch',fetch);
 const primary={field:'final_policy',policy:'Node A',enabled:true,upstreams:[{protocol:'udp',address:'223.6.6.6:53',enabled:true}]};
 const settings={general:{web_listen:'127.0.0.1:1055'},security:{tls_cert:''},dns:{upstreams:[{protocol:'udp',address:'192.0.2.1:53',enabled:true}],policy_upstreams:[primary]}};
 render(<Settings settings={settings} refresh={()=>{}} notify={()=>{}}/>);fireEvent.click(screen.getByRole('button',{name:'DNS'}));
 fireEvent.change(screen.getByLabelText('查询失败后的备用选项'),{target:{value:'upstreams'}});
 const backup=within(screen.getByRole('group',{name:'备用 DNS 服务器'}));fireEvent.change(backup.getByLabelText('服务器 IP:端口'),{target:{value:'1.1.1.1:53'}});
 fireEvent.change(backup.getByLabelText('协议'),{target:{value:'dot'}});fireEvent.change(backup.getByLabelText('TLS 服务器名称'),{target:{value:'cloudflare-dns.com'}});
 fireEvent.click(screen.getByRole('button',{name:'保存设置'}));await waitFor(()=>expect(fetch).toHaveBeenCalled());
 const saved=JSON.parse(fetch.mock.calls.find(([,options])=>options.method==='PUT')[1].body);
 expect(saved.dns.policy_upstreams[0].fallback).toBe('upstreams');expect(saved.dns.policy_upstreams[0].fallback_upstreams[0]).toMatchObject({address:'1.1.1.1:53',protocol:'dot',tls_name:'cloudflare-dns.com'});expect(saved.dns.policy_upstreams[0].upstreams).toEqual(primary.upstreams);
 fireEvent.change(screen.getByLabelText('查询失败后的备用选项'),{target:{value:'surge'}});expect(screen.queryByRole('group',{name:'备用 DNS 服务器'})).not.toBeInTheDocument();
 fireEvent.change(screen.getByLabelText('查询失败后的备用选项'),{target:{value:'real'}});expect(screen.getByLabelText('查询失败后的备用选项')).toHaveValue('real');
 fireEvent.change(screen.getByLabelText('查询失败后的备用选项'),{target:{value:'upstreams'}});expect(within(screen.getByRole('group',{name:'备用 DNS 服务器'})).getByLabelText('服务器 IP:端口')).toHaveValue('1.1.1.1:53');
});
it('localizes fallback attempt details while retaining actual endpoints and policy names',()=>{
 localStorage.setItem('routepilot.locale','en');
 render(<PolicyDNSResult result={{policy_dns:{field:'policy',policy:'Proxy',fallback:'real',fallback_used:true,attempts:[{source:'primary',resolver:'udp://223.6.6.6:53',error:'context deadline exceeded',latency_ms:2000},{source:'real',resolver:'tcp://1.1.1.1:53',rcode:0,latency_ms:12,cache_hit:true}]}}}/>);
 expect(screen.getByText('Global real DNS upstreams',{selector:'td'})).toBeInTheDocument();expect(screen.getByText('context deadline exceeded')).toBeInTheDocument();expect(screen.getByText('tcp://1.1.1.1:53')).toBeInTheDocument();expect(screen.getByText('NOERROR')).toBeInTheDocument();expect(screen.getByText('Hit')).toBeInTheDocument();
});
