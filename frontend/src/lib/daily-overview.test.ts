import {describe,it,expect} from 'vitest';
import {judgeMarket,relative,indexTime,type DailyOverview} from './daily-overview';
const sample=():DailyOverview=>({pulse:{version:'test',observation_line:18100,date:'2026-10-12',session:'closed',at:'2026-10-12 15:00:00',fetched_at:'',source:'fixture',history:[],actual:19800,forecast:null,mean20:19200,previous:18000,same_time_change:null,forecast_method:'',gaps:[],stale:false,breadth:{date:'2026-10-12',up:3100,down:1800,flat:100,source:'fixture',at:''},emotion:null},indexes:['sse','szse','chinext'].map(id=>({id,name:id,code:'',market:'CN',price:100,change_percent:1,trade_time:'2026-10-12T15:00:00+08:00',status:'closed',meta:{source:'fixture',stale:false}})),themes:[],theme_meta:{source:'fixture',stale:false}});
describe('market overview judgement',()=>{
 it('uses the server rule result including its version and zero position lower bound',()=>{const d=sample();d.judgement={version:'market-reference-v2',label:'偏空',range:[0,20],tone:'bear',reason:'规则依据',observation_line:18100,calculated_at:'2026-10-12T15:00:00Z'};expect(judgeMarket(d)).toEqual(d.judgement)});
 it('does not calculate a second independent position rule when the server result is absent',()=>{expect(judgeMarket(sample()).range).toBeNull()});
 it('does not divide by zero',()=>{expect(relative(100,0)).toBeNull()});
 it('does not format unknown index times as year one',()=>{const i=sample().indexes![0];i.trade_time='0001-01-01T00:00:00Z';expect(indexTime(i)).toBe('时间未提供')});
});
