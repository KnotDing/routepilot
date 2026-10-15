import {it,expect,vi,afterEach} from 'vitest';
import {request,requestOptional,localRequest,setCSRF,selectBackend} from './client';
afterEach(()=>{selectBackend('');vi.unstubAllGlobals()});
it('keeps local CSRF and connection configuration separate from remote APIs',async()=>{
 const fetch=vi.fn().mockResolvedValue({ok:true,json:async()=>({ok:true})});vi.stubGlobal('fetch',fetch);
 setCSRF('local-csrf');selectBackend('remote-one','https://192.0.2.10:1055');setCSRF('remote-csrf');
 await request('mode','PUT',{mode:'auto'});await localRequest('management/backends');
 expect(fetch.mock.calls[0][0]).toBe('/api/v1/management/relay/remote-one/mode');
 expect(fetch.mock.calls[0][1].headers['X-CSRF-Token']).toBe('local-csrf');
 expect(fetch.mock.calls[0][1].headers['X-RoutePilot-Expected-Backend']).toBe('https://192.0.2.10:1055');
 expect(fetch.mock.calls[1][0]).toBe('/api/v1/management/backends');
 expect(fetch.mock.calls[1][1].headers['X-RoutePilot-Expected-Backend']).toBeUndefined();
});
it('binds operation polling to the selected address and clears it on local switch',async()=>{
 const fetch=vi.fn().mockResolvedValueOnce({ok:true,json:async()=>({operation:{id:'job'}})}).mockResolvedValueOnce({ok:true,json:async()=>({id:'job',state:'succeeded',result:{available:false}})}).mockResolvedValue({ok:true,json:async()=>({})});vi.stubGlobal('fetch',fetch);
 selectBackend('remote','http://192.0.2.20:1055');
 await request('updates/check','POST',{});
 for(const [,options]of fetch.mock.calls)expect(options.headers['X-RoutePilot-Expected-Backend']).toBe('http://192.0.2.20:1055');
 selectBackend('');await request('mode','PUT',{mode:'auto'});
 expect(fetch.mock.calls.at(-1)[1].headers['X-RoutePilot-Expected-Backend']).toBeUndefined();
});
it('cancels outstanding calls and rejects old backend responses after switching',async()=>{
 let finish;const fetch=vi.fn().mockImplementation(()=>new Promise(resolve=>{finish=resolve}));vi.stubGlobal('fetch',fetch);
 selectBackend('one');const pending=request('status');const signal=fetch.mock.calls[0][1].signal;
 selectBackend('two');expect(signal.aborted).toBe(true);
 finish({ok:true,json:async()=>({name:'wrong backend'})});await expect(pending).rejects.toMatchObject({name:'AbortError'});
});
it('degrades optional remote reads when the selected older backend lacks the endpoint',async()=>{
 const fetch=vi.fn().mockResolvedValue({ok:false,status:404,headers:{get:()=> 'remote'},text:async()=>JSON.stringify({error:'unknown API endpoint'})});vi.stubGlobal('fetch',fetch);
 selectBackend('older-remote');
 await expect(requestOptional('dns/clients',null)).resolves.toBeNull();
 expect(fetch.mock.calls[0][0]).toBe('/api/v1/management/relay/older-remote/dns/clients');
});
it('does not hide local or non-404 failures from optional reads',async()=>{
 const fetch=vi.fn().mockResolvedValue({ok:false,status:503,headers:{get:()=>null},text:async()=>JSON.stringify({error:'backend unavailable'})});vi.stubGlobal('fetch',fetch);
 await expect(requestOptional('dns/clients',null)).rejects.toMatchObject({status:503});
 selectBackend('older-remote');
 await expect(requestOptional('dns/clients',null)).rejects.toMatchObject({status:503});
});
