import React from 'react';
import {it,expect,afterEach,vi} from 'vitest';
import {render,screen,fireEvent,cleanup} from '@testing-library/react';
import {Settings,Resource,Field} from './main';
import {FieldLabel,settingsRequirement} from './form-fields';

afterEach(()=>{cleanup();localStorage.clear();vi.unstubAllGlobals()});
it('keeps accessible names stable and highlights empty required controls',()=>{
 const {rerender}=render(<FieldLabel label="名称" required><input value="" readOnly/></FieldLabel>);
 expect(screen.getByLabelText('名称')).toBeRequired();
 expect(screen.getByLabelText('名称').closest('label')).toHaveAttribute('data-missing','true');
 expect(screen.getByText('必填')).toBeInTheDocument();
 rerender(<FieldLabel label="名称" required><input value="Controller" readOnly/></FieldLabel>);
 expect(screen.getByLabelText('名称').closest('label')).not.toHaveAttribute('data-missing');
});
it('marks Router ID only after enabling BGP',()=>{
 render(<Settings settings={{general:{name:'Test'},bgp:{enabled:false,router_id:''},routing:{mode:'bgp'}}} notify={()=>{}} refresh={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'BGP',exact:true}));
 expect(screen.getByLabelText('Router ID')).not.toBeRequired();
 fireEvent.click(screen.getByLabelText('启用'));
 expect(screen.getByLabelText('Router ID')).toBeRequired();
 expect(screen.getByLabelText('Router ID').closest('label')).toHaveAttribute('data-missing','true');
});
it('marks transparent listeners, ingress and the matching network family when enabled',()=>{
 render(<Settings settings={{general:{name:'Test'},dns:{interception_enabled:false,listeners:['127.0.0.1:1053'],transparent_listeners:[],ingress_interfaces:[],egress_interface:'',source4:'',source6:'',gateway4:'',gateway6:'',upstreams:[]}}} interception={[{enabled:true,prefix:'192.0.2.53/32'}]} notify={()=>{}} refresh={()=>{}}/>);
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
