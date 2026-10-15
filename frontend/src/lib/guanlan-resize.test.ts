import {describe,it,expect} from 'vitest';
import {readableResize,resizedItem,contentScale} from './guanlan-resize';
import type {WidgetInstance} from './guanlan';
const context={cols:1000,maxRows:10000,containerWidth:1000,containerHeight:0,rowHeight:1,margin:[0,0] as const,layout:[]};
const box={i:'one',x:200,y:200,w:400,h:240,minW:200,minH:120,maxW:1000,maxH:2400};
describe('component resize contract',()=>{
 it.each(['nw','ne','sw','se'] as const)('keeps ratio at %s including both bounds',handle=>{
  const resize=readableResize(box,false).constrainSize!;
  for(const [w,h] of [[450,270],[10,10],[2000,2000]]){const next=resize(box,w,h,handle,context);expect(next.w/next.h).toBeCloseTo(400/240,8);expect(next.w).toBeGreaterThanOrEqual(200);expect(next.h).toBeGreaterThanOrEqual(120);expect(next.w).toBeLessThanOrEqual(handle.includes('w')?600:800);expect(next.h).toBeLessThanOrEqual(handle.includes('n')?440:2400)}
 });
 it.each(['n','s','w','e'] as const)('allows single axis on %s; explicit lock overrides',handle=>{
  const next=readableResize(box,false).constrainSize!(box,handle==='w'||handle==='e'?300:400,handle==='n'||handle==='s'?180:240,handle,context);
  expect(next).toEqual(handle==='w'||handle==='e'?{w:300,h:240}:{w:400,h:180});const locked=readableResize(box,true).constrainSize!(box,next.w,next.h,handle,context);expect(locked.w/locked.h).toBeCloseTo(400/240);
 });
 it('retains a stable typography basis through cycles and single-axis adaptation',()=>{
  const item={layout:{x:0,y:0,w:40,h:20},style:{scale_content:true,scale_base_w:40,scale_base_h:20}} as WidgetInstance;
  let live=item;for(let n=0;n<20;n++){live=resizedItem(live,{...live.layout,w:60,h:30},true);expect(contentScale(live)).toBeCloseTo(1.5,12);live=resizedItem(live,item.layout,true)}expect(contentScale(live)).toBe(1);
  live=resizedItem(live,{...live.layout,w:30},false);expect(contentScale(live)).toBe(1);expect(contentScale(resizedItem(live,{...live.layout,w:60,h:40},true))).toBe(2);
  expect(contentScale({...item,style:{...item.style,scale_content:undefined}})).toBe(1);
 });
});
