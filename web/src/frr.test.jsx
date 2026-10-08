import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,waitFor,cleanup} from '@testing-library/react';
import {FRRExport} from './frr';
afterEach(()=>{cleanup();vi.unstubAllGlobals()});
it('exports the gateway perspective without mutating controller settings',async()=>{
 const fetch=vi.fn().mockResolvedValue({ok:true,json:async()=>({filename:'routepilot-frr.conf',content:'router bgp 65001\n neighbor 192.168.20.2 remote-as 64512\n',warnings:['BGP is disabled']})});vi.stubGlobal('fetch',fetch);
 const create=vi.fn(()=> 'blob:test');URL.createObjectURL=create;URL.revokeObjectURL=vi.fn();const click=vi.spyOn(HTMLAnchorElement.prototype,'click').mockImplementation(()=>{});
 const settings={dns:{source4:'192.168.20.2',gateway4:'192.168.20.1'},bgp:{local_asn:64512},routing:{mode:'hybrid'}};
 render(<FRRExport settings={settings} peers={[{id:'ucg',address:'192.168.20.1',asn:65001,enabled:true}]} notify={()=>{}}/>);
 expect(screen.getByLabelText('对端本地 ASN')).toHaveValue(65001);
 fireEvent.click(screen.getByText('一键导出 FRR 配置文件'));
 await waitFor(()=>expect(click).toHaveBeenCalled());
 expect(fetch).toHaveBeenCalledTimes(1);expect(fetch.mock.calls[0][0]).toBe('/api/v1/bgp/export-frr');expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({peer_id:'ucg',gateway_asn:65001,gateway_router_id:'192.168.20.1',controller_address:'192.168.20.2'});
 expect(settings.routing.mode).toBe('hybrid');click.mockRestore();
});
