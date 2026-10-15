import {afterEach,describe,it,expect,vi} from 'vitest';
import {WidgetRefreshScheduler,refreshSeconds} from './guanlan-refresh';
afterEach(()=>vi.useRealTimers());
describe('per-instance shared refresh scheduler',()=>{
 it('coalesces resources but publishes only instances due at their interval',async()=>{
  vi.useFakeTimers();const read=vi.fn(async(_resource:string,_ids:string[],_force:boolean)=>({}));const s=new WidgetRefreshScheduler([{id:'fast',resource:'overview',seconds:60},{id:'slow',resource:'overview',seconds:120},{id:'off',resource:'debates',seconds:0}],read,()=>true);s.start();await vi.advanceTimersByTimeAsync(0);expect(read.mock.calls).toHaveLength(2);
  await vi.advanceTimersByTimeAsync(60000);expect(read).toHaveBeenLastCalledWith('overview',['fast'],false);
  await vi.advanceTimersByTimeAsync(60000);expect(read).toHaveBeenLastCalledWith('overview',['fast','slow'],false);expect(read.mock.calls.filter(c=>c[0]==='debates')).toHaveLength(1);s.stop();
 });
 it('pauses hidden reads and checks overdue components on return; stop removes timers',async()=>{
  vi.useFakeTimers();let visible=true;const read=vi.fn(async(_resource:string,_ids:string[],_force:boolean)=>({}));const s=new WidgetRefreshScheduler([{id:'a',resource:'overview',seconds:60}],read,()=>visible);s.start();await vi.advanceTimersByTimeAsync(0);visible=false;await vi.advanceTimersByTimeAsync(180000);expect(read).toHaveBeenCalledTimes(1);visible=true;s.resume();await vi.advanceTimersByTimeAsync(0);expect(read).toHaveBeenCalledTimes(2);s.stop();await vi.advanceTimersByTimeAsync(180000);expect(read).toHaveBeenCalledTimes(2);
 });
 it('polls only pending overview, never AI archives, with bounded background checking',async()=>{
  vi.useFakeTimers();const read=vi.fn(async(resource:string)=>({pending:resource==='overview'}));const s=new WidgetRefreshScheduler([{id:'a',resource:'overview',seconds:60},{id:'ai',resource:'debates',seconds:300}],read,()=>true);s.start();await vi.advanceTimersByTimeAsync(10000);expect(read.mock.calls.filter(c=>c[0]==='overview')).toHaveLength(6);expect(read.mock.calls.filter(c=>c[0]==='debates')).toHaveLength(1);s.stop();
 });
 it('backs off failures and never overlaps a pending resource request',async()=>{
  vi.useFakeTimers();const read=vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue({});const s=new WidgetRefreshScheduler([{id:'a',resource:'overview',seconds:60}],read,()=>true);s.start();await vi.advanceTimersByTimeAsync(60000);expect(read).toHaveBeenCalledTimes(1);await vi.advanceTimersByTimeAsync(60000);expect(read).toHaveBeenCalledTimes(2);s.stop();
  let resolve!:(value:{})=>void;const deferred=vi.fn(()=>new Promise<{}>(r=>{resolve=r}));const inFlight=new WidgetRefreshScheduler([{id:'a',resource:'overview',seconds:60}],deferred,()=>true);inFlight.start();const explicit=inFlight.refresh();expect(deferred).toHaveBeenCalledTimes(1);resolve({});await explicit;expect(deferred).toHaveBeenCalledTimes(1);inFlight.stop();
 });
 it('validates compatible defaults and disabled intervals',()=>{expect([undefined,0,30,60,120,86401,61.5].map(refreshSeconds)).toEqual([60,0,60,60,120,60,60])});
});

describe('bounded completion when auto refresh is disabled',()=>{
 it.each([false,true])('finishes first pending read with enabled peer=%s then stops disabled subscriptions',async(mixed)=>{
  vi.useFakeTimers();let pending=true;const read=vi.fn(async(_resource:string,_ids:string[])=>({pending}));const s=new WidgetRefreshScheduler([{id:'off',resource:'overview',seconds:0},...(mixed?[{id:'on',resource:'overview',seconds:60}]:[])],read,()=>true);s.start();await vi.advanceTimersByTimeAsync(2000);expect(read.mock.calls.at(-1)?.[1]).toContain('off');pending=false;await vi.advanceTimersByTimeAsync(2000);const calls=read.mock.calls.length;await vi.advanceTimersByTimeAsync(60000);if(mixed){expect(read.mock.calls.at(-1)?.[1]).toEqual(['on'])}else expect(read).toHaveBeenCalledTimes(calls);s.stop();
 });
 it('bounds never-ending first completion to two minutes even with all auto refresh disabled',async()=>{
  vi.useFakeTimers();const read=vi.fn(async()=>({pending:true})),timeout=vi.fn();const s=new WidgetRefreshScheduler([{id:'off',resource:'overview',seconds:0}],read,()=>true,()=>Date.now(),timeout);s.start();await vi.advanceTimersByTimeAsync(122000);const calls=read.mock.calls.length;expect(calls).toBe(61);expect(timeout).toHaveBeenCalledWith('overview',['off']);await vi.advanceTimersByTimeAsync(300000);expect(read).toHaveBeenCalledTimes(calls);s.stop();
 });
 it('allows explicit refresh completion with all instances disabled',async()=>{
  vi.useFakeTimers();let pending=false;const read=vi.fn(async()=>({pending}));const s=new WidgetRefreshScheduler([{id:'off',resource:'overview',seconds:0}],read,()=>true);s.start();await vi.advanceTimersByTimeAsync(0);pending=true;await s.refresh();await vi.advanceTimersByTimeAsync(2000);expect(read).toHaveBeenCalledTimes(3);pending=false;await vi.advanceTimersByTimeAsync(2000);await vi.advanceTimersByTimeAsync(300000);expect(read).toHaveBeenCalledTimes(4);s.stop();
 });
});
