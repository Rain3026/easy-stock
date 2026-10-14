export function runtimeLabel(id?: string) {
	return ({ hermes: 'Hermes', codex: 'Codex', workbuddy: 'WorkBuddy' } as Record<string, string>)[id || 'hermes'] || id || 'Hermes';
}
