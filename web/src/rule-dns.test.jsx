import React from 'react';
import {it,expect,afterEach} from 'vitest';
import {render,screen,cleanup} from '@testing-library/react';
import {RuleDNS} from './rule-dns';
afterEach(()=>{cleanup();localStorage.clear()});
it('distinguishes automatic resolution from explicit lookup and re-evaluation',()=>{
 const {rerender}=render(<RuleDNS result={{rule_dns:{state:'completed',lookup_performed:false,re_evaluated:false},rule_notes:['DNS completed']}}/>);
 expect(screen.getByText('由 Surge 在规则求值时完成解析')).toBeInTheDocument();expect(screen.getByText('已完成')).toBeInTheDocument();expect(screen.queryByRole('alert')).not.toBeInTheDocument();
 rerender(<RuleDNS result={{rule_dns:{state:'completed',lookup_performed:true,re_evaluated:true}}}/>);
 expect(screen.getByText('已触发一次 Surge DNS 补查 · 已重新求值')).toBeInTheDocument();
});
it('displays failed resolution separately from a client DNS response',()=>{
 localStorage.setItem('routepilot.locale','en');
 render(<RuleDNS result={{rule_dns:{state:'failed',lookup_performed:true,re_evaluated:false,reason:'Surge rule DNS lookup failed'}}}/>);
 expect(screen.getByText('Failed')).toBeInTheDocument();expect(screen.getByRole('alert')).toHaveTextContent('Surge rule DNS lookup failed');expect(screen.queryByText(/Re-evaluated the rules/)).not.toBeInTheDocument();
});
