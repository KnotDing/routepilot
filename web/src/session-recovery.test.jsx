import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,waitFor,cleanup,act} from '@testing-library/react';
import {App} from './main';
import {selectBackend,request} from './client';

let streamUpdate;
afterEach(()=>{cleanup();selectBackend('');localStorage.clear();sessionStorage.clear();vi.useRealTimers();vi.unstubAllGlobals();delete window.webkit;delete window.routepilotNativePreferences});
function setup({missing=false,invalid=false,retry=false}={}){
 let authenticated=true,restores=0,rejectOnce=retry;
 const bridge=vi.fn(message=>{
  if(message.action!=='local-login')return;
  restores++;
  let detail={requestID:message.requestID};
  if(rejectOnce){rejectOnce=false;detail={...detail,error:'temporarily unavailable',retryable:true}}
  else if(missing)detail.missing=true;
  else if(invalid)detail.error='保存的本机登录凭证已失效，请重新登录一次';
  else{authenticated=true;detail.session={authenticated:true,csrf:'restored-csrf'}}
  queueMicrotask(()=>window.dispatchEvent(new CustomEvent('routepilot-credentials-result',{detail})));
 });
 window.webkit={messageHandlers:{routepilotPreferences:{postMessage(){}},routepilotCredentials:{postMessage:bridge}}};
 window.routepilotNativePreferences={localCredentialsSupported:true};
 vi.stubGlobal('EventSource',class{addEventListener(name,handler){if(name==='update')streamUpdate=handler}close(){}});
 const fetch=vi.fn(async(url,options)=>{
  const path=String(url).replace('/api/v1/','');
  if(path==='session')return {ok:true,json:async()=>({authenticated,csrf:authenticated?'current-csrf':'anonymous-csrf'})};
  if(path==='logout'){authenticated=false;return {ok:true,json:async()=>({ok:true})}}
  if(!authenticated)return {ok:false,status:401,text:async()=> 'authentication required'};
  const value=path==='management/backends'?[]:path==='settings'?{general:{mode:'auto',theme:'light'},notifications:{},security:{authentication:true},surge:{},dns:{},cache:{}}:path==='status'?{state:'HEALTHY',mode:'auto',counters:{},cache:{decision:{},response:{}}}:[];
  return {ok:true,json:async()=>value};
 });vi.stubGlobal('fetch',fetch);
 return {bridge,fetch,expire(){authenticated=false;streamUpdate()},get restores(){return restores}};
}
it('restores the running local App workspace after a backend restart without exposing credentials',async()=>{
 const server=setup();render(<App/>);await screen.findByRole('heading',{level:1,name:'仪表盘'});
 await act(async()=>server.expire());
 await waitFor(()=>expect(server.restores).toBe(1));
 await screen.findByRole('heading',{level:1,name:'仪表盘'});
 expect(screen.queryByLabelText('密码')).not.toBeInTheDocument();
 expect(server.bridge.mock.calls.find(([message])=>message.action==='local-login')[0]).not.toHaveProperty('password');
 expect(server.fetch.mock.calls.some(([url])=>String(url).endsWith('/login'))).toBe(false);
 await request('mode','PUT',{mode:'auto'});expect(server.fetch.mock.calls.at(-1)[1].headers['X-CSRF-Token']).toBe('restored-csrf');
 // Successful recovery allows another restart in the same management window.
 await act(async()=>server.expire());await waitFor(()=>expect(server.restores).toBe(2));
});
it.each([{missing:true},{invalid:true}])('requires a password when saved credentials cannot authenticate: %j',async options=>{
 const server=setup(options);render(<App/>);await screen.findByRole('heading',{level:1,name:'仪表盘'});
 await act(async()=>server.expire());await screen.findByLabelText('密码');
 expect(server.restores).toBe(1);expect(screen.getByRole('button',{name:'登录',exact:true})).toBeEnabled();
});
it('keeps an explicit logout signed out',async()=>{
 const server=setup();render(<App/>);await screen.findByRole('heading',{level:1,name:'仪表盘'});
 fireEvent.click(screen.getAllByRole('button',{name:'退出',exact:true})[0]);await screen.findByLabelText('密码');
 expect(server.restores).toBe(0);
});
it('retries transient native recovery failures and cancels retries after unmount',async()=>{
 const server=setup({retry:true});const {unmount}=render(<App/>);await screen.findByRole('heading',{level:1,name:'仪表盘'});
 vi.useFakeTimers();await act(async()=>server.expire());
 expect(server.restores).toBe(1);expect(screen.queryByLabelText('密码')).not.toBeInTheDocument();
 await act(async()=>vi.advanceTimersByTimeAsync(3000));expect(server.restores).toBe(2);
 expect(screen.getByRole('heading',{level:1,name:'仪表盘'})).toBeInTheDocument();
 unmount();await act(async()=>vi.advanceTimersByTimeAsync(9000));expect(server.restores).toBe(2);
});

it('retries local relay session recovery when the backend is still starting',async()=>{
 const server=setup();render(<App/>);await screen.findByRole('heading',{level:1,name:'仪表盘'});
 server.fetch.mockImplementationOnce(async()=>{throw new TypeError('backend restarting')});
 vi.useFakeTimers();await act(async()=>window.dispatchEvent(new Event('routepilot-local-session-expired')));
 expect(screen.queryByRole('heading',{level:1,name:'仪表盘'})).not.toBeInTheDocument();
 await act(async()=>vi.advanceTimersByTimeAsync(3000));
 expect(screen.getByRole('heading',{level:1,name:'仪表盘'})).toBeInTheDocument();
});
