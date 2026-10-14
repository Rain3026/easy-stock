import{describe,it,expect}from'vitest';
import{workspaces,parseWorkspaceRoute,workspaceHash}from'./workspaces';
describe('workspace links',()=>{
 it('defines all approved tabs and retains entity context',()=>{expect(workspaces.reduce((n,w)=>n+w.tabs.length,0)).toBe(18);for(const w of workspaces)for(const [tab]of w.tabs){const r=parseWorkspaceRoute(workspaceHash(w.id,tab,new URLSearchParams({symbol:'600176.SH',snapshot:'frozen'})));expect(r.workspace).toBe(w.id);expect(r.tab).toBe(tab);expect(r.params.get('snapshot')).toBe('frozen')}});
 it('keeps old entry points without dropping objects',()=>{expect(parseWorkspaceRoute('#stock-ai?symbol=600176.SH').params.get('symbol')).toBe('600176.SH');expect(parseWorkspaceRoute('#reviews').workspace).toBe('research');expect(parseWorkspaceRoute('#market/core-indexes').tab).toBe('markets');expect(parseWorkspaceRoute('#themes').tab).toBe('themes')});
});
