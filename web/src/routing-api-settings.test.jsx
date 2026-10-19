import React from 'react';
import {it,expect,afterEach} from 'vitest';
import {render,screen,fireEvent,cleanup,within} from '@testing-library/react';
import {RoutingSettingsFields,RoutingPanel} from './routing';
import {RoutingAPISettings} from './routing-api-settings';
afterEach(()=>{cleanup();localStorage.removeItem('routepilot.routing.locale');localStorage.removeItem('routepilot.locale')});

function Harness({children,initial}){
 const [draft,setDraft]=React.useState(initial);
 return children(draft,setDraft);
}

it('offers the API routing mode and backend option',()=>{
 render(<Harness initial={{mode:'bgp',fake_ip_backend:'bgp',dns_backend:'bgp',static_fake_ip_configured:false,static_dns_interception:false,static_dns_configured:false}}>
  {(draft,setDraft)=>{draft.mode;return <RoutingSettingsFields draft={draft} onChange={setDraft}/>}}
 </Harness>);
 const mode=screen.getByLabelText(/路由模式|Routing mode/);
 expect(within(mode).queryByRole('option',{name:'API'})).toBeInTheDocument();
 fireEvent.change(mode,{target:{value:'hybrid'}});
 const backend=screen.getByLabelText(/Fake-IP 路由方式|Fake-IP routing method/);
 expect(within(backend).queryByRole('option',{name:'API'})).toBeInTheDocument();
 fireEvent.change(backend,{target:{value:'api'}});
 const dns=screen.getByLabelText(/DNS 拦截路由方式|DNS interception method/);
 fireEvent.change(dns,{target:{value:'api'}});
});

it('renders router API settings with preset auth fields',()=>{
 const draft={preset:'openwrt',base_url:'http://192.168.1.1',username:'root',password:'',password_configured:true,verify_tls:true};
 render(<RoutingAPISettings draft={draft} onChange={()=>{}}/>);
 expect(screen.getByLabelText(/路由器地址|Router address/)).toHaveValue('http://192.168.1.1');
 expect(screen.getByLabelText(/用户名|Username/)).toHaveValue('root');
 expect(screen.getByLabelText(/密码（留空保留）|Password \(leave blank to keep\)/)).toBeInTheDocument();
 expect(screen.getByRole('checkbox',{name:/验证 HTTPS 证书|Verify HTTPS certificate/})).toBeChecked();
 // The custom template editors stay hidden for a preset.
 expect(screen.queryByText(/添加路由请求|Add route request/)).not.toBeInTheDocument();
});

it('exposes custom request templates with variable validation',()=>{
 const draft={preset:'custom',auth:'header',base_url:'http://192.168.1.1',header_name:'X-API-KEY',header_value:'k',add:{method:'POST',path:'/routes',body:'{"name":"{{name}}","network":"{{prefix}}"}'},remove:{method:'DELETE',path:'/routes/{{id}}'},list:{method:'GET',path:'/routes',id_field:'id',name_field:'name'}};
 render(<RoutingAPISettings draft={draft} onChange={()=>{}}/>);
 expect(screen.getByText(/添加路由请求|Add route request/)).toBeInTheDocument();
 expect(screen.getByText(/删除路由请求|Remove route request/)).toBeInTheDocument();
 expect(screen.getByText(/列表请求（用于对比与恢复）|List request \(reconciliation\)/)).toBeInTheDocument();
 expect(screen.getByText(/\{\{name\}\} \{\{prefix\}\}/)).toBeInTheDocument();
});

it('prefills the preset base address and auth when switching',()=>{
 render(<Harness initial={{preset:'',base_url:''}}>
  {(draft,setDraft)=>{draft.preset;return <RoutingAPISettings draft={draft} onChange={setDraft}/>}}
 </Harness>);
 fireEvent.change(screen.getByLabelText(/API 预设|API preset/),{target:{value:'unifi'}});
 expect(screen.getByLabelText(/API 密钥|API key/)).toBeInTheDocument();
 expect(screen.queryByDisplayValue('https://192.168.1.1')).toBeInTheDocument();
});

it('shows API publication state in the routing panel',()=>{
 const settings={routing:{mode:'api',fake_ip_backend:'api',dns_backend:'api'},dns:{interception_enabled:true}};
 const snapshot={mode:'api',fake_ip_backend:'api',dns_backend:'api',fake_ip_ready:true,api_requested:true,api:{enabled:true,last_error:'',published:1,routes:[{name:'routepilot-fakeip-198.18.0.0-15',present:true}]},routes:[{prefix:'198.18.0.0/15',next_hop:'192.0.2.2',kind:'fake-ip',backend:'api',route_state:'api-published',installed:true},{prefix:'203.0.113.53/32',kind:'dns-interception',backend:'api',route_state:'api-pending',installed:false}]};
 render(<RoutingPanel settings={settings} snapshot={snapshot} refresh={()=>{}} notify={()=>{}}/>);
 expect(screen.getByText(/路由器 API|Router API/)).toBeInTheDocument();
 expect(screen.getByText(/已连接|Connected/)).toBeInTheDocument();
 expect(screen.getAllByText(/已发布到路由器|Published to router/).length).toBeGreaterThan(0);
 expect(screen.getAllByText(/待发布|Pending publication/).length).toBeGreaterThan(0);
 expect(screen.getByText(/转发平面|forwarding plane/)).toBeInTheDocument();
});

it('surfaces router API errors in the panel',()=>{
 const settings={routing:{mode:'api',fake_ip_backend:'api',dns_backend:'api'},dns:{interception_enabled:false}};
 const snapshot={api_requested:true,api:{enabled:true,last_error:'OpenWrt ubus call rejected with code 6'},routes:[]};
 render(<RoutingPanel settings={settings} snapshot={snapshot} refresh={()=>{}} notify={()=>{}}/>);
 expect(screen.getByRole('alert')).toHaveTextContent('OpenWrt ubus call rejected with code 6');
 expect(screen.getByText(/异常|Error/)).toBeInTheDocument();
});
