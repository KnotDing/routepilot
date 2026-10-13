export const destinations=['local','cache','optimistic_cache','real','surge','blocked','unavailable'];
export const destinationLabels={local:'本地记录',cache:'响应缓存',optimistic_cache:'乐观响应缓存',real:'真实 DNS 上游',surge:'Surge DNS',blocked:'本机拒绝 / 空响应',unavailable:'未发往解析器'};
export const ingressLabels={addressed:'直接请求',intercepted:'透明拦截'};
export function share(count,total){return total>0?count/total:0}
export function visibleSources(sources){return sources?.filter(s=>s.source!=='legacy')}
// Keep the original monitoring totals, and filter complete triples only for
// the client-policy chart. Older two-stage rows have no final-policy kind.
export function visibleClientPolicyFlows(rows){return Array.isArray(rows)?rows.filter(row=>row.final_kind!=='unnamed'):rows}

export const flowLabelWidth=140;
export function flowLabelExtents(lines=1,valueLines=1){return {above:(lines-1)*9+17,below:(lines-1)*9+14+(valueLines-1)*16}}

// A common scale on both sides preserves every ingress/destination edge.
// No minimum ribbon width: a small request share must remain a small share.
export function flowLayout(flows,total,sourceKeys=['addressed','intercepted'],targetKeys=destinations,labelLines=1,geometry={}){
 const sources=sourceKeys.map(key=>({key,count:flows.filter(f=>f.ingress===key).reduce((s,f)=>s+f.queries,0)})).filter(n=>n.count);
 const targets=targetKeys.map(key=>({key,count:flows.filter(f=>f.destination===key).reduce((s,f)=>s+f.queries,0)})).filter(n=>n.count);
 // Only labels next to each other determine their gap. A long name in one
 // column must not stretch every row in all columns.
 const nodeCount=Math.max(sources.length,targets.length,geometry.nodeCount||0),extent=key=>geometry.labelExtents?.get(key)||flowLabelExtents(labelLines),columns=geometry.columns||[sources.map(n=>n.key),targets.map(n=>n.key)];
 const gap=(a,b)=>extent(a).below+extent(b).above+4,gaps=keys=>keys.slice(1).reduce((sum,key,i)=>sum+gap(keys[i],key),0),padding=Math.max(0,...columns.map(gaps),geometry.columns?0:Math.max(0,nodeCount-1)*(flowLabelExtents(labelLines).above+flowLabelExtents(labelLines).below+4)),top=32+Math.max(17,...columns.filter(keys=>keys.length).map(keys=>extent(keys[0]).above)),bottom=8+Math.max(14,...columns.filter(keys=>keys.length).map(keys=>extent(keys.at(-1)).below)),flowHeight=Math.min(100,Math.max(60,nodeCount*8)),height=flowHeight+padding,scale=total?flowHeight/total:0;
 function place(nodes){const keys=nodes.map(n=>n.key);let y=top+(padding-gaps(keys))/2;return nodes.map((n,i)=>{const node={...n,y,height:n.count*scale,used:0};y+=node.height+(nodes[i+1]?gap(n.key,nodes[i+1].key):0);return node})}
 const left=place(sources),right=place(targets),edges=[];
 for(const source of left)for(const target of right){
  const count=flows.filter(f=>f.ingress===source.key&&f.destination===target.key).reduce((s,f)=>s+f.queries,0);
  if(!count)continue;
  const thickness=count*scale,y1=source.y+source.used,y2=target.y+target.used;
  const x1=(geometry.sourceX??210)+14,x2=geometry.targetX??716,c1=x1+(x2-x1)*.44,c2=x1+(x2-x1)*.56;
  edges.push({source:source.key,target:target.key,count,path:`M ${x1} ${y1} C ${c1} ${y1}, ${c2} ${y2}, ${x2} ${y2} L ${x2} ${y2+thickness} C ${c2} ${y2+thickness}, ${c1} ${y1+thickness}, ${x1} ${y1+thickness} Z`});
  source.used+=thickness;target.used+=thickness;
 }
 return {left,right,edges,scale,height,svgHeight:height+top+bottom};
}

// Group small sources without inventing their destination shares. Each ribbon
// comes from observed source/destination counts, never multiplied marginals.
export function sourceFlows(snapshot,sources,selected=''){
 if(selected)return {flows:mergeSourceFlows(snapshot.flows.map(f=>({...f,ingress:selected}))),keys:[selected]};
 if(!Array.isArray(sources)||sources.some(s=>s.counts.completed>0&&(!s.counts.flows?.length)))return null;
 if(sources.reduce((n,s)=>n+(s.counts.flows||[]).reduce((sum,f)=>sum+f.queries,0),0)!==snapshot.completed)return null;
 const ranked=visibleSources(sources).filter(s=>s.counts.completed>0).sort((a,b)=>b.counts.completed-a.counts.completed||a.source.localeCompare(b.source));
 const top=ranked.slice(0,10),rest=ranked.slice(10),flows=[];
 for(const s of ranked)for(const f of s.counts.flows||[])flows.push({...f,ingress:top.includes(s)?s.source:'remaining-sources'});
 return {flows:mergeSourceFlows(flows),keys:[...top.map(s=>s.source),...(rest.length?['remaining-sources']:[])]};
}

function mergeSourceFlows(flows){
 const groups=new Map();
 for(const f of flows){const key=JSON.stringify([f.ingress,f.destination]),row=groups.get(key)||{ingress:f.ingress,destination:f.destination,queries:0};row.queries+=f.queries;groups.set(key,row)}
 return [...groups.values()];
}

export function policyDistribution(policies,field){
 const groups=new Map();
 for(const row of policies){
  const name=row[field],kind=row.other?'overflow':name?'named':'unnamed',key=JSON.stringify([kind,name]);
  const existing=groups.get(key)||{key,kind,name,count:0};existing.count+=row.queries;groups.set(key,existing);
 }
 return [...groups.values()].sort((a,b)=>b.count-a.count||a.key.localeCompare(b.key));
}

export function appendSample(samples,snapshot){
 const at=Date.parse(snapshot.observed_at),previous=samples.at(-1);
 if(!Number.isFinite(at))return samples;
 const reset=previous&&(previous.session!==snapshot.session_started_at||snapshot.received<previous.received||snapshot.completed<previous.completed);
 if(previous&&!reset&&at<=previous.at)return samples;
 const seconds=previous&&!reset?(at-previous.at)/1000:0;
 const sample={at,session:snapshot.session_started_at,received:snapshot.received,completed:snapshot.completed,
  incoming:seconds>0&&seconds<=15?(snapshot.received-previous.received)/seconds:null,
  outgoing:seconds>0&&seconds<=15?(snapshot.completed-previous.completed)/seconds:null};
 return [...(reset?[]:samples),sample].filter(s=>at-s.at<=360000).slice(-121);
}
