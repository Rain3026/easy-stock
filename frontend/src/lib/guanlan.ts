import {useCallback,useEffect,useState} from 'react';
import {requestJSON,type BackendConfig} from './backend';

export type WidgetType='volume'|'indexes'|'breadth'|'emotion'|'themes'|'debate'|'risk'|'watchlist';
export type WidgetLayout={x:number;y:number;w:number;h:number};
export type WidgetStyle={title:string;title_size:number;value_size:number;text_size:number;header_height:number;chart_height:number;padding:number;gap:number;columns:number;density:'compact'|'standard'|'loose';align:'left'|'center'|'right';row_height:number;row_count?:number;column_width:number;lock_ratio:boolean};
export type WidgetInstance={id:string;type:WidgetType;schema:number;layout:WidgetLayout;style:WidgetStyle;params:Record<string,unknown>};
export type BoardConfig={schema:number;name:string;snap:boolean;items:WidgetInstance[]};
export type BoardDraft={id:string;revision:number;source_id:string;config:BoardConfig;created_at:string;updated_at:string};
export type BoardVersion={id:string;number:number;name:string;source_id:string;created_at:string;hash:string;config:BoardConfig;archived:boolean;revision:number};
export type BoardActive={version_id:string;revision:number};
export type WidgetDefinition={id:WidgetType;name:string;category:string;description:string;source:string;min_w:number;min_h:number;default:WidgetInstance;params:string[]};
export type WidgetPreference={favorite:boolean;hidden:boolean;default?:WidgetInstance};
export type WidgetPreset={id:string;name:string;revision:number;item:WidgetInstance};
export type BoardWatchlist={id:string;name:string;revision:number;symbols:string[]};
export type BoardState={revision:number;library_revision:number;active:BoardActive;drafts:Record<string,BoardDraft>;library:Record<string,WidgetPreference>;presets:Record<string,WidgetPreset>;watchlists:Record<string,BoardWatchlist>};
export type VersionPage={items:BoardVersion[];total:number;page:number;page_size:number};
export type WidgetReference={version_id:string;number:number;name:string;count:number;archived:boolean};
export const boardEvent='easy-stock:guanlan-changed';
export const boardAPI='/api/v1/boards';
export const indexOptions=[['sse','上证指数'],['szse','深证成指'],['chinext','创业板指'],['csi300','沪深300'],['sse50','上证50'],['csi1000','中证1000'],['star50','科创50'],['hsi','恒生指数'],['dow','道琼斯'],['sp500','标普500'],['nasdaq','纳斯达克综合'],['nasdaq100','纳指100']] as const;
export const copy=<T,>(v:T):T=>structuredClone(v);
export const canonical=(v:unknown):string=>v&&typeof v==='object'?Array.isArray(v)?`[${v.map(canonical).join(',')}]`:`{${Object.keys(v).sort().map(k=>JSON.stringify(k)+':'+canonical((v as Record<string,unknown>)[k])).join(',')}}`:JSON.stringify(v);
export const boardTime=(v?:string)=>!v||v.startsWith('0001')?'时间未提供':new Date(v).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'});
export async function boardRead<T>(config:BackendConfig,path:string,signal?:AbortSignal){return (await requestJSON<{data:T}>(config,boardAPI+path,{signal})).data}
export async function boardWrite<T>(config:BackendConfig,path:string,body?:unknown,method='POST'){
 const result=await requestJSON<{data:T}>(config,boardAPI+path,{method,headers:{'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});
 window.dispatchEvent(new Event(boardEvent));
 try{window.localStorage.setItem('easy-stock.guanlan-notify.v1',crypto.randomUUID())}catch{/* The backend remains authoritative. */}
 return result.data;
}
export function newInstance(def:WidgetDefinition,pref?:WidgetPreference,preset?:WidgetPreset):WidgetInstance{
 const item=copy(preset?.item||pref?.default||def.default);item.id=crypto.randomUUID();return item;
}
export function addInstance(config:BoardConfig,item:WidgetInstance,position?:{x:number;y:number}):BoardConfig{
 const next=copy(config);item=copy(item);if(next.items.some(v=>v.id===item.id))item.id=crypto.randomUUID();
 if(!position){const xs=[...new Set([0,...next.items.map(i=>i.layout.x+i.layout.w)])].sort((a,b)=>a-b),ys=[...new Set([0,...next.items.map(i=>i.layout.y+i.layout.h)])].sort((a,b)=>a-b);outer:for(const y of ys){for(const x of xs){if(x+item.layout.w>120)continue;const hit=next.items.some(i=>x<i.layout.x+i.layout.w&&x+item.layout.w>i.layout.x&&y<i.layout.y+i.layout.h&&y+item.layout.h>i.layout.y);if(!hit){position={x,y};break outer}}}}
 item.layout={...item.layout,x:Math.max(0,Math.min(120-item.layout.w,position?.x||0)),y:Math.max(0,position?.y??next.items.reduce((v,i)=>Math.max(v,i.layout.y+i.layout.h),0))};next.items.push(item);return next;
}
export function useBoardStore(config:BackendConfig|null){
 const[state,setState]=useState<BoardState|null>(null),[catalog,setCatalog]=useState<WidgetDefinition[]>([]),[references,setReferences]=useState<Record<string,WidgetReference[]>>({}),[error,setError]=useState('');
 const refresh=useCallback(async(signal?:AbortSignal)=>{if(!config)return;try{const[s,c]=await Promise.all([boardRead<BoardState>(config,'/state',signal),requestJSON<{data:WidgetDefinition[];references:Record<string,WidgetReference[]>}>(config,boardAPI+'/catalog',{signal})]);if(!signal?.aborted){setState(s);setCatalog(c.data);setReferences(c.references);setError('')}}catch(e){if(!signal?.aborted)setError((e as Error).message)}},[config]);
 useEffect(()=>{const a=new AbortController();void refresh(a.signal);return()=>a.abort()},[refresh]);
 return{state,catalog,references,error,refresh,setError};
}
export type BoardStore=ReturnType<typeof useBoardStore>;
