import React from 'react';
import {it,expect,afterEach,vi} from 'vitest';
import {render,screen,fireEvent,cleanup,within} from '@testing-library/react';
import {CachePolicySettings,CacheGroupStatistics,cacheUpstreamRows,endpointCacheIdentity} from './cache-policies';
import {LanguageSelector} from './language';
afterEach(()=>{cleanup();localStorage.removeItem('routepilot.locale')});
it('edits only optimistic decisions and retains upstream settings',()=>{
 const onChange=vi.fn(),settings={optimistic:true,decisions:{DIRECT:{optimistic:false}},upstreams:{'udp://192.0.2.53:53':{optimistic:false}}};
 render(<CachePolicySettings settings={settings} status={{selective_cache_policies:true}} onChange={onChange}/>);
 expect(screen.getByLabelText('DIRECT 允许乐观缓存')).not.toBeChecked();
 expect(screen.queryByLabelText('UNKNOWN 允许乐观缓存')).not.toBeInTheDocument();
 expect(screen.queryByRole('checkbox',{name:/缓存此项/})).not.toBeInTheDocument();
 fireEvent.click(screen.getByLabelText('DIRECT 允许乐观缓存'));
 expect(onChange).toHaveBeenCalledWith({...settings,decisions:{DIRECT:{optimistic:true}}});
 fireEvent.click(screen.getByLabelText('SURGE 允许乐观缓存'));
 expect(onChange.mock.calls.at(-1)[0].decisions.SURGE).toEqual({optimistic:false});
});
it('lists global, policy, backup and observed original upstreams by real identity',()=>{
 const ep={name:'Primary',protocol:'doh',address:'192.0.2.53:443',url:'https://dns.test/first'};
 const other={...ep,url:'https://dns.test/second'};
 const rows=cacheUpstreamRows({upstreams:[ep],policy_upstreams:[{upstreams:[ep],fallback_upstreams:[other]}]}, {},{'udp://8.8.8.8:53':{},other:{}});
 expect(rows.map(row=>row.key)).toEqual([endpointCacheIdentity(ep),endpointCacheIdentity(other),'udp://8.8.8.8:53']);
 const onChange=vi.fn();render(<CachePolicySettings settings={{optimistic_responses:true}} dns={{upstreams:[ep]}} status={{selective_cache_policies:true}} onChange={onChange}/>);
 fireEvent.click(screen.getByLabelText(`${endpointCacheIdentity(ep)} 允许乐观缓存`));
 expect(onChange.mock.calls[0][0].upstreams[endpointCacheIdentity(ep)]).toEqual({optimistic:false});
});
it('explains unsupported backends without showing editable overrides',()=>{
 render(<CachePolicySettings settings={{optimistic:true}} onChange={()=>{}}/>);
 expect(screen.getByRole('status')).toHaveTextContent('当前后端不支持');expect(screen.queryByLabelText('DIRECT 允许乐观缓存')).not.toBeInTheDocument();
 expect(screen.getByLabelText('乐观决策缓存')).toBeChecked();
});
it('reports mismatch rate using verified requests and translates the breakdown',()=>{
 render(<><LanguageSelector/><CacheGroupStatistics groups={{DIRECT:{entries:2,hits:30,optimistic_hits:20,optimistic_compared:10,optimistic_mismatches:2}}}/></>);
 const direct=screen.getByText('DIRECT').closest('tr');expect(within(direct).getByText('20%')).toBeInTheDocument();
 const unknown=screen.getByText('UNKNOWN').closest('tr');expect(within(unknown).getByText('—')).toBeInTheDocument();
 fireEvent.change(screen.getByLabelText(/页面语言|Page language/),{target:{value:'en'}});
 expect(screen.getByText('Cache statistics by decision')).toBeInTheDocument();expect(screen.getByText('Verified requests')).toBeInTheDocument();
});

it('expands each master independently and retains item overrides while collapsed',()=>{
 function Controlled(){
  const [settings,setSettings]=React.useState({optimistic:false,optimistic_responses:false,optimistic_idle_refresh:true,decisions:{DIRECT:{optimistic:false}},upstreams:{'udp://192.0.2.53:53':{optimistic:false}}});
  return <CachePolicySettings settings={settings} dns={{upstreams:[{protocol:'udp',address:'192.0.2.53:53'}]}} status={{selective_cache_policies:true}} onChange={setSettings}/>;
 }
 render(<Controlled/>);
 expect(screen.getByRole('group',{name:'按决策设置缓存'})).toBeInTheDocument();
 expect(screen.getByRole('group',{name:'按上游设置缓存'})).toBeInTheDocument();
 expect(screen.queryByLabelText('DIRECT 允许乐观缓存')).not.toBeInTheDocument();
 fireEvent.click(screen.getByLabelText('乐观决策缓存'));
 expect(screen.getByLabelText('DIRECT 允许乐观缓存')).not.toBeChecked();
 expect(screen.getByLabelText('决策缓存空闲刷新')).toBeChecked();
 expect(screen.queryByLabelText('udp://192.0.2.53:53 允许乐观缓存')).not.toBeInTheDocument();
 fireEvent.click(screen.getByLabelText('乐观决策缓存'));
 expect(screen.queryByLabelText('DIRECT 允许乐观缓存')).not.toBeInTheDocument();
 expect(screen.queryByLabelText('决策缓存空闲刷新')).not.toBeInTheDocument();
 fireEvent.click(screen.getByLabelText('乐观响应缓存'));
 expect(screen.getByLabelText('udp://192.0.2.53:53 允许乐观缓存')).not.toBeChecked();
 fireEvent.click(screen.getByLabelText('乐观决策缓存'));
 expect(screen.getByLabelText('DIRECT 允许乐观缓存')).not.toBeChecked();
});
