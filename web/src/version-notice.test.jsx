import React from 'react';
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,render,screen} from '@testing-library/react';
import release from '../../internal/buildinfo/release.json';
import {VersionNotice} from './version-notice';

vi.mock('../../internal/buildinfo/release.json',()=>({default:{version:'1.0.1-beta.7',build:'70'}}));
const backend={id:'atlas',name:'Atlas'};
const status=(version,build='50')=>({software:{version,build}});
afterEach(()=>{cleanup();localStorage.clear();release.version='1.0.1-beta.7';release.build='70'});

it.each([
 ['1.0.1-beta.7','1.0.0','本地使用 Beta 测试版，远端使用正式版'],
 ['1.0.0','1.0.1-beta.7','本地使用正式版，远端使用 Beta 测试版'],
 ['v1.0.1-beta.7',' v1.0.0 ','本地使用 Beta 测试版，远端使用正式版'],
 ['1.0.1-beta.7','1.0.1','本地使用 Beta 测试版，远端使用正式版']
])('uses a quiet informational hint for remote release channels: %s / %s',(local,remote,title)=>{
 release.version=local;
 render(<VersionNotice status={status(remote)} backend={backend}/>);
 const notice=screen.getByRole('note',{name:'版本检查'});
 expect(notice).toHaveClass('version-channel-notice');
 expect(screen.getByText(title)).toBeInTheDocument();
 expect(notice).toHaveTextContent(local);
 expect(notice).toHaveTextContent(remote.trim());
 expect(notice).toHaveTextContent('构建 70');
 expect(notice).toHaveTextContent('构建 50');
 expect(screen.queryByRole('alert')).not.toBeInTheDocument();
 expect(screen.queryByText(/请更新/)).not.toBeInTheDocument();
});

it.each([
 ['1.0.1-beta.7','1.0.1-beta.6'],
 ['1.0.1-beta.7','1.0.1-beta.7'],
 ['1.0.1','1.0.0'],
 ['1.0.1-beta.7','unknown'],
 ['1.0.1-beta.7','1.0.1-rc.1'],
 ['1.0.1','1.0.1-beta.0']
])('keeps the alert for same-channel differences or unknown formats: %s / %s',(local,remote)=>{
 release.version=local;
 render(<VersionNotice status={status(remote)} backend={backend}/>);
 expect(screen.getByRole('alert')).toHaveTextContent('前后端版本不一致，请更新');
 expect(screen.queryByRole('note')).not.toBeInTheDocument();
});

it('retains the normal mismatch alert on a local connection even across channels',()=>{
 render(<VersionNotice status={status('1.0.0')}/>);
 expect(screen.getByRole('alert')).toHaveTextContent('前后端版本不一致，请更新');
});

it.each([null,'',' '])('keeps missing backend build information as an alert: %s',build=>{
 render(<VersionNotice status={status('1.0.0',build)} backend={backend}/>);
 expect(screen.getByRole('alert')).toHaveTextContent('无法确认后台版本，请更新后台服务');
});

it('shows no notice when both version and build match',()=>{
 render(<VersionNotice status={status('v1.0.1-beta.7','70')} backend={backend}/>);
 expect(screen.queryByLabelText('版本检查')).not.toBeInTheDocument();
});

it('localizes the quiet channel hint',()=>{
 localStorage.setItem('routepilot.locale','en');
 render(<VersionNotice status={status('1.0.0')} backend={backend}/>);
 expect(screen.getByRole('note')).toHaveTextContent('The local interface uses a Beta release; the remote backend uses a stable release.');
 expect(screen.queryByRole('alert')).not.toBeInTheDocument();
});
