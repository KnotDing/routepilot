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

it('keeps update prompts as the only App update preference',async()=>{
 const postMessage=vi.fn(value=>queueMicrotask(()=>window.dispatchEvent(new CustomEvent('routepilot-preferences-saved',{detail:{requestID:value._requestID,ok:true}}))));
 window.webkit={messageHandlers:{routepilotPreferences:{postMessage}}};
 window.routepilotNativePreferences={assistantReady:true,updatePrompts:true};
 const notify=vi.fn();
 render(<AppPreferencesPanel kind="updates" notify={notify}/>);

 expect(screen.queryByLabelText('自动检查新版本')).not.toBeInTheDocument();
 expect(screen.queryByLabelText('自动安装新版本')).not.toBeInTheDocument();
 expect(screen.queryByLabelText('自动检查 Beta 版本')).not.toBeInTheDocument();
 const prompts=screen.getByLabelText('发现新版本时提示');
 expect(prompts).toBeChecked();
 fireEvent.click(prompts);
 fireEvent.click(screen.getByRole('button',{name:'保存应用偏好'}));

 await waitFor(()=>expect(notify).toHaveBeenCalledWith('应用偏好已保存'));
 expect(postMessage).toHaveBeenCalledWith(expect.objectContaining({updatePrompts:false}));
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
