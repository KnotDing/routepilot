import React from 'react';
import {it,expect,afterEach,vi} from 'vitest';
import {render,screen,fireEvent,cleanup} from '@testing-library/react';
import {SurgeNextHop4Status} from './surge-next-hop';
import {RoutingSettingsFields} from './routing';

afterEach(()=>{cleanup();localStorage.clear()});
it('shows pending, unavailable and localized probe failures without claiming an old address is current',()=>{
 const {rerender}=render(<SurgeNextHop4Status status={{automatic:true,address:'192.0.2.141',detected_address:'192.0.2.142',pending:true}}/>);
 expect(screen.getByRole('status')).toHaveTextContent('已探测到 192.0.2.142，等待旧 Fake-IP 响应到期后应用。');
 rerender(<SurgeNextHop4Status status={{automatic:true,address:'192.0.2.141',error:'Surge VMNET is not running or returned invalid status'}}/>);
 expect(screen.getByRole('alert')).toHaveTextContent('Surge VMNET 未运行或返回了无效状态');
 expect(screen.queryByText(/192.0.2.141/)).not.toBeInTheDocument();
 rerender(<SurgeNextHop4Status/>);
 expect(screen.getByText('当前后端未提供 IPv4 下一跳探测状态。')).toBeInTheDocument();
});
it('requires an explicit static route confirmation when the detected address changes',()=>{
 const onChange=vi.fn();
 render(<RoutingSettingsFields automatic nextHop4={{address:'192.0.2.142'}} draft={{mode:'static',static_fake_ip_configured:true,static_fake_ip_next_hop4:'192.0.2.141'}} onChange={onChange}/>);
 const checkbox=screen.getByRole('checkbox',{name:'Fake-IP 路由已在网关配置'});
 expect(checkbox).not.toBeChecked();
 expect(screen.getByRole('alert')).toHaveTextContent('更新网关静态路由');
 fireEvent.click(checkbox);
 expect(onChange).toHaveBeenCalledWith(expect.objectContaining({static_fake_ip_configured:true,static_fake_ip_next_hop4:''}));
});
