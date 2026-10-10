import React from 'react';
import {it,expect,afterEach,vi} from 'vitest';
import {render,screen,fireEvent,cleanup} from '@testing-library/react';
import {Settings} from './main';
import {SurgeRestartSettings,restartDefaults} from './surge-restart';
import {settingsRequirement} from './form-fields';
afterEach(()=>{cleanup();localStorage.clear()});
it('enables conditional restart controls and independent tolerance values',()=>{
 render(<Settings settings={{general:{},surge:{api_url:'',cli_path:'/Applications/Surge.app/Contents/Applications/surge-cli',trust_direct:false}}} notify={()=>{}} refresh={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'Surge',exact:true}));
 expect(screen.queryByLabelText('连续失败次数')).not.toBeInTheDocument();
 fireEvent.click(screen.getByLabelText('启用 Surge 异常重启'));
 expect(screen.getByLabelText('连续失败次数')).toHaveValue(5);
 expect(screen.getByText('请至少选择一个异常判定条件。')).toBeInTheDocument();
 fireEvent.click(screen.getByLabelText('HTTP API 不可用时重启'));
 expect(screen.getByLabelText('Surge HTTP API 地址')).toBeRequired();
 fireEvent.change(screen.getByLabelText('指定策略组（每行一项）'),{target:{value:'Auto\nFallback'}});
 expect(screen.getByLabelText('策略健康探测 URL')).toBeRequired();
 expect(screen.getByLabelText('指定策略组（每行一项）')).toHaveValue('Auto\nFallback');
 expect(screen.getByLabelText('重启冷却时间（秒）')).toHaveValue(300);
});
it('migrates zero values, emits selected nodes, and marks API requirements',()=>{
 const changed=vi.fn();render(<SurgeRestartSettings value={{enabled:true,failures:0,nodes:['Node']}} onChange={changed} apiURL=""/>);
 expect(screen.getByLabelText('连续失败次数')).toHaveValue(5);
 fireEvent.change(screen.getByLabelText('指定节点（每行一项）'),{target:{value:'Node A\nNode B'}});
 expect(changed).toHaveBeenCalledWith({...restartDefaults,enabled:true,nodes:['Node A','Node B']});
 expect(settingsRequirement('surge','api_url',{surge:{restart:{enabled:true,api:true}}}).required).toBe(true);
});

it('preserves a trailing newline so targets can be typed one line at a time',()=>{
 render(<Settings settings={{general:{},surge:{restart:{...restartDefaults,enabled:true},trust_direct:false}}} notify={()=>{}} refresh={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:'Surge',exact:true}));
 const input=screen.getByLabelText('指定节点（每行一项）');
 fireEvent.change(input,{target:{value:'Node A\n'}});expect(input).toHaveValue('Node A\n');
 fireEvent.change(input,{target:{value:'Node A\nNode B'}});expect(input).toHaveValue('Node A\nNode B');
});
