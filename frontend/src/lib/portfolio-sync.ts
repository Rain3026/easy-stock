import{useEffect,useState}from'react';
import{type BackendConfig,requestJSON}from'./backend';
import{type PortfolioPlans,readPortfolioPlans,portfolioPlansStorageKey,portfolioDraftStorageKey,portfolioDraftChangedEvent}from'./portfolio-draft';
type Version={revision:number;hash:string;created_at:string;origin:string;state:PortfolioPlans|null};
const marker='easy-stock.portfolio-sync.v1';
const canonical=(v:unknown):string=>v&&typeof v==='object'?Array.isArray(v)?`[${v.map(canonical).join(',')}]`:`{${Object.keys(v).sort().map(k=>JSON.stringify(k)+':'+canonical((v as Record<string,unknown>)[k])).join(',')}}`:JSON.stringify(v);
const equal=(a:unknown,b:unknown)=>canonical(a)===canonical(b);
const suffix=(value:unknown)=>{let h=2166136261;for(const c of canonical(value)){h=Math.imul(h^c.charCodeAt(0),16777619)}return(h>>>0).toString(16)};
// Three-way reconciliation retains conflicting edits as separate plans; it
// never silently picks one set of holdings over the other.
export function mergePortfolioPlans(local:PortfolioPlans,remote:PortfolioPlans,base?:PortfolioPlans):PortfolioPlans{
 const plans=remote.plans.filter(r=>!base||local.plans.some(l=>l.id===r.id)||!equal(base.plans.find(b=>b.id===r.id),r)).map(p=>({...p}));
 for(const l of local.plans){const original=base?.plans.find(b=>b.id===l.id);const ri=plans.findIndex(p=>p.id===l.id);if(ri<0){if(!original||!equal(original,l))plans.push(l);continue}const r=plans[ri];if(equal(l,r)||original&&equal(l,original))continue;if(original&&equal(r,original)){plans[ri]=l;continue}const id=`${l.id}-local-${suffix(l)}`;if(!plans.some(p=>p.id===id))plans.push({...l,id,name:`${l.name.slice(0,28)}（本机保留）`})}
 // Concurrent deletion of different plans must not erase the final plan.
 if(!plans.length){const retained=local.plans.find(p=>p.id===local.activeId)||remote.plans[0];if(retained)plans.push({...retained,name:`${retained.name.slice(0,28)}（冲突保留）`})}
 const active=local.activeId!==base?.activeId?local.activeId:remote.activeId;return{activeId:plans.some(p=>p.id===active)?active:plans[0].id,plans};
}
function applyLocal(state:PortfolioPlans){window.localStorage.setItem(portfolioPlansStorageKey,JSON.stringify(state));window.dispatchEvent(new CustomEvent(portfolioDraftChangedEvent));}
export function usePortfolioSync(config:BackendConfig|null){
 const[status,setStatus]=useState('持仓方案保留在本机'),[error,setError]=useState('');
 useEffect(()=>{if(!config)return;let alive=true,writing=false,ready=false,queued=false;let revision=0;let base:PortfolioPlans|undefined;let timer:ReturnType<typeof setTimeout>|undefined;
 try{const stored=JSON.parse(localStorage.getItem(marker)||'null');base=stored?.state}catch{/* Original draft remains available. */}
 const save=async()=>{if(!alive||!ready)return;if(writing){queued=true;return}writing=true;setError('');try{
  const current=readPortfolioPlans();const remote=(await requestJSON<{data:Version}>(config,'/api/v1/portfolio-plans')).data;
  if(!alive)return;const latest=readPortfolioPlans();const merged=remote.state?mergePortfolioPlans(latest,remote.state,base):latest;
  const result=await requestJSON<{data:Version}>(config,'/api/v1/portfolio-plans',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({state:merged,expected_revision:remote.revision,origin:remote.revision?'edit':'browser-import'})});
  if(!alive)return;revision=result.data.revision;base=result.data.state!;localStorage.setItem(marker,JSON.stringify({revision,state:base}));
  // Do not overwrite input typed while the request was in flight.
  if(equal(readPortfolioPlans(),latest)){if(!equal(latest,base))applyLocal(base)}else queued=true;
  setStatus(`方案已保存 · 版本 ${revision}`);
 }catch(e){if(alive){setError((e as Error).message);setStatus('本机改动已保留，数据库同步待重试')}}finally{writing=false;if(queued&&alive){queued=false;timer=setTimeout(()=>void save(),500)}}};
 const changed=()=>{if(!ready)return;try{const stored=JSON.parse(localStorage.getItem(marker)||'null');if(stored?.state)base=stored.state}catch{};if(timer)clearTimeout(timer);timer=setTimeout(()=>void save(),650)};
 window.addEventListener(portfolioDraftChangedEvent,changed);window.addEventListener('storage',changed);
 void(async()=>{try{const remote=(await requestJSON<{data:Version}>(config,'/api/v1/portfolio-plans')).data;if(!alive)return;const local=readPortfolioPlans();const hasLocal=!!(localStorage.getItem(portfolioPlansStorageKey)||localStorage.getItem(portfolioDraftStorageKey));if(remote.state){const merged=hasLocal?mergePortfolioPlans(local,remote.state,base):remote.state;if(!equal(local,merged))applyLocal(merged)}ready=true;await save()}catch(e){if(alive){ready=true;setError((e as Error).message);setStatus('持仓本机副本可用；数据库未连接')}}})();
 const retry=()=>void save();window.addEventListener('easy-stock:retry-portfolio-sync',retry);
 return()=>{alive=false;if(timer)clearTimeout(timer);window.removeEventListener(portfolioDraftChangedEvent,changed);window.removeEventListener('storage',changed);window.removeEventListener('easy-stock:retry-portfolio-sync',retry)};
 },[config]);return{status,error};
}
export async function restorePortfolioVersion(config:BackendConfig,version:Version){const current=(await requestJSON<{data:Version}>(config,'/api/v1/portfolio-plans')).data;if(!version.state)throw new Error('该版本无方案');const saved=await requestJSON<{data:Version}>(config,'/api/v1/portfolio-plans',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({state:version.state,expected_revision:current.revision,origin:'restore'})});localStorage.setItem(marker,JSON.stringify({revision:saved.data.revision,state:saved.data.state}));applyLocal(saved.data.state!);return saved.data}
export type{Version as PortfolioVersion};
