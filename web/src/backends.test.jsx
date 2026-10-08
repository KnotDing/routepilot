import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,cleanup} from '@testing-library/react';
import {Backends} from './backends';
import {selectBackend} from './client';
afterEach(()=>{cleanup();selectBackend('');vi.unstubAllGlobals()});
it('saving a new connection keeps the manager open without switching away from local',async()=>{
 const profile={id:'new',name:'Remote',url:'https://192.0.2.10:1055'};
 vi.stubGlobal('fetch',vi.fn().mockImplementation((url,options)=>Promise.resolve({ok:true,json:async()=>options.method==='GET'?[]:{settings:{remote_backends:[profile]}}})));
 const onSelect=vi.fn();render(<Backends active={null} onSelect={onSelect} onClose={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'添加远程后端'}));
 fireEvent.change(screen.getByLabelText('连接名称'),{target:{value:profile.name}});fireEvent.change(screen.getByLabelText('后端管理地址'),{target:{value:profile.url}});
 fireEvent.click(screen.getByRole('button',{name:'保存连接'}));await screen.findByText('Remote');expect(onSelect).not.toHaveBeenCalled();expect(screen.getByRole('dialog')).toBeInTheDocument();
});
