import React from 'react';
const paths={
 Dashboard:'M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 14h7v7h-7z',
 DNS:'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18 M3 12h18 M12 3c-5 5-5 13 0 18 M12 3c5 5 5 13 0 18',
 Queries:'M5 3h14v18H5z M8 7h8 M8 11h8 M8 15h5',
 Cache:'M4 7h16v13H4z M3 3h18v4H3z M9 12h6',
 Interception:'M3 7h12 M12 4l3 3-3 3 M21 17H9 M12 14l-3 3 3 3 M5 13v7 M19 4v7',
 Hosts:'M4 4h16v6H4z M4 14h16v6H4z M7 7h.01 M7 17h.01 M11 7h6 M11 17h6',
 Overrides:'M4 7h16 M4 17h16 M8 4v6 M16 14v6',
 Rule:'M8 3v5l-4 10a2 2 0 0 0 2 3h12a2 2 0 0 0 2-3L16 8V3 M6 3h12 M7 14h10',
 Routing:'M12 3v6 M5 15v6 M19 15v6 M5 15V9h14v6 M9 3h6 M2 21h6 M16 21h6',
 BGP:'M9 3h6v6H9z M2 15h6v6H2z M16 15h6v6h-6z M12 9v3 M5 15v-3h14v3',
 Peers:'M8 12h8 M8 9l-3 3 3 3 M16 9l3 3-3 3 M3 5h18v14H3z',
 Routes:'M4 5h11 M12 2l3 3-3 3 M20 19H9 M12 16l-3 3 3 3 M4 5v14h1 M20 19V5h-1',
 Health:'M3 12h4l3-7 4 14 3-7h4',
 Events:'M5 4h14v16H5z M9 8h6 M9 12h6 M9 16h3',
 Settings:'M4 6h16 M4 12h16 M4 18h16 M8 3v6 M16 9v6 M10 15v6',
};
export function NavIcon({page}){return <svg className="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d={paths[page]||paths.Settings}/></svg>}
