import React from 'react';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import './release-notes.css';

// Release content is untrusted. Render Markdown without raw HTML or remote images.
function releaseLink(value) {
 try {
  const url=new URL(value,'https://gitea.knotding.com/knot/routepilot/');
  return ['http:','https:'].includes(url.protocol)?url.href:'';
 } catch { return ''; }
}
export function ReleaseNotes({children}) {
 return <div className="markdown-notes"><Markdown remarkPlugins={[remarkGfm]} skipHtml urlTransform={releaseLink} components={{img:()=>null,a:({node,...props})=><a {...props} target="_blank" rel="noopener noreferrer"/>}}>{children}</Markdown></div>;
}
