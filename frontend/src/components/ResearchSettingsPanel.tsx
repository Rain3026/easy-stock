import { useEffect, useState } from 'react';
import { CheckCircle2, LoaderCircle, Save, ShieldCheck } from 'lucide-react';
import { AppSettings, BackendConfig, requestJSON } from '../lib/backend';
import { ResearchSettings, researchRoles } from '../lib/daily-research';
import { runtimeLabel } from '../lib/runtime-label';
import { SettingsFooter, useSettingsDraft } from './SettingsPageLayout';

export function ResearchSettingsPanel({ config, sharedSettings }: { config: BackendConfig | null; sharedSettings?: AppSettings | null }) {
 const [value, setValue] = useState<ResearchSettings | null>(null);
 const [global, setGlobal] = useState<AppSettings | null>(null);
 const [error, setError] = useState(''); const [saved, setSaved] = useState(false); const [busy, setBusy] = useState(false);
 useEffect(() => { if (!config) return; const abort = new AbortController(); Promise.all([requestJSON<{ data: ResearchSettings }>(config, '/api/v1/daily-research/settings', { signal: abort.signal }), requestJSON<{ data: AppSettings }>(config, '/api/v1/settings', { signal: abort.signal })]).then(([a,b]) => { setValue(a.data); pageDraft.accept(a.data); setGlobal(b.data); }).catch(e => { if (!abort.signal.aborted) setError(e.message); }); return () => abort.abort(); }, [config]);
 const pageDraft = useSettingsDraft(value, next => { setValue(next); setError(''); setSaved(false); });
 const globalSettings = sharedSettings || global;
 const change = (patch: Partial<ResearchSettings>) => { setValue(v => v && ({ ...v, ...patch })); setSaved(false); };
 const save = async () => { if (!config || !value) return; setBusy(true); setError(''); try { await requestJSON(config, '/api/v1/daily-research/settings', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(value) }); pageDraft.accept(value); setSaved(true); } catch(e) { setError(e instanceof Error ? e.message : '保存失败'); } finally { setBusy(false); } };
 return <div className="dr-settings">
  <p className="dr-muted">为三个研究角色分配已配置的运行引擎和模型。相同模型也会使用独立会话。每次任务冻结配置，互不改变全局选择。</p>
  {error && <p role="alert" className="dr-error">{error}</p>}
  {!value && !error && <p role="status">正在读取配置…</p>}
  {value && <>
   {researchRoles.map((role,index) => <div className="dr-role-setting" key={role.id}><span className={`dr-avatar dr-${role.id}`}>{index+1}</span><div><strong>{role.name}</strong><small>{role.note}</small></div><label><span>运行引擎</span><select aria-label={`${role.name}运行引擎`} value={value[role.id].runtime} onChange={e => change({ [role.id]: { ...value[role.id], runtime:e.target.value } })}><option value="">沿用全局 · {runtimeLabel(globalSettings?.agent_runtime || 'hermes')}</option>{Object.entries(globalSettings?.runtimes || {}).map(([id,status]) => <option key={id} value={id} disabled={!status.available}>{runtimeLabel(id)}{!status.available ? ' · 未安装' : ''}</option>)}</select></label><label><span>模型方案</span><select aria-label={`${role.name}模型方案`} value={value[role.id].profile_id} onChange={e => change({ [role.id]: { ...value[role.id], profile_id:e.target.value } })}><option value="">沿用全局模型</option>{globalSettings?.llm_profiles?.map(p => <option key={p.id} value={p.id}>{p.name} · {p.model || '尚未配置'}</option>)}</select></label></div>)}
   <div className="dr-settings-grid"><label>交叉回应轮数<select value={value.rounds} onChange={e => change({ rounds:Number(e.target.value), max_calls:3+2*Number(e.target.value) })}>{[0,1,2].map(n => <option value={n} key={n}>{n} 轮 · {3+n*2} 次调用</option>)}</select></label><label>单次调用超时<select value={value.timeout_seconds} onChange={e => change({ timeout_seconds:Number(e.target.value) })}>{[30,60,120,180,300].map(n => <option value={n} key={n}>{n} 秒</option>)}</select></label><label>调用预算上限<select value={value.max_calls} onChange={e => change({ max_calls:Number(e.target.value) })}>{[3,4,5,6,7].filter(n => n>=3+value.rounds*2).map(n => <option value={n} key={n}>{n} 次</option>)}</select></label></div>
   <label className="dr-check"><input type="checkbox" checked={value.keep_responses} onChange={e => change({ keep_responses:e.target.checked })}/>保留各角色的完整回应记录</label>
   <p className="dr-muted"><ShieldCheck size={14}/>结构化观点、关键证据和后续条件始终存档。调用次数限制不是费用上限，费用由所选模型决定。只在点击“开始辨析”后调用模型。</p>
   <SettingsFooter><div className="settings-footer"><span className="settings-message" role="status"><ShieldCheck size={15}/>{saved ? '研究配置已保存' : pageDraft.dirty ? '本页有未保存修改，切换页面会保留。' : '仅保存研究角色与运行规则，不改变全局模型。'}</span><button type="button" onClick={pageDraft.reset} disabled={!pageDraft.dirty || busy}>还原本页</button><button type="button" className="settings-save" onClick={save} disabled={busy}>{busy ? <LoaderCircle size={15} className="spin"/> : saved ? <CheckCircle2 size={15}/> : <Save size={15}/>}保存研究</button></div></SettingsFooter>
  </>}
 </div>;
}
