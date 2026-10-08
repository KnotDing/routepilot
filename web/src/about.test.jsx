import {LanguageSelector} from './language';
import React from 'react';
import {it,expect,afterEach,vi} from 'vitest';
import {render,screen,fireEvent,cleanup,within} from '@testing-library/react';
import {About} from './about';
import {Settings} from './main';
import release from '../../internal/buildinfo/release.json';
afterEach(()=>{cleanup();localStorage.clear();vi.unstubAllGlobals()});

it('separates the web release from the selected remote backend identity',()=>{
 render(<About backend={{name:'Office',url:'https://192.0.2.10:1055'}} status={{name:'Remote controller',software:{version:'0.3.9',build:'8',os:'linux',arch:'amd64',go_version:'go1.26.1'},state:'HEALTHY',uptime_seconds:90061}}/>);
 const ui=within(screen.getByRole('region',{name:'网页界面'})),remote=within(screen.getByRole('region',{name:'当前后端'}));
 expect(ui.getByText(release.version)).toBeInTheDocument();expect(remote.getByText('0.3.9')).toBeInTheDocument();
 expect(remote.getByText('https://192.0.2.10:1055')).toBeInTheDocument();expect(remote.getByText('linux · amd64')).toBeInTheDocument();expect(remote.getByText('1 天 1 小时 1 分钟 1 秒')).toBeInTheDocument();
});
it('handles older backends without inventing their version and supports English',()=>{
 localStorage.setItem('routepilot.locale','en');
 const {rerender}=render(<><LanguageSelector/><About status={{name:'Old backend',uptime_seconds:0}}/></>);
 const backend=within(screen.getByRole('region',{name:'Current backend'}));
 expect(backend.queryByText(release.version)).not.toBeInTheDocument();expect(screen.getByRole('status')).toHaveTextContent('does not provide version');expect(backend.getByText('0 s')).toBeInTheDocument();
 rerender(<><LanguageSelector/><About status={{software:{version:'1.2.3',build:'77',os:'darwin',arch:'arm64'}}}/></>);
 expect(screen.queryByRole('status')).not.toBeInTheDocument();expect(screen.getByText('macOS · arm64')).toBeInTheDocument();
 fireEvent.change(screen.getByLabelText(/页面语言|Page language/),{target:{value:'zh-CN'}});expect(screen.getByText('当前后端')).toBeInTheDocument();
});
it('opens About without a settings form and preserves unsaved edits across tabs',()=>{
 const fetch=vi.fn();vi.stubGlobal('fetch',fetch);
 render(<Settings settings={{general:{name:'Controller'}}} refresh={()=>{}} notify={()=>{}}/>);
 fireEvent.change(screen.getByLabelText('名称'),{target:{value:'Unsaved name'}});
 fireEvent.click(screen.getByRole('button',{name:'关于'}));
 expect(screen.getByRole('region',{name:'网页界面'})).toBeInTheDocument();expect(screen.queryByRole('button',{name:'保存设置'})).not.toBeInTheDocument();expect(fetch).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole('button',{name:'通用'}));expect(screen.getByLabelText('名称')).toHaveValue('Unsaved name');
});
