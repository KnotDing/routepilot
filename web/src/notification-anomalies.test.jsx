import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,waitFor,cleanup} from '@testing-library/react';
import {Settings} from './main';
import {NotificationAnomalies} from './notification-anomalies';
afterEach(()=>{cleanup();vi.unstubAllGlobals();localStorage.removeItem('routepilot.locale')});

it('saves adjustable thresholds and anomaly selections with the connected backend settings',async()=>{
 const settings={general:{web_listen:'127.0.0.1:1055'},security:{tls_cert:''},notifications:{enabled:true,errors:[],cooldown_seconds:300}};
 const fetch=vi.fn(async(url,options)=>({ok:true,json:async()=>({settings:JSON.parse(options.body)})}));vi.stubGlobal('fetch',fetch);
 render(<Settings settings={settings} status={{notification_anomaly_alerts:true}} notify={()=>{}} refresh={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'通知',exact:true}));
 expect(screen.getByLabelText('缓存不一致率阈值（%）')).toHaveValue(5);
 expect(screen.getByLabelText('缓存最少核对请求数')).toHaveValue(100);
 expect(screen.getByLabelText('大量请求持续时间（秒）')).toHaveValue(300);
 fireEvent.click(screen.getByLabelText('缓存不一致率过高'));
 fireEvent.click(screen.getByLabelText('突发大量 DNS 请求'));
 fireEvent.click(screen.getByLabelText('长时间大量 DNS 请求'));
 fireEvent.change(screen.getByLabelText('突发请求阈值（请求/秒）'),{target:{value:'750'}});
 fireEvent.click(screen.getByRole('button',{name:'保存设置'}));
 await waitFor(()=>expect(fetch).toHaveBeenCalledOnce());
 const submitted=JSON.parse(fetch.mock.calls[0][1].body);
 expect(submitted.notifications.errors).toEqual(['cache_mismatch','dns_request_burst','dns_request_sustained']);
 expect(submitted.notifications.anomalies).toEqual({cache_mismatch_percent:5,cache_window_seconds:60,cache_minimum_compared:100,burst_qps:750,burst_window_seconds:10,sustained_qps:100,sustained_seconds:300});
});

it('keeps existing notifications usable on older backends without offering unsupported errors',()=>{
 render(<Settings settings={{notifications:{enabled:true,errors:[]}}} notify={()=>{}} refresh={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'通知',exact:true}));
 expect(screen.queryByLabelText('缓存不一致率过高')).not.toBeInTheDocument();
 expect(screen.getByLabelText('Surge CLI 读取或执行失败')).toBeInTheDocument();
 expect(screen.getByText('当前后端不支持缓存与请求量告警，请升级后端后使用。')).toBeInTheDocument();
});

it('localizes anomaly fields without changing their numeric defaults',()=>{
 localStorage.setItem('routepilot.locale','en');
 render(<NotificationAnomalies supported settings={{cache_mismatch_percent:0}} onChange={()=>{}}/>);
 expect(screen.getByLabelText('Cache mismatch threshold (%)')).toHaveValue(5);
 expect(screen.getByLabelText('Burst threshold (requests/second)')).toHaveValue(500);
});
