import {LanguageSelector} from './language';
import React from 'react';
import {it,expect,afterEach,vi} from 'vitest';
import {render,screen,fireEvent,cleanup,within} from '@testing-library/react';
import {About} from './about';
import {Settings} from './main';
import release from '../../internal/buildinfo/release.json';
afterEach(()=>{cleanup();localStorage.clear();vi.unstubAllGlobals()});

it('separates the web release from the selected remote backend identity',()=>{
 render(<About backend={{name:'Office',url:'https://192.0.2.10:1055'}} status={{name:'Remote controller',software:{version:'0.3.9',build:'8',os:'linux',arch:'amd64',go_version:'go1.26.1'},go_runtime:{goroutines:12,max_procs:4,heap_objects_bytes:1048576,memory_total_bytes:4194304,process_rss_bytes:10485760,gc_cycles:3},state:'HEALTHY',uptime_seconds:90061}}/>);
 const ui=within(screen.getByRole('region',{name:'网页界面'})),remote=within(screen.getByRole('region',{name:'当前后端'}));
 const go=within(screen.getByRole('region',{name:'Go 运行情况'}));
 expect(ui.getByText(release.version)).toBeInTheDocument();expect(remote.getByText('0.3.9')).toBeInTheDocument();
 expect(remote.getByText('https://192.0.2.10:1055')).toBeInTheDocument();expect(remote.getByText('linux · amd64')).toBeInTheDocument();expect(remote.getByText('1 天 1 小时 1 分钟 1 秒')).toBeInTheDocument();
 expect(go.getByText('12')).toBeInTheDocument();expect(go.getByText('4')).toBeInTheDocument();expect(go.getByText('1 MiB')).toBeInTheDocument();expect(go.getByText('4 MiB')).toBeInTheDocument();expect(go.getByText('10 MiB')).toBeInTheDocument();expect(go.getByText('3')).toBeInTheDocument();
 expect(remote.queryByText('服务名称')).not.toBeInTheDocument();expect(remote.queryByText('Remote controller')).not.toBeInTheDocument();
});
it('handles older backends without inventing their version and supports English',()=>{
 localStorage.setItem('routepilot.locale','en');
 const {rerender}=render(<><LanguageSelector/><About status={{name:'Old backend',uptime_seconds:0}}/></>);
 const backend=within(screen.getByRole('region',{name:'Current backend'}));
 const go=within(screen.getByRole('region',{name:'Go runtime'}));
 expect(backend.queryByText(release.version)).not.toBeInTheDocument();expect(backend.getByRole('status')).toHaveTextContent('does not provide version');expect(backend.getByText('0 s')).toBeInTheDocument();
 expect(go.getByRole('status')).toHaveTextContent('does not provide Go runtime metrics');
 expect(backend.queryByText('Service name')).not.toBeInTheDocument();expect(backend.queryByText('Old backend')).not.toBeInTheDocument();
 rerender(<><LanguageSelector/><About status={{software:{version:'1.2.3',build:'77',os:'darwin',arch:'arm64'}}}/></>);
 expect(backend.queryByRole('status')).not.toBeInTheDocument();expect(screen.getByText('macOS · arm64')).toBeInTheDocument();
 fireEvent.change(screen.getByLabelText(/页面语言|Page language/),{target:{value:'zh-CN'}});expect(screen.getByText('当前后端')).toBeInTheDocument();
});
it('opens About without a settings form and preserves unsaved edits across tabs',()=>{
 const fetch=vi.fn();vi.stubGlobal('fetch',fetch);
 render(<Settings settings={{general:{name:'Controller',theme:'system'}}} refresh={()=>{}} notify={()=>{}}/>);
 fireEvent.change(screen.getByLabelText('主题'),{target:{value:'dark'}});
 fireEvent.click(screen.getByRole('button',{name:'关于'}));
 expect(screen.getByRole('region',{name:'网页界面'})).toBeInTheDocument();expect(screen.queryByRole('button',{name:'保存设置'})).not.toBeInTheDocument();expect(fetch).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole('button',{name:'通用'}));expect(screen.getByLabelText('主题')).toHaveValue('dark');
});
