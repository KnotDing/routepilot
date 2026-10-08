import React from 'react';
import {it,expect,afterEach,vi} from 'vitest';
import {render,screen,fireEvent,cleanup} from '@testing-library/react';
import {LanguageSelector} from './language';
import {Settings} from './main';
import {About} from './about';
import {RoutingPanel} from './routing';
afterEach(()=>{cleanup();localStorage.clear();vi.unstubAllGlobals();document.documentElement.lang='zh-CN'});

it('uses one global preference across pages and backends without saving settings or losing edits',()=>{
 const fetch=vi.fn();vi.stubGlobal('fetch',fetch);
 render(<><LanguageSelector/><Settings settings={{general:{name:'Controller'},cache:{expire_decisions:false}}} refresh={()=>{}} notify={()=>{}}/><RoutingPanel settings={{routing:{mode:'static'},dns:{interception_enabled:false}}} snapshot={{routes:[]}} refresh={()=>{}} notify={()=>{}}/><About status={{software:{version:'1.0',build:'1',os:'linux',arch:'amd64'}}}/></>);
 expect(screen.getAllByLabelText(/页面语言|Page language/)).toHaveLength(1);
 fireEvent.change(screen.getByLabelText('名称'),{target:{value:'Unsaved'}});
 fireEvent.change(screen.getByLabelText(/页面语言|Page language/),{target:{value:'en'}});
 expect(screen.getByLabelText('Routing mode')).toBeInTheDocument();expect(screen.getByText('Current backend')).toBeInTheDocument();expect(screen.getByLabelText('Name')).toHaveValue('Unsaved');expect(fetch).not.toHaveBeenCalled();
 expect(localStorage.getItem('routepilot.locale')).toBe('en');expect(document.documentElement.lang).toBe('en');
 cleanup();
 render(<><LanguageSelector/><About backend={{name:'Remote',url:'https://192.0.2.1:1055'}} status={{}}/></>);
 expect(screen.getAllByLabelText(/页面语言|Page language/)).toHaveLength(1);expect(screen.getByLabelText(/页面语言|Page language/)).toHaveValue('en');expect(screen.getByText('Remote service')).toBeInTheDocument();
});
