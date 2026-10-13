import {useEffect,useState} from 'react';

// Width changes only move horizontal coordinates. SVG text and vertical
// geometry stay in pixels; they never scale with the card width.
export function useFlowCanvas(minimumWidth){
 const [element,ref]=useState(null),[width,setWidth]=useState(minimumWidth);
 useEffect(()=>{
  if(!element)return;
  const resize=()=>setWidth(Math.max(minimumWidth,element.clientWidth));
  resize();
  if(typeof ResizeObserver==='undefined'){window.addEventListener('resize',resize);return()=>window.removeEventListener('resize',resize)}
  const observer=new ResizeObserver(resize);observer.observe(element);return()=>observer.disconnect();
 },[element,minimumWidth]);
 return {ref,width};
}
