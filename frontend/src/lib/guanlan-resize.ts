import type {LayoutConstraint} from 'react-grid-layout/core';
import type {WidgetInstance,WidgetLayout} from './guanlan';

export const contentScale=(item:WidgetInstance)=>item.style.scale_content?Math.sqrt(item.layout.w*item.layout.h/((item.style.scale_base_w||item.layout.w)*(item.style.scale_base_h||item.layout.h))):1;
// Store a stable basis. Single-axis adaptation retains typography; corners scale it.
export function resizedItem(item:WidgetInstance,layout:WidgetLayout,proportional:boolean):WidgetInstance{
 const s=contentScale(item),style={...item.style};
 if(style.scale_content){
  style.scale_base_w=proportional?(style.scale_base_w||item.layout.w):layout.w/s;
  style.scale_base_h=proportional?(style.scale_base_h||item.layout.h):layout.h/s;
 }
 return {...item,layout,style};
}
export function readableResize(start:{w:number;h:number},locked:boolean):LayoutConstraint{
 return {name:'guanlan-readable-ratio',constrainSize(item,w,h,handle,{cols,maxRows}){
  const minW=item.minW||1,minH=item.minH||1;
  const maxW=Math.min(item.maxW||cols,handle.includes('w')?item.x+item.w:cols-item.x);
  const maxH=Math.min(item.maxH||2400,handle.includes('n')?item.y+item.h:maxRows-item.y);
  if(handle.length===2||locked){
   const dw=w/start.w-1,dh=h/start.h-1;
   let factor=handle==='n'||handle==='s'?h/start.h:handle==='w'||handle==='e'?w/start.w:Math.abs(dw)>=Math.abs(dh)?w/start.w:h/start.h;
   factor=Math.max(Math.max(minW/start.w,minH/start.h),Math.min(Math.min(maxW/start.w,maxH/start.h),factor));
   return {w:start.w*factor,h:start.h*factor};
  }
  return {w:Math.max(minW,Math.min(maxW,w)),h:Math.max(minH,Math.min(maxH,h))};
 }};
}
