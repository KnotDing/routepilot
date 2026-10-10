import React from 'react';
import {afterEach,expect,it,vi} from 'vitest';
import {act,cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {AppPreferencesPanel,LocalAppUpdateControls,usePreferenceDraft} from './app-preferences';
function LocalUpdates(){const [draft,setDraft]=usePreferenceDraft(['updatePrompts']);return <><label><input type="checkbox" checked={draft.updatePrompts} onChange={e=>setDraft({...draft,updatePrompts:e.target.checked})}/>发现新版本时提示</label><LocalAppUpdateControls preferences={draft}/></>}

afterEach(()=>{
 cleanup();
 delete window.webkit;
 delete window.routepilotNativePreferences;
 vi.restoreAllMocks();
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

it('preserves unsaved notification options while updating metadata',()=>{
 window.webkit={messageHandlers:{routepilotPreferences:{postMessage:vi.fn()}}};
 window.routepilotNativePreferences={desktopNotifications:false,notificationSound:true};
 render(<AppPreferencesPanel notify={()=>{}}/>);
 fireEvent.click(screen.getByLabelText('启用本机系统通知'));
 fireEvent.click(screen.getByLabelText('通知声音'));
 act(()=>{window.routepilotNativePreferences.updateDownloading=true;window.dispatchEvent(new Event('routepilot-preferences-changed'))});
 expect(screen.getByLabelText('启用本机系统通知')).toBeChecked();
 expect(screen.getByLabelText('通知声音')).not.toBeChecked();
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
