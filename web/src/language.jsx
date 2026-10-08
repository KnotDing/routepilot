import React,{useEffect} from 'react';
import {useLocale} from './locale';

// The language belongs to the browser's management interface, across backends.
export function LanguageSelector(){
 const [locale,setLocale]=useLocale();
 useEffect(()=>{document.documentElement.lang=locale},[locale]);
 return <label className="language-control">{locale==='en'?'Language':'语言'}<select aria-label={locale==='en'?'Page language':'页面语言'} value={locale} onChange={e=>setLocale(e.target.value)}><option value="zh-CN">中文</option><option value="en">English</option></select></label>;
}
