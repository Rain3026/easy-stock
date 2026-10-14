import {useCallback,useEffect,useMemo,useRef,useState} from 'react';
import {requestJSON,type BackendConfig,type Quote} from './backend';
import type {DailyOverview} from './daily-overview';
import {boardAPI,canonical,type WidgetInstance,type BoardState} from './guanlan';

export type DebateResult={id:string;updated_at:string;status:string;error?:string;subject:string;snapshot:{id:string;cutoff:string;trade_date:string;gaps:string[];version:string};debate:{snapshot_id:string;subject:string;responses:{role:string;stage:string;runtime:string;model:string;skill_refs?:{name:string;hash:string;loaded_at?:string}[];argument:{thesis:string;evidence:string[];risks:string[];conditions:string[];uncertainty:string}}[]}|null};
export type RiskResult={id:string;status:string;error?:string;request:{portfolio_plan_id?:string};started_at?:string;completed_at?:string;updated_at?:string;coverage_percent?:number;report:{id:string;generated_at:string;prompt_version:string;algorithm_version?:string;model?:string;conclusion:{risk_level:string;executive_summary:string;primary_risks:string[];source:string;data_limitations?:string[]}}|null};
export type AsyncData<T>={value:T|null;error:string;loading:boolean};
export type WidgetData={overview:AsyncData<DailyOverview>;debates:AsyncData<DebateResult[]>;risks:AsyncData<RiskResult[]>;quotes:AsyncData<Quote[]>;refreshing:boolean;refresh:()=>Promise<void>};
type CacheEntry={at:number;value?:unknown;promise?:Promise<unknown>};
const caches=new WeakMap<BackendConfig,Map<string,CacheEntry>>();
export async function sharedRead<T>(config:BackendConfig,path:string,ttl=30000,force=false):Promise<T>{
 let cache=caches.get(config);if(!cache){cache=new Map();caches.set(config,cache)}
 const old=cache.get(path);if(old?.promise)return old.promise as Promise<T>;if(!force&&old?.value!==undefined&&Date.now()-old.at<ttl)return old.value as T;
 const entry:CacheEntry={at:old?.at||0,value:old?.value};const promise=requestJSON<T>(config,path).then(value=>{entry.value=value;entry.at=Date.now();return value}).finally(()=>{entry.promise=undefined});entry.promise=promise;cache.set(path,entry);return promise;
}
const blank=<T,>():AsyncData<T>=>({value:null,error:'',loading:false});
export function useWidgetData(config:BackendConfig|null,items:WidgetInstance[],state:BoardState|null):WidgetData{
 const[overview,setOverview]=useState<AsyncData<DailyOverview>>(blank),[debates,setDebates]=useState<AsyncData<DebateResult[]>>(blank),[risks,setRisks]=useState<AsyncData<RiskResult[]>>(blank),[quotes,setQuotes]=useState<AsyncData<Quote[]>>(blank),[refreshing,setRefreshing]=useState(false);
 const bindings=(type:string,key:string)=>[...new Set(items.filter(i=>i.type===type).map(i=>String(i.params[key]||'')).filter(Boolean))].sort();
 const key=canonical({types:[...new Set(items.map(i=>i.type))].sort(),watch:items.filter(i=>i.type==='watchlist').map(i=>state?.watchlists[String(i.params.list_id)]).filter(Boolean),debateIDs:bindings('debate','result_id'),snapshots:bindings('debate','snapshot_id'),riskIDs:bindings('risk','result_id'),plans:bindings('risk','plan_id')});
 const capabilities=useMemo(()=>JSON.parse(key) as {types:string[];watch:{symbols:string[]}[];debateIDs:string[];snapshots:string[];riskIDs:string[];plans:string[]},[key]);
 const epoch=useRef(0),alive=useRef(true);
 const load=useCallback(async(force=false)=>{
  if(!config)return;const current=++epoch.current;const valid=()=>alive.current&&epoch.current===current;
  const promises:Promise<unknown>[]=[];
  if(capabilities.types.some(t=>['volume','indexes','breadth','emotion','themes'].includes(t))){
   setOverview(p=>({...p,loading:true}));promises.push(sharedRead<{data:DailyOverview|null;refreshing:boolean}>(config,`/api/v1/daily-research/overview${force?'?refresh=1':''}`,1000,force).then(r=>{if(valid()){setOverview(p=>({value:r.data||p.value,error:'',loading:false}));setRefreshing(r.refreshing)}}).catch(e=>{if(valid()){setOverview(p=>({...p,error:e.message,loading:false}));setRefreshing(false)}}));
  }
  if(capabilities.types.includes('debate')){setDebates(p=>({...p,loading:true}));promises.push(sharedRead<{data:DebateResult[]}>(config,boardAPI+'/research-results?'+new URLSearchParams({ids:capabilities.debateIDs.join(','),snapshots:capabilities.snapshots.join(',')}),30000,force).then(r=>{if(valid())setDebates({value:r.data,error:'',loading:false})}).catch(e=>{if(valid())setDebates(p=>({...p,error:e.message,loading:false}))}));}
  if(capabilities.types.includes('risk')){setRisks(p=>({...p,loading:true}));promises.push(sharedRead<{data:RiskResult[]}>(config,boardAPI+'/risk-results?'+new URLSearchParams({ids:capabilities.riskIDs.join(','),plans:capabilities.plans.join(',')}),30000,force).then(r=>{if(valid())setRisks({value:r.data,error:'',loading:false})}).catch(e=>{if(valid())setRisks(p=>({...p,error:e.message,loading:false}))}));}
  const symbols=[...new Set(capabilities.watch.flatMap(w=>w.symbols))];
  if(symbols.length){setQuotes(p=>({...p,loading:true}));promises.push(sharedRead<{data:Quote[]}>(config,`/api/v1/quotes/realtime?symbols=${encodeURIComponent(symbols.slice(0,100).join(','))}`,15000,force).then(r=>{if(valid())setQuotes({value:r.data,error:symbols.length>100?'单屏最多刷新100只，未刷新股票保留缺口':'',loading:false})}).catch(e=>{if(valid())setQuotes(p=>({...p,error:e.message,loading:false}))}));}
  else setQuotes(blank());
  await Promise.all(promises);
 },[config,capabilities]);
 useEffect(()=>{alive.current=true;void load();const id=setInterval(()=>{if(document.visibilityState==='visible')void load()},60000);return()=>{alive.current=false;epoch.current++;clearInterval(id)}},[load]);
 useEffect(()=>{if(!refreshing)return;const id=setInterval(()=>{if(document.visibilityState==='visible')void load()},2000);return()=>clearInterval(id)},[refreshing,load]);
 return{overview,debates,risks,quotes,refreshing,refresh:()=>load(true)};
}
