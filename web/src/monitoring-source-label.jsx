import React,{useMemo,useState} from 'react';
import {useT} from './i18n';
import {sourceName} from './monitoring-history';
import {useMonitoringDisplay} from './monitoring-preferences';
import {flowLabelWidth} from './monitoring-data';

// Conservative glyph widths keep full names and IPv6 addresses inside the
// source column, without requiring canvas or hiding part of the label.
export function wrapSourceLabel(value,maxWidth=180){
 const parts=Array.from(new Intl.Segmenter(undefined,{granularity:'grapheme'}).segment(value),part=>part.segment),lines=[];let line='',width=0;
 for(const part of parts){const size=/^[ .il:]+$/.test(part)?4:/^[0-9]+$/.test(part)?8:/^[\x00-\x7f]+$/.test(part)&&!/[MW]/.test(part)?8:14;if(width+size>maxWidth&&line){lines.push(line);line='';width=0}line+=part;width+=size}
 if(line)lines.push(line);return lines.length?lines:[''];
}

export function wrapFlowValue(value){return wrapSourceLabel(value,flowLabelWidth*14/12)}

export function useSourceLabels(settings,maxWidth=180){
 const t=useT(),display=useMonitoringDisplay(),[ips,setIPs]=useState({}),names=useMemo(()=>new Map(settings.filter(row=>row.name?.trim()).map(row=>[row.ip,row.name.trim()])),[settings]);
 const label=key=>display.protected?display.source(key):key==='remaining-sources'?t('其余来源'):names.has(key)&&!ips[key]?names.get(key):sourceName(key,t);
 return {label,lines:key=>wrapSourceLabel(label(key),maxWidth),named:key=>!display.protected&&names.has(key),showingIP:key=>Boolean(ips[key]),toggle:key=>setIPs(old=>({...old,[key]:!old[key]})),title:key=>display.protected?label(key):names.has(key)?`${names.get(key)} · ${key}`:label(key)};
}

export function FlowSourceLabel({node,labels,value,x=190}){
 const t=useT(),lines=labels.lines(node.key),center=node.y+node.height/2,named=labels.named(node.key);
 const toggle=()=>labels.toggle(node.key);
 return <><text x={x} y={center-(lines.length-1)*9-4} textAnchor="end" className="monitor-source-label" role={named?'button':undefined} tabIndex={named?0:undefined} aria-label={named?t(labels.showingIP(node.key)?'切换为备注名称：{source}':'切换为 IP 地址：{source}',{source:node.key}):undefined} onClick={named?toggle:undefined} onKeyDown={named?event=>{if(event.key==='Enter'||event.key===' '){event.preventDefault();toggle()}}:undefined}>
  {lines.map((line,i)=><tspan key={i} x={x} dy={i?18:0}>{line}</tspan>)}<title>{labels.title(node.key)}</title>
 </text><text x={x} y={center+(lines.length-1)*9+12} textAnchor="end" className="monitor-svg-value">{wrapFlowValue(value).map((line,i)=><tspan key={i} x={x} dy={i?16:0}>{line}</tspan>)}</text></>;
}
