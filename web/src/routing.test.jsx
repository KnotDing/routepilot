import {LanguageSelector} from './language';
import React from 'react';
import {it,expect,afterEach} from 'vitest';
import {render,screen,fireEvent,cleanup} from '@testing-library/react';
import {RoutingPanel} from './routing';
afterEach(()=>{cleanup();localStorage.removeItem('routepilot.routing.locale');localStorage.removeItem('routepilot.locale')});
it('localizes route controls and external state without exposing verbose backend guidance',()=>{
 render(<><LanguageSelector/><RoutingPanel settings={{routing:{mode:'static',fake_ip_backend:'static',dns_backend:'static',static_fake_ip_configured:true,static_dns_interception:false,static_dns_configured:false},dns:{interception_enabled:false}}} snapshot={{fake_ip_backend:'static',fake_ip_ready:true,routes:[{prefix:'198.18.0.0/15',next_hop:'192.0.2.141',kind:'fake-ip',backend:'static',route_state:'operator-confirmed',guidance:'VERY LONG RAW GUIDE'}],limitations:['VERY LONG RAW LIMITATION']}} refresh={()=>{}} notify={()=>{}}/></>);
 expect(screen.queryByText('VERY LONG RAW GUIDE')).not.toBeInTheDocument();expect(screen.queryByText('VERY LONG RAW LIMITATION')).not.toBeInTheDocument();
 expect(screen.getByText('未知（外部）')).toBeInTheDocument();expect(document.querySelector('.routing-probe')).not.toHaveAttribute('open');
 fireEvent.change(screen.getByLabelText(/页面语言|Page language/),{target:{value:'en'}});
 expect(screen.getByLabelText('Routing mode')).toHaveValue('static');expect(screen.getByText('Unknown (external)')).toBeInTheDocument();expect(screen.getByLabelText('Fake-IP routes configured on the gateway')).toBeChecked();
 expect(screen.queryByText('operator-confirmed')).not.toBeInTheDocument();expect(localStorage.getItem('routepilot.locale')).toBe('en');
});
