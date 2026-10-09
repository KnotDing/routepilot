import React from 'react';
import {createRoot} from 'react-dom/client';
import {ReleaseNotes} from './release-notes';
import './native-notes.css';
window.routepilotNotesAppearance=appearance=>{
 for(const [key,value] of Object.entries(appearance||{}))document.documentElement.style.setProperty('--'+key,value);
};
const payload=window.routepilotReleaseNotes||{};
document.documentElement.lang=payload.locale||'zh-CN';
window.routepilotNotesAppearance(payload.appearance);
createRoot(document.getElementById('root')).render(<ReleaseNotes>{payload.body||''}</ReleaseNotes>);
