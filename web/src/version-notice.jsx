import React from 'react';
import release from '../../internal/buildinfo/release.json';
import {useT} from './i18n';

export function VersionNotice({status,backend=null}){
 const t=useT();
 if(!status)return null;
 const software=status.software||{};
 const complete=software.version!=null&&String(software.version).trim()!==''&&software.build!=null&&String(software.build).trim()!=='';
 const version=String(software.version||'').trim().replace(/^v/,'');
 const build=String(software.build??'').trim();
 if(complete&&version===release.version.replace(/^v/,'')&&build===String(release.build))return null;
 return <section className="notice version-notice" role="alert" aria-label={t('版本检查')}>
  <strong>{t(complete?'前后端版本不一致，请更新':'无法确认后台版本，请更新后台服务')}</strong>
  <p>{t('网页界面：{version}（构建 {build}）',{version:release.version,build:release.build})}{complete&&' · '+t('后台：{version}（构建 {build}）',{version:software.version,build:software.build})}</p>
  <p>{backend?t(status.online_update?'可在下方检查并升级远程后台；本机 App 需单独更新。':'此后台尚未支持在线升级，请先在远程主机手动更新一次，并将本机 App 更新至同一发布版本。'):t('请通过“设置 → 软件更新”在线升级 App 与本机后台；后台更新完成后请刷新页面。')}</p>
 </section>;
}
