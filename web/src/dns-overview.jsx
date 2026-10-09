import React from 'react';
import {useT} from './i18n';
import {currentLocale} from './locale';

export function DNSForwardingOverview({settings={},surge={},Table}){
 const t=useT(),snapshot=surge.snapshot||{};
 const synchronized=snapshot.always_real_ip_observed_at&&!snapshot.always_real_ip_observed_at.startsWith('0001');
 return <section className="panel dns-forwarding-overview"><header className="section-heading"><h2>{t('转发配置')}</h2><p>{t('真实 DNS 上游与 Surge 的 IPv6、规则同步状态。')}</p></header><div className="forwarding-grid">
  <section className="forwarding-upstreams"><h3>{t('真实 DNS 上游')}</h3><p>{t(settings.dns?.preserve_original?'透明请求保留原始解析器，直接请求使用配置的上游池':'使用配置的上游池')}</p><Table rows={settings.dns?.upstreams||[]} columns={[{key:'name'},{key:'protocol'},{key:'address',label:t('上游服务器地址')},{key:'enabled',render:r=><span className={'badge '+(r.enabled?'good':'neutral')}>{t(r.enabled?'启用':'禁用')}</span>}]}/></section>
  <section className="forwarding-status"><h3>{t('IPv6 与规则同步')}</h3><dl><dt>Surge IPv6</dt><dd><span className={'badge '+(snapshot.ipv6_state==='enabled'?'good':'neutral')}>{t(({enabled:'已开启',disabled:'已关闭'}[snapshot.ipv6_state])||'尚未确认')}</span></dd><dt>{t('AAAA 回落')}</dt><dd>{t(({block:'关闭 IPv6 时拦截 AAAA（成功空响应）',upstream:'关闭 IPv6 时直接查询上游',surge:'沿用原有分流规则'}[settings.dns?.aaaa_fallback])||'沿用原有分流规则')}</dd><dt>always-real-ip</dt><dd>{t('{count} 条匹配项',{count:snapshot.always_real_ip_count??0})}</dd><dt>{t('最近同步')}</dt><dd>{synchronized?new Date(snapshot.always_real_ip_observed_at).toLocaleString(currentLocale()):t('尚未读取')}</dd></dl>{snapshot.always_real_ip_read_error&&<p className="error">{t('always-real-ip 读取失败，保留最后确认的列表。')}</p>}</section>
 </div></section>;
}
