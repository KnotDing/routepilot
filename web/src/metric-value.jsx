import React,{useLayoutEffect,useRef} from 'react';

const fitters=new Map();
let observer=null;
const refitAll=()=>{for(const fit of fitters.values())fit()};
function observeMetric(element,fit){
 if(!fitters.size){
  if(typeof ResizeObserver!=='undefined')observer=new ResizeObserver(entries=>{for(const entry of entries)fitters.get(entry.target)?.()});
  else window.addEventListener('resize',refitAll);
  document.fonts?.addEventListener('loadingdone',refitAll);
 }
 fitters.set(element,fit);observer?.observe(element);
 return()=>{
  observer?.unobserve(element);fitters.delete(element);
  if(!fitters.size){observer?.disconnect();observer=null;window.removeEventListener('resize',refitAll);document.fonts?.removeEventListener('loadingdone',refitAll)}
 };
}

// Keep the complete value on one line. Recalculate only for changed content,
// available space or loaded fonts; all visible cards share one observer.
export function MetricValue({children}){
 const element=useRef(null),refit=useRef(null);
 useLayoutEffect(()=>{
  const value=element.current;
  const fit=()=>{
   const width=value.clientWidth,previous=Number(value.style.getPropertyValue('--metric-fit'))||1;
   const natural=value.firstElementChild.getBoundingClientRect().width/previous;
   if(width<=1||natural<=0)return;
   const scale=Math.min(1,(width-1)/natural);
   if(Math.abs(scale-previous)>.001)value.style.setProperty('--metric-fit',String(scale));
  };
  refit.current=fit;return observeMetric(value,fit);
 },[]);
 useLayoutEffect(()=>{refit.current?.()},[children]);
 return <strong ref={element} className="metric-value"><span className="metric-value-text">{children}</span></strong>;
}
