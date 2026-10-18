import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,waitFor,cleanup} from '@testing-library/react';
import {DHCPOptionFields,DHCPPanel,emptyDHCP} from './dhcp';
import {selectBackend} from './client';

afterEach(()=>{cleanup();vi.unstubAllGlobals();vi.restoreAllMocks();selectBackend('');localStorage.removeItem('routepilot.locale')});
const config=()=>({...emptyDHCP(),interface:'en0',server_ip:'192.0.2.10',subnet:'192.0.2.0/24',pool_start:'192.0.2.100',pool_end:'192.0.2.200'});
const status={device_settings:true,supported:true,state:'disabled',received:0,sent:0,leases:[]};

it('keeps every device option independently inheritable and can explicitly omit DNS',()=>{
 const onChange=vi.fn();const global={...emptyDHCP().options,dns:['1.1.1.1'],gateway:'192.0.2.1'};
 const {rerender}=render(<DHCPOptionFields value={{}} globalOptions={global} onChange={onChange}/>);
 expect(screen.getAllByRole('checkbox')).toHaveLength(9);
 fireEvent.click(screen.getByLabelText('继承全局：DNS 服务器地址'));
 expect(onChange).toHaveBeenLastCalledWith({dns:['1.1.1.1']});
 rerender(<DHCPOptionFields value={{dns:['1.1.1.1']}} globalOptions={global} onChange={onChange}/>);
 fireEvent.change(screen.getByRole('textbox',{name:'DNS 服务器地址'}),{target:{value:''}});
 expect(onChange).toHaveBeenLastCalledWith({dns:[]});
 fireEvent.click(screen.getByLabelText('继承全局：DNS 服务器地址'));
 expect(onChange).toHaveBeenLastCalledWith({});
});

it('checks before enabling, shows conflicts and allows cancellation without saving',async()=>{
 const warning='Another DHCP server responded on the selected interface. Multiple DHCP servers may distribute conflicting addresses and options.';
 const fetch=vi.fn(async()=>({ok:true,json:async()=>({interface:'en0',checked_at:new Date().toISOString(),address_mode:'dhcp',servers:[{address:'192.0.2.1'}],warnings:[warning]})}));vi.stubGlobal('fetch',fetch);vi.spyOn(window,'confirm').mockReturnValue(false);
 render(<DHCPPanel settings={{dhcp:config()}} snapshot={status} refresh={vi.fn()} notify={vi.fn()}/>);
 fireEvent.click(screen.getByLabelText('启用 DHCPv4 服务'));fireEvent.click(screen.getByRole('button',{name:'保存 DHCP 配置'}));
 await waitFor(()=>expect(window.confirm).toHaveBeenCalled());
 expect(fetch).toHaveBeenCalledOnce();expect(fetch.mock.calls[0][0]).toBe('/api/v1/dhcp/check');
 expect(screen.getByText('发现 DHCP 服务器：192.0.2.1')).toBeInTheDocument();
});

it('saves DHCP only to the selected backend after successful checks',async()=>{
 selectBackend('target-mac');
 const fetch=vi.fn(async url=>({ok:true,json:async()=>url.endsWith('/check')?{interface:'en0',checked_at:new Date().toISOString(),address_mode:'static',servers:[],warnings:[]}:{ok:true}}));vi.stubGlobal('fetch',fetch);
 const refresh=vi.fn();render(<DHCPPanel settings={{dhcp:config()}} snapshot={status} refresh={refresh} notify={vi.fn()}/>);
 fireEvent.click(screen.getByLabelText('启用 DHCPv4 服务'));fireEvent.click(screen.getByRole('button',{name:'保存 DHCP 配置'}));
 await waitFor(()=>expect(refresh).toHaveBeenCalledOnce());
 expect(fetch.mock.calls.map(([url])=>url)).toEqual(['/api/v1/management/relay/target-mac/dhcp/check','/api/v1/management/relay/target-mac/dhcp/configuration']);
 expect(JSON.parse(fetch.mock.calls[1][1].body)).toMatchObject({enabled:true,interface:'en0'});
});

it('does not overwrite an edited DHCP draft during polling and disables unsupported backends',()=>{
 const props={snapshot:status,refresh:vi.fn(),notify:vi.fn()};const {rerender}=render(<DHCPPanel {...props} settings={{dhcp:config()}}/>);
 fireEvent.change(screen.getByRole('textbox',{name:'DHCP 服务器 IPv4'}),{target:{value:'192.0.2.20'}});
 rerender(<DHCPPanel {...props} settings={{dhcp:{...config(),server_ip:'192.0.2.30'}}}/>);
 expect(screen.getByRole('textbox',{name:'DHCP 服务器 IPv4'})).toHaveValue('192.0.2.20');
 rerender(<DHCPPanel {...props} snapshot={null} settings={{}}/>);
 expect(screen.getByLabelText('启用 DHCPv4 服务')).toBeDisabled();
});

it('edits device properties from the lease list without saving an unrelated global draft',async()=>{
 const fetch=vi.fn(async()=>({ok:true,json:async()=>({ok:true})}));vi.stubGlobal('fetch',fetch);
 const settings=config(),lease={address:'192.0.2.120',mac:'02:00:00:00:00:01',hostname:'Phone',state:'bound',expires_at:'2026-10-19T00:00:00Z'};
 render(<DHCPPanel settings={{dhcp:settings}} snapshot={{...status,leases:[lease]}} refresh={vi.fn()} notify={vi.fn()}/>);
 expect(screen.queryByRole('heading',{name:'设备专属配置'})).not.toBeInTheDocument();
 fireEvent.change(screen.getByRole('textbox',{name:'DHCP 服务器 IPv4'}),{target:{value:'192.0.2.20'}});
 fireEvent.click(screen.getByRole('button',{name:'编辑 DHCP 设备 02:00:00:00:00:01'}));
 fireEvent.change(screen.getByRole('textbox',{name:'设备名称'}),{target:{value:'My iPhone'}});
 fireEvent.click(screen.getByRole('button',{name:'保存设备属性'}));
 await waitFor(()=>expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
 expect(fetch).toHaveBeenCalledOnce();expect(fetch.mock.calls[0][0]).toBe('/api/v1/dhcp/devices');
 expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual([{mac:lease.mac,name:'My iPhone',address:lease.address,options:{}}]);
 expect(screen.getByRole('textbox',{name:'DHCP 服务器 IPv4'})).toHaveValue('192.0.2.20');
 expect(screen.getByText('My iPhone')).toBeInTheDocument();
});

it('keeps configured devices without a lease editable and preserves failed editor drafts',async()=>{
 vi.stubGlobal('fetch',vi.fn(async()=>({ok:false,status:422,json:async()=>({error:'invalid DHCP domain'}),text:async()=> 'invalid DHCP domain'})));
 const device={mac:'02:00:00:00:00:02',name:'Offline',address:'192.0.2.220',options:{}};
 render(<DHCPPanel settings={{dhcp:{...config(),devices:[device]}}} snapshot={status} refresh={vi.fn()} notify={vi.fn()}/>);
 expect(screen.getByText('无有效租约')).toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'编辑 DHCP 设备 '+device.mac}));
 fireEvent.change(screen.getByRole('textbox',{name:'设备名称'}),{target:{value:'Still here'}});
 fireEvent.click(screen.getByRole('button',{name:'保存设备属性'}));
 await screen.findByRole('alert');expect(screen.getByRole('textbox',{name:'设备名称'})).toHaveValue('Still here');
});
