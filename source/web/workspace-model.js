'use strict';
const Workspace = (()=>{
  const jobState=j=>j.displayState||(j.finishedAt&&['printing','paused'].includes(j.state)?'printed':j.state);
  const live=['printing','paused','stopped','checking','starting','uploading','assigned','queued'];
  const username=(data,user=data.user)=>data.members.find(m=>m.id===user?.id)?.username||data.members.find(m=>m.name===user?.name)?.username||user?.username||user?.name||'';
  const visible=(spool,data)=>spool.owner==='team'||spool.owner===username(data)||spool.shared;
  const editable=(spool,data)=>spool.owner===username(data)||(spool.owner==='team'&&data.user?.admin);
  const reserved=(data,id,exclude)=>data.jobs.filter(j=>j.id!==exclude&&live.includes(j.state)).reduce((n,j)=>n+(j.printOptions?.mapping?.length?j.printOptions.mapping.filter(m=>m.spoolId===id).reduce((s,m)=>s+m.grams,0):j.spoolId===id?Number(j.grams)||0:0),0);
  const available=(data,spool,exclude)=>Math.max(0,spool.remaining-reserved(data,spool.id,exclude));
  function slots(printer){
    const model=printerModel(printer),combo=model&&(printer.configuration==='ams'||!printer.configuration&&/AMS/i.test(printer.model||''));
    const dual=['h2d','h2d-pro','h2c','x2d'].includes(printer.modelId),out=[{id:'external',label:dual?(globalThis.I18n?.t('外挂料 · 主供料位置')??'外挂料 · 主供料位置'):(globalThis.I18n?.t('外置料架')??'外置料架')},...(dual?[{id:'external:1',label:(globalThis.I18n?.t('外挂料 · 第二供料位置')??'外挂料 · 第二供料位置')}]:[]),...(combo?Array.from({length:model.id==='h2d-pro'?5:4},(_,i)=>({id:String(i+1),label:i===4?(globalThis.I18n?.t('AMS HT · 原登记位置')??'AMS HT · 原登记位置'):model.ams.split(' + ')[0]+(globalThis.I18n?.t(' 槽位 ')??' 槽位 ')+(i+1)})):[])];
    const additions=[...(printer.equipment?.feeds||[]).map(f=>({id:f.slot,label:f.label})),...((typeof data!=='undefined'?data.spools:[])||[]).filter(s=>s.mount?.printerId===printer.id).map(s=>({id:s.mount.slot,label:sourceLabel(s.mount.slot)}))];
    out.forEach(s=>{if(s.id==='5'){s.id='ams:128:0';s.label=(globalThis.I18n?.t('AMS HT · 单元 128')??'AMS HT · 单元 128')}});additions.forEach(s=>{const found=out.find(x=>x.id===s.id);if(!found)out.push(s);else{const feed=printer.equipment?.feeds?.find(f=>f.slot===s.id);if(feed)found.label=feed.label}});return out;
  }
  function sourceLabel(id){const m=/^ams:(\d+):(\d+)$/.exec(id);return m?(globalThis.I18n?.msg||((s,...v)=>s.reduce((r,p,i)=>r+p+(i<v.length?v[i]:''),'')))`AMS 单元 ${m[1]} · 槽 ${+m[2]+1}`:id==='external:1'?(globalThis.I18n?.t('外挂料 · 第二供料位置')??'外挂料 · 第二供料位置'):id}
  function mountLabel(data,spool){
    if(!spool.mount)return (globalThis.I18n?.t('未装载 · 库存中')??'未装载 · 库存中');
    const p=data.printers.find(p=>p.id===spool.mount.printerId);
    if(!p)return (globalThis.I18n?.t('装载设备不存在 · 请重新登记')??'装载设备不存在 · 请重新登记');
    const slot=slots(p).find(s=>s.id===spool.mount.slot);
    return p.name.replace('Bambu Lab ','')+' · '+(slot?.label||(globalThis.I18n?.t('原槽位需重新确认')??'原槽位需重新确认'));
  }
  function mountIssue(data,id,mount){
    const current=data.spools.find(s=>s.id===id)?.mount;
    const same=mount?.printerId===current?.printerId&&mount?.slot===current?.slot;
    const active=id&&data.jobs.find(j=>j.spoolId===id&&j.state==='printing');
    if(active&&(!mount||mount.printerId!==active.printer||current&&!same))return (globalThis.I18n?.t('这卷耗材正在打印，完成任务后才能卸下或转移')??'这卷耗材正在打印，完成任务后才能卸下或转移');
    if(!mount)return '';
    const p=data.printers.find(p=>p.id===mount.printerId);
    if(!p||!slots(p).some(s=>s.id===mount.slot))return (globalThis.I18n?.t('请选择有效的打印机与供料位置')??'请选择有效的打印机与供料位置');
    if(data.spools.some(s=>s.id!==id&&s.mount?.printerId===mount.printerId&&s.mount.slot===mount.slot))return (globalThis.I18n?.t('该位置已登记其他耗材，请先将原记录改为未装载')??'该位置已登记其他耗材，请先将原记录改为未装载');
    return '';
  }
  function configurationIssue(data,id,printer){
    const valid=slots(printer);
    return data.spools.some(s=>s.mount?.printerId===id&&!valid.some(v=>v.id===s.mount.slot))?(globalThis.I18n?.t('该设备还有耗材登记在原供料槽位，请先卸下或调整装载记录，再修改机型配置')??'该设备还有耗材登记在原供料槽位，请先卸下或调整装载记录，再修改机型配置'):'';
  }
  function printMountIssue(data,spool,printerId){
    if(!spool?.mount)return '';
    if(spool.mount.printerId!==printerId)return (globalThis.I18n?.t('耗材装在另一台打印机上，请先转移耗材并更新装载记录')??'耗材装在另一台打印机上，请先转移耗材并更新装载记录');
    const p=data.printers.find(p=>p.id===printerId);
    return !p||!slots(p).some(s=>s.id===spool.mount.slot)?(globalThis.I18n?.t('耗材装载槽位需要重新确认')??'耗材装载槽位需要重新确认'):'';
  }
  function spoolIssue(data,spool,grams=0,exclude){
    if(!spool||!visible(spool,data))return (globalThis.I18n?.t('请选择你有权使用的耗材')??'请选择你有权使用的耗材');
    if(spool.state!=='available')return ({drying:(globalThis.I18n?.t('耗材正在干燥')??'耗材正在干燥'),reserved:(globalThis.I18n?.t('耗材暂不可用')??'耗材暂不可用'),empty:(globalThis.I18n?.t('耗材已用尽')??'耗材已用尽')})[spool.state]||(globalThis.I18n?.t('耗材暂不可用')??'耗材暂不可用');
    if(available(data,spool,exclude)<grams||spool.remaining<=0)return (globalThis.I18n?.t('可分配余量不足（已扣除队列预留）')??'可分配余量不足（已扣除队列预留）');
    return '';
  }
  function migrate(data){
    if(!Array.isArray(data.spools))data.spools=[];
    data.spools.forEach(s=>{if(s.mount===undefined)s.mount=null});
    data.jobs.forEach((j,i)=>{if(!Number.isFinite(j.order))j.order=i+1});
  }
  function minutes(value,fallback=0){
    const h=String(value||'').match(/(\d+)\s*小时/),m=String(value||'').match(/(\d+)\s*分/);
    return h||m?(h?+h[1]*60:0)+(m?+m[1]:0):fallback;
  }
  function queue(data,id){
    const p=data.printers.find(p=>p.id===Number(id));
    const jobs=data.jobs.filter(j=>j.printer===Number(id)&&live.includes(j.state));
    const current=jobs.find(j=>['printing','paused','stopped','checking'].includes(j.state));
    // Transferred jobs reserve the device and cannot be bypassed by a later task.
    const waiting=jobs.filter(j=>!['printing','paused','stopped','checking'].includes(j.state)).sort((a,b)=>(a.state==='assigned'?0:1)-(b.state==='assigned'?0:1)||a.order-b.order);
    const running=!!p&&['printing','pause','confirming'].includes(p.state);
    const now=Number(data.serverTime)?Number(data.serverTime)+(data.receivedAt?Math.max(0,Date.now()-data.receivedAt):0):Date.now(),age=now-(p?.updated||0)*1000,fresh=!!p?.connected&&p.updated>0&&age>=-5000&&age<90000;
    let reason=!p?(globalThis.I18n?.t('等待确认设备')??'等待确认设备'):!p.enabled?(globalThis.I18n?.t('设备已停用')??'设备已停用'):!fresh||p.state==='offline'?(globalThis.I18n?.t('设备离线或状态过期')??'设备离线或状态过期'):p.state==='pause'||current?.state==='paused'?(globalThis.I18n?.t('打印已暂停')??'打印已暂停'):p.state==='confirming'||['stopped','checking'].includes(current?.state)?(globalThis.I18n?.t('等待人工核对')??'等待人工核对'):p.state==='failed'?(globalThis.I18n?.t('设备异常待核对')??'设备异常待核对'):jobs.some(j=>['uploading','starting'].includes(j.state))?(globalThis.I18n?.t('正在传输或确认启动')??'正在传输或确认启动'):current&&p.state==='idle'?(globalThis.I18n?.t('任务与设备状态待核对')??'任务与设备状态待核对'):!['idle','printing'].includes(p.state)?(globalThis.I18n?.t('设备状态待确认')??'设备状态待确认'):'';
    let offset=0;
    if(!reason&&running){const remaining=minutes(p.remaining,-1);if(remaining<=0)reason=(globalThis.I18n?.t('当前剩余时间未知')??'当前剩余时间未知');else offset=Math.max(0,remaining-Math.floor(Math.max(0,now-p.updated*1000)/60000))}
    if(reason)offset=null;
    const rows=waiting.map((job,i)=>{const estimate=Number(job.estimate),known=Number.isFinite(estimate)&&estimate>0;const start=offset;const end=start!==null&&known?start+estimate:null;const row={job,position:i+1,start,end,startAt:start===null?null:now+start*60000,endAt:end===null?null:now+end*60000,reason:reason||(!known?(globalThis.I18n?.t('任务时长未知')??'任务时长未知'):start===null?(globalThis.I18n?.t('前序任务时长未知')??'前序任务时长未知'):'')};offset=end;return row});
    return {printer:p,current,rows,total:offset,paused:p?.state==='pause'||current?.state==='paused',disabled:!p?.enabled,running,reason,now};
  }
  function recommend(data){
    return data.printers.filter(p=>p.enabled&&['idle','printing'].includes(p.state)).map(p=>({printer:p,queue:queue(data,p.id)})).filter(x=>!x.queue.reason&&x.queue.total!==null).sort((a,b)=>a.queue.total-b.queue.total||a.printer.id-b.printer.id)[0];
  }
  function canTransfer(data,job){
    const q=queue(data,job.printer);
    if(!q.printer?.enabled||q.printer.state!=='idle')return (globalThis.I18n?.t('设备尚未空闲')??'设备尚未空闲');
    if(q.rows[0]?.job.id!==job.id)return (globalThis.I18n?.t('请等待前面的任务完成，按单机队列顺序传输')??'请等待前面的任务完成，按单机队列顺序传输');
    if(q.rows.some(r=>r.job.state==='assigned'&&r.job.id!==job.id))return (globalThis.I18n?.t('设备已有就绪任务')??'设备已有就绪任务');
    return '';
  }
  const nextOrder=data=>Math.max(0,...data.jobs.map(j=>Number(j.order)||0))+1;
  return {jobState,username,visible,editable,reserved,available,slots,sourceLabel,mountLabel,mountIssue,configurationIssue,printMountIssue,spoolIssue,migrate,minutes,queue,recommend,canTransfer,nextOrder};
})();
