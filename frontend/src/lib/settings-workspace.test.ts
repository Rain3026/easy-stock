import { describe, expect, it } from 'vitest';
import { agentSettingsUpdateForTab, settingsTabFromHash, settingsUpdateForTab } from './settings-workspace';

describe('settings page save boundaries', () => {
  const combined = {
    agent_runtime: 'workbuddy', llm_profiles: [{ id: 'profile-a', api_key: 'draft-model-key' }],
    llm: { response_timeout_seconds: 300 }, active_llm_profile_id: 'profile-a',
    credentials: { tushare_token: 'draft-data-key' }, clear_secrets: ['eastmoney_cookie'],
  };
  it('does not save or clear data credentials when saving a model draft', () => {
    const result = settingsUpdateForTab('model', combined);
    expect(result).toEqual({ agent_runtime: 'workbuddy', llm_profiles: combined.llm_profiles, llm: combined.llm, active_llm_profile_id: 'profile-a' });
    expect(JSON.stringify(result)).not.toContain('draft-data-key');
  });
  it('does not switch runtimes or save model drafts when saving data credentials', () => {
    const result = settingsUpdateForTab('data', combined);
    expect(result).toEqual({ credentials: combined.credentials, clear_secrets: ['eastmoney_cookie'] });
    expect(JSON.stringify(result)).not.toContain('draft-model-key');
  });
  it('saves skill enablement without sending MCP configuration or secrets', () => {
    const result = agentSettingsUpdateForTab('skills', { skills: [{ name: 'research', enabled: false }], mcp_servers: [{ name: 'private-tool', env: { TOKEN: 'pending' } }] });
    expect(result).toEqual({ skills: [{ name: 'research', enabled: false }] });
  });
  it('allows clearing MCP servers without overwriting pending skill enablement', () => {
    expect(agentSettingsUpdateForTab('mcp', { skills: [{ name: 'research', enabled: false }], mcp_servers: [] })).toEqual({ mcp_servers: [] });
  });
  it('restores direct links for skills, MCP, notification and update entry points', () => {
    for (const tab of ['skills', 'mcp', 'notifications', 'updates']) expect(settingsTabFromHash(`#settings/${tab}`)).toBe(tab);
    expect(settingsTabFromHash('#settings/skills?job=original')).toBe('skills');
    expect(settingsTabFromHash('#settings/unknown')).toBe('model');
    expect(settingsTabFromHash('#settings')).toBe('model');
  });
});
