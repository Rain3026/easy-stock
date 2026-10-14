import {beforeEach,describe,expect,it,vi} from 'vitest';
import {addInstance,canonical,newInstance,type WidgetDefinition,type WidgetInstance} from './guanlan';
import {sharedRead} from './guanlan-data';
import {requestJSON} from './backend';
import {parseWorkspaceRoute,legacyWorkspaceMode} from './workspaces';

vi.mock('./backend',()=>({requestJSON:vi.fn()}));
const request=vi.mocked(requestJSON);
const fixture=(type:'volume'|'indexes'|'breadth',w:number,h:number):WidgetDefinition=>({id:type,name:type,category:'fixture',description:'test configuration only',source:'fixture',min_w:24,min_h:12,params:[],default:{id:'',type,schema:1,layout:{x:0,y:0,w,h},params:{index_ids:['chinext']},style:{title:type,title_size:16,value_size:28,text_size:12,header_height:44,chart_height:220,padding:16,gap:12,columns:0,density:'standard',align:'left',row_height:34,column_width:120,lock_ratio:false}}});
beforeEach(()=>vi.clearAllMocks());
describe('Guanlan configuration isolation',()=>{
 it('keeps one usage route and management routes without replacing the original home',()=>{
  expect(parseWorkspaceRoute('').workspace).toBe('today');expect(parseWorkspaceRoute('#boards').tab).toBe('versions');expect(parseWorkspaceRoute('#boards/library').tab).toBe('library');expect(parseWorkspaceRoute('#boards/edit?draft=one').params.get('draft')).toBe('one');expect(parseWorkspaceRoute('#boards/unknown').tab).toBe('versions');expect(parseWorkspaceRoute('#guanlan/another').tab).toBe('overview');expect(legacyWorkspaceMode(parseWorkspaceRoute('#guanlan'))).toBe('guanlan');expect(parseWorkspaceRoute('#daily-research').workspace).toBe('today');
 });
 it('copies defaults/presets at creation and keeps repeated instances independent',()=>{
  const d=fixture('indexes',60,16),preset={id:'p',revision:1,name:'preset',item:d.default};const a=newInstance(d,undefined,preset),b=newInstance(d,undefined,preset);a.params.index_ids=['csi300'];a.style.title='changed';expect(b.id).not.toBe(a.id);expect(b.params.index_ids).toEqual(['chinext']);expect(preset.item.style.title).toBe('indexes');expect(d.default.style.title).toBe('indexes');
 });
 it('fills available grid space without changing earlier items or identity',()=>{
  const base={schema:1,name:'layout',snap:true,items:[] as WidgetInstance[]};const header=newInstance(fixture('indexes',120,16));const first=addInstance(base,header);const volume=addInstance(first,newInstance(fixture('volume',72,34)));const breadth=addInstance(volume,newInstance(fixture('breadth',48,22)));expect(base.items).toHaveLength(0);expect(first.items).toHaveLength(1);expect(breadth.items.map(i=>i.layout)).toEqual([{x:0,y:0,w:120,h:16},{x:0,y:16,w:72,h:34},{x:72,y:16,w:48,h:22}]);
  const duplicate=addInstance(breadth,breadth.items[0]);expect(duplicate.items[3].id).not.toBe(breadth.items[0].id);expect(duplicate.items[3].layout.y).toBe(50);
 });
 it('preserves ordered index selection in fingerprints',()=>{
  expect(canonical({b:2,a:1})).toBe(canonical({a:1,b:2}));expect(canonical({ids:['chinext','csi300']})).not.toBe(canonical({ids:['csi300','chinext']}));
 });
});
describe('shared widget reads',()=>{
 it('deduplicates simultaneous reads from repeated widgets, including force refresh',async()=>{
  const config={backendUrl:'http://isolated.test',token:''};let resolve!:(value:unknown)=>void;request.mockReturnValue(new Promise(r=>{resolve=r}));
  const one=sharedRead(config,'/quotes'),two=sharedRead(config,'/quotes'),forced=sharedRead(config,'/quotes',30000,true);expect(request).toHaveBeenCalledTimes(1);resolve({data:[1]});await expect(Promise.all([one,two,forced])).resolves.toEqual([{data:[1]},{data:[1]},{data:[1]}]);await sharedRead(config,'/quotes');expect(request).toHaveBeenCalledTimes(1);
 });
 it('isolates backend configuration and supports retry after a failed read',async()=>{
  const one={backendUrl:'http://one.test',token:'one'},two={backendUrl:'http://two.test',token:'two'};request.mockRejectedValueOnce(new Error('offline')).mockResolvedValue({data:'recovered'});await expect(sharedRead(one,'/quotes')).rejects.toThrow('offline');await sharedRead(one,'/quotes');await sharedRead(two,'/quotes');expect(request).toHaveBeenCalledTimes(3);
 });
});
