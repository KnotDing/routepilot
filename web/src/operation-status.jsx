import React,{useEffect,useState} from 'react';
import {useT} from './i18n';

export function OperationNotice(){
 const t=useT(),[tasks,setTasks]=useState({});
 useEffect(()=>{
  const changed=e=>setTasks(previous=>{
   const next={...previous};
   if(e.detail.phase==='finished')delete next[e.detail.token];
   else next[e.detail.token]=e.detail;
   return next;
  });
  window.addEventListener('routepilot-operation',changed);
  return()=>window.removeEventListener('routepilot-operation',changed);
 },[]);
 const values=Object.values(tasks);
 if(!values.length)return null;
 return <section className="notice" role="status">{t(values.some(task=>task.phase==='running')?'后台任务已接收，正在处理；完成后会显示结果。':'正在提交操作…')}</section>;
}
