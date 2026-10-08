import React from 'react';
import {useLocale} from './locale';

const sections=[
 {name:'listeners',title:{'zh-CN':'监听与接收',en:'Listeners and ingress'},fields:['listeners','transparent_listeners','ingress_interfaces','interception_enabled']},
 {name:'egress',title:{'zh-CN':'上游网络出口',en:'Upstream network egress'},fields:['egress_interface','source4','source6','gateway4','gateway6','bypass_verified']},
 {name:'forwarding',title:{'zh-CN':'转发参数',en:'Forwarding options'},fields:['timeout_ms','source_port_first','source_port_last','preserve_original']}
];
export function DNSSettingsFields({entries,renderField}){
 const [locale]=useLocale();
 const values=new Map(entries),known=new Set(sections.flatMap(section=>section.fields));
 return <div className="dns-settings-fields">
  {sections.map(section=>{
   const fields=section.fields.filter(name=>values.has(name));
   if(!fields.length)return null;
   return <section className={`form-section dns-${section.name}`} key={section.name}><h3>{section.title[locale]||section.title['zh-CN']}</h3><div className="form-grid">{fields.map(name=>renderField([name,values.get(name)]))}</div></section>;
  })}
  {entries.filter(([name])=>!known.has(name)).map(entry=><div className="form-grid" key={entry[0]}>{renderField(entry)}</div>)}
 </div>;
}
