import React from 'react';
import {it,expect,afterEach,vi} from 'vitest';
import {render,screen,fireEvent,cleanup,within} from '@testing-library/react';
import {OverridesTable} from './overrides';
afterEach(()=>{cleanup();localStorage.clear()});
const manual={id:'manual-rule',domain:'manual.test',match:'suffix',decision:'REJECT',enabled:true,description:'manual rule'};
it('shows automatic inclusions and exclusions in Surge order before manual overrides',()=>{
 const onEdit=vi.fn();
 render(<OverridesTable rows={[manual]} snapshot={{always_real_ip:['-private.*.test','*.test','time.example.com']}} onEdit={onEdit}/>);
 const rows=screen.getAllByRole('row').slice(1);
 expect(rows.map(row=>within(row).getAllByRole('cell')[0].textContent)).toEqual(['-private.*.test','*.test','time.example.com','manual.test']);
 expect(within(rows[0]).getByText('排除旁路')).toBeInTheDocument();expect(screen.getAllByText('Surge always-real-ip')).toHaveLength(3);expect(screen.getAllByText('只读')).toHaveLength(3);
 fireEvent.click(screen.getByText('*.test'));expect(onEdit).not.toHaveBeenCalled();
 fireEvent.click(screen.getByText('manual.test'));expect(onEdit).toHaveBeenCalledWith(manual);expect(onEdit.mock.calls[0][0]).toBe(manual);
});
it('replaces confirmed automatic entries while preserving manual edit behavior',()=>{
 const onEdit=vi.fn();const {rerender}=render(<OverridesTable rows={[manual]} snapshot={{always_real_ip:['*.old.test']}} onEdit={onEdit}/>);
 rerender(<OverridesTable rows={[manual]} snapshot={{always_real_ip:['*.new.test']}} onEdit={onEdit}/>);
 expect(screen.queryByText('*.old.test')).not.toBeInTheDocument();expect(screen.getByText('*.new.test')).toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'编辑'}));expect(onEdit).toHaveBeenCalledTimes(1);expect(onEdit).toHaveBeenCalledWith(manual);
});
it('localizes retained entries and handles older remote backends without hiding manual rows',()=>{
 localStorage.setItem('routepilot.locale','en');
 const {rerender}=render(<OverridesTable rows={[manual]} snapshot={{always_real_ip:['*.test'],always_real_ip_read_error:'failed'}} onEdit={()=>{}}/>);
 expect(screen.getByText('Read only')).toBeInTheDocument();expect(screen.getByRole('status')).toHaveTextContent('last confirmed entries');
 rerender(<OverridesTable rows={[manual]} snapshot={{always_real_ip_count:2}} onEdit={()=>{}}/>);
 expect(screen.getByRole('status')).toHaveTextContent('Update the backend');expect(screen.getByText('manual.test')).toBeInTheDocument();
});
