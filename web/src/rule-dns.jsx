import {message} from './i18n';
import React from 'react';
import {useLocale} from './locale';

export function RuleDNS({result}) {
 const [locale]=useLocale(),en=locale==='en',trace=result.rule_dns,notes=result.rule_notes||[];
 if(!trace&&!notes.length)return null;
 const states=en?{completed:'Completed',failed:'Failed',incomplete:'Incomplete'}:{completed:'已完成',failed:'失败',incomplete:'未完成'};
 return <section className="panel"><h3>{en?'DNS for rule evaluation':'规则求值 DNS 解析'}</h3>{trace&&<><p>{en?'Resolution status: ':'解析状态：'}<strong>{states[trace.state]||trace.state}</strong></p><p>{trace.lookup_performed?(en?'Performed an explicit Surge DNS lookup':'已触发一次 Surge DNS 补查'):(en?'Resolved during Surge rule evaluation':'由 Surge 在规则求值时完成解析')}{trace.re_evaluated&&(en?' · Re-evaluated the rules':' · 已重新求值')}</p>{trace.reason&&<p role="alert" className="error">{message(trace.reason,locale)}</p>}</>}{notes.length>0&&<details><summary>{en?'Surge evaluation notes':'Surge 原始求值记录'}</summary><ul>{notes.map((note,i)=><li key={i}>{note}</li>)}</ul></details>}</section>;
}
