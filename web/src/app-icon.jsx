import React,{useId} from 'react';

export function AppIcon(){
 const id=useId();
 return <svg className="brand-icon" viewBox="0 0 64 64" role="img" aria-label="RoutePilot"><defs><linearGradient id={id} x1="0" y1="0" x2="1" y2="1"><stop stopColor="#3769ed"/><stop offset="1" stopColor="#2548bd"/></linearGradient></defs><rect x="1" y="1" width="62" height="62" rx="15" fill={`url(#${id})`}/><path d="M17 32h10c8 0 7-13 15-13h5M27 32c8 0 7 13 15 13h5" fill="none" stroke="#fff" strokeWidth="4.5" strokeLinecap="round"/><circle cx="17" cy="32" r="5" fill="#fff"/><circle cx="47" cy="19" r="5" fill="#8af2db"/><circle cx="47" cy="45" r="5" fill="#fff"/></svg>;
}
