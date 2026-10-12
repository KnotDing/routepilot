import React from 'react';
import {it,expect,afterEach} from 'vitest';
import {render,screen,cleanup} from '@testing-library/react';
import {DownloadProgress} from './download-progress';
afterEach(cleanup);
it('renders actual bytes and a percentage only when the size is known',()=>{
 const {rerender}=render(<DownloadProgress bytes={1048576} total={2097152}/>);
 expect(screen.getByRole('progressbar')).toHaveAttribute('value','50');
 expect(screen.getByRole('status')).toHaveTextContent('1 MiB / 2 MiB（50%）');
 rerender(<DownloadProgress bytes={1048576}/>);
 expect(screen.getByRole('progressbar')).not.toHaveAttribute('value');
 expect(screen.getByRole('status')).toHaveTextContent('1 MiB，总大小未知');
});
