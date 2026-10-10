import React from 'react';
import {it,expect,afterEach} from 'vitest';
import {render,screen,cleanup} from '@testing-library/react';
import {ClientsPanel} from './clients';

afterEach(cleanup);

it('shows client totals and per-client DNS decision statistics',()=>{
 render(<ClientsPanel snapshot={{
  clients:[{client_ip:'192.0.2.10',queries:4,direct:2,surge:1,reject:1,unknown:0,errors:1,last_seen:'2026-10-10T04:00:00Z'}],
  stats:{clients:1,queries:4,direct:2,surge:1,reject:1,unknown:0,errors:1},
  capacity:4096,evicted_clients:0
 }}/>);
 expect(screen.getByText('客户端列表')).toBeInTheDocument();
 expect(screen.getByText('192.0.2.10')).toBeInTheDocument();
 expect(screen.getByText('DNS 请求总数')).toBeInTheDocument();
 expect(screen.getByText('DNS 错误')).toBeInTheDocument();
 expect(screen.getByText('DIRECT 请求数')).toBeInTheDocument();
});

it('explains when old client entries were evicted from the bounded list',()=>{
 render(<ClientsPanel snapshot={{clients:[],stats:{clients:1},capacity:4096,evicted_clients:3}}/>);
 expect(screen.getByText(/客户端列表容量为 4096 个；已淘汰 3 个不活跃客户端/)).toBeInTheDocument();
});
