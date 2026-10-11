import React from 'react';
import {cleanup,render} from '@testing-library/react';
import {afterEach,expect,it} from 'vitest';
import {BGPPeerDetails} from './bgp-peer-details';

afterEach(cleanup);

it('sorts remote advertised capabilities deterministically while preserving local order',()=>{
 const peer={id:'peer',address:'192.0.2.1',asn:64512,enabled:true};
 const live={state:'ESTABLISHED',details:{state:{local_cap:[{'@type':'gobgp.GracefulRestartCapability','time':120},{'@type':'gobgp.AddPathCapability','send_max':2}],remote_cap:[{'@type':'gobgp.GracefulRestartCapability','time':90},{'@type':'gobgp.AddPathCapability','send_max':1}]}}};
 const {container}=render(<BGPPeerDetails peer={peer} live={live} settings={{bgp:{local_asn:64513,router_id:'192.0.2.2'}}} onClose={()=>{}}/>);
 const lists=container.querySelectorAll('.peer-capabilities');
 const local=[...lists[0].querySelectorAll('summary')].map(node=>node.textContent);
 const remote=[...lists[1].querySelectorAll('summary')].map(node=>node.textContent);
 expect(local).toEqual(['优雅重启','多路径通告']);
 expect(remote).toEqual(['多路径通告','优雅重启']);
});
