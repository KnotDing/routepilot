import React from 'react';
import {it,expect,afterEach,vi} from 'vitest';
import {render,screen,fireEvent,cleanup,waitFor} from '@testing-library/react';
import {Settings,Resource,Field} from './main';
import {FieldLabel,settingsRequirement} from './form-fields';

afterEach(()=>{cleanup();localStorage.clear();vi.unstubAllGlobals()});
it('does not expose a legacy backend name in general settings',()=>{
 render(<Settings settings={{general:{name:'Legacy name',web_listen:'127.0.0.1:1055',mode:'auto',theme:'system'}}} notify={()=>{}} refresh={()=>{}}/>);
 expect(screen.queryByLabelText('名称')).not.toBeInTheDocument();
 expect(screen.getByLabelText('管理监听地址')).toBeInTheDocument();
});
it('keeps accessible names stable and highlights empty required controls',()=>{
 const {rerender}=render(<FieldLabel label="名称" required><input value="" readOnly/></FieldLabel>);
 expect(screen.getByLabelText('名称')).toBeRequired();
 expect(screen.getByLabelText('名称').closest('label')).toHaveAttribute('data-missing','true');
 expect(screen.getByText('必填')).toBeInTheDocument();
 rerender(<FieldLabel label="名称" required><input value="Controller" readOnly/></FieldLabel>);
 expect(screen.getByLabelText('名称').closest('label')).not.toHaveAttribute('data-missing');
});
it('shows BGP settings only for BGP-capable routing modes',()=>{
 render(<Settings settings={{general:{},bgp:{enabled:false,router_id:''},routing:{mode:'static',fake_ip_backend:'static',dns_backend:'static'}}} notify={()=>{}} refresh={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'路由',exact:true}));
 expect(screen.queryByRole('heading',{name:'BGP'})).not.toBeInTheDocument();
 fireEvent.change(screen.getByLabelText('路由模式'),{target:{value:'bgp'}});
 expect(screen.getByRole('heading',{name:'BGP'})).toBeInTheDocument();
 expect(screen.getByLabelText('Router ID')).not.toBeRequired();
 fireEvent.click(screen.getByLabelText('启用'));
 expect(screen.getByLabelText('Router ID')).toBeRequired();
 expect(screen.getByLabelText('Router ID').closest('label')).toHaveAttribute('data-missing','true');
 fireEvent.change(screen.getByLabelText('路由模式'),{target:{value:'hybrid'}});
 expect(screen.getByLabelText('Router ID')).toBeInTheDocument();
 fireEvent.change(screen.getByLabelText('路由模式'),{target:{value:'static'}});
 expect(screen.queryByRole('heading',{name:'BGP'})).not.toBeInTheDocument();
});
it('marks transparent listeners, ingress and the matching network family when enabled',()=>{
 render(<Settings settings={{general:{},dns:{interception_enabled:false,listeners:['127.0.0.1:1053'],transparent_listeners:[],ingress_interfaces:[],egress_interface:'',source4:'',source6:'',gateway4:'',gateway6:'',upstreams:[]}}} interception={[{enabled:true,prefix:'192.0.2.53/32'}]} notify={()=>{}} refresh={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'DNS',exact:true}));
 expect(screen.getByLabelText('接收接口（每行一项）')).toBeDisabled();
 fireEvent.click(screen.getByLabelText('启用透明拦截'));
 expect(screen.getByLabelText('接收接口（每行一项）')).toBeRequired();
 expect(screen.getByLabelText('透明 DNS 监听地址')).toBeRequired();
 expect(screen.getByLabelText('IPv4 出口源地址')).toBeRequired();
 expect(screen.getByLabelText('IPv4 网关')).toBeRequired();
 expect(screen.getByLabelText('IPv6 网关')).not.toBeRequired();
});
it('requests an initial password only when authentication is enabled and none is saved',()=>{
 const draft={security:{authentication:false,username:'admin',password:'',password_configured:false}};
 expect(settingsRequirement('security','password',draft).required).toBe(false);
 draft.security.authentication=true;
 expect(settingsRequirement('security','password',draft).required).toBe(true);
 draft.security.password_configured=true;
 expect(settingsRequirement('security','password',draft).required).toBe(false);
});
it('marks resource requirements and hides irrelevant transport fields',()=>{
 render(<Resource page="Hosts" rows={[]} refresh={()=>{}} notify={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'＋ 添加'}));
 expect(screen.getByLabelText('域名')).toBeRequired();
 expect(screen.getByLabelText('IP 地址（每行一项）')).toBeRequired();
 cleanup();
 render(<Field name="upstreams" value={[{protocol:'udp',address:'',enabled:true}]} onChange={()=>{}}/>);
 expect(screen.getByLabelText('上游服务器地址')).toBeRequired();
 expect(screen.queryByLabelText('TLS 证书名称')).not.toBeInTheDocument();
 expect(screen.queryByLabelText('DoH HTTPS 地址')).not.toBeInTheDocument();
});

it('edits hostname probing in logging and privacy for the selected backend',async()=>{
 const fetch=vi.fn().mockResolvedValue({ok:true,json:async()=>({})});vi.stubGlobal('fetch',fetch);
 render(<Settings settings={{general:{},logging:{level:'info',probe_hostnames:false}}} notify={()=>{}} refresh={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'日志与隐私',exact:true}));
 const toggle=screen.getByRole('checkbox',{name:'探测主机名'});
 expect(toggle).not.toBeChecked();
 fireEvent.click(toggle);
 expect(toggle).toBeChecked();
 expect(screen.getByText(/当前后端使用其系统 DNS/)).toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'保存设置',exact:true}));
 await waitFor(()=>expect(fetch).toHaveBeenCalled());
 expect(fetch.mock.calls[0][0]).toBe('/api/v1/settings');
 expect(JSON.parse(fetch.mock.calls[0][1].body).logging.probe_hostnames).toBe(true);
});
