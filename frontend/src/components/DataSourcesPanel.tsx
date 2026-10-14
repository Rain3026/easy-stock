import { useEffect, useState } from 'react';
import { Database, RefreshCw } from 'lucide-react';
import { BackendConfig, requestJSON } from '../lib/backend';
import './data-sources.css';

type Health = { id: string; name: string; category: string; role: string; ok: boolean; status: string; message: string; checked_at?: string; last_success_at?: string; latency_ms: number; capability?: string; observation?: { count: number; trade_date?: string; data_at?: string; freshness: string; detail?: string } };
type Candidate = { family: string; channel: string; state: string; equivalent: boolean; note?: string };
type Contract = { id: string; name: string; unit: string; universe: string; frequency: string; primary: Candidate; backups: Candidate[]; rule: string };
type Status = { sources: Health[]; refreshing: boolean };
const labels: Record<string,string> = { unknown:'未检测', available:'样本可用', partial:'部分可用', stale:'数据过期', expired:'检测已过期', empty:'样本为空', unconfigured:'未配置', permission_denied:'权限不足', rate_limited:'达到限流', timeout:'请求超时', error:'检测失败' };
const names: Record<string,string> = {tushare:'Tushare',eastmoney:'东方财富',sina:'新浪',tencent:'腾讯',cls:'财联社',duanxianxia:'开盘啦',ths:'同花顺',cffex:'中金所',exchange:'交易所',cninfo:'巨潮',publisher:'原始机构',original:'作者原文',author_mirror:'作者同步账号',github:'GitHub 知识源',local_copy:'本地版本'};
const timeLabel=(value?:string)=>value ? new Date(value).toLocaleString('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}) : '尚未检测';

export function DataSourcesPanel({config,active,revision}:{config:BackendConfig|null;active:boolean;revision?:number}){
 const [state,setState]=useState<Status>({sources:[],refreshing:false});
 const [contracts,setContracts]=useState<Contract[]>([]);
 const [error,setError]=useState('');
 const [submitting,setSubmitting]=useState(false);
 useEffect(()=>{
  if(!config||!active)return;
  let cancelled=false;let timer:ReturnType<typeof setTimeout>|undefined;
  const load=()=>requestJSON<Status>(config,'/api/v1/sources').then(data=>{if(cancelled)return;setState(data);setError('');if(data.refreshing)timer=setTimeout(load,1500);}).catch(e=>{if(!cancelled)setError(e instanceof Error?e.message:'读取来源状态失败');});
  void load();void requestJSON<{capabilities:Contract[]}>(config,'/api/v1/data/capabilities').then(data=>{if(!cancelled)setContracts(data.capabilities);}).catch(()=>{});
  return()=>{cancelled=true;if(timer)clearTimeout(timer);};
 },[config,active,revision,submitting]);
 const probe=async()=>{
  if(!config)return;setSubmitting(true);setError('');
  try{setState(await requestJSON<Status>(config,'/api/v1/sources/probe',{method:'POST'}));}
  catch(e){setError(e instanceof Error?e.message:'检测请求失败');}
  finally{setSubmitting(false);}
 };
 return <section className="data-source-panel" aria-label="来源健康与主备能力">
  <header><div><h3><Database size={17}/>来源健康</h3><p>Tushare、东方财富为主来源；各项数据分别配置备用。检测结果只代表所列能力样本。</p></div><button type="button" onClick={()=>void probe()} disabled={!config||state.refreshing||submitting}><RefreshCw size={14} className={state.refreshing?'spin':''}/>{state.refreshing?'正在检测…':'检测全部来源'}</button></header>
  {error&&<p role="alert" className="data-source-error">{error}</p>}
  <div className="source-health-grid">{state.sources.map(source=><article key={source.id} className={`source-health-card ${source.status}`}>
   <div className="source-health-name"><strong>{source.name}</strong>{source.role==='primary'&&<small>主来源</small>}<span className={`source-health-status ${source.ok?'ok':''}`} title={source.message}>{labels[source.status]||'未知状态'}</span></div>
   <p>{source.category}</p>{!source.ok&&<p>{source.message}</p>}
   {source.observation?.detail&&<small className="source-health-detail">{source.observation.detail}</small>}
   <footer><span>检测 {timeLabel(source.checked_at)}</span>{source.observation?.trade_date&&<span>数据日 {source.observation.trade_date}</span>}{source.checked_at&&<span>{source.latency_ms}ms</span>}</footer>
  </article>)}</div>
  <details className="source-contracts"><summary>查看各项数据的主备链路与口径 <span>{contracts.length} 项能力</span></summary>
   <p>“已接入”表示存在业务适配，不等于当前检测通过。“降级”表示覆盖或频率不同；待接入项不会自动作为备用。</p>
   <div className="source-contract-scroll"><table><thead><tr><th>数据能力</th><th>当前首选</th><th>备用与缺口</th><th>数据口径</th></tr></thead><tbody>{contracts.map(c=><tr key={c.id}><th>{c.name}<small>{c.frequency}</small></th><td>{names[c.primary.family]||c.primary.family}</td><td>{c.backups.map((b,i)=><div key={`${b.family}-${i}`}><strong>{names[b.family]||b.family}</strong><span className="source-contract-state">{b.state==='implemented'?'已接入':b.state==='degraded'?'降级':'待接入'}</span>{b.note&&<small>{b.note}</small>}</div>)}</td><td><span>{c.universe}</span><small>{c.rule}</small></td></tr>)}</tbody></table></div>
  </details>
 </section>;
}
