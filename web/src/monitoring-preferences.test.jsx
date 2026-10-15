import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,cleanup,act,waitFor} from '@testing-library/react';
import {clientDisplaySettings} from './clients';
import {MonitoringPanel} from './monitoring';
import {MonitoringSettings,monitoringCards,readMonitoringPreferences,saveMonitoringPreferences} from './monitoring-preferences';
import {request} from './client';
vi.mock('./client',()=>({request:vi.fn()}));

const ip='192.0.2.10',ipv6='2001:db8::5',remark='Private office workstation',policy='Private group',final='Private upstream node';
const counts={started_at:'2026-10-12T00:00:00Z',session_started_at:'session',observed_at:'2026-10-12T00:01:00Z',received:10,completed:10,in_flight:0,unconfirmed:0,latency_ms_sum:30,decisions:{SURGE:10},response_codes:{NOERROR:10},flows:[{ingress:'addressed',destination:'surge',queries:10}],policies:[{policy,final_policy:final,destination:'surge',queries:10}]};
const snapshot={...counts,sources:[{source:ip,counts},{source:ipv6,counts:{...counts,received:0,completed:0,flows:[],policies:[]}}],source_policy_flows:[{source:ip,policy,final_policy:final,kind:'named',final_kind:'named',queries:10}]};
const props={snapshot,clientSettings:[{ip,name:remark}],backendName:'Private backend',backendURL:'https://192.0.2.1:1055',onFocus:vi.fn()};
afterEach(()=>{cleanup();localStorage.clear();vi.restoreAllMocks();vi.resetAllMocks();delete window.webkit;delete window.routepilotNativePreferences});
const assertProtected=container=>{for(const secret of [ip,ipv6,remark,policy,final,props.backendName,props.backendURL])expect(container.innerHTML).not.toContain(secret)};

it('saves individual cards, retains them after reopening and leaves counts unchanged',async()=>{
 const notify=vi.fn(),settings=render(<MonitoringSettings notify={notify}/>);
 expect(screen.getAllByRole('checkbox')).toHaveLength(monitoringCards.length+1);
 fireEvent.click(screen.getByRole('checkbox',{name:'DNS 请求流向'}));fireEvent.click(screen.getByRole('checkbox',{name:'平均处理耗时'}));
 fireEvent.click(screen.getByRole('button',{name:'保存设置'}));await waitFor(()=>expect(notify).toHaveBeenCalledWith('大屏监控设置已保存'));
 settings.unmount();const panel=render(<MonitoringPanel {...props}/>);
 expect(screen.queryByRole('heading',{name:'DNS 请求流向'})).toBeNull();expect(screen.queryByText('平均处理耗时')).toBeNull();
 expect(screen.getByRole('heading',{name:'客户端策略流向'})).toBeInTheDocument();expect(document.querySelector('.monitor-metrics')).toHaveTextContent('已生成响应10');
 panel.unmount();render(<MonitoringSettings notify={notify}/>);expect(screen.getByRole('checkbox',{name:'DNS 请求流向'})).not.toBeChecked();
});

it('protects IPv4, IPv6, remarks, policy names, attributes and errors without breaking source filters',async()=>{
 await saveMonitoringPreferences({protectSensitive:true});request.mockResolvedValue(counts);
 const {container}=render(<MonitoringPanel {...props} historyStatus={{storage_error:`Cannot write ${ip} via ${props.backendURL}`}}/>);
 assertProtected(container);expect(screen.getAllByText('客户端 1').length).toBeGreaterThan(1);
 expect(document.querySelector('.monitor-source-label[role=button]')).toBeNull();
 fireEvent.change(screen.getByRole('combobox',{name:'请求来源'}),{target:{value:'客户端 1'}});
 await waitFor(()=>expect(request).toHaveBeenCalledWith('dns/monitoring?source=192.0.2.10'));
 await screen.findByText('当前统计来源：客户端 1');assertProtected(container);
 expect(screen.getByRole('combobox',{name:'请求来源'})).toHaveValue('客户端 1');
 await act(()=>saveMonitoringPreferences({protectSensitive:false}));expect(container.innerHTML).toContain(ip);expect(container.innerHTML).toContain(remark);
});

it('protects historical data and all hidden detail tables using the same aliases',async()=>{
 await saveMonitoringPreferences({protectSensitive:true});
 const history={summary:snapshot,sources:snapshot.sources,source_policy_flows:snapshot.source_policy_flows,points:[{from:counts.started_at,to:counts.observed_at,seconds:60,rate_valid:true,counts}],from:counts.started_at,to:counts.observed_at,total_received:10};
 request.mockResolvedValue(history);
 const {container}=render(<MonitoringPanel {...props} historyStatus={{supported:true}}/>);
 fireEvent.click(screen.getByRole('button',{name:'历史数据'}));await screen.findByText(/历史保留七天/);assertProtected(container);
 for(const detail of container.querySelectorAll('details'))detail.open=true;
 assertProtected(container);expect(container.querySelector('.monitor-client-policy')).toHaveTextContent('策略 1');expect(container.querySelector('.monitor-policies')).toHaveTextContent('策略 1');
});

it('handles all cards hidden, cross-window changes and failed reads without exposing identities',async()=>{
 await saveMonitoringPreferences({hiddenCards:monitoringCards.map(([key])=>key),protectSensitive:true});
 const {container}=render(<MonitoringPanel {...props}/>);expect(container.querySelector('.monitor-screen .panel')).toBeNull();expect(screen.getByText(/所有卡片已隐藏/)).toBeInTheDocument();
 localStorage.setItem('routepilot.monitoringPreferences','{invalid');act(()=>window.dispatchEvent(new StorageEvent('storage',{key:'routepilot.monitoringPreferences'})));
 expect(screen.getByRole('alert')).toHaveTextContent('大屏监控设置读取失败');expect(screen.getByRole('heading',{name:'DNS 请求流向'})).toBeInTheDocument();assertProtected(container);
});

it('clears a previous raw policy filter immediately when protection is enabled',async()=>{
 const {container}=render(<MonitoringPanel {...props}/>);
 fireEvent.change(screen.getByRole('textbox',{name:'筛选策略'}),{target:{value:policy}});
 await act(()=>saveMonitoringPreferences({protectSensitive:true}));
 expect(screen.getByRole('textbox',{name:'筛选策略'})).toHaveValue('');assertProtected(container);
});

it('reports storage write failure and preserves the last saved settings',async()=>{
 const notify=vi.fn();render(<MonitoringSettings notify={notify}/>);fireEvent.click(screen.getByRole('checkbox',{name:'启用敏感数据保护'}));
 vi.spyOn(Storage.prototype,'setItem').mockImplementation(()=>{throw Error('storage unavailable')});
 fireEvent.click(screen.getByRole('button',{name:'保存设置'}));await waitFor(()=>expect(notify).toHaveBeenCalledWith('大屏监控设置保存失败：storage unavailable',true));expect(readMonitoringPreferences().protectSensitive).toBe(false);
});

it('persists App preferences through the acknowledged native bridge and refuses an older unsupported bridge',async()=>{
 window.routepilotNativePreferences={monitoring:{hiddenCards:[],protectSensitive:false},desktopNotifications:false,notificationSound:false,updatePrompts:false};
 const postMessage=vi.fn(value=>{window.routepilotNativePreferences={...value};queueMicrotask(()=>window.dispatchEvent(new CustomEvent('routepilot-preferences-saved',{detail:{requestID:value._requestID,ok:true}})))});
 window.webkit={messageHandlers:{routepilotPreferences:{postMessage}}};
 await saveMonitoringPreferences({hiddenCards:['sources'],protectSensitive:true});
 expect(readMonitoringPreferences()).toMatchObject({hiddenCards:['sources'],protectSensitive:true});expect(localStorage.length).toBe(0);expect(postMessage.mock.calls[0][0].notificationSound).toBe(false);expect(postMessage.mock.calls[0][0].updatePrompts).toBe(false);
 delete window.routepilotNativePreferences.monitoring;await expect(saveMonitoringPreferences({protectSensitive:false})).rejects.toThrow('更新 App');expect(postMessage).toHaveBeenCalledTimes(1);expect(readMonitoringPreferences().protectSensitive).toBe(true);
});

it('uses detected hostnames as fallback names and protects them with monitoring privacy',async()=>{
 const hostname='detected-desktop.lan';
 const rows=[{client_ip:ip,hostname}];
 const configured=[{ip,name:remark}];
 expect(clientDisplaySettings(configured,rows)[0].name).toBe(remark);
 const clientSettings=clientDisplaySettings([{ip,name:''}],rows);
 const {container}=render(<MonitoringPanel {...props} clientSettings={clientSettings}/>);
 expect(container.textContent).toContain(hostname);
 await act(()=>saveMonitoringPreferences({protectSensitive:true}));
 expect(container.innerHTML).not.toContain(hostname);
 expect(container.innerHTML).not.toContain(ip);
});
