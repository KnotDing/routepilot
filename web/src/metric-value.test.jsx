import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,cleanup,act} from '@testing-library/react';
import {MetricValue} from './metric-value';

afterEach(()=>{cleanup();vi.unstubAllGlobals();vi.restoreAllMocks()});
function measurements(){
 let width=100;
 vi.spyOn(HTMLElement.prototype,'clientWidth','get').mockImplementation(()=>width);
 vi.spyOn(HTMLElement.prototype,'getBoundingClientRect').mockImplementation(function(){return {width:this.textContent.length*10*(Number(this.parentElement?.style.getPropertyValue('--metric-fit'))||1)}});
 return next=>{width=next};
}
it('shrinks complete values, recovers on resize and refits when content changes with one observer',()=>{
 const resize=measurements(),instances=[];
 vi.stubGlobal('ResizeObserver',class{constructor(callback){this.callback=callback;this.observe=vi.fn();this.unobserve=vi.fn();this.disconnect=vi.fn();instances.push(this)}});
 const {rerender,unmount}=render(<><MetricValue>1537234</MetricValue><MetricValue>1537234123456789</MetricValue></>);
 const short=screen.getByText('1537234').parentElement,long=screen.getByText('1537234123456789').parentElement;
 expect(instances).toHaveLength(1);expect(instances[0].observe).toHaveBeenCalledTimes(2);
 expect(short.style.getPropertyValue('--metric-fit')).toBe('');
 expect(Number(long.style.getPropertyValue('--metric-fit'))).toBeCloseTo(99/160);
 resize(220);act(()=>instances[0].callback([{target:short},{target:long}]));expect(Number(long.style.getPropertyValue('--metric-fit'))).toBe(1);
 rerender(<><MetricValue>15372341234567891537234123</MetricValue><MetricValue>1537234123456789</MetricValue></>);
 expect(Number(short.style.getPropertyValue('--metric-fit'))).toBeCloseTo(219/260);
 unmount();expect(instances[0].disconnect).toHaveBeenCalledOnce();
});
it('ignores hidden zero widths and supports window resize without ResizeObserver',()=>{
 const resize=measurements();vi.stubGlobal('ResizeObserver',undefined);resize(0);
 const {unmount}=render(<MetricValue>1537234123456789</MetricValue>);
 const value=screen.getByText('1537234123456789').parentElement;
 expect(value.style.getPropertyValue('--metric-fit')).toBe('');
 resize(80);fireResize();expect(Number(value.style.getPropertyValue('--metric-fit'))).toBeCloseTo(79/160);
 unmount();
});
function fireResize(){act(()=>window.dispatchEvent(new Event('resize')))}
