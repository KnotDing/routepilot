import React from 'react';
import {it,expect,afterEach} from 'vitest';
import {render,screen,cleanup} from '@testing-library/react';
import {DNSResult} from './dns-result';
afterEach(cleanup);
it('shows actual DNS records and TTL separately from policy decisions',()=>{
 render(<DNSResult record={{response:{id:2216,status:'NOERROR',answers:['example.test. 30 IN A 198.18.1.2'],authority:[],additional:[]},response_cache_hit:true}}/>);
 expect(screen.getByText('example.test. 30 IN A 198.18.1.2')).toBeInTheDocument();expect(screen.getByText(/查询 ID：2216/)).toBeInTheDocument();
});
it('explains normal empty service binding responses and failures',()=>{
 const {rerender}=render(<DNSResult record={{response:{id:1,status:'NOERROR',answers:[]}}}/>);
 expect(screen.getByText(/正常空响应/)).toBeInTheDocument();
 rerender(<DNSResult record={{response:{id:1,status:'SERVFAIL',answers:[]},resolution_error:'Surge DNS 返回真实地址'}}/>);
 expect(screen.getByRole('alert')).toHaveTextContent('Surge DNS 返回真实地址');expect(screen.getByText('DNS 处理失败，没有返回地址。')).toBeInTheDocument();
});
