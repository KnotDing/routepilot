import {useEffect,useState} from 'react';

export function useWindowVisible(){
 const [visible,setVisible]=useState(()=>document.visibilityState!=='hidden');
 useEffect(()=>{
  const onDocumentVisibility=()=>setVisible(document.visibilityState!=='hidden');
  const onWindowVisibility=event=>setVisible(Boolean(event.detail?.visible));
  document.addEventListener('visibilitychange',onDocumentVisibility);
  window.addEventListener('routepilot-window-visibility',onWindowVisibility);
  return()=>{
   document.removeEventListener('visibilitychange',onDocumentVisibility);
   window.removeEventListener('routepilot-window-visibility',onWindowVisibility);
  };
 },[]);
 return visible;
}
