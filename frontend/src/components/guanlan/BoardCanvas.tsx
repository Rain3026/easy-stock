import {useMemo,useRef,type CSSProperties,type ReactNode} from 'react';
import GridLayout,{useContainerWidth,type Layout} from 'react-grid-layout';
import {aspectRatio,gridBounds,minMaxSize,noCompactor,snapToGrid} from 'react-grid-layout/core';
import {GripVertical,Settings2,Trash2} from 'lucide-react';
import {copy,type BoardConfig,type BoardState,type WidgetDefinition,type WidgetInstance} from '../../lib/guanlan';
import type {WidgetData} from '../../lib/guanlan-data';
import {WidgetContent,widgetIcons} from './Widgets';
import 'react-grid-layout/css/styles.css';
import 'react-resizable/css/styles.css';

type Props={config:BoardConfig;catalog:WidgetDefinition[];state:BoardState|null;data:WidgetData;editing?:boolean;selected?:string;onSelect?:(id:string)=>void;onChange?:(c:BoardConfig)=>void;onRemove?:(id:string)=>void;dragItem?:WidgetInstance|null;onDropItem?:(i:WidgetInstance,p:{x:number;y:number},base?:BoardConfig)=>void;onWidth?:(w:number)=>void};
export function BoardCanvas({config,catalog,state,data,editing=false,selected,onSelect,onChange,onRemove,dragItem,onDropItem,onWidth}:Props){
 const{width,containerRef}=useContainerWidth();
 // A fast native move+release can precede RGL's state commit. Preserve the
 // last constrained layout instead of accepting stale stop-callback sizes.
 const latestResize=useRef<Layout|null>(null);
 const pixels=Math.max(1,Math.round(width||1000)),scale=pixels/120;
 const layout=useMemo<Layout>(()=>config.items.map(item=>{const d=catalog.find(d=>d.id===item.type);return {i:item.id,x:item.layout.x*scale,y:item.layout.y*12,w:item.layout.w*scale,h:item.layout.h*12,minW:(d?.min_w||24)*scale,minH:(d?.min_h||12)*12,maxW:pixels,maxH:2400,resizeHandles:['n','s','e','w','ne','nw','se','sw'],constraints:item.style.lock_ratio?[aspectRatio((item.layout.w*scale)/(item.layout.h*12)),minMaxSize]:undefined}}),[config.items,catalog,scale,pixels]);
 const constraints=useMemo(()=>config.snap?[snapToGrid(20,12),gridBounds,minMaxSize]:[gridBounds,minMaxSize],[config.snap]);
 const withLayout=(l:Layout)=>{const next=copy(config);for(const item of next.items){const p=l.find(v=>v.i===item.id);if(p)item.layout={x:p.x/scale,y:p.y/12,w:p.w/scale,h:p.h/12}};return next};
 const apply=(l:Layout)=>onChange?.(withLayout(l));
 const move=(item:WidgetInstance,dx:number,dy:number)=>{const next=copy(config),v=next.items.find(v=>v.id===item.id)!;v.layout.x=Math.max(0,Math.min(120-v.layout.w,v.layout.x+dx));v.layout.y=Math.max(0,v.layout.y+dy);const collision=next.items.some(o=>o.id!==v.id&&v.layout.x<o.layout.x+o.layout.w&&v.layout.x+v.layout.w>o.layout.x&&v.layout.y<o.layout.y+o.layout.h&&v.layout.y+v.layout.h>o.layout.y);if(!collision)onChange?.(next)};
 const frame=(item:WidgetInstance):ReactNode=><WidgetFrame item={item} selected={selected===item.id} editing={editing} onSelect={()=>onSelect?.(item.id)} onRemove={()=>onRemove?.(item.id)} onMove={(dx,dy)=>move(item,dx,dy)}><WidgetContent item={item} data={data} state={state}/></WidgetFrame>;
 return <div className={`gl-canvas-scroll ${editing?'is-editing':''}`}><div className={editing?'gl-canvas-measure is-editing':'gl-canvas-measure'} ref={el=>{containerRef.current=el;if(el&&onWidth)onWidth(el.clientWidth)}} aria-label={editing?'可编辑看板画布':'看板内容'}>
 {!config.items.length&&<div className="gl-canvas-empty"><span className="gl-plus-grid">＋</span><h3>{editing?'拖入第一个组件':'此配置还没有组件'}</h3><p>{editing?'从右侧组件库拖入，或点击 ＋ 添加；同类组件可重复使用。':'请在看板与组件中编辑配置。'}</p></div>}
 {!editing&&width<720?<div className="gl-canvas-stack">{[...config.items].sort((a,b)=>a.layout.y-b.layout.y||a.layout.x-b.layout.x).map(i=><div key={i.id} style={{minHeight:Math.max(180,i.layout.h*12)}}>{frame(i)}</div>)}</div>:<GridLayout width={pixels} layout={layout} gridConfig={{cols:pixels,rowHeight:1,margin:[0,0],containerPadding:[0,0],maxRows:122400}} dragConfig={{enabled:editing,handle:'.gl-drag-handle',cancel:'a,input,select,textarea,.gl-widget-actions'}} resizeConfig={{enabled:editing,handles:['n','s','e','w','ne','nw','se','sw']}} dropConfig={{enabled:editing&&!!dragItem,defaultItem:dragItem?{w:dragItem.layout.w*scale,h:dragItem.layout.h*12}:{w:40*scale,h:240}}} droppingItem={{i:'__new_widget__',x:0,y:0,w:(dragItem?.layout.w||40)*scale,h:(dragItem?.layout.h||20)*12}} compactor={noCompactor} constraints={constraints} onDragStop={l=>apply(l)} onResizeStart={()=>{latestResize.current=null}} onResize={l=>{latestResize.current=l.map(v=>({...v}))}} onResizeStop={l=>{apply(latestResize.current||l);latestResize.current=null}} onDrop={(l,d)=>{if(d&&dragItem)onDropItem?.(dragItem,{x:d.x/scale,y:d.y/12},withLayout(l))}} onDropDragOver={()=>dragItem?{w:dragItem.layout.w*scale,h:dragItem.layout.h*12}:false} className="gl-grid" style={{minHeight:config.items.length?undefined:editing?540:250}}>{config.items.map(item=><div key={item.id} data-widget-id={item.id} className="gl-grid-cell">{frame(item)}</div>)}</GridLayout>}
 </div></div>;
}
function WidgetFrame({item,children,editing,selected,onSelect,onRemove,onMove}:{item:WidgetInstance;children:ReactNode;editing:boolean;selected:boolean;onSelect:()=>void;onRemove:()=>void;onMove:(dx:number,dy:number)=>void}){
 const Icon=widgetIcons[item.type]||widgetIcons.indexes;const s=item.style;
 const style={'--gl-title-size':`${s.title_size}px`,'--gl-value-size':`${s.value_size}px`,'--gl-text-size':`${s.text_size}px`,'--gl-header-height':`${s.header_height}px`,'--gl-padding':`${s.padding}px`,'--gl-gap':`${s.gap}px`,'--gl-cols':s.columns||'auto-fit','--gl-row-height':`${s.row_height}px`,'--gl-column-width':`${s.column_width}px`,textAlign:s.align} as CSSProperties;
 return <article className={`gl-widget density-${s.density} ${selected&&editing?'is-selected':''}`} style={style} aria-label={s.title||item.type} onMouseDown={()=>{if(editing)onSelect()}}>
 <header className="gl-widget-header">{editing&&<button type="button" className="gl-drag-handle" aria-label={`移动${s.title}`} title="拖动；方向键移动，Shift加速" onKeyDown={e=>{const directions:Record<string,[number,number]>={ArrowLeft:[-1,0],ArrowRight:[1,0],ArrowUp:[0,-1],ArrowDown:[0,1]};const d=directions[e.key];if(d){e.preventDefault();onMove(d[0]*(e.shiftKey?5:1),d[1]*(e.shiftKey?5:1))}}}><GripVertical size={15}/></button>}<Icon size={18}/><h3>{s.title||'未命名组件'}</h3>{editing&&<div className="gl-widget-actions"><button type="button" aria-label={`设置${s.title}`} onClick={onSelect}><Settings2 size={15}/></button><button type="button" aria-label={`移除${s.title}`} onClick={onRemove}><Trash2 size={14}/></button></div>}</header><div className="gl-widget-body">{children}</div>
 </article>;
}
export function BoardThumbnail({config,catalog}:{config:BoardConfig;catalog:WidgetDefinition[]}){
 const max=Math.max(32,...config.items.map(i=>i.layout.y+i.layout.h));
 return <svg className="gl-thumbnail" viewBox={`0 0 1200 ${Math.max(380,max*8)}`} role="img" aria-label={`${config.name}的布局缩略图（不含行情和私人数据）`}><rect width="1200" height={Math.max(380,max*8)} rx="12" fill="#f4f8ff"/>{config.items.map(item=>{const l=item.layout,x=l.x*10+7,y=l.y*8+7,w=Math.max(1,l.w*10-14),h=Math.max(1,l.h*8-14);return <g key={item.id}><rect x={x} y={y} width={w} height={h} rx="7" fill="white" stroke="#cfdef1"/><rect x={x+12} y={y+14} width="7" height="18" rx="2" fill="#2768d5"/><text x={x+28} y={y+29} fill="#23375c" fontSize="16">{(item.style.title||catalog.find(d=>d.id===item.type)?.name||item.type).slice(0,14)}</text>{h>80&&Array.from({length:3},(_,n)=><rect key={n} x={x+16+n*(w-32)/3} y={y+55} width={Math.max(2,(w-50)/3)} height={Math.max(8,h-75)} rx="4" fill={n===0?'#e1edff':'#eff4fa'}/>)}</g>})}{!config.items.length&&<text x="600" y="190" textAnchor="middle" fill="#7a8ba4" fontSize="22">空白配置</text>}</svg>;
}
