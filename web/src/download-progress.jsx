import React from 'react';
import {useT} from './i18n';
import {currentLocale} from './locale';

export function DownloadProgress({bytes,total}){
 const t=useT(),hasBytes=Number.isFinite(bytes)&&bytes>=0,received=hasBytes?bytes:0,known=Number.isFinite(total)&&total>0&&received<=total;
 const size=value=>`${(value/1048576).toLocaleString(currentLocale(),{maximumFractionDigits:1})} MiB`;
 const percent=known?received/total*100:null;
 return <div className="download-progress"><progress aria-label={t('安装包下载进度')} max={100} {...(known?{value:percent}:{})}/><span role="status">{known?t('已下载 {bytes} / {total}（{percent}%）',{bytes:size(received),total:size(total),percent:percent.toLocaleString(currentLocale(),{maximumFractionDigits:1})}):hasBytes?t('已下载 {bytes}，总大小未知',{bytes:size(received)}):t('正在下载，等待进度信息…')}</span></div>;
}
