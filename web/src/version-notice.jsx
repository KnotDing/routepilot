import React from 'react';
import release from '../../internal/buildinfo/release.json';
import {useT} from './i18n';

// Only recognized stable and numbered Beta versions qualify for a channel hint.
// Incomplete identities and unknown version formats retain the normal warning.
function releaseChannel(raw){
 const version=String(raw||'').trim().replace(/^v/,'');
 const core='(?:0|[1-9][0-9]*)\\.(?:0|[1-9][0-9]*)\\.(?:0|[1-9][0-9]*)';
 if(new RegExp('^'+core+'$').test(version))return 'stable';
 if(new RegExp('^'+core+'-beta\\.[1-9][0-9]*$').test(version))return 'beta';
 return null;
}

export function VersionNotice({status,backend=null}){
 const t=useT();
 if(!status)return null;
 const software=status.software||{};
 const complete=software.version!=null&&String(software.version).trim()!==''&&software.build!=null&&String(software.build).trim()!=='';
 const version=String(software.version||'').trim().replace(/^v/,'');
 const build=String(software.build??'').trim();
 if(complete&&version===release.version.replace(/^v/,'')&&build===String(release.build))return null;
 const localChannel=releaseChannel(release.version),remoteChannel=releaseChannel(version);
 if(backend&&complete&&localChannel&&remoteChannel&&localChannel!==remoteChannel)return <section className="notice version-notice version-channel-notice" role="note" aria-label={t('版本检查')}>
  <strong>{t(localChannel==='beta'?'本地使用 Beta 测试版，远端使用正式版':'本地使用正式版，远端使用 Beta 测试版')}</strong>
  <p>{t('网页界面：{version}（构建 {build}）',{version:release.version,build:release.build})}{' · '+t('后台：{version}（构建 {build}）',{version:software.version,build:software.build})}</p>
 </section>;
 return <section className="notice version-notice" role="alert" aria-label={t('版本检查')}>
  <strong>{t(complete?'前后端版本不一致，请更新':'无法确认后台版本，请更新后台服务')}</strong>
  <p>{t('网页界面：{version}（构建 {build}）',{version:release.version,build:release.build})}{complete&&' · '+t('后台：{version}（构建 {build}）',{version:software.version,build:software.build})}</p>
  <p>{t(backend?'请通过“设置 → 软件更新”升级远端完整 App、管理页面和网络服务；运行此窗口的本机 App 需在本机更新。':'请通过“设置 → 软件更新”升级该 Mac 的完整 App、管理页面和网络服务。')}</p>
 </section>;
}
