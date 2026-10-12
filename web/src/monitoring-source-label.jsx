import React,{useMemo,useState} from 'react';
import {useT} from './i18n';
import {sourceName} from './monitoring-history';

// Conservative glyph widths keep full names and IPv6 addresses inside the
// source column, without requiring canvas or hiding part of the label.
export function wrapSourceLabel(value){
 const parts=Array.from(new Intl.Segmenter(undefined,{granularity:'grapheme'}).segment(value),part=>part.segment),lines=[];let line='',width=0;
 for(const part of parts){const size=/^[ .il:]+$/.test(part)?6:/^[0-9]+$/.test(part)?12:/^[\x00-\x7f]+$/.test(part)&&!/[MW]/.test(part)?13:20;if(width+size>180&&line){lines.push(line);line='';width=0}line+=part;width+=size}
 if(line)lines.push(line);return lines.length?lines:[''];
}

export function useSourceLabels(settings){
 const t=useT(),[ips,setIPs]=useState({}),names=useMemo(()=>new Map(settings.filter(row=>row.name?.trim()).map(row=>[row.ip,row.name.trim()])),[settings]);
 const label=key=>key==='remaining-sources'?t('其余来源'):names.has(key)&&!ips[key]?names.get(key):sourceName(key,t);
 return {label,lines:key=>wrapSourceLabel(label(key)),named:key=>names.has(key),showingIP:key=>Boolean(ips[key]),toggle:key=>setIPs(old=>({...old,[key]:!old[key]})),title:key=>names.has(key)?`${names.get(key)} · ${key}`:label(key)};
}

export function FlowSourceLabel({node,labels,value}){
 const t=useT(),lines=labels.lines(node.key),center=node.y+node.height/2,named=labels.named(node.key);
 const toggle=()=>labels.toggle(node.key);
 return <><text x="190" y={center-(lines.length-1)*11-5} textAnchor="end" className="monitor-source-label" role={named?'button':undefined} tabIndex={named?0:undefined} aria-label={named?t(labels.showingIP(node.key)?'切换为备注名称：{source}':'切换为 IP 地址：{source}',{source:node.key}):undefined} onClick={named?toggle:undefined} onKeyDown={named?event=>{if(event.key==='Enter'||event.key===' '){event.preventDefault();toggle()}}:undefined}>
  {lines.map((line,i)=><tspan key={i} x="190" dy={i?22:0}>{line}</tspan>)}<title>{labels.title(node.key)}</title>
 </text><text x="190" y={center+(lines.length-1)*11+16} textAnchor="end" className="monitor-svg-value">{value}</text></>;
}
