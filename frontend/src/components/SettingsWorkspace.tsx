import { SkillLifecycle } from './SkillLifecycle';
import { runtimeLabel } from '../lib/runtime-label';
import { DataSourcesPanel } from './DataSourcesPanel';
import {
	Bot,
	CheckCircle2,
	CircleAlert,
	Clock3,
	Database,
	Eye,
	EyeOff,
	FolderOpen,
	Globe2,
	KeyRound,
	LoaderCircle,
	LogIn,
	PlugZap,
	Plus,
	RefreshCw,
	QrCode,
	Save,
	Server,
	Trash2,
	Bell, Puzzle, Scale, Download, ShieldCheck,
} from 'lucide-react';
import { FormEvent, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { AppSettings, BackendConfig, BackendRequestError, LLMConnectionTestResult, LLMModelOption, LLMModelsResult, LLMProfile, RuntimeLogStatus, SecretSettingStatus, requestJSON } from '../lib/backend';
import { llmBaseURLForAPIMode, llmProviderDefinition, llmProviders } from '../lib/llm-providers';
import { AppUpdatePanel } from './AppUpdatePanel';
import { AgentSettingsPanel } from './AgentSettingsPanel';
import { SettingsSection } from './SettingsSection';
import { ResearchSettingsPanel } from './ResearchSettingsPanel';
import './daily-research.css';
import { NotificationSettingsPanel } from './NotificationSettingsPanel';
import { SettingsPageContext, SettingsFooter, useSettingsDraft } from './SettingsPageLayout';
import { settingsTabs, settingsUpdateForTab, type SettingsTab } from '../lib/settings-workspace';
import './settings-workspace.css';

type Props = {
	config: BackendConfig | null;
	active: boolean;
	tab: SettingsTab;
	onTabChange: (tab: SettingsTab) => void;
	onSaved?: () => void;
};

type SecretKey = 'llm_api_key' | 'tushare_token' | 'ths_cookie' | 'xueqiu_cookie' | 'eastmoney_cookie' | 'wechat_api_token';
type ModelListState = 'idle' | 'loading' | 'success' | 'error';

const manualModelOption = '__manual_model_input__';
const defaultResponseTimeoutSeconds = 300;

const emptySecrets = (): Record<SecretKey, string> => ({
	llm_api_key: '',
	tushare_token: '',
	ths_cookie: '',
	xueqiu_cookie: '',
	eastmoney_cookie: '',
	wechat_api_token: '',
});

export function SettingsWorkspace({ config, active, tab, onTabChange, onSaved }: Props) {
	const [settings, setSettings] = useState<AppSettings | null>(null);
	const [agentRuntime, setAgentRuntime] = useState('hermes');
	const [llmProfiles, setLLMProfiles] = useState<LLMProfile[]>([]);
	const [activeLLMProfileID, setActiveLLMProfileID] = useState('');
	const [profileName, setProfileName] = useState('');
	const [profileKeyValues, setProfileKeyValues] = useState<Record<string, string>>({});
	const [clearProfileKeys, setClearProfileKeys] = useState<Set<string>>(new Set());
	const [provider, setProvider] = useState('openai');
	const [baseURL, setBaseURL] = useState(llmProviderDefinition('openai').baseURL);
	const [model, setModel] = useState('');
	const [apiMode, setAPIMode] = useState<string>(llmProviderDefinition('openai').apiMode);
	const [responseTimeoutSeconds, setResponseTimeoutSeconds] = useState(defaultResponseTimeoutSeconds);
	const [secrets, setSecrets] = useState<Record<SecretKey, string>>(emptySecrets);
	const [clearSecrets, setClearSecrets] = useState<Set<SecretKey>>(new Set());
	const [state, setState] = useState<'idle' | 'loading' | 'saving' | 'saved' | 'error'>('idle');
	const [message, setMessage] = useState('');
	const [messageScope, setMessageScope] = useState<SettingsTab>('model');
	const [testState, setTestState] = useState<'idle' | 'testing' | 'success' | 'error'>('idle');
	const [testResult, setTestResult] = useState<LLMConnectionTestResult | null>(null);
	const [modelOptions, setModelOptions] = useState<LLMModelOption[]>([]);
	const [modelListState, setModelListState] = useState<ModelListState>('idle');
	const [modelListMessage, setModelListMessage] = useState('');
	const [manualModel, setManualModel] = useState(true);
	const [runtimeLogStatus, setRuntimeLogStatus] = useState<RuntimeLogStatus | null>(null);
	const [openingRuntimeLogs, setOpeningRuntimeLogs] = useState(false);
	const modelFetchSequence = useRef(0);
	const [footer, setFooter] = useState<HTMLDivElement | null>(null);
	const [visited, setVisited] = useState<Set<SettingsTab>>(() => new Set([tab]));
	const [dirtyTabs, setDirtyTabs] = useState<Partial<Record<SettingsTab, boolean>>>({});
	const [loadRevision, setLoadRevision] = useState(0);
	const [secretRevision, setSecretRevision] = useState(0);
	const panelRefs = useRef<Partial<Record<SettingsTab, HTMLDivElement | null>>>({});
	const tabRefs = useRef<Partial<Record<SettingsTab, HTMLButtonElement | null>>>({});
	const reportDirty = useCallback((id: SettingsTab, dirty: boolean) => setDirtyTabs(prev => prev[id] === dirty ? prev : { ...prev, [id]: dirty }), []);
	const contexts = useMemo(() => Object.fromEntries(settingsTabs.map(id => [id, { active: active && tab === id, footer, reportDirty: (dirty: boolean) => reportDirty(id, dirty) }])) as Record<SettingsTab, { active: boolean; footer: HTMLDivElement | null; reportDirty: (dirty: boolean) => void }>, [active, tab, footer, reportDirty]);
	useEffect(() => { setVisited(prev => prev.has(tab) ? prev : new Set([...prev, tab])); }, [tab]);
	useEffect(() => { if (panelRefs.current[tab]) panelRefs.current[tab]!.scrollTop = 0; }, [tab]);
	const hasDrafts = Object.values(dirtyTabs).some(Boolean);
	useEffect(() => {
		if (!hasDrafts) return;
		const warn = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = ''; };
		window.addEventListener('beforeunload', warn);
		return () => window.removeEventListener('beforeunload', warn);
	}, [hasDrafts]);
	const modelDraftValue = { agentRuntime, llmProfiles, activeLLMProfileID, profileName, provider, baseURL, model, apiMode, responseTimeoutSeconds, profileKeyValues, clearProfileKeys: [...clearProfileKeys] };
	const modelDraft = useSettingsDraft(modelDraftValue, value => {
		setAgentRuntime(value.agentRuntime); setLLMProfiles(value.llmProfiles); setActiveLLMProfileID(value.activeLLMProfileID);
		setProfileName(value.profileName); setProvider(value.provider); setBaseURL(value.baseURL); setModel(value.model); setAPIMode(value.apiMode);
		setResponseTimeoutSeconds(value.responseTimeoutSeconds); setProfileKeyValues(value.profileKeyValues); setClearProfileKeys(new Set(value.clearProfileKeys));
		setSecretRevision(n => n + 1); resetModelList(); setTestState('idle'); setTestResult(null); setMessage('');
	});
	const dataDraft = useSettingsDraft({ secrets, clearSecrets: [...clearSecrets] }, value => { setSecrets(value.secrets); setClearSecrets(new Set(value.clearSecrets)); setSecretRevision(n => n + 1); setMessage(''); });
	useEffect(() => { reportDirty('model', modelDraft.dirty); }, [modelDraft.dirty, reportDirty]);
	useEffect(() => { reportDirty('data', dataDraft.dirty); }, [dataDraft.dirty, reportDirty]);
	const acceptModel = (data: AppSettings) => {
		const profiles = normalizeLLMProfiles(data);
		const selected = profiles.find(p => p.id === data.active_llm_profile_id) || profiles[0];
		const profileProvider = selected.provider || 'openai';
		const profileMode = selected.api_mode === 'responses' ? 'codex_responses' : selected.api_mode || (profileProvider === 'anthropic' ? 'anthropic_messages' : 'chat_completions');
		setAgentRuntime(data.agent_runtime || 'hermes'); setLLMProfiles(profiles); setActiveLLMProfileID(selected.id); loadProfileFields(selected);
		setResponseTimeoutSeconds(data.llm.response_timeout_seconds || defaultResponseTimeoutSeconds); setProfileKeyValues({}); setClearProfileKeys(new Set());
		modelDraft.accept({ agentRuntime: data.agent_runtime || 'hermes', llmProfiles: profiles, activeLLMProfileID: selected.id, profileName: selected.name, provider: profileProvider, baseURL: selected.base_url || llmBaseURLForAPIMode(profileProvider, '', profileMode), model: selected.model || '', apiMode: profileMode, responseTimeoutSeconds: data.llm.response_timeout_seconds || defaultResponseTimeoutSeconds, profileKeyValues: {}, clearProfileKeys: [] });
	};
	const acceptData = () => { setSecrets(emptySecrets()); setClearSecrets(new Set()); dataDraft.accept({ secrets: emptySecrets(), clearSecrets: [] }); };

	useEffect(() => {
		if (!config) return;
		let cancelled = false;
		modelFetchSequence.current += 1;
		setState('loading');
		setMessage('');
		setTestState('idle');
		setTestResult(null);
		setModelOptions([]);
		setModelListState('idle');
		setModelListMessage('');
		setManualModel(true);
		requestJSON<{ data: AppSettings }>(config, '/api/v1/settings')
			.then((payload) => {
				if (cancelled) return;
				setSettings(payload.data);
				acceptModel(payload.data);
				acceptData();
				setState('idle');
			})
			.catch((error) => {
				if (cancelled) return;
				setState('error');
				setMessage(error instanceof Error ? error.message : '读取设置失败');
			});
		return () => {
			cancelled = true;
			modelFetchSequence.current += 1;
		};
	}, [config, loadRevision]);

	useEffect(() => {
		if (!active || tab !== 'logs' || !window.aStock?.getRuntimeLogStatus) {
			setRuntimeLogStatus(null);
			return;
		}
		void window.aStock.getRuntimeLogStatus().then(setRuntimeLogStatus).catch(() => setRuntimeLogStatus(null));
	}, [active, tab]);

	const selectedModelCapability = modelOptions.find((option) => option.id === model)?.reasoning;
	const selectedLLMProfile = useMemo(() => llmProfiles.find((profile) => profile.id === activeLLMProfileID), [activeLLMProfileID, llmProfiles]);

	const patchSelectedLLMProfile = (patch: Partial<LLMProfile>) => setLLMProfiles((current) => current.map((profile) => profile.id === activeLLMProfileID ? { ...profile, ...patch } : profile));

	const loadProfileFields = (profile: LLMProfile) => {
		const profileProvider = profile.provider || 'openai';
		const profileMode = profile.api_mode === 'responses' ? 'codex_responses' : profile.api_mode || (profileProvider === 'anthropic' ? 'anthropic_messages' : 'chat_completions');
		setProfileName(profile.name);
		setProvider(profileProvider);
		setBaseURL(profile.base_url || llmBaseURLForAPIMode(profileProvider, '', profileMode));
		setModel(profile.model || '');
		setAPIMode(profileMode);
	};

	const selectLLMProfile = (id: string) => {
		const profile = llmProfiles.find((item) => item.id === id);
		if (!profile) return;
		setActiveLLMProfileID(id);
		loadProfileFields(profile);
		resetModelList();
		setTestState('idle');
		setTestResult(null);
	};

	const addLLMProfile = () => {
		const id = `llm-${Date.now()}-${Math.random().toString(16).slice(2)}`;
		const definition = llmProviderDefinition('openai');
		const profile: LLMProfile = { id, name: '新模型配置', provider: 'openai', base_url: definition.baseURL, model: definition.defaultModel, api_mode: definition.apiMode, api_key: { configured: false } };
		setLLMProfiles((current) => [...current, profile]);
		setActiveLLMProfileID(id);
		loadProfileFields(profile);
		resetModelList();
	};

	const removeLLMProfile = () => {
		if (llmProfiles.length <= 1) return;
		const remaining = llmProfiles.filter((profile) => profile.id !== activeLLMProfileID);
		const next = remaining[0];
		setLLMProfiles(remaining);
		setActiveLLMProfileID(next.id);
		loadProfileFields(next);
		setProfileKeyValues((current) => { const copy = { ...current }; delete copy[activeLLMProfileID]; return copy; });
		setClearProfileKeys((current) => { const copy = new Set(current); copy.delete(activeLLMProfileID); return copy; });
		resetModelList();
	};

	const selectableModels = useMemo(() => {
		const currentModel = model.trim();
		if (!currentModel || modelOptions.some((option) => option.id === currentModel)) return modelOptions;
		return [{ id: currentModel, display_name: '当前配置' }, ...modelOptions];
	}, [model, modelOptions]);

	const resetModelList = () => {
		modelFetchSequence.current += 1;
		setModelOptions([]);
		setModelListState('idle');
		setModelListMessage('');
		setManualModel(true);
	};

	const updateProvider = (nextProvider: string) => {
		const nextDefinition = llmProviderDefinition(nextProvider);
		setProvider(nextProvider);
		setBaseURL(nextDefinition.baseURL);
		setModel(nextDefinition.defaultModel);
		setAPIMode(nextDefinition.apiMode);
		patchSelectedLLMProfile({ provider: nextProvider, base_url: nextDefinition.baseURL, model: nextDefinition.defaultModel, api_mode: nextDefinition.apiMode });
		resetModelList();
		setTestState('idle');
		setTestResult(null);
		setModelListMessage(nextDefinition.baseURL
			? '请确认 Base URL，输入模型 API Key，然后点击“获取模型”。'
			: '请输入兼容接口的 Base URL 和 API Key，再获取模型列表。');
	};

	const updateAPIMode = (nextMode: string) => {
		const nextBaseURL = llmBaseURLForAPIMode(provider, baseURL, nextMode);
		setAPIMode(nextMode);
		setBaseURL(nextBaseURL);
		patchSelectedLLMProfile({ api_mode: nextMode, base_url: nextBaseURL });
		resetModelList();
		setTestState('idle');
		setTestResult(null);
	};

	const updateSecret = (key: SecretKey, value: string) => {
		if (key === 'llm_api_key') {
			setProfileKeyValues((current) => ({ ...current, [activeLLMProfileID]: value }));
			setClearProfileKeys((current) => { const next = new Set(current); if (value) next.delete(activeLLMProfileID); return next; });
			resetModelList(); setTestState('idle'); setTestResult(null); return;
		}
		setSecrets((current) => ({ ...current, [key]: value }));
		if (value) {
			setClearSecrets((current) => {
				const next = new Set(current);
				next.delete(key);
				return next;
			});
		}
	};

	const toggleClear = (key: SecretKey) => {
		if (key === 'llm_api_key') {
			setClearProfileKeys((current) => { const next = new Set(current); if (next.has(activeLLMProfileID)) next.delete(activeLLMProfileID); else next.add(activeLLMProfileID); return next; });
			setProfileKeyValues((current) => ({ ...current, [activeLLMProfileID]: '' }));
			resetModelList(); return;
		}
		setClearSecrets((current) => {
			const next = new Set(current);
			if (next.has(key)) next.delete(key);
			else next.add(key);
			return next;
		});
		setSecrets((current) => ({ ...current, [key]: '' }));
	};

	const updateModel = (nextModel: string) => {
		setModel(nextModel);
		patchSelectedLLMProfile({ model: nextModel });
		setTestState('idle');
		setTestResult(null);
	};

	const updateBaseURL = (nextBaseURL: string) => {
		setBaseURL(nextBaseURL);
		patchSelectedLLMProfile({ base_url: nextBaseURL });
		resetModelList();
		setTestState('idle');
		setTestResult(null);
	};

	const fetchModels = async () => {
		if (!config) return;
		const fetchID = ++modelFetchSequence.current;
		const requestProvider = provider;
		const requestBaseURL = baseURL.trim();
		const requestAPIKey = (profileKeyValues[activeLLMProfileID] || '').trim();
		setModelListState('loading');
		setModelListMessage('正在读取模型服务的模型列表…');
		try {
			const request: { provider: string; base_url: string; api_mode: string; api_key?: string } = { provider: requestProvider, base_url: requestBaseURL, api_mode: apiMode };
			if (requestAPIKey || clearProfileKeys.has(activeLLMProfileID)) request.api_key = requestAPIKey;
			(request as { profile_id?: string }).profile_id = activeLLMProfileID;
			const payload = await requestJSON<{ data: LLMModelsResult }>(config, '/api/v1/settings/llm/models', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(request),
			});
			if (fetchID !== modelFetchSequence.current) return;
			setModelOptions(payload.data.models);
			setModelListState('success');
			setModelListMessage(`已从 ${payload.data.source_url} 获取 ${payload.data.models.length} 个模型`);
			setManualModel(false);
		} catch (error) {
			if (fetchID !== modelFetchSequence.current) return;
			setModelOptions([]);
			setModelListState('error');
			setModelListMessage(error instanceof Error ? error.message : '获取模型列表失败');
			setManualModel(true);
		}
	};

	const readSavedModelKey = async (signal: AbortSignal) => {
		if (!config) throw new Error('后端尚未连接');
		const payload = await requestJSON<{ data: { api_key: string } }>(config, '/api/v1/settings/llm/api-key/reveal', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ profile_id: activeLLMProfileID }),
			cache: 'no-store',
			signal,
		});
		return payload.data.api_key;
	};

	const persistSettings = async (scope: 'model' | 'data' = 'model') => {
		if (!config) throw new Error('后端尚未连接');
		const credentials: Record<string, string> = {};
		for (const key of ['tushare_token', 'ths_cookie', 'xueqiu_cookie', 'eastmoney_cookie'] as SecretKey[]) {
			if (secrets[key].trim()) credentials[key] = secrets[key].trim();
		}
		const modelProfiles = llmProfiles.map((profile) => {
			const current = profile.id === activeLLMProfileID ? { ...profile, name: profileName.trim(), provider, base_url: baseURL.trim(), model: model.trim(), api_mode: apiMode } : profile;
			return { id: current.id, name: current.name.trim(), provider: current.provider, base_url: current.base_url.trim(), model: current.model.trim(), api_mode: current.api_mode, api_key: (profileKeyValues[current.id] || '').trim() || undefined, clear_api_key: clearProfileKeys.has(current.id) };
		});
		const payload = await requestJSON<{ data: AppSettings }>(config, '/api/v1/settings', {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(settingsUpdateForTab(scope, { agent_runtime: agentRuntime, llm: { response_timeout_seconds: responseTimeoutSeconds }, llm_profiles: modelProfiles, active_llm_profile_id: activeLLMProfileID, credentials, clear_secrets: [...clearSecrets].filter((key) => key !== 'llm_api_key') })),
		});
		setSettings(payload.data);
		if (scope === 'model') acceptModel(payload.data);
		else acceptData();
		return payload.data;
	};

	const save = async (event: FormEvent, scope: 'model' | 'data') => {
		event.preventDefault();
		if (!config) return;
		setState('saving');
		setMessageScope(scope);
		setMessage('');
		try {
			await persistSettings(scope);
			setState('saved');
			setMessage(scope === 'model' ? '模型配置已保存' : '数据源配置已保存');
			onSaved?.();
		} catch (error) {
			setState('error');
			setMessage(error instanceof Error && error.message ? `保存失败：${error.message}` : '保存失败');
		}
	};

	const testConnection = async () => {
		if (!config) return;
		setState('saving');
		setMessageScope('model');
		setMessage('正在保存设置并通过 Agent 调用模型探针');
		setTestState('testing');
		setTestResult(null);
		try {
			await persistSettings();
			const payload = await requestJSON<{ data: LLMConnectionTestResult }>(config, '/api/v1/settings/llm/test', { method: 'POST' });
			setTestResult(payload.data);
			setTestState('success');
			setState('saved');
			setMessage(`Agent 模型连接成功，耗时 ${payload.data.latency_ms}ms`);
		} catch (error) {
			setTestState('error');
			setState('error');
			setMessage(error instanceof Error ? error.message : '模型连接测试失败');
		}
	};

	const openRuntimeLogs = async () => {
		if (!window.aStock?.openRuntimeLogs) return;
		setOpeningRuntimeLogs(true);
		try {
			await window.aStock.openRuntimeLogs();
		} catch (error) {
			setState('error');
			setMessage(error instanceof Error ? error.message : '打开日志目录失败');
		} finally {
			setOpeningRuntimeLogs(false);
		}
	};

	const labels: Record<SettingsTab, string> = { model: '模型', skills: 'SKILL', mcp: 'MCP', research: '研究', data: '数据源', notifications: '通知', updates: '更新', logs: '日志' };
	const icons = { model: Bot, skills: Puzzle, mcp: Server, research: Scale, data: Database, notifications: Bell, updates: Download, logs: FolderOpen };
	const footerFor = (scope: 'model' | 'data') => <SettingsFooter><footer className="settings-footer">
		<div className={`settings-message ${state}`} role={state === 'error' ? 'alert' : 'status'}>{state === 'saved' ? <CheckCircle2 size={15}/> : state === 'error' ? <CircleAlert size={15}/> : <ShieldCheck size={15}/>}<span>{(messageScope === scope ? message : '') || (dirtyTabs[scope] ? '本页有未保存修改，切换标签或功能页会保留。' : '留空的密钥会保留原值，仅保存当前页配置。')}</span></div>
		<button type="button" disabled={!(scope === 'model' ? modelDraft.dirty : dataDraft.dirty) || state === 'saving' || testState === 'testing'} onClick={scope === 'model' ? modelDraft.reset : dataDraft.reset}>还原本页</button>
		<button type="submit" form={`settings-${scope}-form`} className="settings-save" disabled={!settings || state === 'loading' || state === 'saving' || testState === 'testing'}>{state === 'saving' ? <LoaderCircle className="spin" size={16}/> : <Save size={16}/>}保存{labels[scope]}</button>
	</footer></SettingsFooter>;
	return <section className="settings-workspace" aria-label="系统设置工作区">
		<nav className="settings-page-tabs" role="tablist" aria-label="设置分类">{settingsTabs.map((id, index) => { const Icon = icons[id]; return <button type="button" role="tab" id={`settings-tab-${id}`} aria-controls={`settings-panel-${id}`} aria-selected={tab === id} tabIndex={tab === id ? 0 : -1} ref={el => { tabRefs.current[id] = el; }} key={id} onClick={() => { setMessage(''); onTabChange(id); }} onKeyDown={event => {
			const next = event.key === 'ArrowRight' ? (index + 1) % settingsTabs.length : event.key === 'ArrowLeft' ? (index + settingsTabs.length - 1) % settingsTabs.length : event.key === 'Home' ? 0 : event.key === 'End' ? settingsTabs.length - 1 : -1;
			if (next < 0) return; event.preventDefault(); setMessage(''); onTabChange(settingsTabs[next]); tabRefs.current[settingsTabs[next]]?.focus();
		}}><Icon size={17}/><span>{labels[id]}</span>{dirtyTabs[id] && <i className="settings-draft-dot" aria-label="未保存"/>}</button>; })}</nav>
		{settingsTabs.map(id => <SettingsPageContext.Provider key={id} value={contexts[id]}><div className="settings-tab-panel" role="tabpanel" id={`settings-panel-${id}`} aria-labelledby={`settings-tab-${id}`} hidden={tab !== id} tabIndex={0} ref={el => { panelRefs.current[id] = el; }}>
		{(visited.has(id) || id === tab) && <>
		{(id === 'model' || id === 'data') && (!settings ? <div className="settings-page-load" role={state === 'error' ? 'alert' : 'status'}>{state === 'error' ? <><CircleAlert size={22}/><p>{message}</p><button type="button" onClick={() => setLoadRevision(n => n + 1)}>重新读取</button></> : <><LoaderCircle className="spin" size={22}/><p>读取本机设置…</p></>}</div> : id === 'model' ? <form id="settings-model-form" onSubmit={event => void save(event, 'model')}><fieldset className="settings-page-fieldset" disabled={state === 'saving' || testState === 'testing'}>
						<SettingsSection title="模型与引擎" description="设置新对话与投研任务使用的引擎和模型。" icon={<Bot size={18} />}>
							<div className="runtime-selector">
								<span className="runtime-selector-label">运行引擎</span>
								<div className="runtime-selector-buttons" role="group" aria-label="运行引擎">
									<button type="button" aria-pressed={agentRuntime === 'hermes'} onClick={() => setAgentRuntime('hermes')}>Hermes</button>
									<button type="button" aria-pressed={agentRuntime === 'codex'} onClick={() => setAgentRuntime('codex')}>Codex<span className="runtime-beta-badge">Beta</span></button>
									<button type="button" aria-pressed={agentRuntime === 'workbuddy'} onClick={() => setAgentRuntime('workbuddy')}>WorkBuddy<span className="runtime-beta-badge">Beta</span></button>
								</div>
								<small>保存后新对话轮次和新任务使用所选引擎；进行中的任务继续完成。</small>
							</div>
                            {agentRuntime === 'workbuddy' && <p className={`model-list-message ${apiMode === 'chat_completions' ? '' : 'error'}`}>使用本机 WorkBuddy 引擎，共享下方模型和 API Key。请选择 Chat Completions 协议；不会调用 WorkBuddy 账号额度。</p>}
                            {agentRuntime === 'codex' && !['auto', 'responses', 'codex_responses'].includes(apiMode) && <p className="model-list-message error">保存时会检测此连接是否支持 Responses；检测通过后统一使用 Responses，不支持时无法启用 Codex。</p>}
                            <div className="settings-runtime-statuses">{Object.entries(settings?.runtimes || {}).map(([id, status]) => <div key={id} className={`llm-connection-test ${status.available && status.configured ? 'success' : ''}`}><div><Bot size={17} /><span><strong>{runtimeLabel(id)} {status.version || ''} · {status.available ? '已安装' : '不可用'}</strong><small>{status.message || (status.configured ? '共享模型配置已就绪' : '请配置模型连接')}</small></span></div></div>)}</div>
                            <div className="llm-profile-toolbar">
								<label><span>模型配置</span><select value={activeLLMProfileID} onChange={(event) => selectLLMProfile(event.target.value)}>{llmProfiles.map((profile) => <option value={profile.id} key={profile.id}>{profile.name} · {profile.model || llmProviderDefinition(profile.provider).label}</option>)}</select></label>
								<button type="button" onClick={addLLMProfile}><Plus size={14} />新增配置</button>
								<button type="button" className="danger" onClick={removeLLMProfile} disabled={llmProfiles.length <= 1}><Trash2 size={14} />删除</button>
							</div>
							<label><span>方案名称</span><input value={profileName} onChange={(event) => { setProfileName(event.target.value); patchSelectedLLMProfile({ name: event.target.value }); }} placeholder="例如 DeepSeek 日常 / GPT-5.6 Sol 深度分析" /></label>
							<div className="settings-grid two-columns">
								<label><span>服务商</span><select value={provider} onChange={(event) => updateProvider(event.target.value)}>{llmProviders.map((item) => <option value={item.id} key={item.id}>{item.label}</option>)}</select></label>
								<label><span>接口协议</span><select value={apiMode} onChange={(event) => updateAPIMode(event.target.value)}><option value="codex_responses">Responses API</option><option value="chat_completions">Chat Completions</option><option value="anthropic_messages">Anthropic Messages</option><option value="auto">自动检测（优先 Responses）</option></select></label>
							</div>
								<label className="settings-api-url"><span>接口地址</span><input value={baseURL} onChange={(event) => updateBaseURL(event.target.value)} placeholder="https://api.example.com/v1" />{llmProviderDefinition(provider).protocolBaseURLs && <small>切换协议时同步对应的官方地址；自定义地址保持不变。</small>}</label>
								<SecretField key={`llm-api-key-${activeLLMProfileID}-${secretRevision}`} label="模型 API Key" secretKey="llm_api_key" status={selectedLLMProfile?.api_key} value={profileKeyValues[activeLLMProfileID] || ''} clearing={clearProfileKeys.has(activeLLMProfileID)} onChange={updateSecret} onClear={toggleClear} hint="留空保留，输入新值替换" revealable readSavedValue={readSavedModelKey} />
								<div className="model-field">
									<span className="model-field-heading"><span>模型</span>{modelListState === 'success' && <button type="button" onClick={() => setManualModel((current) => !current)}>{manualModel ? '使用下拉' : '手动输入'}</button>}</span>
									<span className="model-picker-row">
										{modelListState === 'success' && !manualModel ? <select value={model} onChange={(event) => { if (event.target.value === manualModelOption) setManualModel(true); else updateModel(event.target.value); }}><option value="">请选择模型</option>{selectableModels.map((option) => <option value={option.id} key={option.id}>{modelOptionLabel(option)}</option>)}<option value={manualModelOption}>手动输入其他模型…</option></select> : <input value={model} onChange={(event) => updateModel(event.target.value)} placeholder="例如 gpt-5.5 或 deepseek-v4-pro" />}
										<button type="button" className="model-refresh-button" onClick={() => void fetchModels()} disabled={!config || state === 'saving' || modelListState === 'loading'}>{modelListState === 'loading' ? <LoaderCircle className="spin" size={14} /> : <RefreshCw size={14} />}{modelListState === 'success' ? '刷新' : '获取模型'}</button>
									</span>
									<small className={`model-list-message ${modelListState}`}>{modelListMessage || '请先填写 Base URL 和 API Key，再获取模型列表；也可继续手动输入。'}</small>
									{selectedModelCapability && <small className="model-list-message">思考选项：{selectedModelCapability.options.map((option) => option.label).join('、')} · {selectedModelCapability.source === 'model_api' ? '来源：模型接口' : selectedModelCapability.source === 'hermes' ? '来源：Hermes 模型目录' : selectedModelCapability.source === 'unknown' ? '能力未确认' : '来源：官方文档'}</small>}
								</div>
							<label className="settings-timeout-field"><span>响应超时（秒）</span><input type="number" min={30} max={3600} step={30} value={responseTimeoutSeconds} onChange={(event) => setResponseTimeoutSeconds(Number(event.target.value) || defaultResponseTimeoutSeconds)} /><small>默认 300 秒，范围 30–3600 秒；模型长时间无响应时将等待更久再重试。</small></label>
							<div className={`llm-connection-test ${testState}`}>
								<div><PlugZap size={17} /><span><strong>{testState === 'success' ? 'Agent 模型连接可用' : testState === 'error' ? '连接测试未通过' : testState === 'testing' ? 'Agent 正在请求模型' : 'Agent 真实模型探针'}</strong><small>{testResult ? `${runtimeLabel(testResult.runtime)} · ${testResult.model} · ${testResult.api_mode} · ${testResult.latency_ms}ms · ${testResult.response}` : '自动检测会发送一次简短的 Responses 请求；密钥或网络错误会保留原设置。保存后可测试当前引擎。'}</small></span></div>
								<button type="button" onClick={testConnection} disabled={!config || state === 'saving' || testState === 'testing'}>{testState === 'testing' ? <LoaderCircle className="spin" size={15} /> : <PlugZap size={15} />}保存并测试连接</button>
							</div>
						</SettingsSection>
		</fieldset>{footerFor('model')}</form> : <form id="settings-data-form" key={`data-${secretRevision}`} onSubmit={event => void save(event, 'data')}><fieldset className="settings-page-fieldset" disabled={state === 'saving'}>
<DataSourcesPanel config={config} active={active && tab === 'data'} revision={secretRevision}/>
<SettingsSection title="行情与资讯凭据" description="保存凭据后可检测相应能力；保存成功不等于接口已通过验证。" icon={<Database size={18} />}>
							<SecretField label="Tushare Pro Token" secretKey="tushare_token" status={settings?.credentials.tushare_token} value={secrets.tushare_token} clearing={clearSecrets.has('tushare_token')} onChange={updateSecret} onClear={toggleClear} hint="已接入沪深量能、指数和分钟数据；具体接口权限以检测结果为准" />
							<SecretField label="同花顺 Cookie / Token" secretKey="ths_cookie" status={settings?.credentials.ths_cookie} value={secrets.ths_cookie} clearing={clearSecrets.has('ths_cookie')} onChange={updateSecret} onClear={toggleClear} hint="预留：涨停原因与题材催化数据" />
							<SecretField label="东方财富 Cookie" secretKey="eastmoney_cookie" status={settings?.credentials.eastmoney_cookie} value={secrets.eastmoney_cookie} clearing={clearSecrets.has('eastmoney_cookie')} onChange={updateSecret} onClear={toggleClear} hint="当前公共行情不需要，预留登录态接口" />
						</SettingsSection>
		</fieldset>{footerFor('data')}</form>)}
		{(id === 'skills' || id === 'mcp') && <AgentSettingsPanel config={config} open section={id} />}
 {id==='skills'&&<SkillLifecycle config={config}/>}
		{id === 'research' && <SettingsSection title="多空辨析" description="为多方、空方与审校员分配独立引擎和模型。" icon={<Scale size={18}/>}><ResearchSettingsPanel config={config} sharedSettings={settings}/></SettingsSection>}
		{id === 'notifications' && <NotificationSettingsPanel config={config}/>}
		{id === 'updates' && <AppUpdatePanel/>}
		{id === 'logs' && <SettingsSection className="runtime-log-section" title="运行日志" description="遇到问题时，可将此目录中的日志文件提供给开发者排查。" icon={<FolderOpen size={18} />}>
							<div className="runtime-log-summary">
								<span><strong>{runtimeLogStatus?.available ? '日志正在自动保存' : '请在桌面应用中查看日志'}</strong><small>{runtimeLogStatus ? `每个文件最多 ${runtimeLogStatus.max_file_mb} MB，保留 ${runtimeLogStatus.backup_files} 份历史记录；密钥和登录凭据会在写入前隐藏。` : '浏览器开发模式不保存桌面运行日志。'}</small>{runtimeLogStatus?.directory && <code title={runtimeLogStatus.directory}>{runtimeLogStatus.directory}</code>}</span>
								<button type="button" onClick={() => void openRuntimeLogs()} disabled={!runtimeLogStatus?.available || openingRuntimeLogs}>{openingRuntimeLogs ? <LoaderCircle className="spin" size={15} /> : <FolderOpen size={15} />}打开日志目录</button>
							</div>
						</SettingsSection>}
		</>}
		</div></SettingsPageContext.Provider>)}
		<div className="settings-page-footer" ref={setFooter}/>
	</section>;
}

function normalizeLLMProfiles(settings: AppSettings): LLMProfile[] {
	const profiles = settings.llm_profiles?.length ? settings.llm_profiles : [{ id: 'llm-default', name: settings.llm.model || llmProviderDefinition(settings.llm.provider).label, provider: settings.llm.provider, base_url: settings.llm.base_url, model: settings.llm.model, api_mode: settings.llm.api_mode, api_key: settings.llm.api_key }];
	return profiles.map((profile) => {
		// The backend's initial empty profile uses legacy display defaults. Apply
		// new form defaults only to that untouched profile, never a saved connection.
		if (profile.provider || profile.base_url || profile.model || profile.api_key.configured) return profile;
		const definition = llmProviderDefinition('openai');
		return { ...profile, provider: definition.id, base_url: definition.baseURL, api_mode: definition.apiMode };
	});
}

function modelOptionLabel(option: LLMModelOption) {
	const detail = option.display_name || option.owned_by;
	return detail && detail !== option.id ? `${option.id} · ${detail}` : option.id;
}

function SecretField({ label, secretKey, status, value, clearing, onChange, onClear, hint, revealable = false, readSavedValue }: {
	label: string;
	secretKey: SecretKey;
	status?: SecretSettingStatus;
	value: string;
	clearing: boolean;
	onChange: (key: SecretKey, value: string) => void;
	onClear: (key: SecretKey) => void;
	hint?: string;
	revealable?: boolean;
	readSavedValue?: (signal: AbortSignal) => Promise<string>;
}) {
	const [revealed, setRevealed] = useState(false);
	// Viewing a saved key must not turn it (or the mask) into an unsaved edit.
	const [savedValue, setSavedValue] = useState('');
	const [loading, setLoading] = useState(false);
	const [revealError, setRevealError] = useState('');
	const revealRequest = useRef<AbortController | null>(null);
	useEffect(() => () => revealRequest.current?.abort(), []);

	const cancelReveal = () => {
		revealRequest.current?.abort();
		revealRequest.current = null;
		setLoading(false);
		setSavedValue('');
		setRevealError('');
	};
	const toggleRevealed = async () => {
		cancelReveal();
		if (revealed) {
			setRevealed(false);
			return;
		}
		if (value) {
			setRevealed(true);
			return;
		}
		if (!status?.configured || !readSavedValue) return;
		const controller = new AbortController();
		revealRequest.current = controller;
		setLoading(true);
		try {
			const key = await readSavedValue(controller.signal);
			if (controller.signal.aborted) return;
			setSavedValue(key);
			setRevealed(true);
		} catch (error) {
			if (!controller.signal.aborted) {
				const missingEndpoint = error instanceof BackendRequestError && error.status === 404 && error.message.trim() === '404 page not found';
				setRevealError(missingEndpoint ? '当前后端尚未加载密钥查看功能，请重新启动应用后再试' : '读取已保存密钥失败，请重试');
			}
		} finally {
			if (!controller.signal.aborted) {
				revealRequest.current = null;
				setLoading(false);
			}
		}
	};
	const page = useContext(SettingsPageContext);
	useEffect(() => { if (page && !page.active) { cancelReveal(); setSavedValue(''); setRevealed(false); setRevealError(''); } }, [page?.active]);
	const hasSavedValue = revealable && status?.configured && !clearing;
	return (
		<label className={`secret-field ${clearing ? 'clearing' : ''}`}>
			<span className="secret-field-heading"><span>{label}{hint && <small>{hint}</small>}</span>{status?.configured && <em>{clearing ? '等待清除' : `已配置 ${status.masked || ''}`}</em>}</span>
			<span className="secret-input-row">
				<KeyRound size={15} />
				<input aria-label={label} type={revealable && revealed ? 'text' : 'password'} autoComplete="new-password" value={value || (revealed ? savedValue : '')} disabled={clearing} onChange={(event) => { cancelReveal(); onChange(secretKey, event.target.value); }} placeholder={hasSavedValue ? '••••••••••••' : status?.configured ? '输入新值可覆盖，留空保持不变' : '输入凭据'} />
				{revealable && <button type="button" className="secret-visibility-button" onClick={() => void toggleRevealed()} disabled={clearing || loading || (!value && !(hasSavedValue && readSavedValue))} title={revealed ? '隐藏 API Key' : '显示 API Key'} aria-label={revealed ? '隐藏 API Key' : '显示 API Key'} aria-pressed={revealed} aria-busy={loading}>{loading ? <LoaderCircle className="spin" size={15} /> : revealed ? <EyeOff size={15} /> : <Eye size={15} />}</button>}
				{status?.configured && <button type="button" className="secret-clear-button" onClick={() => { cancelReveal(); setRevealed(false); onClear(secretKey); }} title={clearing ? '取消清除' : '清除已保存凭据'}><Trash2 size={14} />{clearing ? '撤销' : '清除'}</button>}
			</span>
			{revealError && <small className="model-list-message error" role="alert">{revealError}</small>}
		</label>
	);
}
