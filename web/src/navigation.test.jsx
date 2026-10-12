import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,cleanup,within} from '@testing-library/react';
import {Navigation} from './navigation';
const props={pages:[['Dashboard','仪表盘'],['Monitor','大屏监控']],sections:{Dashboard:'总览'},page:'Dashboard',onSelect:vi.fn(),onManage:vi.fn(),onSignOut:vi.fn()};
afterEach(()=>{cleanup();localStorage.clear();vi.restoreAllMocks()});
it('persists icon-only sidebar mode without changing DNS mode or menu accessibility',()=>{
 const {unmount}=render(<Navigation {...props} mode="force-direct"/>);
 expect(within(document.querySelector('.desktop-sidebar')).getByLabelText('当前分流模式')).toHaveTextContent('强制直连');
 fireEvent.click(screen.getByRole('button',{name:'收起侧边栏'}));
 expect(document.querySelector('.desktop-sidebar')).toHaveClass('sidebar-icons');expect(localStorage.getItem('routepilot.sidebar')).toBe('icons');
 expect(within(document.querySelector('.desktop-sidebar')).getByRole('button',{name:'大屏监控'})).toHaveAttribute('title','大屏监控');
 unmount();render(<Navigation {...props}/>);expect(document.querySelector('.desktop-sidebar')).toHaveClass('sidebar-icons');
 fireEvent.click(screen.getByRole('button',{name:'展开侧边栏'}));expect(document.querySelector('.desktop-sidebar')).not.toHaveClass('sidebar-icons');
});
it('reveals hidden navigation on the left edge and pins it again',()=>{
 vi.spyOn(window,'scrollTo').mockImplementation(()=>{});
 render(<Navigation {...props}/>);fireEvent.click(screen.getByRole('button',{name:'隐藏侧边栏'}));
 const aside=document.querySelector('.desktop-sidebar'),edge=screen.getByRole('button',{name:'显示侧边栏'});
 expect(aside).toHaveAttribute('hidden');fireEvent.mouseEnter(edge);expect(aside).not.toHaveAttribute('hidden');
 fireEvent.mouseLeave(edge,{relatedTarget:document.body});expect(aside).toHaveAttribute('hidden');fireEvent.mouseEnter(edge);
 fireEvent.mouseLeave(aside);expect(aside).toHaveAttribute('hidden');
 fireEvent.focus(edge);expect(aside).not.toHaveAttribute('hidden');
 fireEvent.click(within(aside).getByRole('button',{name:'大屏监控'}));expect(props.onSelect).toHaveBeenCalledWith('Monitor');expect(aside).toHaveAttribute('hidden');
 fireEvent.mouseEnter(edge);fireEvent.click(within(aside).getByRole('button',{name:'固定侧边栏'}));expect(localStorage.getItem('routepilot.sidebar')).toBe('expanded');expect(screen.queryByRole('button',{name:'显示侧边栏'})).toBeNull();
});
