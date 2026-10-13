import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,waitFor,cleanup} from '@testing-library/react';
import {Resource} from './main';
import {selectBackend} from './client';

const routing={mode:'static',fake_ip_backend:'static',dns_backend:'static',static_dns_interception:true,static_dns_configured:false};
afterEach(()=>{cleanup();vi.unstubAllGlobals();selectBackend('');localStorage.removeItem('routepilot.locale')});

it('saves the confirmation from the interception page to the connected backend using its latest settings',async()=>{
 selectBackend('remote-mac');
 const latest={general:{mode:'force-direct'},routing:{...routing,static_fake_ip_configured:true},dns:{source4:'192.0.2.2'},cache:{capacity:8192}};
 const fetch=vi.fn(async(url,options)=>({ok:true,json:async()=>options.method==='GET'?latest:{settings:JSON.parse(options.body)}}));
 vi.stubGlobal('fetch',fetch);
 const refresh=vi.fn(),notify=vi.fn();
 render(<Resource page="Interception" rows={[]} settings={{routing}} refresh={refresh} notify={notify}/>);
 fireEvent.click(screen.getByRole('button',{name:'确认网关路由已配置'}));
 await waitFor(()=>expect(notify).toHaveBeenCalledWith('DNS 拦截路由确认已保存'));
 expect(fetch.mock.calls.map(([url,options])=>[url,options.method])).toEqual([
  ['/api/v1/management/relay/remote-mac/settings','GET'],
  ['/api/v1/management/relay/remote-mac/settings','PUT']
 ]);
 expect(JSON.parse(fetch.mock.calls[1][1].body)).toEqual({...latest,routing:{...latest.routing,static_dns_configured:true}});
 expect(refresh).toHaveBeenCalledOnce();
});

it('disables an already confirmed button and enables it again after backend invalidation',()=>{
 const props={page:'Interception',rows:[],refresh:vi.fn(),notify:vi.fn()};
 const fetch=vi.fn();vi.stubGlobal('fetch',fetch);
 const {rerender}=render(<Resource {...props} settings={{routing:{...routing,static_dns_configured:true}}}/>);
 const button=screen.getByRole('button',{name:'✓ 网关路由已确认'});
 expect(button).toBeDisabled();
 fireEvent.click(button);
 expect(fetch).not.toHaveBeenCalled();
 rerender(<Resource {...props} rows={[{id:'resolver',name:'Resolver',prefix:'192.0.2.53/32',enabled:true}]} settings={{routing}}/>);
 expect(screen.getByRole('button',{name:'确认网关路由已配置'})).toBeEnabled();
});

it('keeps failed saves unconfirmed and reports the failure',async()=>{
 const fetch=vi.fn().mockResolvedValueOnce({ok:true,json:async()=>({routing})}).mockResolvedValueOnce({ok:false,status:409,text:async()=>JSON.stringify({error:'another configuration save is in progress'})});
 vi.stubGlobal('fetch',fetch);
 const notify=vi.fn(),refresh=vi.fn();
 render(<Resource page="Interception" rows={[]} settings={{routing}} refresh={refresh} notify={notify}/>);
 fireEvent.click(screen.getByRole('button',{name:'确认网关路由已配置'}));
 await waitFor(()=>expect(notify).toHaveBeenCalledWith('another configuration save is in progress',true));
 expect(screen.getByRole('button',{name:'确认网关路由已配置'})).toBeEnabled();
 expect(refresh).not.toHaveBeenCalled();
});

it('hides static confirmation for BGP or without persistent interception permission',()=>{
 const props={page:'Interception',rows:[],refresh:()=>{},notify:()=>{}};
 const {rerender}=render(<Resource {...props} settings={{routing:{...routing,mode:'bgp'}}}/>);
 expect(screen.queryByRole('button',{name:'确认网关路由已配置'})).not.toBeInTheDocument();
 rerender(<Resource {...props} settings={{routing:{...routing,static_dns_interception:false}}}/>);
 expect(screen.queryByRole('button',{name:'确认网关路由已配置'})).not.toBeInTheDocument();
 cleanup();
 localStorage.setItem('routepilot.locale','en');
 render(<Resource {...props} settings={{routing:{...routing,mode:'hybrid',fake_ip_backend:'bgp'}}}/>);
 expect(screen.getByRole('button',{name:'Confirm gateway routes are configured'})).toBeEnabled();
 expect(screen.getByRole('button',{name:'Confirm gateway routes are configured'})).toHaveAttribute('title',expect.stringContaining('This is a user declaration'));
});

it('does not write a stale declaration when the backend has switched to BGP',async()=>{
 const fetch=vi.fn().mockResolvedValue({ok:true,json:async()=>({routing:{...routing,mode:'bgp'}})});
 vi.stubGlobal('fetch',fetch);
 const notify=vi.fn();
 render(<Resource page="Interception" rows={[]} settings={{routing}} refresh={()=>{}} notify={notify}/>);
 fireEvent.click(screen.getByRole('button',{name:'确认网关路由已配置'}));
 await waitFor(()=>expect(notify).toHaveBeenCalledWith('DNS 拦截路由配置已变更，请刷新后重新确认。',true));
 expect(fetch).toHaveBeenCalledOnce();
});
