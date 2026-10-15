import type {WidgetInstance,BoardState} from './guanlan';
import type {WidgetData} from './guanlan-data';

export function dataTime(v?:string):string{
 if(!v||v.startsWith('0001'))return '时间未提供';
 if(/^\d{4}-\d{2}-\d{2}$/.test(v))return v;
 const d=new Date(v);return Number.isNaN(d.getTime())?'时间未提供':d.toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hour12:false});
}
const sourceTime=(at?:string,date?:string)=>dataTime(at)==='时间未提供'?date:at;
export function debateFor(item:WidgetInstance,data:WidgetData){return (data.debates.value||[]).find(d=>(!item.params.result_id||d.id===item.params.result_id)&&(!item.params.snapshot_id||d.snapshot.id===item.params.snapshot_id)&&(!item.params.subject||d.subject.includes(String(item.params.subject))))}
export function riskFor(item:WidgetInstance,data:WidgetData){return (data.risks.value||[]).find(d=>(!item.params.result_id||d.id===item.params.result_id)&&(!item.params.plan_id||d.request.portfolio_plan_id===item.params.plan_id))}
export type DataTimeEntry={label:string;at?:string;source?:string;stale?:boolean};
export function widgetMetadata(item:WidgetInstance,data:WidgetData,state:BoardState|null){
 const o=data.overview.value,p=o?.pulse;let entries:DataTimeEntry[]=[],prefix='数据截至',extra:string[]=[];
 switch(item.type){
  case 'volume':entries=[{label:'沪深量能',at:sourceTime(p?.at,p?.date),source:p?.source,stale:p?.stale}];break;
  case 'breadth':entries=[{label:'市场宽度',at:sourceTime(p?.breadth?.at,p?.breadth?.date),source:p?.breadth?.source,stale:p?.stale}];break;
  // Emotion only exposes a trade date. Never borrow pulse.at.
  case 'emotion':entries=[{label:'涨跌停与连板',at:p?.emotion?.date,source:p?.emotion?.source,stale:p?.emotion?.partial}];break;
  case 'themes':entries=[...(o?.themes||[])].sort((a,b)=>item.params.sort==='change'?b.change_percent-a.change_percent:(b.daily_strength_score??b.trend_score??0)-(a.daily_strength_score??a.trend_score??0)).slice(0,Number(item.params.rows)||5).map(t=>({label:t.name||t.theme,at:t.trade_date||o?.theme_meta?.trade_date,source:t.source||o?.theme_meta?.source,stale:!!t.carry_forward||o?.theme_meta?.stale}));break;
  case 'indexes':entries=((item.params.index_ids||[]) as string[]).map(id=>{const q=o?.indexes?.find(v=>v.id===id);return{label:q?.name||id,at:q?.trade_time,source:q?.meta.source,stale:q?.meta.stale}});break;
  case 'watchlist':entries=(state?.watchlists[String(item.params.list_id)]?.symbols||[]).map(symbol=>{const q=data.quotes.value?.find(q=>q.symbol===symbol);return{label:q?.name||symbol,at:q?.trade_time,source:q?.meta.source,stale:q?.meta.stale}});break;
  case 'debate':{const r=debateFor(item,data);prefix='报告生成';entries=[{label:'报告生成（原任务完成时间）',at:r?.generated_at,source:r?.debate?.responses.map(v=>`${v.runtime} / ${v.model}`).join('、')},{label:'证据截至',at:r?.snapshot.cutoff}];extra=[`原任务状态：${r?.status||'无结果'}`];break}
  case 'risk':{const r=riskFor(item,data);prefix='报告生成';entries=[{label:'报告生成',at:r?.report?.generated_at,source:r?.report?.conclusion.source},...(r?.report?.evidence_times||[])];if(entries.length===1)entries.push({label:'证据截至（原报告未提供）'});extra=[`原任务状态：${r?.status||'无结果'}`];break}
 }
 const times=entries.map(e=>dataTime(e.at));const ai=['debate','risk'].includes(item.type);
 const label=ai?`${prefix} ${times[0]||'时间未提供'}`:!times.length?`${prefix} 时间未提供`:new Set(times).size===1?`${prefix} ${times[0]}`:'各项时间不同';
 const warnings=[...new Set(entries.filter(e=>e.stale).map(()=>item.type==='emotion'?'部分数据':'缓存/延迟数据'))];
 return {label,entries,extra,warnings};
}
