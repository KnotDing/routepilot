import React from 'react';
import {it,expect,afterEach,vi} from 'vitest';
import {render,screen,fireEvent,cleanup,waitFor} from '@testing-library/react';
import {StatusBarSettings} from './status-bar-settings';
afterEach(()=>{cleanup();delete window.webkit;delete window.routepilotNativePreferences;localStorage.clear();vi.restoreAllMocks()});
it('explains App-only status bar settings in browsers',()=>{
 render(<StatusBarSettings notify={()=>{}}/>);
 expect(screen.getByText('请在 RoutePilot App 中配置状态栏。')).toBeInTheDocument();
 expect(screen.getByLabelText('启用图标动画')).toBeDisabled();
});
it('saves animation and selected values together while preserving update preferences',async()=>{
 const postMessage=vi.fn(value=>queueMicrotask(()=>window.dispatchEvent(new CustomEvent('routepilot-preferences-saved',{detail:{requestID:value._requestID,ok:true}})))),notify=vi.fn();window.webkit={messageHandlers:{routepilotPreferences:{postMessage}}};window.routepilotNativePreferences={automaticCheck:false,notificationSound:false};
 render(<StatusBarSettings notify={notify}/>);
 fireEvent.click(screen.getByLabelText('启用图标动画'));
 fireEvent.change(screen.getByLabelText('图标旁显示'),{target:{value:'health_value'}});
 fireEvent.change(screen.getByLabelText('健康检查名称'),{target:{value:'BGP peer 192.0.2.1'}});
 fireEvent.change(screen.getByLabelText('健康检查数值'),{target:{value:'failures'}});
 fireEvent.click(screen.getByRole('button',{name:'保存状态栏设置'}));
 await waitFor(()=>expect(notify).toHaveBeenCalledWith('应用偏好已保存'));
 expect(postMessage).toHaveBeenCalledWith(expect.objectContaining({automaticCheck:false,notificationSound:false,statusBar:{animation:false,display:'health_value',metric:'queries_per_second',health:'BGP peer 192.0.2.1',healthValue:'failures'}}));
});
it('offers QPS, metric and health-state display choices',()=>{
 window.webkit={messageHandlers:{routepilotPreferences:{postMessage:vi.fn()}}};render(<StatusBarSettings notify={()=>{}}/>);
 fireEvent.change(screen.getByLabelText('图标旁显示'),{target:{value:'metric'}});
 expect(screen.getByLabelText('显示指标')).toHaveValue('queries_per_second');
 fireEvent.change(screen.getByLabelText('图标旁显示'),{target:{value:'health_state'}});
 expect(screen.getByLabelText('健康检查名称')).toBeRequired();
 expect(screen.queryByLabelText('健康检查数值')).not.toBeInTheDocument();
});
