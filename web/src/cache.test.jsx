import {LanguageSelector} from './language';
import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,waitFor,cleanup} from '@testing-library/react';
import {CachePanel} from './cache';
afterEach(()=>{cleanup();localStorage.removeItem('routepilot.locale');vi.unstubAllGlobals()});
it('shows invalidation causes and requests all-cache search with pagination',async()=>{
 const fetch=vi.fn().mockResolvedValue({ok:true,json:async()=>({stats:{entries:120,invalidations:3,last_invalidation_reason:'策略组选择变化',last_invalidation_at:'2026-10-08T00:00:00Z'},capacity:10000,total:120,entries:[{domain:'cached.test.',decision:'SURGE',query_type:1,confidence:'uncertain',expires_at:'2099-10-08T00:00:00Z'}],ttl:{direct_ttl_seconds:300,surge_ttl_seconds:300,unknown_ttl_seconds:10,error_ttl_seconds:1}})});
 vi.stubGlobal('fetch',fetch);render(<><LanguageSelector/><CachePanel notify={()=>{}}/></>);
 await screen.findByText('cached.test.');expect(screen.getByText('策略组选择变化')).toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'下一页'}));
 await waitFor(()=>expect(fetch.mock.calls.some(([url])=>url.includes('offset=50'))).toBe(true));
 fireEvent.change(screen.getByLabelText('搜索缓存域名'),{target:{value:'other.test'}});
 await waitFor(()=>expect(fetch.mock.calls.some(([url])=>url.includes('domain=other.test')&&url.includes('offset=0'))).toBe(true));
});

it('shows stable decisions as persistent and uncertain decisions with short TTL',async()=>{
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({stats:{entries:2},total:2,entries:[{domain:'stable.test.',decision:'DIRECT',persistent:true,expires_at:'0001-01-01T00:00:00Z'},{domain:'retry.test.',decision:'UNKNOWN',expires_at:new Date(Date.now()+10000).toISOString()}],ttl:{expire_decisions:false,invalidate_on_read_failure:false,invalidate_direct_on_selection_change:false,invalidate_reject_on_selection_change:true,unknown_ttl_seconds:10,error_ttl_seconds:1}})}));
 render(<><LanguageSelector/><CachePanel notify={()=>{}}/></>);await screen.findByText('stable.test.');
 expect(screen.getByText('直到配置更新')).toBeInTheDocument();expect(screen.getByText(/长期保留，直到确认配置更新/)).toBeInTheDocument();expect(screen.getByText('保留旧决策')).toBeInTheDocument();
});

it('translates cache lifecycle, filters and persistent entries to English',async()=>{
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({stats:{entries:1},total:1,entries:[{domain:'stable.test.',decision:'DIRECT',persistent:true,confidence:'certain'}],ttl:{expire_decisions:false,invalidate_on_read_failure:false}})}));
 render(<><LanguageSelector/><CachePanel notify={()=>{}}/></>);await screen.findByText('stable.test.');fireEvent.change(screen.getByLabelText(/页面语言|Page language/),{target:{value:'en'}});
 expect(screen.getByText('Cache lifecycle')).toBeInTheDocument();expect(screen.getByLabelText('Search cached domains')).toBeInTheDocument();expect(screen.getByText('Until configuration update')).toBeInTheDocument();expect(screen.getByText('Keep cached decisions')).toBeInTheDocument();expect(screen.getByRole('button',{name:'Clear all decisions'})).toBeInTheDocument();
});

it('explains independent optimistic decision and response cache options',async()=>{
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({stats:{entries:1,stale_entries:1},total:1,entries:[{domain:'stale.test.',decision:'DIRECT',persistent:true}],ttl:{optimistic:true,optimistic_responses:true}})}));
 render(<CachePanel notify={()=>{}}/>);await screen.findByText('stale.test.');
 expect(screen.getByText('待刷新决策')).toBeInTheDocument();
 expect(screen.getByText(/乐观缓存已开启/)).toBeInTheDocument();
 expect(screen.getByText(/乐观响应缓存已开启/)).toBeInTheDocument();
 expect(screen.queryByText('直到配置更新')).not.toBeInTheDocument();
});
