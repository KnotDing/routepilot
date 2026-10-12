import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,cleanup,waitFor,within} from '@testing-library/react';
import {ClientsPanel} from './clients';
import {request} from './client';
vi.mock('./client',()=>({request:vi.fn()}));
afterEach(()=>{cleanup();vi.resetAllMocks()});
it('edits a discovered client from its list row and persists its name and policy',async()=>{
 request.mockResolvedValue({});const refresh=vi.fn();render(<ClientsPanel configs={[]} snapshot={{clients:[{client_ip:'192.0.2.10',queries:12}],stats:{queries:12}}} refresh={refresh}/>);
 expect(screen.queryByRole('heading',{name:'客户端设置'})).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'编辑客户端 192.0.2.10'}));
 expect(screen.getByLabelText('客户端 IP')).toHaveValue('192.0.2.10');expect(screen.getByLabelText('客户端 IP')).toHaveAttribute('readonly');
 fireEvent.change(screen.getByLabelText('备注名称'),{target:{value:'工作电脑'}});fireEvent.change(screen.getByLabelText('查询策略'),{target:{value:'SURGE'}});
 fireEvent.click(screen.getByRole('button',{name:'保存',exact:true}));
 await waitFor(()=>expect(request).toHaveBeenCalledWith('dns/client-settings','POST',{ip:'192.0.2.10',name:'工作电脑',decision:'SURGE'}));
 await waitFor(()=>expect(refresh).toHaveBeenCalled());expect(screen.queryByRole('dialog')).toBeNull();
});
it('edits persisted clients without activity and clears settings without deleting statistics',async()=>{
 request.mockResolvedValue({});const row={id:'a',ip:'2001:db8::10',name:'服务器',decision:'REJECT'};render(<ClientsPanel configs={[row]} snapshot={{clients:[],stats:{clients:0,queries:0}}}/>);
 const cells=within(screen.getAllByRole('row')[1]).getAllByRole('cell');expect(cells[3]).toHaveTextContent('—');
 fireEvent.click(screen.getByRole('button',{name:'编辑客户端 2001:db8::10'}));fireEvent.change(screen.getByLabelText('查询策略'),{target:{value:''}});fireEvent.click(screen.getByRole('button',{name:'保存',exact:true}));
 await waitFor(()=>expect(request).toHaveBeenCalledWith('dns/client-settings/a','PUT',{ip:row.ip,name:row.name,decision:''}));await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull());
 fireEvent.click(screen.getByRole('button',{name:'编辑客户端 2001:db8::10'}));fireEvent.click(screen.getByRole('button',{name:'清除客户端设置'}));await waitFor(()=>expect(request).toHaveBeenCalledWith('dns/client-settings/a','DELETE'));
 expect(screen.getByRole('cell',{name:row.ip})).toBeInTheDocument();
});
it('keeps failed edits visible and disables editing for unsupported backends',async()=>{
 request.mockRejectedValue(new Error('save failed'));const snapshot={clients:[{client_ip:'192.0.2.10'}]};const {rerender}=render(<ClientsPanel configs={[]} snapshot={snapshot}/>);
 fireEvent.click(screen.getByRole('button',{name:'编辑客户端 192.0.2.10'}));fireEvent.click(screen.getByRole('button',{name:'保存',exact:true}));
 expect(await screen.findByRole('alert')).toHaveTextContent('save failed');expect(screen.getByRole('dialog')).toBeInTheDocument();fireEvent.click(screen.getByRole('button',{name:'取消'}));
 rerender(<ClientsPanel configs={null} snapshot={snapshot}/>);expect(screen.getByText('此后端暂不支持客户端设置，请升级后端。')).toBeInTheDocument();expect(screen.queryByRole('button',{name:'编辑客户端 192.0.2.10'})).toBeNull();expect(screen.getByRole('table')).toBeInTheDocument();
 rerender(<ClientsPanel configs={[]} snapshot={{...snapshot,storage_error:'disk full'}}/>);expect(screen.getByRole('alert')).toHaveTextContent('客户端统计保存失败');expect(screen.getByRole('button',{name:'编辑客户端 192.0.2.10'})).toBeInTheDocument();
});
