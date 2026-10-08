import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,cleanup,waitFor} from '@testing-library/react';
import {RouteImport} from './route-import';
import {Resource} from './main';
import {selectBackend} from './client';
afterEach(()=>{cleanup();localStorage.clear();selectBackend('');vi.unstubAllGlobals()});
const prefixes=['91.108.56.0/22','2001:b28:f23d::/48'];
function prepare(locale='zh-CN'){
 localStorage.setItem('routepilot.locale',locale);
 const fetch=vi.fn(async(url,options)=>({ok:true,status:200,json:async()=>String(url).endsWith('import-url')?{prefixes,skipped:[{line:3,reason:'duplicate route destination'}]}:{imported:JSON.parse(options.body).prefixes.length,skipped:0}}));
 vi.stubGlobal('fetch',fetch);return fetch;
}
async function preview(){fireEvent.change(screen.getByRole('textbox',{name:/路由列表 URL|Route list URL/}),{target:{value:'https://core.telegram.org/resources/cidr.txt'}});fireEvent.click(screen.getByRole('button',{name:/获取并预览|Fetch and preview/}));await screen.findByText(prefixes[1]);}
it('previews mixed-family destinations and imports one atomic snapshot with distinct next hops',async()=>{
 const fetch=prepare(),refresh=vi.fn(),notify=vi.fn(),close=vi.fn();
 render(<RouteImport settings={{surge:{next_hop4:'192.168.20.5',next_hop6:'fd00:20::5'}}} refresh={refresh} notify={notify} onClose={close}/>);
 await preview();expect(refresh).not.toHaveBeenCalled();expect(screen.getByText('第 3 行：目的网段重复或已存在')).toBeInTheDocument();
 fireEvent.change(screen.getByLabelText('健康绑定'),{target:{value:'surge'}});
 fireEvent.click(screen.getByRole('button',{name:'导入所选 2 项'}));
 await waitFor(()=>expect(close).toHaveBeenCalled());
 const body=JSON.parse(fetch.mock.calls[1][1].body);
 expect(body).toMatchObject({prefixes,next_hop4:'192.168.20.5',next_hop6:'fd00:20::5',health_binding:'surge',enabled:true,local_preference:100});expect(refresh).toHaveBeenCalledTimes(1);
});
it('allows IPv4-only import when an IPv6 next hop is unavailable, through the selected remote backend',async()=>{
 const fetch=prepare('en');selectBackend('remote-peer');
 render(<RouteImport settings={{surge:{next_hop4:'192.168.20.5'}}} refresh={()=>{}} notify={()=>{}} onClose={()=>{}}/>);await preview();
 expect(screen.getByRole('button',{name:'Import 2 selected entries'})).toBeDisabled();
 fireEvent.click(screen.getByLabelText('Import IPv6 (1 entries)'));
 expect(screen.getByRole('button',{name:'Import 1 selected entries'})).toBeEnabled();
 fireEvent.click(screen.getByRole('button',{name:'Import 1 selected entries'}));
 await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(2));
 expect(fetch.mock.calls[1][0]).toBe('/api/v1/management/relay/remote-peer/bgp/routes/import');
 expect(JSON.parse(fetch.mock.calls[1][1].body).prefixes).toEqual([prefixes[0]]);
 const walker=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);let node;while(node=walker.nextNode())expect(node.textContent).not.toMatch(/[\u4e00-\u9fff]/);
});
it('invalidates the preview after a URL edit and retains selections after an atomic import failure',async()=>{
 const fetch=prepare(),refresh=vi.fn(),close=vi.fn();
 render(<RouteImport settings={{surge:{next_hop4:'192.168.20.5',next_hop6:'fd00:20::5'}}} refresh={refresh} notify={()=>{}} onClose={close}/>);await preview();
 fetch.mockResolvedValueOnce({ok:false,status:422,text:async()=>JSON.stringify({error:'hard BGP route limit exceeded'})});
 fireEvent.click(screen.getByRole('button',{name:'导入所选 2 项'}));await screen.findByRole('alert');expect(refresh).not.toHaveBeenCalled();expect(close).not.toHaveBeenCalled();expect(screen.getByRole('checkbox',{name:prefixes[0]})).toBeChecked();
 fireEvent.change(screen.getByRole('textbox',{name:'路由列表 URL'}),{target:{value:'https://example.com/another.txt'}});expect(screen.queryByRole('button',{name:'导入所选 2 项'})).not.toBeInTheDocument();
});
it('opens URL import from manual routes and supplies the configured Surge next hops',async()=>{
 prepare();render(<Resource page="Routes" rows={[]} settings={{surge:{next_hop4:'192.168.20.5',next_hop6:'fd00:20::5'}}} refresh={()=>{}} notify={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'从 URL 导入'}));await preview();expect(screen.getByLabelText('IPv4 下一跳')).toHaveValue('192.168.20.5');
});
