import React from 'react';
import {afterEach,expect,it,vi} from 'vitest';
import {act,cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {LocalAppUpdateControls,usePreferenceDraft,observeServices} from './app-preferences';
function LocalUpdates(){const [draft,setDraft]=usePreferenceDraft(['updatePrompts']);return <><label><input type="checkbox" checked={draft.updatePrompts} onChange={e=>setDraft({...draft,updatePrompts:e.target.checked})}/>发现新版本时提示</label><LocalAppUpdateControls preferences={draft}/></>}

afterEach(()=>{
 cleanup();
 delete window.webkit;
 delete window.routepilotNativePreferences;
 vi.restoreAllMocks();
 vi.unstubAllGlobals();
 localStorage.clear();
});

it('formats cache alarm evidence and does not report recovery while samples are unknown',()=>{
 const delivered=[];
 class FakeNotification{static permission='granted';constructor(title,options){delivered.push({title,...options})}}
 vi.stubGlobal('Notification',FakeNotification);
 localStorage.setItem('routepilot.appPreferences',JSON.stringify({desktopNotifications:true}));
 const settings={enabled:true,errors:['cache_mismatch'],cooldown_seconds:0,recovery:true};
 const observe=state=>observeServices({scope:'cache-alert-test',health:[{name:'DNS cache mismatch decision',state,last_error:state==='DOWN'?'50% ≥ 5% · 100 / 200 · 60s':''}],settings});
 observe('DOWN');observe('DOWN');observe('UNKNOWN');
 expect(delivered).toHaveLength(1);
 expect(delivered[0].body).toContain('决策缓存不一致率过高');
 expect(delivered[0].body).toContain('100 / 200');
 observe('UP');
 expect(delivered).toHaveLength(2);
 expect(delivered[1].title).toBe('RoutePilot · 服务已恢复');
});

it('preserves an unsaved update prompt while native download status changes',async()=>{
 window.webkit={messageHandlers:{routepilotPreferences:{postMessage:vi.fn()}}};
 window.routepilotNativePreferences={updatePrompts:true};
 render(<LocalUpdates/>);
 fireEvent.click(screen.getByLabelText('发现新版本时提示'));
 act(()=>{Object.assign(window.routepilotNativePreferences,{updateAvailable:true,updateDownloading:true,updateVersion:'v9.0.0'});window.dispatchEvent(new Event('routepilot-preferences-changed'))});
 expect(screen.getByLabelText('发现新版本时提示')).not.toBeChecked();
 expect(screen.getByText('已发现可用更新：v9.0.0')).toBeInTheDocument();
});

it('offers immediate installation only after update download is complete',async()=>{
 const postAction=vi.fn();
 window.webkit={messageHandlers:{routepilotPreferences:{postMessage:vi.fn()},routepilotUpdateAction:{postMessage:postAction}}};
 window.routepilotNativePreferences={assistantReady:true,updateAvailable:true,updateVersion:'v1.0.1',updateDownloading:true};
 render(<LocalUpdates/>);

 expect(screen.getByText('已发现可用更新：v1.0.1')).toBeInTheDocument();
 expect(screen.getByRole('button',{name:'正在下载更新…'})).toBeDisabled();

 window.routepilotNativePreferences.updateDownloading=false;
 window.dispatchEvent(new Event('routepilot-preferences-changed'));
 const install=await screen.findByRole('button',{name:'立即安装'});
 expect(install).toBeEnabled();
 fireEvent.click(install);
 expect(postAction).toHaveBeenCalledWith('installAvailable');
});

it('disables duplicate actions and displays native installation bytes',()=>{
 const postAction=vi.fn();
 window.webkit={messageHandlers:{routepilotPreferences:{postMessage:vi.fn()},routepilotUpdateAction:{postMessage:postAction}}};
 window.routepilotNativePreferences={updateAvailable:true,updateDownloading:true,updatePhase:'downloading',updateDownloadBytes:1048576,updateDownloadTotal:2097152};
 render(<LocalUpdates/>);
 expect(screen.getByRole('progressbar')).toHaveAttribute('value','50');
 expect(screen.getByRole('button',{name:'立即检查更新'})).toBeDisabled();
 fireEvent.click(screen.getByRole('button',{name:'正在下载更新…'}));expect(postAction).not.toHaveBeenCalled();
});
