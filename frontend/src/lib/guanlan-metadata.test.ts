import {describe,it,expect} from 'vitest';
import {dataTime,widgetMetadata} from './guanlan-metadata';
import type {WidgetData} from './guanlan-data';
import type {WidgetInstance} from './guanlan';
const data={overview:{value:{pulse:{date:'2026-10-15',at:'2026-10-15T10:32:00+08:00',emotion:{date:'2026-10-14',source:'emotion'}},theme_meta:{trade_date:'2026-10-13',source:'themes'},themes:[{theme:'one',trade_date:'2026-10-13'}],indexes:[{id:'sse',trade_time:'2026-10-15T10:32:00+08:00',meta:{source:'A'}},{id:'dow',trade_time:'2026-10-14T05:00:00+08:00',meta:{source:'B',stale:true}}]}},debates:{value:[{id:'pinned',generated_at:'2026-10-14T11:00:00+08:00',snapshot:{cutoff:'2026-10-13T15:00:00+08:00'},status:'succeeded',subject:'test'}]},risks:{value:[]},quotes:{value:[]}} as unknown as WidgetData;
const item=(type:string,params={})=>({type,params}) as WidgetInstance;
describe('real source time metadata',()=>{
 it('never formats date-only or absent timestamps as invented clock times',()=>{expect(dataTime('2026-10-15')).toBe('2026-10-15');expect(dataTime('0001-01-01')).toBe('时间未提供');expect(dataTime('invalid')).toBe('时间未提供')});
 it('uses own dates for emotion/themes instead of volume time',()=>{expect(widgetMetadata(item('emotion'),data,null).label).toBe('数据截至 2026-10-14');expect(widgetMetadata(item('themes'),data,null).label).toBe('数据截至 2026-10-13')});
 it('retains mixed index times, unknown values and stale warnings',()=>{const m=widgetMetadata(item('indexes',{index_ids:['sse','dow','missing']}),data,null);expect(m.label).toBe('各项时间不同');expect(m.entries).toHaveLength(3);expect(m.warnings).toContain('缓存/延迟数据');expect(dataTime(m.entries[2].at)).toBe('时间未提供')});
 it('pins fixed reports and separates generation from evidence cutoff',()=>{const m=widgetMetadata(item('debate',{result_id:'pinned'}),data,null);expect(m.entries[0].at).toBe('2026-10-14T11:00:00+08:00');expect(m.entries[1].at).toBe('2026-10-13T15:00:00+08:00');expect(widgetMetadata(item('debate',{result_id:'absent'}),data,null).label).toContain('时间未提供')});
});
