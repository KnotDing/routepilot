import React from 'react';
import {it,expect,afterEach} from 'vitest';
import {render,screen,cleanup} from '@testing-library/react';
import {ReleaseNotes} from './release-notes';
afterEach(cleanup);
it('renders headings, lists, GFM tables and code without executing HTML',()=>{
 const {container}=render(<ReleaseNotes>{'# Release\n\n- **Fixed**\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\n`code`\n\n<script>alert(1)</script>\n\n![tracker](https://example.invalid/x)'}</ReleaseNotes>);
 expect(screen.getByRole('heading',{name:'Release'})).toBeInTheDocument();expect(screen.getByText('Fixed').tagName).toBe('STRONG');expect(screen.getByRole('table')).toBeInTheDocument();expect(screen.getByText('code').tagName).toBe('CODE');expect(container.querySelector('script,img')).toBeNull();
});
it('rejects executable link schemes and resolves repository-relative links',()=>{
 render(<ReleaseNotes>{'[safe](./compare) [evil](javascript:alert%281%29) [data](data:text/html,evil)'}</ReleaseNotes>);
 expect(screen.getByText('safe')).toHaveAttribute('href','https://gitea.knotding.com/knot/routepilot/compare');for(const name of ['evil','data'])expect(screen.getByText(name)).toHaveAttribute('href','');
});
