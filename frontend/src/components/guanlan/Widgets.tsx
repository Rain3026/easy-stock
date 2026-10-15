import {Activity,BarChart3,BookOpen,BrainCircuit,ChartNoAxesCombined,Flame,Scale,ShieldCheck,Star,TrendingUp} from 'lucide-react';
import {useEffect,useRef,useState,type ReactNode} from 'react';
import {amountText,percentText,type Pulse} from '../../lib/daily-overview';
import {indexOptions,boardTime,type BoardState,type WidgetInstance,type WidgetType} from '../../lib/guanlan';
import {contentScale} from '../../lib/guanlan-resize';
import {dataTime,debateFor,riskFor} from '../../lib/guanlan-metadata';
import type {WidgetData} from '../../lib/guanlan-data';

export const widgetIcons:Record<WidgetType,typeof Activity>={volume:BarChart3,indexes:ChartNoAxesCombined,breadth:Activity,emotion:Flame,themes:TrendingUp,debate:Scale,risk:ShieldCheck,watchlist:Star};
export function WidgetEmpty({children}:{children:ReactNode}){return <div className="gl-widget-empty"><BookOpen size={23}/><span>{children}</span></div>}
function Source({source,at,visible=true}:{source?:string;at?:string;stale?:boolean;visible?:boolean}){return <>{visible&&<small className="gl-source">{source||'来源未提供'}{at&&` · ${dataTime(at)}`}</small>}</>}
function ErrorNote({text}:{text:string}){return text?<p className="gl-inline-warning" role="status">{text} · 最近结果仅供回看</p>:null}
const value=(v:unknown,fallback:number)=>typeof v==='number'&&Number.isFinite(v)?v:fallback;
const fields=(i:WidgetInstance,fallback:string[])=>Array.isArray(i.params.fields)?i.params.fields as string[]:fallback;
const numberText=(v?:number|null)=>v==null||!Number.isFinite(v)?'—':v.toLocaleString('zh-CN',{maximumFractionDigits:2});
const quotePercent=(n?:number|null)=>n==null||!Number.isFinite(n)?'—':`${n>0?'+':''}${n.toFixed(2)}%`;
const tradeClass=(v?:number)=>v==null?'':v>=0?'gl-up':'gl-down';

export function WidgetContent({item,data,state}:{item:WidgetInstance;data:WidgetData;state:BoardState|null}){
 const o=data.overview.value,p=o?.pulse;
 const overviewError=<ErrorNote text={data.overview.error}/>;
 if(item.schema!==1)return <WidgetEmpty>此组件版本暂不兼容，请复制配置后修正</WidgetEmpty>;
 switch(item.type){
 case 'volume':return p?<>{overviewError}<div className="gl-metric-row"><Metric label={p.session==='closed'?'实际全天 · 亿元':'已成交 · 亿元'} value={amountText(p.actual)}/>{item.params.show_forecast!==false&&p.session!=='closed'&&<Metric label="预计全天 · 亿元" value={amountText(p.forecast)} primary/>}<Metric label="前20日均额" value={amountText(p.mean20)}/></div><VolumeChart pulse={p} item={item}/><Source visible={item.style.show_source!==false} source={p.source} at={p.at} stale={p.stale}/>{p.session!=='closed'&&p.forecast==null&&<p className="gl-muted">预测暂缺，保留真实数据缺口</p>}{!!p.gaps?.length&&<details className="gl-data-notes"><summary>数据说明 · {p.gaps.length}</summary><ul>{p.gaps.map((g,i)=><li key={i}>{g}</li>)}</ul></details>}</>:<WidgetEmpty>{data.overview.error||'量能数据加载中或暂缺'}</WidgetEmpty>;
 case 'indexes':{
  const ids=Array.isArray(item.params.index_ids)?item.params.index_ids as string[]:[];const show=fields(item,['price','change','time']);
  return <>{overviewError}<div className={`gl-indexes mode-${item.params.mode||'horizontal'}`}>{ids.map(id=>{const idx=o?.indexes?.find(v=>v.id===id);return <div key={id} className="gl-index" title={idx?`${idx.code} · ${idx.meta.source}`:undefined}><span>{indexOptions.find(v=>v[0]===id)?.[1]||id}</span>{show.includes('price')&&<strong>{idx&&idx.price>0?numberText(idx.price):'—'}</strong>}{show.includes('change')&&<b className={tradeClass(idx?.change_percent)}>{idx&&idx.price>0?quotePercent(idx.change_percent):'—'}</b>}{show.includes('time')&&<small>{idx?dataTime(idx.trade_time):'数据暂缺'}</small>}{item.style.show_source!==false&&<small>{idx?.meta?.source||'来源暂缺'}</small>}</div>})}</div>{!ids.length&&<WidgetEmpty>请在组件设置中选择指数</WidgetEmpty>}</>;
 }
 case 'breadth':{
  const b=p?.breadth;if(!b||b.up+b.down+b.flat===0)return <WidgetEmpty>{data.overview.error||'同口径市场宽度暂缺'}</WidgetEmpty>;const total=b.up+b.down+b.flat;const show=fields(item,['up','down','flat','percent']);
  return <>{overviewError}<div className="gl-metric-row">{show.includes('up')&&<Metric label="上涨" value={numberText(b.up)} tone="gl-up"/>}{show.includes('down')&&<Metric label="下跌" value={numberText(b.down)} tone="gl-down"/>}{show.includes('flat')&&<Metric label="平盘" value={numberText(b.flat)}/>}</div>{show.includes('percent')&&total>0&&<><div className="gl-breadth-bar" aria-label={`上涨占比${(b.up/total*100).toFixed(1)}%`}><span style={{width:`${b.up/total*100}%`}}/><span style={{width:`${b.down/total*100}%`}}/><span style={{width:`${b.flat/total*100}%`}}/></div><p className="gl-muted">上涨占比 {(b.up/total*100).toFixed(1)}% · 有效 {total} 只</p></>}<Source visible={item.style.show_source!==false} source={b.source} at={b.at} stale={p?.stale}/></>;
 }
 case 'emotion':{
  const e=p?.emotion;if(!e||(e.partial&&e.up+e.down+e.height+e.boards===0))return <WidgetEmpty>{data.overview.error||'涨跌停与连板数据暂缺'}</WidgetEmpty>;
  return <>{overviewError}<div className="gl-metric-row"><Metric label="涨停" value={numberText(e.up)} tone="gl-up"/><Metric label="跌停" value={numberText(e.down)} tone="gl-down"/><Metric label="最高连板" value={`${e.height}板`}/></div><div className="gl-levels">{(e.levels||[]).slice(0,value(item.params.rows,5)).map(l=><span key={l.height}>{l.height}板 <b>{l.count}家</b></span>)}</div><p className="gl-muted">连板 {numberText(e.boards)} 家 · 炸板率 {e.partial?'暂缺（部分数据）':`${e.broken_rate.toFixed(1)}%`}</p><Source visible={item.style.show_source!==false} source={e.source} at={e.date} stale={e.partial}/><a href="#opportunity/sentiment">查看完整梯队 →</a></>;
 }
 case 'themes':{
  const themes=[...(o?.themes||[])];if(item.params.sort==='change')themes.sort((a,b)=>b.change_percent-a.change_percent);else themes.sort((a,b)=>(b.daily_strength_score??b.trend_score??0)-(a.daily_strength_score??a.trend_score??0));
  return <>{overviewError}{themes.length?<div className="gl-theme-list">{themes.slice(0,value(item.params.rows,5)).map((t,n)=><a key={t.theme} href={`#opportunity/themes?theme=${encodeURIComponent(t.theme)}`}><span className="gl-rank">{n+1}</span><strong>{t.name||t.theme}</strong><small>{t.trend_stage||'阶段待确认'}{t.trade_date&&` · ${t.trade_date}`}{t.carry_forward&&' · 延续旧快照'}</small><b className={tradeClass(t.change_percent)}>{percentText(t.change_percent)}</b></a>)}</div>:<WidgetEmpty>核心主线暂无有效数据</WidgetEmpty>}<Source visible={item.style.show_source!==false} source={o?.theme_meta?.source} at={o?.theme_meta?.trade_date} stale={o?.theme_meta?.stale}/></>;
 }
 case 'debate':{
  const result=debateFor(item,data);
  if(!result)return <><ErrorNote text={data.debates.error}/><WidgetEmpty>{data.debates.loading?'读取研究档案中':'尚无符合条件的研究结果'}</WidgetEmpty><a href="#research/debate">进入多空辨析 →</a></>;
  if(result.status!=='succeeded'||!result.debate)return <><ErrorNote text={data.debates.error}/><WidgetEmpty>{({running:'分析进行中',failed:'分析失败',interrupted:'分析已中断',cancelled:'分析已取消'} as Record<string,string>)[result.status]||'研究结果尚未保存'}{result.error&&`：${result.error}`}</WidgetEmpty><a href={`#research/debate?snapshot=${encodeURIComponent(result.snapshot.id)}`}>查看任务与证据 →</a></>;
  const responses=result.debate.responses;const conclusion=[...responses].reverse().find(r=>r.role==='reviewer'||r.role==='judge')||responses[responses.length-1];
  return <><ErrorNote text={data.debates.error}/><span className="gl-badge">已有Agent结果 · {result.subject}</span><h3 className="gl-ai-thesis">{conclusion?.argument.thesis||'结论暂未提供'}</h3><ul className="gl-findings">{(conclusion?.argument.risks||[]).slice(0,2).map((r,n)=><li key={n}>{r}</li>)}</ul><Source visible={item.style.show_source!==false} source={`${conclusion?.runtime||'Agent'} · ${conclusion?.model||'模型未提供'}`} at={result.generated_at}/><p className="gl-muted">证据截至 {boardTime(result.snapshot.cutoff)} · {result.snapshot.version}</p>{conclusion?.skill_refs?.length?<p className="gl-muted">Skill {conclusion.skill_refs.map(v=>`${v.name}@${v.hash.slice(0,8)}`).join('、')}</p>:null}<details className="gl-data-notes"><summary>条件与数据缺口</summary>{conclusion?.argument.uncertainty&&<p>{conclusion.argument.uncertainty}</p>}<ul>{[...(conclusion?.argument.conditions||[]),...(result.snapshot.gaps||[])].map((v,n)=><li key={n}>{v}</li>)}</ul>{!conclusion?.skill_refs?.length&&<p>原结果未提供Skill版本引用</p>}</details><a href={`#research/debate?snapshot=${encodeURIComponent(result.snapshot.id)}`}>查看原判断与证据 →</a></>;
 }
 case 'risk':{
  const result=riskFor(item,data);const report=result?.report;
  return <><ErrorNote text={data.risks.error}/>{report?<><span className="gl-badge">已有组合研究 · {report.conclusion.risk_level||'风险级别未提供'}</span><h3 className="gl-ai-thesis">{report.conclusion.executive_summary||'暂无摘要'}</h3><ul className="gl-findings">{(report.conclusion.primary_risks||[]).slice(0,3).map((r,n)=><li key={n}>{r}</li>)}</ul><Source visible={item.style.show_source!==false} source={`${report.conclusion.source||'原报告'} · ${report.model||'模型未提供'}`} at={report.generated_at}/><p className="gl-muted">方法 {report.algorithm_version||report.prompt_version||'未提供'} · 原证据见报告</p>{result?.status!=='succeeded'&&<p className="gl-inline-warning">原任务状态：{result?.status||'未知'}，以下仅为已保存的结果</p>}{report.conclusion.data_limitations?.length?<details><summary>原报告数据缺口</summary><ul>{report.conclusion.data_limitations.map((v,n)=><li key={n}>{v}</li>)}</ul></details>:null}</>:<WidgetEmpty>{result?`巡检${result.status==='running'?'进行中':result.status==='failed'?'失败':'尚未形成完整报告'}`:'尚无符合条件的巡检结果'}</WidgetEmpty>}<a href={`#portfolio/risk${result?`?record=${encodeURIComponent(result.id)}`:''}`}>查看原巡检 →</a></>;
 }
 case 'watchlist':{
  const list=state?.watchlists[String(item.params.list_id)];if(!list)return <WidgetEmpty>请在组件设置中选择或创建关注名单</WidgetEmpty>;
  const show=fields(item,['price','change','time']);const rows=list.symbols.map(symbol=>({symbol,quote:data.quotes.value?.find(q=>q.symbol===symbol)}));if(item.params.sort==='change')rows.sort((a,b)=>(b.quote?.change_percent??-Infinity)-(a.quote?.change_percent??-Infinity));
  return <><ErrorNote text={data.quotes.error}/><p className="gl-muted">{list.name} · {list.symbols.length}只</p>{rows.length?<div className="gl-table-scroll"><table><thead><tr><th>股票</th>{show.includes('price')&&<th>现价</th>}{show.includes('change')&&<th>涨跌幅</th>}{show.includes('time')&&<th>时间 / 来源</th>}</tr></thead><tbody>{rows.slice(0,item.style.row_count||8).map(({symbol,quote:q})=><tr key={symbol}><td><a href={`#stock/overview?symbol=${encodeURIComponent(symbol)}`}>{q?.name||symbol}</a><small>{symbol}</small></td>{show.includes('price')&&<td>{q&&q.price>0?numberText(q.price):'—'}</td>}{show.includes('change')&&<td className={tradeClass(q?.change_percent)}>{q&&q.price>0?quotePercent(q.change_percent):'—'}</td>}{show.includes('time')&&<td><small>{q?dataTime(q.trade_time):'数据暂缺'}</small>{item.style.show_source!==false&&<small>{q?.meta.source||'来源暂缺'}</small>}{q?.meta.stale&&<small>缓存/延迟</small>}</td>}</tr>)}</tbody></table></div>:<WidgetEmpty>关注名单为空，可在组件设置中维护</WidgetEmpty>}</>;
 }
 default:return <WidgetEmpty>此组件类型暂不受支持，请保留配置并复制修正</WidgetEmpty>;
 }
}
function Metric({label,value,primary=false,tone=''}:{label:string;value:string;primary?:boolean;tone?:string}){return <div className="gl-metric"><span>{label}</span><strong className={`${primary?'gl-primary-text':''} ${tone}`}>{value}</strong></div>}
function VolumeChart({pulse:p,item}:{pulse:Pulse;item:WidgetInstance}){
 const ref=useRef<HTMLDivElement>(null),[width,setWidth]=useState(600),[available,setAvailable]=useState(220);const factor=contentScale(item);
 useEffect(()=>{if(!ref.current)return;const observer=new ResizeObserver(()=>{if(!ref.current)return;setWidth(Math.max(160,ref.current.clientWidth));const body=ref.current.closest('.gl-widget-body');if(body)setAvailable(Math.max(120,body.clientHeight-140*factor))});observer.observe(ref.current);if(ref.current.parentElement)observer.observe(ref.current.parentElement);return()=>observer.disconnect()},[factor]);
 const days=[...(p.history||[]).slice(-value(item.params.history_days,20)),{date:p.date,amount:p.actual}];const line=value(item.params.reference_line,p.observation_line);const forecast=item.params.show_forecast===false?null:p.forecast;const mean=item.params.show_mean===false?null:p.mean20;const max=Math.ceil(Math.max(line,mean||0,forecast||0,...days.map(d=>d.amount||0),1000)/5000)*5000;
 const height=Math.max(90,Math.min(item.style.chart_height*factor,available)-36*factor),left=Math.max(49,item.style.text_size*factor*3.6+12),right=18,bottom=height-28,top=16;
 const y=(v:number)=>bottom-v/max*(bottom-top);const step=(width-left-right)/Math.max(days.length,1);
 return <div className="gl-volume-chart" style={{height:height+36*factor}} ref={ref}><div className="gl-chart-legend">{mean!=null&&<span>┄ 前20日均额 {amountText(mean)}</span>}<span className="gl-reference-line">┄ 参考线 {amountText(line)}</span><span>■ 实际</span>{forecast!=null&&<span>▧ 预计</span>}</div><svg viewBox={`0 0 ${width} ${height}`} style={{height,minHeight:height,flex:'none'}} preserveAspectRatio="xMidYMid meet" role="img" aria-label={`沪深成交额，单位亿元，${p.date}实际${amountText(p.actual)}，预计${amountText(forecast)}`}>
 {Array.from({length:5},(_,n)=>{const v=max*n/4;return <g key={n}><line x1={left} x2={width-right} y1={y(v)} y2={y(v)} className="gl-gridline"/><text x={left-8} y={y(v)+4} textAnchor="end" style={{fontSize:Math.max(10,item.style.text_size*factor)}}>{amountText(v)}</text></g>})}
 {days.map((d,n)=>{const x=left+step*n+step/2,w=Math.min(24,step*.65),last=n===days.length-1;const showDate=last||n===0||(width>500&&n%5===0);return <g key={d.date}><title>{d.date} · 实际{amountText(d.amount)}亿元{last?` · 预计${amountText(forecast)}亿元`:''}</title>{d.amount!=null&&<rect x={x-w/2} y={y(d.amount)} width={w} height={Math.max(0,bottom-y(d.amount))} className={last?'gl-bar-current':'gl-bar'}/>}{last&&forecast!=null&&d.amount!=null&&forecast>d.amount&&<rect x={x-w/2} y={y(forecast)} width={w} height={y(d.amount)-y(forecast)} className="gl-bar-forecast"/>}{showDate&&<text x={x} y={bottom+21} textAnchor="middle" style={{fontSize:Math.max(10,item.style.text_size*factor)}}>{d.date.slice(5)}</text>}</g>})}
 {mean!=null&&<line x1={left} x2={width-right} y1={y(mean)} y2={y(mean)} className="gl-mean-line"/>}<line x1={left} x2={width-right} y1={y(line)} y2={y(line)} className="gl-floor-line"/>
 </svg></div>;
}
