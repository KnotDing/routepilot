import React,{createContext,useContext,useEffect,useState} from 'react';

const FlowHeightContext=createContext(0);
export const useSharedFlowHeight=()=>useContext(FlowHeightContext);

// Match intrinsic chart heights only when both cards occupy the same row.
// Observe natural heights, never the expanded SVG height, to allow shrinking.
export function FlowChartGrid({children}){
 const [element,ref]=useState(null),[height,setHeight]=useState(0);
 useEffect(()=>{
  if(!element)return;
  const update=()=>{
   const cards=[...element.children],charts=cards.map(card=>card.querySelector('svg.monitor-sankey'));
   const boxes=cards.map(card=>card.getBoundingClientRect());
   const parallel=cards.length===2&&charts.every(Boolean)&&Math.abs(boxes[0].y-boxes[1].y)<1&&Math.abs(boxes[0].x-boxes[1].x)>1;
   const next=parallel?Math.max(...charts.map(chart=>Number(chart.dataset.naturalHeight))):0;
   setHeight(old=>old===next?old:next);
  };
  update();
  const changes=new MutationObserver(update);changes.observe(element,{subtree:true,childList:true,attributes:true,attributeFilter:['data-natural-height']});
  const resize=typeof ResizeObserver==='undefined'?null:new ResizeObserver(update);
  if(resize)resize.observe(element);else window.addEventListener('resize',update);
  return()=>{changes.disconnect();if(resize)resize.disconnect();else window.removeEventListener('resize',update)};
 },[element]);
 return <FlowHeightContext.Provider value={height}><div ref={ref} className="monitor-flow-grid">{children}</div></FlowHeightContext.Provider>;
}
