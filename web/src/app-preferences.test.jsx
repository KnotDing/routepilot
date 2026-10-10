import React from 'react';
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {AppPreferencesPanel} from './app-preferences';

afterEach(()=>{
 cleanup();
 delete window.webkit;
 delete window.routepilotNativePreferences;
 vi.restoreAllMocks();
});

it('keeps the automatic check interval independent from the scheduled install time',async()=>{
 const postMessage=vi.fn(value=>queueMicrotask(()=>window.dispatchEvent(new CustomEvent('routepilot-preferences-saved',{detail:{requestID:value._requestID,ok:true}}))));
 window.webkit={messageHandlers:{routepilotPreferences:{postMessage}}};
 window.routepilotNativePreferences={automaticCheck:true,automaticInstall:true,checkBetaUpdates:false,assistantReady:true,checkIntervalHours:12,updateTime:'03:00'};
 const notify=vi.fn();
 render(<AppPreferencesPanel kind="updates" notify={notify}/>);

 const interval=screen.getByLabelText('自动检查间隔（小时）');
 const time=screen.getByLabelText('自动更新时间（本地时间）');
 const beta=screen.getByLabelText('自动检查 Beta 版本');
 expect(interval).toHaveValue(12);
 expect(time).toHaveValue('03:00');
 expect(beta).not.toBeChecked();
 fireEvent.click(beta);
 fireEvent.change(time,{target:{value:'04:30'}});
 fireEvent.click(screen.getByRole('button',{name:'保存应用偏好'}));

 await waitFor(()=>expect(notify).toHaveBeenCalledWith('应用偏好已保存'));
 expect(postMessage).toHaveBeenCalledWith(expect.objectContaining({checkBetaUpdates:true,checkIntervalHours:12,updateTime:'04:30'}));
});

it('offers immediate installation only after update download is complete',async()=>{
 const postAction=vi.fn();
 window.webkit={messageHandlers:{routepilotPreferences:{postMessage:vi.fn()},routepilotUpdateAction:{postMessage:postAction}}};
 window.routepilotNativePreferences={assistantReady:true,updateAvailable:true,updateVersion:'v1.0.1',updateDownloading:true};
 render(<AppPreferencesPanel kind="updates" notify={()=>{}}/>);

 expect(screen.getByText('已发现可用更新：v1.0.1')).toBeInTheDocument();
 expect(screen.getByRole('button',{name:'正在下载更新…'})).toBeDisabled();

 window.routepilotNativePreferences.updateDownloading=false;
 window.dispatchEvent(new Event('routepilot-preferences-changed'));
 const install=await screen.findByRole('button',{name:'立即安装'});
 expect(install).toBeEnabled();
 fireEvent.click(install);
 expect(postAction).toHaveBeenCalledWith('installAvailable');
});
