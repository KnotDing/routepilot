import {useEffect,useState} from 'react';
const key='routepilot.locale',event='routepilot-locale-changed';
export function currentLocale(){return localStorage.getItem(key)||localStorage.getItem('routepilot.routing.locale')||'zh-CN'}
export function useLocale(){
 const [locale,update]=useState(currentLocale);
 useEffect(()=>{const listener=()=>update(currentLocale());window.addEventListener(event,listener);window.addEventListener('storage',listener);return()=>{window.removeEventListener(event,listener);window.removeEventListener('storage',listener)}},[]);
 const setLocale=value=>{localStorage.setItem(key,value);update(value);window.dispatchEvent(new Event(event));try{window.webkit?.messageHandlers?.routepilotLocale?.postMessage(value)}catch{}};
 return [locale,setLocale];
}
