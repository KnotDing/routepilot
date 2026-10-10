import {afterEach,expect,it,vi} from 'vitest';
import {request,selectBackend} from './client';
import {saveSettings} from './configuration';

afterEach(()=>{selectBackend('');vi.unstubAllGlobals()});

it('acknowledges durable settings without waiting for runtime activation or another session request',async()=>{
 const settings={general:{theme:'system'}};
 const fetch=vi.fn().mockResolvedValue({ok:true,json:async()=>({settings})});
 vi.stubGlobal('fetch',fetch);
 const progress=vi.fn();
 expect(await saveSettings(settings,progress)).toEqual({settings});
 expect(progress).toHaveBeenCalledExactlyOnceWith('submitting');
 expect(fetch).toHaveBeenCalledTimes(1);
 expect(fetch.mock.calls[0][0]).toBe('/api/v1/settings');
});

it('requests deferred diagnostics and returns only their completed result',async()=>{
 const events=[];const listener=e=>events.push(e.detail.phase);
 window.addEventListener('routepilot-operation',listener);
 try{
  const fetch=vi.fn().mockResolvedValueOnce({ok:true,json:async()=>({operation:{id:'job',state:'running'}})}).mockResolvedValueOnce({ok:true,json:async()=>({id:'job',state:'succeeded',result:{decision:'DIRECT'}})});
  vi.stubGlobal('fetch',fetch);
  expect(await request('test/domain','POST',{domain:'example.test'})).toEqual({decision:'DIRECT'});
  expect(fetch.mock.calls[0][1].headers.Prefer).toBe('respond-async');
  expect(fetch.mock.calls[1][0]).toBe('/api/v1/operations/status?id=job');
  expect(events).toEqual(['submitting','running','finished']);
 }finally{window.removeEventListener('routepilot-operation',listener)}
});

it('reports a background diagnostic failure instead of presenting it as success',async()=>{
 vi.stubGlobal('fetch',vi.fn().mockResolvedValueOnce({ok:true,json:async()=>({operation:{id:'job'}})}).mockResolvedValueOnce({ok:true,json:async()=>({id:'job',state:'failed',error:'probe failed'})}));
 await expect(request('routing/verify','POST',{})).rejects.toThrow('probe failed');
});
