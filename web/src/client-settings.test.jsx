import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,cleanup,waitFor} from '@testing-library/react';
import {ClientSettings} from './client-settings';
import {request} from './client';
vi.mock('./client',()=>({request:vi.fn()}));
afterEach(()=>{cleanup();vi.resetAllMocks()});
it('saves a discovered client remark and forced policy through its dedicated resource',async()=>{
 request.mockResolvedValue({});const refresh=vi.fn();render(<ClientSettings configs={[]} clients={[{client_ip:'192.0.2.10'}]} refresh={refresh}/>);
 fireEvent.change(screen.getByLabelText('从已发现客户端添加'),{target:{value:'192.0.2.10'}});
 expect(screen.getByLabelText('客户端 IP')).toHaveValue('192.0.2.10');
 fireEvent.change(screen.getByLabelText('备注名称'),{target:{value:'工作电脑'}});fireEvent.change(screen.getByLabelText('查询策略'),{target:{value:'SURGE'}});
 fireEvent.click(screen.getByRole('button',{name:'保存',exact:true}));
 await waitFor(()=>expect(request).toHaveBeenCalledWith('dns/client-settings','POST',{ip:'192.0.2.10',name:'工作电脑',decision:'SURGE'}));
 await waitFor(()=>expect(refresh).toHaveBeenCalled());expect(screen.queryByRole('dialog')).toBeNull();
});
it('edits to automatic without dropping the name and deletes by setting ID',async()=>{
 request.mockResolvedValue({});const row={id:'a',ip:'2001:db8::10',name:'服务器',decision:'REJECT'};render(<ClientSettings configs={[row]}/>);
 fireEvent.click(screen.getByRole('button',{name:'编辑'}));fireEvent.change(screen.getByLabelText('查询策略'),{target:{value:''}});fireEvent.click(screen.getByRole('button',{name:'保存',exact:true}));
 await waitFor(()=>expect(request).toHaveBeenCalledWith('dns/client-settings/a','PUT',{ip:row.ip,name:row.name,decision:''}));await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull());
 fireEvent.click(screen.getByRole('button',{name:'删除'}));await waitFor(()=>expect(request).toHaveBeenCalledWith('dns/client-settings/a','DELETE'));
});
it('keeps invalid edits visible and gracefully disables unsupported backends',async()=>{
 request.mockRejectedValue(new Error('client IP must be a concrete IPv4 or IPv6 address'));const {rerender}=render(<ClientSettings configs={[]}/>);
 fireEvent.click(screen.getByRole('button',{name:'添加客户端'}));fireEvent.change(screen.getByLabelText('客户端 IP'),{target:{value:'invalid'}});fireEvent.click(screen.getByRole('button',{name:'保存',exact:true}));
 expect(await screen.findByRole('alert')).toHaveTextContent('客户端 IP 必须');expect(screen.getByRole('dialog')).toBeInTheDocument();cleanup();
 render(<ClientSettings configs={null}/>);expect(screen.getByText('此后端暂不支持客户端设置，请升级后端。')).toBeInTheDocument();expect(screen.queryByRole('button',{name:'添加客户端'})).toBeNull();
});
