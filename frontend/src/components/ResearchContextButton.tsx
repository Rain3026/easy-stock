import {useState} from 'react';
import {type BackendConfig,requestJSON} from '../lib/backend';
export function ResearchContextButton({config,kind,recordID}:{config:BackendConfig|null;kind:'stock'|'portfolio';recordID:string}){
 const[busy,setBusy]=useState(false),[error,setError]=useState('');
 const open=async()=>{if(!config||busy)return;setBusy(true);setError('');try{const p=await requestJSON<{data:{id:string};subject:string}>(config,'/api/v1/research/contexts',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({kind,record_id:recordID})});window.location.hash=`#research/debate?snapshot=${p.data.id}&subject=${encodeURIComponent(p.subject)}`}catch(e){setError((e as Error).message)}finally{setBusy(false)}};
 return <div className="workspace-context"><button type="button" className="workspace-context-link" disabled={!config||busy} onClick={()=>void open()}>{busy?'固定证据中…':'带入多空辨析'}</button><span>使用这份报告的原始证据，进入后再选择是否调用模型。</span>{error&&<span role="alert" className="dr-error">{error}</span>}</div>
}
