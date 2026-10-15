// One scheduler per rendered board. Network reads remain shared across consumers.
export type RefreshTarget={id:string;resource:string;seconds:number};
export type RefreshResult={pending?:boolean};
export class WidgetRefreshScheduler {
 private targets: (RefreshTarget&{next:number;failures:number})[];
 private inFlight=new Map<string,Promise<void>>();
 private polls=new Map<string,{ids:string[];at:number;started:number;includeDisabled:boolean}>();
 private timer:ReturnType<typeof setTimeout>|undefined;
 private stopped=false;
 constructor(targets:RefreshTarget[],private read:(resource:string,ids:string[],force:boolean)=>Promise<RefreshResult>,private visible=()=>document.visibilityState==='visible',private now=()=>Date.now(),private onTimeout:(resource:string,ids:string[])=>void=()=>{}){
  this.targets=targets.map(t=>({...t,next:Infinity,failures:0}));
 }
 start(){void this.run(this.targets,false,true)}
 stop(){this.stopped=true;clearTimeout(this.timer);this.polls.clear()}
 async refresh(){await this.run(this.targets,true,true)}
 resume(){if(!this.stopped)void this.tick()}
 private arm(){
  clearTimeout(this.timer);if(this.stopped||!this.visible())return;
  const next=Math.min(...this.targets.map(t=>t.next),...Array.from(this.polls.values(),p=>p.at));
  if(Number.isFinite(next))this.timer=setTimeout(()=>void this.tick(),Math.max(100,next-this.now()));
 }
 private async tick(){
  if(this.stopped||!this.visible()){this.arm();return}
  const due=this.targets.filter(t=>t.seconds>0&&t.next<=this.now());
  for(const [resource,poll] of this.polls){if(poll.at<=this.now())for(const id of poll.ids){const t=this.targets.find(t=>t.id===id&&t.resource===resource);if(t&&!due.includes(t))due.push(t)}}
  await this.run(due,false);
 }
 private async run(targets:typeof this.targets,force:boolean,includeDisabled=false){
  if(this.stopped)return;
  const groups=new Map<string,typeof this.targets>();for(const t of targets){const group=groups.get(t.resource)||[];group.push(t);groups.set(t.resource,group)}
  await Promise.all(Array.from(groups,async([resource,group])=>{
   const old=this.inFlight.get(resource);if(old){await old;return}
   const poll=this.polls.get(resource);this.polls.delete(resource);
   for(const t of group)t.next=Infinity;
   const task=(async()=>{
    try{
     const result=await this.read(resource,group.map(t=>t.id),force);if(this.stopped)return;
     for(const t of group){t.failures=0;t.next=t.seconds>0?this.now()+t.seconds*1000:Infinity}
     const completeInitial=includeDisabled||poll?.includeDisabled||false;
     const ids=group.filter(t=>completeInitial||t.seconds>0).map(t=>t.id);
     if(result.pending&&ids.length){if(this.now()-(poll?.started??this.now())<120000)this.polls.set(resource,{ids,at:this.now()+2000,started:poll?.started??this.now(),includeDisabled:completeInitial});else this.onTimeout(resource,ids)}
    }catch{
     if(this.stopped)return;
     for(const t of group){t.failures++;const base=t.seconds*1000;t.next=t.seconds>0?this.now()+Math.max(base,Math.min(900000,base*2**t.failures)):Infinity}
    }finally{this.inFlight.delete(resource)}
   })();this.inFlight.set(resource,task);await task;
  }));this.arm();
 }
}
export function refreshSeconds(value?:number){return value===0?0:value!==undefined&&Number.isInteger(value)&&value>=60&&value<=86400?value:60}
