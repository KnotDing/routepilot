import React from 'react';

const icons={
 Dashboard:<><rect x="3.5" y="3.5" width="7" height="7" rx="1.5"/><rect x="13.5" y="3.5" width="7" height="7" rx="1.5"/><rect x="3.5" y="13.5" width="7" height="7" rx="1.5"/><rect x="13.5" y="13.5" width="7" height="7" rx="1.5"/></>,
 DNS:<><circle cx="12" cy="12" r="8.75"/><path d="M3.5 12h17M12 3.25c-4.9 4.8-4.9 12.7 0 17.5M12 3.25c4.9 4.8 4.9 12.7 0 17.5"/></>,
 Queries:<><path d="M6 3.5h8l4 4v13H6zM14 3.5v4h4M9 11h6M9 14.5h6M9 18h4"/></>,
 Cache:<><path d="M5 4.5h14l2 3-2 3H5l-2-3zM5 11.5h14l2 3-2 3H5l-2-3zM12 19h8"/><circle cx="8" cy="7.5" r=".7"/><circle cx="8" cy="14.5" r=".7"/></>,
 Interception:<><path d="M3.5 7h12m-3-3 3 3-3 3M20.5 17h-12m3-3-3 3 3 3M6 13v7M18 4v7"/></>,
 Hosts:<><rect x="4" y="3.5" width="16" height="7" rx="1.5"/><rect x="4" y="13.5" width="16" height="7" rx="1.5"/><path d="M11 7h5M11 17h5"/><circle cx="7.5" cy="7" r=".7"/><circle cx="7.5" cy="17" r=".7"/></>,
 Overrides:<><path d="M4 6h16M4 12h16M4 18h16"/><circle cx="9" cy="6" r="2"/><circle cx="15" cy="12" r="2"/><circle cx="7" cy="18" r="2"/></>,
 Rule:<><path d="M8 3.5v5l-4.2 10a1.8 1.8 0 0 0 1.7 2.5h13a1.8 1.8 0 0 0 1.7-2.5L16 8.5v-5M7 3.5h10M7 15h10"/><path d="M10 11.5h4"/></>,
 Routing:<><circle cx="12" cy="4.5" r="2"/><circle cx="5" cy="19" r="2"/><circle cx="19" cy="19" r="2"/><path d="M12 6.5v4M5 17v-3.5h14V17M12 10.5v3"/></>,
 BGP:<><circle cx="12" cy="12" r="3"/><circle cx="5" cy="5" r="2"/><circle cx="19" cy="5" r="2"/><circle cx="5" cy="19" r="2"/><circle cx="19" cy="19" r="2"/><path d="m10 10-3.5-3.5M14 10l3.5-3.5M10 14l-3.5 3.5M14 14l3.5 3.5"/></>,
 Peers:<><rect x="3.5" y="4.5" width="17" height="15" rx="2"/><path d="M7 9h10l-2.5-2.5M17 15H7l2.5 2.5"/></>,
 Routes:<><path d="M6 20V5.5a2 2 0 0 1 2-2h9M14.5 1.5l4 2-4 2M18 4v14.5a2 2 0 0 1-2 2H8"/><circle cx="6" cy="20" r="1.5"/><path d="m10 16-3 4 3 2"/></>,
 Health:<><path d="M3 12h4l2.5-6 4.5 12 2.5-6H21"/><circle cx="12" cy="12" r="9"/></>,
 Events:<><rect x="4" y="5" width="16" height="16" rx="2"/><path d="M8 3v4M16 3v4M4 9h16M8 13h2M13 13h3M8 17h2"/></>,
 Settings:<><path d="M4 6h16M4 12h16M4 18h16"/><circle cx="9" cy="6" r="2"/><circle cx="15" cy="12" r="2"/><circle cx="7" cy="18" r="2"/></>,
};

export function NavIcon({page}){
 return <svg className="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{icons[page]||icons.Settings}</svg>;
}
