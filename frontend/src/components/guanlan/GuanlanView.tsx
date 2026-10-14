import {useEffect,useState} from 'react';
import {Activity,Plus,RefreshCw} from 'lucide-react';
import type {BackendConfig} from '../../lib/backend';
import {boardEvent,boardRead,boardTime,useBoardStore,type BoardVersion} from '../../lib/guanlan';
import {useWidgetData} from '../../lib/guanlan-data';
import {BoardCanvas} from './BoardCanvas';
import './guanlan.css';

export function GuanlanView({backend,onManage}:{backend:BackendConfig|null;onManage:()=>void}){
 const store=useBoardStore(backend),[version,setVersion]=useState<BoardVersion|null>(null),[error,setError]=useState('');
 const activeID=store.state?.active.version_id;
 useEffect(()=>{if(!backend||!activeID){setVersion(null);return};const a=new AbortController();void boardRead<BoardVersion>(backend,`/versions/${activeID}`,a.signal).then(v=>{if(!a.signal.aborted){setVersion(v);setError('')}}).catch(e=>{if(!a.signal.aborted)setError(e.message)});return()=>a.abort()},[backend,activeID,store.state?.revision]);
 useEffect(()=>{const refresh=()=>{void store.refresh()};const storage=(e:StorageEvent)=>{if(e.key==='easy-stock.guanlan-notify.v1')refresh()};const interval=setInterval(()=>{if(document.visibilityState==='visible')refresh()},5000);window.addEventListener(boardEvent,refresh);window.addEventListener('storage',storage);return()=>{clearInterval(interval);window.removeEventListener(boardEvent,refresh);window.removeEventListener('storage',storage)}},[store.refresh]);
 const current=version?.id===activeID?version:null;
 const data=useWidgetData(backend,current?.config.items||[],store.state);
 return <div className="gl-workspace gl-guanlan-view">{(error||store.error)&&<div className="gl-error" role="alert">{error||store.error}<button type="button" onClick={()=>void store.refresh()}>重新读取配置</button></div>}{current?<><div className="gl-toolbar"><div className="gl-current-version"><Activity size={17} className="gl-primary-text"/><h2>{current.name}</h2><span className="gl-badge">V{current.number} · 当前启用</span><span className="gl-muted">配置于 {boardTime(current.created_at)}</span></div><div className="gl-actions"><span className="gl-muted">{data.refreshing?'行情后台更新中':'各组件保留来源与证据时间'}</span><button type="button" disabled={data.overview.loading||data.refreshing} onClick={()=>void data.refresh()}><RefreshCw size={14}/>刷新行情</button></div></div><BoardCanvas config={current.config} catalog={store.catalog} state={store.state} data={data}/></>:<div className="gl-canvas-empty"><Activity size={37}/><h3>{!store.state?'正在读取观澜配置':activeID?'正在读取启用版本':'先为观澜配置你的看板'}</h3><p>在“看板与组件”中新建配置、拖入组件并保存启用。<br/>原今日总览可以继续使用。</p><button type="button" className="gl-primary" onClick={onManage}><Plus size={15}/>前往我的看板</button></div>}</div>;
}
