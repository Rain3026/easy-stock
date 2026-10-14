export const settingsTabs = ['model', 'skills', 'mcp', 'research', 'data', 'notifications', 'updates', 'logs'] as const;
export type SettingsTab = typeof settingsTabs[number];
export function settingsTabFromHash(hash: string): SettingsTab {
  const tab = hash.split('?')[0].split('/')[1];
  return settingsTabs.includes(tab as SettingsTab) ? tab as SettingsTab : 'model';
}

// /settings supports partial updates. Never submit drafts from an unrelated tab.
export function settingsUpdateForTab<T extends { credentials: unknown; clear_secrets: unknown }>(tab: 'model' | 'data', draft: T) {
  const { credentials, clear_secrets, ...model } = draft;
  return tab === 'data' ? { credentials, clear_secrets } : model;
}

export function agentSettingsUpdateForTab<T extends { skills: unknown; mcp_servers: unknown }>(tab: 'skills' | 'mcp', draft: T) {
  return tab === 'skills' ? { skills: draft.skills } : { mcp_servers: draft.mcp_servers };
}
