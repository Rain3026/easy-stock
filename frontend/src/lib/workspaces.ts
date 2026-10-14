export const workspaces = [
 { id:'today', name:'今日总览', description:'量能、情绪与主线，先看清市场', tabs:[['overview','总览'],['markets','市场参照']] },
 { id:'opportunity', name:'机会发现', description:'从题材强度到个股梯队，发现值得研究的机会', tabs:[['themes','题材主线'],['sentiment','短线情绪'],['flows','资金动向']] },
 { id:'stock', name:'个股研究', description:'一份主判断，逐层展开依据与交易条件', tabs:[['overview','概览'],['analysis','深度研判'],['plan','交易预案'],['sources','资料与历史']] },
 { id:'portfolio', name:'持仓管理', description:'行情、风险与研究方案集中管理', tabs:[['overview','持仓概览'],['risk','风险巡检'],['adjustments','调整方案'],['tomorrow','明日预案']] },
 { id:'research', name:'研究中心', description:'用共同证据研究分歧，让原判断接受时间检验', tabs:[['debate','多空辨析'],['opinions','观点资料'],['verify','复盘验证'],['archives','研究档案'],['knowledge','策略知识']] },
] as const;
export type WorkspaceID = typeof workspaces[number]['id'];
export type WorkspaceRoute = { workspace:WorkspaceID|'ai'|'news'|'tasks'|'settings';tab:string;params:URLSearchParams };
const legacy:Record<string,string>={ 'daily-research':'today/overview', reviews:'research/opinions', themes:'opportunity/themes', 'limit-up':'opportunity/sentiment',mastery:'research/knowledge','stock-ai':'stock/overview','portfolio-inspection':'portfolio/risk','token-usage':'tasks/usage'};
export function parseWorkspaceRoute(hash:string):WorkspaceRoute{
 const [path='',query='']=hash.replace(/^#/,'').split('?');const params=new URLSearchParams(query);
 let [space,tab]= (legacy[path]||path).split('/');
 if(space==='market'){
  if(tab==='core-indexes'){params.set('view',tab);space='today';tab='markets'}
  else if(['industry-flow','theme-flow','stock-flow','industry-momentum','billboard','margin-balance','futures-position'].includes(tab)){params.set('view',tab);space='opportunity';tab=tab==='billboard'?'sentiment':'flows'}
  else{params.set('view',tab||'pulse');space='news';tab='search'}
 }
 const area=workspaces.find(v=>v.id===space);
 if(area) return {workspace:area.id,tab:area.tabs.some(v=>v[0]===tab)?tab:area.tabs[0][0],params};
 if(['ai','news','tasks','settings'].includes(space))return{workspace:space as WorkspaceRoute['workspace'],tab:tab||({ai:'chat',news:'search',tasks:'tasks',settings:'model'}[space]!),params};
 return {workspace:'today',tab:'overview',params};
}
export function workspaceHash(workspace:WorkspaceRoute['workspace'],tab:string,params:URLSearchParams=new URLSearchParams()){
 const query=params.toString();return `#${workspace}/${tab}${query?'?'+query:''}`;
}
export function legacyWorkspaceMode(r:WorkspaceRoute){
 if(r.workspace==='settings')return 'settings';if(r.workspace==='ai')return 'ai';if(r.workspace==='tasks')return 'token-usage';
 if(r.workspace==='today')return r.tab==='overview'?'daily-research':'market';
 if(r.workspace==='opportunity')return r.tab==='themes'?'themes':r.tab==='sentiment'&&r.params.get('view')!=='billboard'?'limit-up':'market';
 if(r.workspace==='stock')return 'stock-ai';if(r.workspace==='portfolio')return 'portfolio-inspection';
 if(r.workspace==='research')return r.tab==='knowledge'?'mastery':'research-center';return 'market';
}
