import {useCallback,useEffect,useMemo,useRef,useState} from 'react';
import {requestJSON,type BackendConfig,type Quote} from './backend';
import type {DailyOverview} from './daily-overview';
import {WidgetRefreshScheduler,refreshSeconds} from './guanlan-refresh';
import {boardAPI,canonical,type WidgetInstance,type BoardState} from './guanlan';

export type DebateResult={id:string;updated_at:string;generated_at?:string;status:string;error?:string;subject:string;snapshot:{id:string;cutoff:string;trade_date:string;gaps:string[];version:string};debate:{snapshot_id:string;subject:string;responses:{role:string;stage:string;runtime:string;model:string;skill_refs?:{name:string;hash:string;loaded_at?:string}[];argument:{thesis:string;evidence:string[];risks:string[];conditions:string[];uncertainty:string}}[]}|null};
export type RiskResult={id:string;status:string;error?:string;request:{portfolio_plan_id?:string};started_at?:string;completed_at?:string;updated_at?:string;coverage_percent?:number;report:{id:string;generated_at:string;evidence_times?:{label:string;at:string;source?:string}[];prompt_version:string;algorithm_version?:string;model?:string;conclusion:{risk_level:string;executive_summary:string;primary_risks:string[];source:string;data_limitations?:string[]}}|null};
export type AsyncData<T>={value:T|null;error:string;loading:boolean};
export type WidgetData={overview:AsyncData<DailyOverview>;debates:AsyncData<DebateResult[]>;risks:AsyncData<RiskResult[]>;quotes:AsyncData<Quote[]>;refreshing:boolean;refresh:()=>Promise<void>;forItem?:(id:string)=>WidgetData};
type CacheEntry={at:number;value?:unknown;promise?:Promise<unknown>};
const caches=new WeakMap<BackendConfig,Map<string,CacheEntry>>();
export async function sharedRead<T>(config:BackendConfig,path:string,ttl=30000,force=false):Promise<T>{
 let cache=caches.get(config);if(!cache){cache=new Map();caches.set(config,cache)}
 const cacheKey=path.replace(/(\/daily-research\/overview)\?refresh=1$/,'$1');
 const old=cache.get(cacheKey);if(old?.promise)return old.promise as Promise<T>;if(!force&&old?.value!==undefined&&Date.now()-old.at<ttl)return old.value as T;
 const entry:CacheEntry={at:old?.at||0,value:old?.value};const promise=requestJSON<T>(config,path).then(value=>{entry.value=value;entry.at=Date.now();return value}).finally(()=>{entry.promise=undefined});entry.promise=promise;cache.set(cacheKey,entry);return promise;
}
const blank=<T,>():AsyncData<T>=>({value:null,error:'',loading:false});
type Resource='overview'|'debates'|'risks'|'quotes';
type Snapshot=Pick<WidgetData,Resource|'refreshing'>;
const emptySnapshot=():Snapshot=>({overview:blank(),debates:blank(),risks:blank(),quotes:blank(),refreshing:false});
export function useWidgetData(config:BackendConfig|null,items:WidgetInstance[],state:BoardState|null):WidgetData{
 const [latest,setLatest]=useState<Snapshot>(emptySnapshot),[instances,setInstances]=useState<Record<string,Snapshot>>({});
 const bindings=(type:string,key:string)=>[...new Set(items.filter(i=>i.type===type).map(i=>String(i.params[key]||'')).filter(Boolean))].sort();
 const key=canonical({targets:items.map(i=>({id:i.id,type:i.type,seconds:refreshSeconds(i.refresh_seconds)})),watch:items.filter(i=>i.type==='watchlist').map(i=>state?.watchlists[String(i.params.list_id)]).filter(Boolean),debateIDs:bindings('debate','result_id'),snapshots:bindings('debate','snapshot_id'),riskIDs:bindings('risk','result_id'),plans:bindings('risk','plan_id')});
 const capabilities=useMemo(()=>JSON.parse(key) as {targets:{id:string;type:string;seconds:number}[];watch:{symbols:string[]}[];debateIDs:string[];snapshots:string[];riskIDs:string[];plans:string[]},[key]);
 const scheduler=useRef<WidgetRefreshScheduler|null>(null);
 useEffect(()=>{
  if(!config)return;let alive=true;
  const symbols=[...new Set(capabilities.watch.flatMap(w=>w.symbols))];
  const resource=(type:string):Resource=>type==='debate'?'debates':type==='risk'?'risks':type==='watchlist'?'quotes':'overview';
  const targets=capabilities.targets.map(t=>({id:t.id,resource:resource(t.type),seconds:t.seconds}));
  const update=(family:Resource,ids:string[],fn:(v:AsyncData<never>)=>AsyncData<unknown>,pending?:boolean)=>{
   if(!alive)return;
   const change=(p:Snapshot):Snapshot=>({...p,[family]:fn(p[family] as AsyncData<never>),...(pending===undefined?{}:{refreshing:pending})});
   setLatest(p=>change(p));setInstances(p=>{const next={...p};for(const id of ids)next[id]=change(next[id]||emptySnapshot());return next});
  };
  const read=async(family:string,ids:string[],force:boolean)=>{
   const f=family as Resource;update(f,ids,p=>({...p,loading:true}));
   try{
    let value:unknown,pending=false;
    if(f==='overview'){
     const r=await sharedRead<{data:DailyOverview|null;refreshing:boolean}>(config,`/api/v1/daily-research/overview${force?'?refresh=1':''}`,1000,force);value=r.data;pending=r.refreshing;
    }else if(f==='debates')value=(await sharedRead<{data:DebateResult[]}>(config,boardAPI+'/research-results?'+new URLSearchParams({ids:capabilities.debateIDs.join(','),snapshots:capabilities.snapshots.join(',')}),1000,force)).data;
    else if(f==='risks')value=(await sharedRead<{data:RiskResult[]}>(config,boardAPI+'/risk-results?'+new URLSearchParams({ids:capabilities.riskIDs.join(','),plans:capabilities.plans.join(',')}),1000,force)).data;
    else value=symbols.length?(await sharedRead<{data:Quote[]}>(config,`/api/v1/quotes/realtime?symbols=${encodeURIComponent(symbols.slice(0,100).join(','))}`,15000,force)).data:[];
    update(f,ids,p=>({value:value??p.value,error:f==='quotes'&&symbols.length>100?'单屏最多刷新100只，其余保留缺口':'',loading:false}),f==='overview'?pending:undefined);return{pending};
   }catch(e){update(f,ids,p=>({...p,error:(e as Error).message,loading:false}),f==='overview'?false:undefined);throw e}
  };
  const instance=new WidgetRefreshScheduler(targets,read,()=>document.visibilityState==='visible',()=>Date.now(),(family,ids)=>update(family as Resource,ids,p=>({...p,error:'后台更新尚未完成，可稍后使用页面刷新重试',loading:false}),family==='overview'?false:undefined));scheduler.current=instance;instance.start();
  const visibility=()=>instance.resume();document.addEventListener('visibilitychange',visibility);
  return()=>{alive=false;instance.stop();document.removeEventListener('visibilitychange',visibility);if(scheduler.current===instance)scheduler.current=null};
 },[config,capabilities]);
 const refresh=useCallback(async()=>{await scheduler.current?.refresh()},[]);
 return {...latest,refresh,forItem:(id:string)=>({...instances[id]||emptySnapshot(),refresh})};
}
