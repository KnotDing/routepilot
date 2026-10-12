import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,cleanup,waitFor,within,act} from '@testing-library/react';
import {BackendUpdate} from './backend-update';
import {request} from './client';
vi.mock('./client',()=>({request:vi.fn()}));
const backend={id:'remote',name:'Remote fixture'};
const available={supported:true,available:true,current:'0.5.1',latest:'v0.5.2',phase:'idle',release_notes:'## 修复说明\n\n- **重要修复**\n\n[项目](https://gitea.knotding.com/knot/routepilot/releases)\n\n<script>window.untrustedRelease=true</script>\n\n![tracking](https://example.invalid/tracker.png)'};
afterEach(()=>{cleanup();localStorage.clear();vi.useRealTimers();vi.resetAllMocks();delete window.untrustedRelease});
function mount(onReady=vi.fn()){return render(<BackendUpdate backend={backend} capable software={{version:'0.5.1'}} onReady={onReady}/>)}
async function open(){request.mockResolvedValue(available);mount();fireEvent.click(await screen.findByRole('button',{name:'升级远程应用'}));return screen.getByRole('dialog')}
it('hides the update banner when the backend is current',async()=>{
 request.mockResolvedValue({...available,available:false});mount();await waitFor(()=>expect(screen.queryByLabelText('远程软件更新')).not.toBeInTheDocument());expect(request).toHaveBeenCalledWith('updates/check');
});
it('uses one dialog with three choices and safe rendered Markdown',async()=>{
 const dialog=await open();expect(screen.getAllByRole('dialog')).toHaveLength(1);
 for(const name of ['跳过该版本','稍后升级','立即升级'])expect(within(dialog).getByRole('button',{name})).toBeInTheDocument();
 fireEvent.click(within(dialog).getByText('更新说明'));await within(dialog).findByRole('heading',{name:'修复说明'});
 expect(within(dialog).getByText('重要修复').tagName).toBe('STRONG');expect(within(dialog).getByRole('link',{name:'项目'})).toHaveAttribute('href','https://gitea.knotding.com/knot/routepilot/releases');
 expect(dialog.querySelector('img,script,input[type=checkbox]')).toBeNull();expect(window.untrustedRelease).toBeUndefined();
});
it('skips only the selected backend version and allows manual rechecking',async()=>{
 const dialog=await open();request.mockResolvedValue({...available,skipped_version:'v0.5.2'});fireEvent.click(within(dialog).getByRole('button',{name:'跳过该版本'}));
 await waitFor(()=>expect(screen.queryByRole('dialog')).not.toBeInTheDocument());expect(request).toHaveBeenCalledWith('updates/skip','POST',{version:'v0.5.2'});expect(screen.queryByLabelText('远程软件更新')).not.toBeInTheDocument();
 cleanup();request.mockResolvedValue({...available,latest:'v0.5.3'});mount();expect(await screen.findByRole('button',{name:'升级远程应用'})).toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'立即检查更新'}));expect(await screen.findByRole('dialog')).toBeInTheDocument();
});

it('keeps the dialog open and reports a failed durable skip',async()=>{
 const dialog=await open();request.mockRejectedValue(new Error('Persistent update storage is unavailable'));
 fireEvent.click(within(dialog).getByRole('button',{name:'跳过该版本'}));
 expect(await screen.findByRole('alert')).toHaveTextContent('无法使用持久化更新记录');
 expect(screen.getByRole('dialog')).toBeInTheDocument();
});

it('shows a durable skip in settings and can unskip it',async()=>{
 request.mockResolvedValue({...available,skipped_version:'v0.5.2'});
 render(<BackendUpdate backend={backend} capable software={{version:'0.5.1'}} showCurrent/>);
 expect(await screen.findByText('已跳过版本 v0.5.2，不会自动安装')).toBeInTheDocument();
 request.mockResolvedValue(available);fireEvent.click(screen.getByRole('button',{name:'取消跳过'}));
 await waitFor(()=>expect(screen.queryByText('已跳过版本 v0.5.2，不会自动安装')).not.toBeInTheDocument());
 expect(request).toHaveBeenCalledWith('updates/skip','POST',{version:''});
});

it('displays scheduled check failures even when no update is available',async()=>{
 request.mockResolvedValue({...available,available:false,check_error:'release server offline',next_retry_at:'2099-01-01T00:00:00Z'});mount();
 expect(await screen.findByRole('alert')).toHaveTextContent('release server offline');
 expect(screen.getByText(/后重试/)).toBeInTheDocument();
});
it('later and Escape preserve the backend and restore page scrolling',async()=>{
 let dialog=await open();expect(document.body.style.overflow).toBe('hidden');fireEvent.click(within(dialog).getByRole('button',{name:'稍后升级'}));expect(document.body.style.overflow).not.toBe('hidden');
 fireEvent.click(screen.getByRole('button',{name:'升级远程应用'}));dialog=screen.getByRole('dialog');fireEvent.keyDown(document,{key:'Escape'});expect(screen.queryByRole('dialog')).not.toBeInTheDocument();expect(request.mock.calls.some(([path])=>path==='updates/install')).toBe(false);
});
it('posts the exact release once, survives disconnection and completes polling',async()=>{
 const dialog=await open(),onReady=vi.fn();cleanup();
 request.mockImplementation(path=>Promise.resolve(path==='updates/check'?available:path==='updates/install'?{...available,phase:'restarting'}:{...available,available:false,phase:'succeeded'}));mount(onReady);fireEvent.click(await screen.findByRole('button',{name:'升级远程应用'}));
 vi.useFakeTimers();await act(async()=>fireEvent.click(screen.getByRole('button',{name:'立即升级'})));
 expect(request).toHaveBeenCalledWith('updates/install','POST',{version:'v0.5.2'});expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
 request.mockImplementation(path=>path==='updates/status'?Promise.reject(new Error('offline')):Promise.resolve(available));await act(async()=>vi.advanceTimersByTimeAsync(2000));expect(screen.getByText('后台暂时离线，正在等待升级完成并重连…')).toBeInTheDocument();
 request.mockResolvedValue({...available,available:false,phase:'succeeded'});await act(async()=>vi.advanceTimersByTimeAsync(2000));expect(onReady).toHaveBeenCalledTimes(1);expect(screen.queryByLabelText('远程软件更新')).not.toBeInTheDocument();expect(dialog).not.toBeInTheDocument();
});
it('stops polling and asks to log in after authentication expires',async()=>{
 request.mockImplementation(path=>Promise.resolve(path==='updates/install'?{...available,phase:'restarting'}:available));mount();fireEvent.click(await screen.findByRole('button',{name:'升级远程应用'}));vi.useFakeTimers();await act(async()=>fireEvent.click(screen.getByRole('button',{name:'立即升级'})));
 request.mockRejectedValue(Object.assign(new Error('forbidden'),{status:403}));await act(async()=>vi.advanceTimersByTimeAsync(2000));expect(screen.getByRole('alert')).toHaveTextContent('后台已重启，请重新登录查看升级结果');const count=request.mock.calls.length;await act(async()=>vi.advanceTimersByTimeAsync(6000));expect(request.mock.calls.length).toBe(count);
});
it('shows installation errors and ignores responses after unmount',async()=>{
 await open();request.mockRejectedValue(new Error('SHA-256 verification failed'));fireEvent.click(screen.getByRole('button',{name:'立即升级'}));expect(await screen.findByRole('alert')).toHaveTextContent('SHA-256');cleanup();
 let complete;request.mockImplementation(()=>new Promise(resolve=>{complete=resolve}));const view=mount();view.unmount();await act(async()=>complete(available));expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
});

it('upgrades the complete application even when only the installed App is old',async()=>{
 request.mockResolvedValue({...available,current:'0.5.2',app_current:'0.5.1'});mount();
 fireEvent.click(await screen.findByRole('button',{name:'升级远程应用'}));
 expect(screen.getByText(/远端 App：0.5.1/)).toBeInTheDocument();
 expect(within(screen.getByRole('dialog')).getByText(/完整应用和网络服务/)).toBeInTheDocument();
 expect(screen.getByText(/同步更新 App、管理页面和网络服务/)).toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'稍后升级'}));expect(request.mock.calls.some(([path])=>path==='updates/install')).toBe(false);
});

it('continues checking failed App restart state until a manual relaunch clears it',async()=>{
 vi.useFakeTimers();const onReady=vi.fn();request.mockResolvedValue({...available,available:false,phase:'succeeded',app_restart:'failed',app_restart_error:'Remote App did not quit; reopen RoutePilot on the remote Mac'});mount(onReady);
 await act(async()=>{await Promise.resolve()});expect(screen.getByText('远端 App 未能重启，请在远端手动重新打开')).toBeInTheDocument();
 await act(async()=>vi.advanceTimersByTimeAsync(4000));expect(onReady).not.toHaveBeenCalled();
 request.mockResolvedValue({...available,available:false,phase:'succeeded',app_restart:'restarted'});
 await act(async()=>vi.advanceTimersByTimeAsync(2000));expect(onReady).toHaveBeenCalledTimes(1);expect(screen.queryByLabelText('远程软件更新')).not.toBeInTheDocument();
 const count=request.mock.calls.length;await act(async()=>vi.advanceTimersByTimeAsync(6000));expect(request.mock.calls.length).toBe(count);
});

it('shows download bytes from the selected backend without a second install request',async()=>{
 request.mockResolvedValue({...available,phase:'downloading',download_bytes:1048576,download_total:2097152});
 mount();await waitFor(()=>expect(screen.getByRole('progressbar')).toHaveAttribute('value','50'));
 expect(screen.getByRole('button',{name:'升级远程应用'})).toBeDisabled();
 expect(request).toHaveBeenCalledTimes(1);
});
