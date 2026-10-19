import {LanguageSelector} from './language';
import React from 'react';
import {it,expect,afterEach} from 'vitest';
import {render,screen,fireEvent,cleanup,within} from '@testing-library/react';
import {RoutingPanel} from './routing';
afterEach(()=>{cleanup();localStorage.removeItem('routepilot.routing.locale');localStorage.removeItem('routepilot.locale')});
it('localizes route controls and external state without exposing verbose backend guidance',()=>{
 render(<><LanguageSelector/><RoutingPanel settings={{routing:{mode:'static',fake_ip_backend:'static',dns_backend:'static',static_fake_ip_configured:true,static_dns_interception:false,static_dns_configured:false},dns:{interception_enabled:false}}} snapshot={{fake_ip_backend:'static',fake_ip_ready:true,routes:[{prefix:'198.18.0.0/15',next_hop:'192.0.2.141',kind:'fake-ip',backend:'static',route_state:'operator-confirmed',guidance:'VERY LONG RAW GUIDE'}],limitations:['VERY LONG RAW LIMITATION']}} refresh={()=>{}} notify={()=>{}}/></>);
 expect(screen.queryByText('VERY LONG RAW GUIDE')).not.toBeInTheDocument();expect(screen.queryByText('VERY LONG RAW LIMITATION')).not.toBeInTheDocument();
 expect(screen.getByText('未知（外部）')).toBeInTheDocument();expect(document.querySelector('.routing-probe')).not.toHaveAttribute('open');
 fireEvent.change(screen.getByLabelText(/页面语言|Page language/),{target:{value:'en'}});
 expect(screen.queryByLabelText('Routing mode')).not.toBeInTheDocument();expect(screen.getByText('Unknown (external)')).toBeInTheDocument();expect(screen.queryByLabelText('Fake-IP routes configured on the gateway')).not.toBeInTheDocument();
 expect(screen.queryByText('operator-confirmed')).not.toBeInTheDocument();expect(localStorage.getItem('routepilot.locale')).toBe('en');
});
it('shows route publication state separately from optional local TCP diagnostics',()=>{
 const settings={routing:{mode:'hybrid',fake_ip_backend:'static',dns_backend:'bgp',manual_backend:'static'},dns:{interception_enabled:true}};
 const routes=[
  {prefix:'198.18.0.0/16',kind:'fake-ip',backend:'static',route_state:'operator-confirmed',connectivity:{prefix:'198.18.0.0/16',target:'198.18.1.1',port:443,success:true,checked_at:'2026-10-08T00:00:00Z'}},
  {prefix:'203.0.113.53/32',kind:'dns-interception',backend:'bgp',route_state:'advertised',installed:true},
  {prefix:'203.0.113.0/24',next_hop:'192.0.2.9',kind:'manual',backend:'static',route_state:'declared'},
  {prefix:'198.51.100.0/24',next_hop:'192.0.2.9',kind:'manual',backend:'api',route_state:'api-published',installed:true}
 ];
 render(<RoutingPanel settings={settings} snapshot={{routes,api_requested:true,api:{enabled:true,last_error:''}}} refresh={()=>{}} notify={()=>{}}/>);
 const tables=screen.getAllByRole('table');
 expect(within(tables[0]).getByRole('cell',{name:'已发布'})).toBeInTheDocument();
 expect(within(tables[0]).getByRole('cell',{name:'用户已确认'})).toBeInTheDocument();
 expect(within(tables[0]).queryByRole('columnheader',{name:'本机探测'})).not.toBeInTheDocument();
 expect(within(tables[0]).queryByText('TCP 可达')).not.toBeInTheDocument();
 expect(screen.queryByText('未探测')).not.toBeInTheDocument();
 expect(within(tables[0]).getAllByRole('cell',{name:'手动路由'})).toHaveLength(2);
 expect(within(tables[0]).getAllByRole('cell',{name:'外部静态'})).toHaveLength(2);
 expect(within(tables[0]).getAllByRole('cell',{name:'已发布到路由器'}).length).toBeGreaterThan(0);
 expect(within(tables[1]).getByRole('cell',{name:'198.18.1.1'})).toBeInTheDocument();
 expect(within(tables[1]).getByRole('cell',{name:'TCP 可达'})).toBeInTheDocument();
});
