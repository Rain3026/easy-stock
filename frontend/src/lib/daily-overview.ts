import type { ThemeOverview } from './backend';
export type Pulse = {
 fallback_reason?:string;
 version:string; observation_line:number; date:string; session:'closed'|'intraday'|'lunch'; at:string; fetched_at:string; source:string;
 history:{date:string;amount:number}[]; actual:number|null; forecast:number|null; mean20:number|null; previous:number|null;
 same_time_change:number|null; forecast_method:string; gaps:string[]; stale:boolean;
 breadth:{date:string;up:number;down:number;flat:number;source:string;at:string}|null;
 emotion:{date:string;up:number;down:number;boards:number;height:number;broken_rate:number;levels:{height:number;count:number}[];source:string;partial:boolean}|null;
};
export type OverviewIndex={id:string;name:string;code:string;market:string;price:number;change_percent:number;trade_time:string;status:string;meta:{source:string;stale:boolean}};
export type DailyOverview={judgement?:MarketJudgement;pulse:Pulse;indexes:OverviewIndex[]|null;themes:ThemeOverview[]|null;theme_meta:{source:string;stale:boolean;trade_date?:string}};
export const amountText=(n:number|null|undefined)=> n==null||!Number.isFinite(n)?'—':n.toLocaleString('zh-CN',{maximumFractionDigits:0});
export const percentText=(n:number|null|undefined)=> n==null||!Number.isFinite(n)?'—':`${n>0?'+':''}${n.toFixed(1)}%`;
export const relative=(n:number|null|undefined,d:number|null|undefined)=>n!=null&&d!=null&&d>0?(n/d-1)*100:null;
export function chinaDate(v:string){if(!v||v.startsWith('0001'))return '';const d=new Date(v);return Number.isNaN(d.getTime())?'':new Intl.DateTimeFormat('sv-SE',{timeZone:'Asia/Shanghai',year:'numeric',month:'2-digit',day:'2-digit'}).format(d);}
export function indexTime(i:OverviewIndex){if(!i.trade_time||i.trade_time.startsWith('0001'))return '时间未提供';const d=new Date(i.trade_time);if(Number.isNaN(d.getTime()))return '时间未提供';const time=d.toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hour12:false});return `${time}${i.market==='US'?' 源时间':i.status==='closed'?' 收盘':''}${i.meta?.stale?' · 缓存':''}`;}
export type MarketJudgement={version:string;label:string;range:[number,number]|null;tone:string;reason:string;observation_line:number;calculated_at:string};
export function judgeMarket(data:DailyOverview):MarketJudgement {
 return data.judgement||{version:'unavailable',label:'待判断',range:null,tone:'neutral',reason:'等待服务端同口径规则计算',observation_line:data.pulse.observation_line,calculated_at:''};
}
