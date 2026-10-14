import {describe,it,expect} from 'vitest';
import {renderToStaticMarkup} from 'react-dom/server';
import type {WidgetInstance} from '../../lib/guanlan';
import type {WidgetData} from '../../lib/guanlan-data';
import type {DailyOverview} from '../../lib/daily-overview';
import {WidgetContent} from './Widgets';

const item=(type:WidgetInstance['type']):WidgetInstance=>({id:'fixture',type,schema:1,layout:{x:0,y:0,w:60,h:20},params:{index_ids:['sse'],fields:['price','change']},style:{title:type,title_size:16,value_size:28,text_size:12,header_height:44,chart_height:220,padding:16,gap:12,columns:0,density:'standard',align:'left',row_height:34,column_width:100,lock_ratio:false}});
const data=():WidgetData=>({overview:{value:null,error:'',loading:false},debates:{value:null,error:'',loading:false},risks:{value:null,error:'',loading:false},quotes:{value:null,error:'',loading:false},refreshing:false,refresh:async()=>{}});
describe('widget evidence and gaps',()=>{
 it('does not invent a zero quote when an unavailable index has a default zero price',()=>{
  const d=data();d.overview.value={indexes:[{id:'sse',price:0,change_percent:0,meta:{source:'fixture'}}]} as unknown as DailyOverview;
  const html=renderToStaticMarkup(<WidgetContent item={item('indexes')} state={null} data={d}/>);
  expect(html).toContain('—');expect(html).not.toContain('0.00%');
 });
 it('keeps empty breadth and partial zero emotion unknown',()=>{
  const d=data();d.overview.value={pulse:{breadth:{up:0,down:0,flat:0},emotion:{up:0,down:0,height:0,boards:0,partial:true}}} as unknown as DailyOverview;
  expect(renderToStaticMarkup(<WidgetContent item={item('breadth')} state={null} data={d}/>)).toContain('暂缺');
  expect(renderToStaticMarkup(<WidgetContent item={item('emotion')} state={null} data={d}/>)).toContain('暂缺');
 });
 it('shows original failed research instead of treating its partial response as a successful conclusion',()=>{
  const d=data();d.debates.value=[{id:'failed',status:'failed',subject:'fixture',error:'invalid evidence',updated_at:'',snapshot:{id:'s',cutoff:'',trade_date:'',gaps:[],version:'v1'},debate:{snapshot_id:'s',subject:'fixture',responses:[{role:'judge',stage:'review',model:'fixture',runtime:'fixture',argument:{thesis:'must not present as final',evidence:[],risks:[],conditions:[],uncertainty:''}}]}}];
  const html=renderToStaticMarkup(<WidgetContent item={item('debate')} state={null} data={d}/>);
  expect(html).toContain('分析失败');expect(html).not.toContain('must not present as final');
 });
 it('retains an incompatible saved widget as a diagnostic placeholder',()=>{
  const old={...item('indexes'),schema:99};expect(renderToStaticMarkup(<WidgetContent item={old} state={null} data={data()}/>)).toContain('不兼容');
 });
});
