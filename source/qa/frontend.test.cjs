'use strict';
// npm install --prefix qa/frontend-deps --no-save --no-package-lock jsdom@30.1.3
// node --test qa/frontend.test.cjs
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const {JSDOM}=require('./frontend-deps/node_modules/jsdom');
const root=path.resolve(__dirname,'..');
const read=name=>fs.readFileSync(path.join(root,'web',name),'utf8');
const testLocale={t:v=>v,system:v=>v,language:()=>"zh-CN",locale:()=>"zh-CN",html:(s,...v)=>typeof s==="string"?s:s.map((p,i)=>p+(i<v.length?v[i]:"")).join(""),msg:(s,...v)=>s.map((p,i)=>p+(i<v.length?v[i]:"")).join("")};
async function motionApp(t,{mask=7,member=false}={}){
 const a=await app(t);const p=a.state.snapshot.printers[0];p.modelId='x2d';p.rawState='IDLE';p.state='idle';p.equipment.motion={nativeHome:true,nativeJog:true,homeMask:mask,homeAt:new Date().toISOString(),capabilitiesAt:new Date().toISOString()};a.state.snapshot.jobs=[];a.state.snapshot.user.admin=!member;
 await a.Remote.refresh();a.PrinterControl.open(1);await settle();a.$('[data-control-tab="motion"]').click();return a;
}
test('A1 lamp switch reports last request without claiming static telemetry is hardware confirmation',async t=>{
 const a=await app(t),p=a.state.snapshot.printers[0];p.modelId='a1';a.state.snapshot.jobs=[];p.state='confirming';p.equipment.settings=[{key:'chamber_light',value:-1,min:0,max:1,updated:new Date().toISOString(),feedback:'command_receipt'}];await a.Remote.refresh();a.PrinterControl.open(1);await settle();
 assert.equal(a.$('[data-equipment="chamber_light"]').getAttribute('aria-checked'),'mixed');assert.match(a.$('[data-equipment="chamber_light"]').textContent,/状态未上报/);
 a.$('[data-equipment="chamber_light"]').click();await settle();const post=writes(a);assert.equal(post.length,1);assert.equal(JSON.parse(post[0].options.body).value,1);
 p.equipment.settings[0].value=0;a.state.commands=[{id:88,action:'set:chamber_light',state:'accepted',message:'printer accepted request',user:'Admin',created:Date.now()}];await a.PrinterControl.refresh();assert.match(a.$('[data-equipment="chamber_light"]').textContent,/上次请求：关灯/);assert.doesNotMatch(a.$('[data-equipment="chamber_light"]').textContent,/已关闭/);assert.match(a.$('.control-history').textContent,/设备已接受/);
});

test('missing completed job stays visible across refresh and material reconciliation cannot auto-submit or retry',async t=>{
 const a=await app(t),p=a.state.snapshot.printers[0];a.state.snapshot.jobs=[];p.file='job-16';p.rawState='FINISH';p.state='confirming';p.percent=100;p.completionReview={jobId:16,commandId:83};a.state.snapshot.spools.forEach(s=>{s.owner='team';s.shared=false});await a.Remote.refresh();a.PrinterControl.open(1);await settle();assert.ok(a.$('#settleMissingTask'));assert.match(a.$('.control-filename').textContent,/job-16/);await a.PrinterControl.refresh();assert.ok(a.$('#settleMissingTask'));assert.equal(writes(a).length,0);
 a.$('#settleMissingTask').click();const f=a.$('#orphanSettlement');f.elements.task.value='job-99';f.elements.note.value='checked';f.elements.collected.checked=true;submitForm(a.w,f);await settle();assert.equal(writes(a).length,0);
 f.elements.task.value='job-16';const input=f.querySelector('[data-use]');input.value='12.125';a.state.failWrite=true;submitForm(a.w,f);await settle();assert.equal(writes(a).length,1);const post=writes(a)[0];assert.equal(post.url,'/api/printers/orphans/settle');assert.equal(JSON.parse(post.options.body).uses[input.dataset.use],12.125);submitForm(a.w,f);await settle();assert.equal(writes(a).length,1);assert.match(a.$('#orphanSettleError').textContent,/不要重复提交/);
});

test('A1 controls show decimal temperatures and describe camera source limits',async t=>{
 const a=await app(t),p=a.state.snapshot.printers[0];p.modelId='a1';p.equipment.extruders[0].temperatureExact=78.03;p.equipment.bedExact=52.59;p.equipment.settings=p.equipment.settings.filter(s=>!s.key.startsWith('chamber_light'));await a.Remote.refresh();a.PrinterControl.open(1);await settle();assert.match(a.$('.equipment-panel').textContent,/78.03/);assert.match(a.$('.equipment-panel').textContent,/52.59/);assert.equal(a.$('[data-equipment="chamber_light"]'),null);assert.match(a.$('[data-camera-hint]').textContent,/低帧率/);assert.doesNotMatch(a.$('[data-camera-mode]').textContent,/8 帧|2 帧/);assert.equal(writes(a).length,0);
});
test('orphan recovery requires exact task identity and explicit review without creating jobs',async t=>{
 const a=await app(t),p=a.state.snapshot.printers[0];a.state.snapshot.jobs=[];p.file='job-16';p.rawState='RUNNING';p.state='printing';a.state.commands=[{id:83,jobId:16,action:'start',state:'uncertain',user:'Admin',created:Date.now(),message:'启动结果未确认'}];await a.Remote.refresh();a.PrinterControl.open(1);await settle();a.$('[data-orphan-id="83"]').click();const form=a.$('#orphanReview'),button=a.$('[form="orphanReview"]');assert.equal(button.disabled,true);form.elements.confirmTask.value='job-99';change(a.w,form.elements.confirmed,true);assert.equal(button.disabled,true);form.elements.confirmTask.value='job-16';form.dispatchEvent(new a.w.Event('input',{bubbles:true}));assert.equal(button.disabled,false);submitForm(a.w,form);await settle();assert.equal(writes(a).length,1);const post=writes(a)[0];assert.equal(post.url,'/api/printers/orphans/review');assert.equal(JSON.parse(post.options.body).confirmTask,'job-16');assert.equal(a.state.snapshot.jobs.length,0);
});
test('motion tab preserves selection and clearance on telemetry refresh; one click only',async t=>{
 const a=await motionApp(t),{$,w}=a;assert.equal($('[data-motion-axis]').disabled,true);change(w,$('[data-motion-clear]'),true);assert.equal($('[data-motion-axis]').disabled,false);
 const panel=$('.motion-panel');await a.PrinterControl.refresh();assert.equal($('.motion-panel'),panel);assert.equal(panel.hidden,false);assert.equal($('[data-motion-clear]').checked,true);
 $('[data-motion-axis="X"][data-dir="1"]').click();await settle();const posts=writes(a);assert.equal(posts.length,1);const body=JSON.parse(posts[0].options.body);assert.equal(body.action,'jog');assert.equal(body.step,1);assert.match(body.requestId,/^[\w-]{16,80}$/);assert.equal($('[data-motion-clear]').checked,false);
});
test('motion requires all-axis home, admin access, and a separate home confirmation',async t=>{
 const a=await motionApp(t,{mask:6});change(a.w,a.$('[data-motion-clear]'),true);assert.equal(a.$('[data-motion-axis]').disabled,true);a.$('[data-motion-home]').click();assert.equal(writes(a).length,0);a.$('[data-motion-cancel]').click();assert.equal(writes(a).length,0);
 const member=await motionApp(t,{member:true});change(member.w,member.$('[data-motion-clear]'),true);assert.equal(member.$('[data-motion-axis]').disabled,true);assert.equal(member.$('[data-motion-home]').disabled,true);
});
test('lost motion response blocks resends and restart persists the connection review',async t=>{
 const a=await motionApp(t);change(a.w,a.$('[data-motion-clear]'),true);a.state.failWrite=true;a.$('[data-motion-axis="X"]').click();await settle();assert.equal(writes(a).length,1);assert.ok(a.$('[data-motion-reread]'));assert.equal(a.$('[data-motion-axis]').disabled,true);assert.equal(a.Remote.needsReview(1),true);assert.match(a.w.sessionStorage.getItem('layerhub.pendingReview'),/printers\/motion/);
});
const reply=(status,value)=>({status,ok:status>=200&&status<300,json:async()=>value});
const tick=()=>new Promise(resolve=>setImmediate(resolve));
async function settle(){for(let i=0;i<8;i++)await tick()}

function diagnosticFixture(){return {ok:true,schema:1,version:'0.3.0-alpha',platform:'linux/amd64',checkedAt:Date.now(),mode:'runtime',privateToken:'NEVER-EXPORT-THIS',checks:[{id:'printer',group:'printers',status:'ok',label:'打印机',detail:'已有连接与遥测新鲜；不等于所有控制或打印均获授权。',printerId:1,observedAt:Date.now(),ip:'NEVER-EXPORT-IP',serial:'NEVER-EXPORT-SERIAL'},{id:'data_directory',group:'storage',status:'unknown',label:'数据目录',detail:'目录存在；没有执行写入测试。',fix:'可由服务账户运行 --doctor --probe-write 检查临时文件读写。'}]}}

test('diagnostics separate the gateway and application, export only safe fields and never write',async t=>{
 const a=await app(t),{$,w,state}=a,base=w.fetch;
 w.fetch=(url,o)=>url==='/api/diagnostics'?(state.calls.push({url,options:o}),Promise.resolve(reply(200,diagnosticFixture()))):url==='/gateway-healthz'?(state.calls.push({url,options:o}),Promise.resolve(reply(200,{ok:true,role:'layerhub-gateway',schema:1}))):base(url,o);
 $('[data-page="diagnostics"]').click();await settle();assert.match($('#diagnosticChain').textContent,/NAS 业务服务/);assert.match($('#diagnosticChain').textContent,/同源网关独立健康标记/);
 assert.equal($('#diagnosticExport').disabled,false);const report=w.testing.Diagnostics.exportReport(),raw=JSON.stringify(report);
 assert.equal(report.gatewayMarker,'ok');assert.equal(report.stale,false);assert.doesNotMatch(raw,/NEVER-EXPORT|localhost|csrf/);
 assert.equal(writes(a).length,0);const root=$('#diagnosticsRoot');await stateTick(a,3000);assert.equal($('#diagnosticsRoot'),root);assert.equal(state.calls.filter(c=>c.url==='/api/diagnostics').length,1);
 const now=w.Date.now;w.Date.now=()=>now()+61000;w.testing.Remote.paintConnection();assert.match($('#diagnosticSummary').textContent,/已过期/);assert.equal($('#diagnosticChecks .diagnostic-status.ok'),null);assert.equal(w.testing.Diagnostics.exportReport().stale,true);
});
test('gateway success with NAS failure stays distinct, and failed checks can be exported without raw errors',async t=>{
 const a=await app(t),{$,w}=a,base=w.fetch;
 w.fetch=(url,o)=>url==='/api/diagnostics'?Promise.resolve(reply(503,{ok:false,error:'PRIVATE-ENDPOINT-ERROR'})):url==='/gateway-healthz'?Promise.resolve(reply(200,{ok:true,role:'layerhub-gateway',schema:1})):base(url,o);
 $('[data-page="diagnostics"]').click();await settle();assert.match($('#diagnosticChain').textContent,/云端可达，业务通道未确认/);assert.equal($('#diagnosticExport').disabled,false);
 const raw=JSON.stringify(w.testing.Diagnostics.exportReport());assert.doesNotMatch(raw,/PRIVATE-ENDPOINT/);assert.match(raw,/"failure":true/);assert.match(raw,/"report":null/);assert.equal(writes(a).length,0);
});
test('direct access is not falsely classified as a cloud outage and English checks translate remedies',async t=>{
 const a=await app(t,{language:'en'}),{$,w}=a,base=w.fetch;
 w.fetch=(url,o)=>url==='/api/diagnostics'?Promise.resolve(reply(200,diagnosticFixture())):url==='/gateway-healthz'?Promise.resolve(reply(404,{ok:false})):base(url,o);
 $('[data-page="diagnostics"]').click();await settle();assert.match($('#diagnosticChain').textContent,/direct local connection/);assert.match($('#diagnosticChecks').textContent,/Write access was not tested/);assert.doesNotMatch($('#pageContent').textContent,/[\u3400-\u9fff]/u);
 assert.equal(w.testing.Diagnostics.exportReport().gatewayMarker,'unknown');assert.equal(writes(a).length,0);
});
test('late diagnostic responses cannot replace a different page, and members cannot open diagnostics',async t=>{
 const a=await app(t),{$,w,state}=a,base=w.fetch;let finish;
 w.fetch=(url,o)=>url==='/api/diagnostics'?new Promise(resolve=>{finish=()=>resolve(reply(200,diagnosticFixture()))}):url==='/gateway-healthz'?Promise.resolve(reply(404,{ok:false})):base(url,o);
 $('[data-page="diagnostics"]').click();await settle();$('[data-page="board"]').click();finish();await settle();assert.equal($('#diagnosticsRoot'),null);
 state.snapshot.user.admin=false;state.snapshot.user.owner=false;await a.Remote.refresh();assert.equal($('[data-page="diagnostics"]').hidden,true);$('[data-page="diagnostics"]').click();assert.equal($('#diagnosticsRoot'),null);assert.match($('#toast').textContent,/系统诊断仅对管理员开放/);assert.equal(writes(a).length,0);
});
async function stateTick(a,ms){await a.intervals.find(v=>v.ms===ms).fn();await settle()}

test('diagnostics keep stable DOM between freshness changes and clear immediately when administrator access is lost',async t=>{
 const a=await app(t),{$,w,state}=a,base=w.fetch;
 w.fetch=(url,o)=>url==='/api/diagnostics'?Promise.resolve(reply(200,diagnosticFixture())):url==='/gateway-healthz'?Promise.resolve(reply(404,{ok:false})):base(url,o);
 $('[data-page="diagnostics"]').click();await settle();const row=$('.diagnostic-check');w.testing.Remote.paintConnection();assert.equal($('.diagnostic-check'),row);
 state.snapshot.user.admin=false;state.snapshot.user.owner=false;await a.Remote.refresh({renderPage:false});await settle();assert.equal($('#diagnosticsRoot'),null);assert.equal(w.testing.Diagnostics.exportReport(),null);assert.equal(writes(a).length,0);
});

test('English navigation, account and material pages preserve user-created names',async t=>{
 const a=await app(t,{language:'en'}),{w,$,state}=a;
 assert.equal(w.document.documentElement.lang,'en');assert.equal(w.I18n.locale(),'en-US');
 assert.equal($('[data-page="slicer"]'),null);
 assert.match($('#pageContent').textContent,/Printers and jobs at a glance/);
 state.snapshot.printers[0].name='关闭 <Printer>';state.snapshot.spools[0].label='选择 <Spool>';state.snapshot.members[0].name='模型工作台';state.snapshot.user.name='模型工作台';
 await a.Remote.refresh();assert.match($('#pageContent').textContent,/关闭 <Printer>/);assert.equal($('#pageContent Printer'),null);
 $('[data-page="materials"]').click();assert.match($('#pageContent').textContent,/Every spool has a purpose/);assert.match($('#pageContent').textContent,/选择 <Spool>/);
 $('[data-page="queues"]').click();assert.match($('#pageContent').textContent,/Every printer has its own rhythm/);
 $('[data-page="team"]').click();assert.match($('#pageContent').textContent,/Creating is better together/);assert.match($('#pageContent').textContent,/Workspace owner/);assert.doesNotMatch($('#pageContent').textContent,/空间所有者/);assert.match($('#pageContent').textContent,/模型工作台/);
 $('#profileBtn').click();await settle();assert.match($('#modal').textContent,/Personal account/);assert.ok($('#accountLanguage'));
 assert.equal(writes(a).length,0);
});

test('English printer settings keep physical control keys and numeric payloads unchanged',async t=>{
 const a=await app(t,{language:'en'}),{$,w}=a;a.PrinterControl.open(1);await settle();
 assert.match($('#controlRoot').textContent,/Monitor & control/);assert.match($('#controlRoot').textContent,/Chamber light/);
 $('[data-equipment="fan:1"]').click();assert.match($('#equipmentDialog').textContent,/Fan speed/);
 $('[data-setting-preset="75"]').click();assert.equal(writes(a).length,0);$('#equipmentForm').requestSubmit();await settle();
 assert.deepEqual(JSON.parse(writes(a)[0].options.body),{printerId:1,key:'fan:1',value:75});
 assert.equal(writes(a)[0].options.headers['Accept-Language'],'en');
 assert.equal(w.document.querySelector('#equipmentDialog'),null);
});

test('English preparation retains spool IDs, color codes and per-material mapping',async t=>{
 const a=await app(t,{language:'en'}),{$,w}=a;await a.PrinterControl.options(a.data.jobs.find(j=>j.id===4));await settle();
 assert.match($('#modal').textContent,/Print preparation/);assert.match($('#modal').textContent,/Colors & feed|plate|Plate/);
 const next=$('#prepareNext');if(next)next.click();await settle();
 const candidate=$('input[name="spool-0"][value="white"]');assert.ok(candidate);choose(w,candidate);
 assert.equal(candidate.value,'white');assert.match($('#modal').textContent,/#EEEAE2/i);assert.equal(writes(a).length,0);
});

test('English catalogs keep substitution-like filenames literal and translate formatted system errors',async t=>{
 const a=await app(t,{language:'en'}),{w}=a;
 const name='<script>__lh_expr_1__ $1.gcode</script>';
 assert.equal(w.I18n.msg`对象 ${name}：`,'Object '+name+':');
 assert.equal(w.I18n.system('副本 2 与 3 距离不足，请自动排版或增大间距'),'Copies 2 and 3 are too close. Auto-arrange or increase spacing.');
 assert.equal(w.I18n.system(undefined),undefined);assert.equal(w.I18n.system(42),42);
 const original=w.fetch;w.fetch=(url,options)=>options?.method==='POST'?Promise.resolve(reply(400,{ok:false,error:'请选择有效的打印机'})):original(url,options);
 await assert.rejects(a.Remote.request('/api/jobs/assign',{jobId:4,printerId:0}),/Choose a valid printer/);
});

test('settlement distinguishes exact estimates from reservations, records measured precision, and never sends printer commands',async t=>{
 const a=await app(t),{w,state,$}=a,original=w.fetch;
 state.snapshot.capabilities.settlement=true;
 const job={...state.snapshot.jobs[0],state:'printing',displayState:'printed',finishedAt:123,grams:25};
 w.fetch=async(url,opts={})=>url.startsWith('/api/jobs/settlement?')?reply(200,{ok:true,job,rolls:[{spoolId:'white',label:'白色 PETG',color:'#FFFFFF',reserved:25,available:800,estimate:24.16}]}):original(url,opts);
 await a.Remote.refresh();a.PrinterControl.complete(job);await settle();
 assert.match($('#settlementRoot').textContent,/24.16 g/);assert.match($('#settlementRoot').textContent,/估算结算，不代表称重实测/);
 const form=$('#settlementForm');choose(w,form.querySelector('[value=measured]'));assert.equal(form.elements.grams0.disabled,false);form.elements.grams0.value='23.45';form.elements.grams0.dispatchEvent(new w.Event('input',{bubbles:true}));
 assert.match($('.settlement-booked').textContent,/将扣减 24 g/);
 form.elements.collected.checked=true;form.dispatchEvent(new w.Event('submit',{bubbles:true,cancelable:true}));await settle();
 const writes=state.calls.filter(c=>c.options.method==='POST');assert.equal(writes.length,1);assert.equal(writes[0].url,'/api/jobs/settle');
 assert.deepEqual(JSON.parse(writes[0].options.body),{jobId:job.id,collected:true,uses:{white:{source:'measured',grams:23.45}}});
});

test('lost settlement acknowledgement blocks resubmission and offers a read-only result check',async t=>{
 const a=await app(t),{w,state,$}=a,original=w.fetch;state.snapshot.capabilities.settlement=true;
 const job={...state.snapshot.jobs[0],finishedAt:123,state:'printing'};
 w.fetch=async(url,opts={})=>url.startsWith('/api/jobs/settlement?')?reply(200,{ok:true,job,rolls:[{spoolId:'white',label:'white',color:'#FFFFFF',reserved:25,available:800,estimate:24.16}]}):original(url,opts);
 await a.Remote.refresh();a.PrinterControl.complete(job);await settle();state.failWrite=true;
 const form=$('#settlementForm');form.elements.collected.checked=true;form.dispatchEvent(new w.Event('submit',{bubbles:true,cancelable:true}));await settle();
 assert.equal(form.querySelector('[type=submit]').disabled,true);assert.match($('#settlementError').textContent,/不要重复提交/);
 form.dispatchEvent(new w.Event('submit',{bubbles:true,cancelable:true}));await settle();assert.equal(state.calls.filter(c=>c.url==='/api/jobs/settle').length,1);
 const read=[...form.querySelectorAll('button')].find(b=>b.textContent==='重新读取结算记录');assert.ok(read);read.click();await settle();assert.equal(state.calls.filter(c=>c.options.method==='POST').length,1);
});


function transport(fetch){const context=vm.createContext({I18n:testLocale,fetch,AbortController,setTimeout,clearTimeout});vm.runInContext(read('transport.js')+'\nglobalThis.api=Transport;',context);return context.api}

test('deadline includes response body; GET is safe to read again, POST outcome stays unknown',async()=>{
 for(const payload of [undefined,{action:'resume'}]){
  let calls=0,signal;const api=transport(async(_,options)=>{calls++;signal=options.signal;return {ok:true,status:200,json:()=>new Promise(()=>{})}});
  await assert.rejects(api.request('/test',payload,'csrf',{timeoutMs:15}),e=>e.unavailable&&e.unconfirmed===(payload!==undefined)&&/超时/.test(e.message));
  assert.equal(calls,1);assert.equal(signal.aborted,true);
 }
});
test('lost connections, invalid gateway bodies and 5xx never cause automatic command retries',async()=>{
 for(const fetch of [async()=>{throw Error('lost')},async()=>({ok:false,status:502,json:async()=>{throw Error('html')}}),async()=>reply(500,{ok:false,error:'internal'})]){
  let calls=0;const api=transport((...args)=>{calls++;return fetch(...args)});
  await assert.rejects(api.request('/control',{},'csrf'),e=>e.unconfirmed&&e.unavailable);
  assert.equal(calls,1);
 }
});
test('explicit application 4xx is a definitive rejection; valid success preserves result',async()=>{
 const api=transport(async()=>reply(403,{ok:false,error:'无权限'}));
 await assert.rejects(api.request('/control',{},'csrf'),e=>!e.unconfirmed&&!e.unavailable&&e.message==='无权限');
 const success=transport(async()=>reply(200,{ok:true,commandId:9}));
 assert.equal((await success.request('/control',{},'csrf')).commandId,9);
});

async function roleApp(t,who=1){const a=await app(t);const members=[{id:1,username:'admin',name:'所有者',admin:true,owner:true,role:'空间所有者',color:'blue'},{id:2,username:'manager',name:'管理员甲',admin:true,owner:false,role:'管理员',color:'green'},{id:3,username:'peer',name:'管理员乙',admin:true,owner:false,role:'管理员',color:'blue'},{id:4,username:'maker',name:'普通成员',admin:false,owner:false,role:'成员',color:'blue'}];a.state.snapshot.members=members;a.state.snapshot.user={...members.find(m=>m.id===who)};Object.assign(a.state.snapshot.capabilities,{accountManagement:true,ownerRoles:true,backups:who===1});await a.Remote.refresh({renderPage:false});a.$('[data-page="team"]').click();return a}
test('owner, administrator and member see only their permitted team and backup controls',async t=>{
 const owner=await roleApp(t);assert.ok(owner.$('#dataTools'));assert.ok(owner.$('[data-manage-member="2"]'));owner.$('[data-manage-member="1"]').click();assert.equal(owner.$('#memberForm'),null);owner.$('[data-cancel]').click();owner.$('#addUserBtn').click();assert.ok(owner.$('#userForm [name=admin]'));
 const admin=await roleApp(t,2);assert.equal(admin.$('#dataTools'),null);assert.equal(admin.$('[data-manage-member="1"]'),null);assert.equal(admin.$('[data-manage-member="3"]'),null);assert.ok(admin.$('[data-manage-member="2"]'));assert.ok(admin.$('[data-manage-member="4"]'));admin.$('[data-manage-member="4"]').click();assert.equal(admin.$('#memberForm [name=role]'),null);admin.$('[data-cancel]').click();admin.$('#addUserBtn').click();assert.equal(admin.$('#userForm [name=admin]'),null);
 const member=await roleApp(t,4);assert.equal(member.$('[data-manage-member]'),null);assert.equal(member.$('#addUserBtn'),null);assert.equal(member.$('#dataTools'),null);member.$('#accountBtn').click();assert.match(member.$('#modalContent').textContent,/成员/);assert.equal(member.$('#transferOwnership'),null);assert.equal(writes(owner).length+writes(admin).length+writes(member).length,0);
});
test('only the owner role review sends an explicit changed role with the current password',async t=>{
 const a=await roleApp(t),{w,$}=a;$('[data-manage-member="4"]').click();const form=$('#memberForm');assert.equal(form.elements.role.value,'member');change(w,form.elements.role,'admin');form.elements.currentPassword.value='TestPassword123';submitForm(w,form);await settle();const body=JSON.parse(writes(a)[0].options.body);assert.equal(body.role,'admin');assert.equal(body.userId,4);assert.equal(body.currentPassword,'TestPassword123');assert.equal(writes(a).length,1);
});
test('ownership transfer requires a chosen account, exact username, password and confirmation; cancellation is inert',async t=>{
 const a=await roleApp(t),{w,$}=a;$('#accountBtn').click();$('#transferOwnership').click();const form=$('#ownerTransferForm'),button=$('#ownerTransferSubmit');assert.equal(button.disabled,true);change(w,form.elements.userId,'4');form.elements.confirmUsername.value='wrong';form.elements.currentPassword.value='TestPassword123';change(w,form.elements.confirmed,true);assert.equal(button.disabled,true);form.elements.confirmUsername.value='maker';form.elements.confirmUsername.dispatchEvent(new w.Event('input',{bubbles:true}));assert.equal(button.disabled,false);assert.match($('#ownerTransferTarget').textContent,/@maker/);$('[data-cancel]').click();submitForm(w,form);await settle();assert.equal(writes(a).length,0);
});
test('unknown ownership transfer acknowledgement never automatically submits again',async t=>{
 const a=await roleApp(t),{w,$}=a;$('#accountBtn').click();$('#transferOwnership').click();const form=$('#ownerTransferForm');change(w,form.elements.userId,'4');form.elements.confirmUsername.value='maker';form.elements.currentPassword.value='TestPassword123';change(w,form.elements.confirmed,true);a.state.failWrite=true;submitForm(w,form);await settle();assert.equal($('#ownerTransferSubmit').disabled,true);assert.match($('#ownerTransferError').textContent,/不要重复转让/);submitForm(w,form);await settle();assert.equal(writes(a).length,1);
});
test('member names remain plain text in owner management headings',async t=>{
 const a=await roleApp(t);a.state.snapshot.members.find(m=>m.id===4).name='<img src=x>';await a.Remote.refresh();a.$('[data-manage-member="4"]').click();assert.equal(a.$('#modalContent h2 img'),null);assert.match(a.$('#modalContent h2').textContent,/<img src=x>/);
});
test('owner can cancel a staged restore preview before transferring ownership',async t=>{
 const a=await roleApp(t),{w,$,state}=a,original=w.fetch;let pending=true;w.fetch=async(url,options={})=>{if(url==='/api/backup/status')return reply(200,{ok:true,pending:false,previewReady:pending});if(url==='/api/restore/cancel'){state.calls.push({url,options});pending=false;return reply(200,{ok:true})}return original(url,options)};$('#dataTools').click();await settle();assert.ok($('#cancelRestore'));assert.equal($('#exportBackupForm').hidden,false);assert.match($('#backupState').textContent,/恢复预览/);$('#cancelRestore').click();await settle();assert.equal($('#cancelRestore'),null);assert.equal(writes(a).filter(c=>c.url==='/api/restore/cancel').length,1);
});
async function securityApp(t,who=1){
 const a=await roleApp(t,who),{w,state}=a,original=w.fetch;state.snapshot.capabilities.accountSecurity=true;
 const sessions=[{id:'a'.repeat(64),client:'Chrome · Windows',created:100,expires:500,current:true},{id:'b'.repeat(64),client:'<img src=x>',created:0,expires:600,current:false}];
 w.fetch=async(url,options={})=>{
  if(url==='/api/account/sessions'){state.calls.push({url,options});return reply(200,{ok:true,total:sessions.length,other:sessions.length-1,sessions:structuredClone(sessions)})}
  if(url==='/api/account/sessions/revoke'){state.calls.push({url,options});if(state.failWrite)throw Error('ack lost');const n=sessions.length-1;sessions.splice(1);return reply(200,{ok:true,revoked:n})}
  if(url.startsWith('/api/users/role-events')){state.calls.push({url,options});return reply(200,{ok:true,events:[{id:url.includes('?')?1:51,actor:{id:1,name:'<img src=x>',username:'admin'},target:{id:4,name:'普通成员',username:'maker'},oldRole:'member',newRole:'admin',reason:'role_change',created:1000}],nextCursor:url.includes('?')?0:51})}
  return original(url,options);
 };await a.Remote.refresh({renderPage:false});a.$('#accountBtn').click();a.$('#accountSecurity').click();await settle();return {...a,sessions};
}

test('account sessions are on demand, escape client labels and require explicit password confirmation',async t=>{
 const a=await securityApp(t,4),{w,$,state}=a;assert.equal($('[data-security-tab="audit"]'),null);assert.equal($('#sessionList img'),null);assert.match($('#sessionList').textContent,/未记录/);assert.equal(writes(a).length,0);
 $('[data-revoke-session]').click();const form=$('#sessionRevokeForm');submitForm(w,form);await settle();assert.equal(writes(a).length,0);$('#sessionCancel').click();assert.equal($('#sessionRevokeForm'),null);
 $('#sessionOthers').click();const confirmed=$('#sessionRevokeForm');confirmed.elements.currentPassword.value='MemberPassword123';submitForm(w,confirmed);submitForm(w,confirmed);await settle();
 assert.equal(writes(a).length,1);assert.deepEqual(JSON.parse(writes(a)[0].options.body),{currentPassword:'MemberPassword123',others:true});assert.equal($('#sessionOthers').disabled,true);assert.match($('#sessionSummary').textContent,/1 个有效登录/);assert.equal(state.calls.some(c=>c.url.includes('/printers/')&&c.options.method==='POST'),false);
});

test('individual logout uses only the chosen opaque session ID and cancelled forms cannot submit',async t=>{
 const a=await securityApp(t),{w,$}=a;$('[data-revoke-session]').click();const form=$('#sessionRevokeForm');form.elements.currentPassword.value='TestPassword123';a.Remote.account();submitForm(w,form);await settle();assert.equal(writes(a).length,0);
 $('#accountBtn').click();$('#accountSecurity').click();await settle();$('[data-revoke-session]').click();const next=$('#sessionRevokeForm');next.elements.currentPassword.value='TestPassword123';submitForm(w,next);await settle();assert.deepEqual(JSON.parse(writes(a)[0].options.body),{currentPassword:'TestPassword123',sessionId:'b'.repeat(64)});
});

test('lost logout acknowledgement blocks repeats until the user rereads session state',async t=>{
 const a=await securityApp(t),{w,$,state}=a;$('#sessionOthers').click();const form=$('#sessionRevokeForm');form.elements.currentPassword.value='TestPassword123';state.failWrite=true;submitForm(w,form);await settle();assert.equal(form.elements.currentPassword.value,'');assert.equal(form.querySelector('[type=submit]').disabled,true);assert.equal($('#sessionOthers').disabled,true);assert.match($('#sessionRevokeError').textContent,/刷新列表/);
 submitForm(w,form);await settle();assert.equal(writes(a).length,1);$('#sessionRefresh').click();await settle();assert.equal($('#sessionRevokeForm'),null);assert.equal($('#sessionOthers').disabled,false);assert.equal(writes(a).length,1);
});

test('owner role history loads stable cursor pages, escapes names and never writes',async t=>{
 const a=await securityApp(t),{$}=a;$('[data-security-tab="audit"]').click();await settle();assert.equal($('#roleEventList img'),null);assert.match($('#roleEventList').textContent,/调整角色/);assert.equal($('#auditMore').hidden,false);$('#auditMore').click();await settle();assert.equal($('#roleEventList').children.length,2);assert.equal($('#auditMore').hidden,true);assert.ok(a.state.calls.some(c=>c.url==='/api/users/role-events?before=51'));assert.equal(writes(a).length,0);
});

test('late security reads cannot replace another dialog or another account content',async t=>{
 const a=await securityApp(t),{w,$}=a;let resolve;w.fetch=()=>new Promise(r=>resolve=r);$('#sessionRefresh').click();a.Remote.account();resolve(reply(200,{ok:true,sessions:[],total:0,other:0}));await settle();assert.match($('#modalContent').textContent,/个人账户/);assert.equal($('#sessionList'),null);assert.equal($('#modal').classList.contains('account-security-modal'),false);
});

function fixture(){
 const printer={id:1,name:'测试 P1S',modelId:'p1s',configuration:'ams',model:'P1S',ip:'192.0.2.1',serial:'TEST',enabled:true,connected:true,state:'pause',percent:50,updated:Math.floor(Date.now()/1000),nozzle:210,bed:50,remaining:'20分钟',layer:10,layers:100,file:'current.3mf',material:'PLA',equipment:{extruders:[{name:'喷嘴',temperature:210,target:210}],bed:50,bedTarget:50,settings:[{key:'chamber_light',label:'机舱灯',value:0,unit:'',updated:new Date().toISOString()},{key:'fan:1',label:'模型风扇',value:0,unit:'%',min:0,max:100,updated:new Date().toISOString()},{key:'bed',label:'热床温度',value:50,unit:'°C',min:0,max:100,updated:new Date().toISOString()}],feeds:[]}};
 const spool=(id,colorName,color,slot,remaining=800,material='PLA')=>({id,label:colorName+'测试卷',brand:'测试',material,colorName,color,mount:{printerId:1,slot},state:'available',owner:'team',shared:true,capacity:1000,remaining,location:'测试架'});
 return {ok:true,csrf:'test-csrf',user:{id:1,name:'测试员',username:'admin',admin:true,owner:true},printers:[printer],jobs:[{id:1,filename:'current.3mf',user:'测试员',userId:1,state:'paused',printer:1,created:'今天',grams:20,spoolId:'white',estimate:45},{id:4,filename:'color-demo.3mf',user:'测试员',userId:1,state:'queued',printer:1,created:'今天',grams:19,spoolId:'white',estimate:86}],spools:[spool('white','白色','#EEEAE2','1'),spool('blue','蓝色','#4778BC','2'),spool('short','红色','#CC3333','3',5),spool('petg','绿色','#33AA33','4',800,'PETG'),spool('external','黑色','#222222','external')],members:[{id:1,name:'测试员',username:'admin',admin:true,owner:true,role:'空间所有者',color:'blue'}],completed:0,notifications:[],occupiedSlots:[],capabilities:{deviceMode:'simulated',printStart:true,printerControl:true,projectPrint:true}};
}
const plate=(id,color,grams=19)=>({id,filaments:[{index:0,material:'PLA',color,grams}],seconds:5160,thumbnail:false});
async function app(t,{pendingReview,sidebarPreference,appearancePreference,viewer,language='zh-CN'}={}){
 const dom=new JSDOM(read('index.html'),{url:'http://localhost:5181/',runScripts:'outside-only'}),w=dom.window;
 w.localStorage.setItem('layerhub.language',language);
 t.after(()=>w.close());
 w.HTMLDialogElement.prototype.showModal=function(){this.setAttribute('open','')};
 w.HTMLDialogElement.prototype.close=function(){this.removeAttribute('open')};
 w.scrollTo=()=>{};w.HTMLElement.prototype.scrollIntoView=()=>{};
 w.HTMLElement.prototype.animate=()=>({});
 const intervals=[];w.setInterval=(fn,ms)=>{intervals.push({fn,ms});return intervals.length};w.clearInterval=()=>{};
 w.AbortController=AbortController;
 if(pendingReview)w.sessionStorage.setItem('layerhub.pendingReview',pendingReview);
 if(sidebarPreference!==undefined)w.localStorage.setItem('layerhub.ui.sidebarCollapsed',sidebarPreference);
 if(appearancePreference!==undefined)w.localStorage.setItem('layerhub.ui.appearance.user.1',appearancePreference);
 const state={snapshot:fixture(),slice:{model:'P1S',plates:[plate(1,'#EEEAE2'),plate(2,'#4778BC',12)]},commands:[],calls:[],failWorkspace:false,failWrite:false};
 w.fetch=async(url,options={})=>{
  state.calls.push({url,options});
  if(url==='/api/workspace'){if(state.failWorkspace)throw Error('NAS unavailable');return reply(200,structuredClone(state.snapshot))}
  if(url.startsWith('/api/printers/commands'))return reply(200,{ok:true,commands:structuredClone(state.commands)});
  if(url.startsWith('/api/jobs/slice'))return reply(200,{ok:true,slice:structuredClone(state.slice)});
  if(viewer&&url.startsWith('/api/slicing/model?'))return {ok:true,arrayBuffer:async()=>new ArrayBuffer(84)};
  if(options.method==='POST'){if(state.failWrite)throw Error('ack lost');return reply(200,{ok:true})}
  throw Error('Unexpected request '+url);
 };
 const scripts=[...w.document.querySelectorAll('script[src]')].map(s=>read(s.getAttribute('src').replace('./','')));
 const source=scripts.join('\n');
 w.eval(source+'\nwindow.testing={UploadPreparation,PrintFlow,BatchQueue,FileIntake,Diagnostics,Remote,PrinterControl,EquipmentUI,Finish,EmailUI,TeamTools,Workspace,get data(){return data}};');
 await settle();assert.equal(w.testing.Remote.ready(),true);
 return {w,...w.testing,state,intervals,$:selector=>w.document.querySelector(selector)};
}
function change(w,el,value){if(el.type==='checkbox')el.checked=value;else el.value=value;el.dispatchEvent(new w.Event('change',{bubbles:true}))}
function choose(w,el){el.checked=true;el.dispatchEvent(new w.Event('change',{bubbles:true}))}

async function unifiedPreparation(t,{multi=false,busy=false,language='zh-CN'}={}){
 const a=await betaIntakeApp(t,{language}),{state,w}=a;
 a.review.sliced.info.model='P1S';a.review.sliced.info.plates=[plate(1,'#EEEAE2',10),...(multi?[plate(7,'#4778BC',12)]:[])];
 state.snapshot.jobs=[];state.snapshot.printers[0].state=busy?'printing':'idle';state.snapshot.printers[0].rawState=busy?'RUNNING':'IDLE';await a.Remote.refresh();
 const base=w.fetch;w.fetch=async(url,o={})=>{
  if(url==='/api/upload/batch'){
   state.calls.push({url,options:o});const batch=JSON.parse(o.body.get('batch'));
   const jobs=batch.items.map((item,n)=>({id:21+n,user:state.snapshot.user.name,userId:1,filename:a.file.name,size:0.1,state:'queued',order:n+1,created:'今天',selectedPlate:item.plate,...item}));state.snapshot.jobs.push(...jobs);
   return reply(200,{ok:true,receipt:{requestId:batch.requestId,jobIds:jobs.map(j=>j.id)}});
  }
  if(url==='/api/jobs/options'){const b=JSON.parse(o.body);state.snapshot.jobs.find(j=>j.id===b.jobId).printOptions=b.options}
  if(url==='/api/jobs/dispatch'){const b=JSON.parse(o.body);state.snapshot.jobs.find(j=>j.id===b.jobId).state='uploading'}
  return base(url,o);
 };
 await a.FileIntake.open(a.file);return a;
}

test('unified preparation cancel exits to queues without creating jobs or reserving stock',async t=>{
 const a=await unifiedPreparation(t),before=structuredClone(a.state.snapshot.spools);assert.ok(a.$('#uploadPreparation'));a.$('#upCancel').click();assert.equal(a.$('#modal').open,false);assert.equal(a.w.location.hash,'#queues');assert.equal(a.state.snapshot.jobs.length,0);assert.deepEqual(a.state.snapshot.spools,before);assert.deepEqual(writes(a).map(c=>c.url),['/api/files/inspect']);
});

test('unified one confirmation creates once, saves exact material mapping, and dispatches once',async t=>{
 const a=await unifiedPreparation(t);assert.equal(a.$('#upStart').disabled,true);change(a.w,a.$('#upClear'),true);assert.equal(a.$('#upStart').disabled,false,JSON.stringify({checks:a.$('#upChecks').textContent,note:a.$('#upPrinterNote').textContent,printer:a.w.testing.data.printers[0],blocked:a.Remote.deviceBlocked(1)}));a.$('#upStart').click();a.$('#upStart').click();await settle();
 assert.deepEqual(writes(a).map(c=>c.url),['/api/files/inspect','/api/upload/batch','/api/jobs/options','/api/jobs/dispatch']);const opts=JSON.parse(writes(a)[2].options.body).options;assert.equal(opts.plate,1);assert.equal(opts.mapping[0].spoolId,'white');assert.equal(opts.mapping[0].grams,10);assert.equal(opts.bedLeveling,true);assert.equal(a.state.snapshot.jobs.length,1);assert.equal(a.w.sessionStorage.getItem('layerhub.pendingImport.1'),null);
});

test('unified multi-plate enqueue keeps actual plate IDs and saves each preparation without starting',async t=>{
 const a=await unifiedPreparation(t,{multi:true});assert.equal(a.$('#upStart').disabled,true);a.$('#upAll').click();assert.match(a.$('#upTotal').textContent,/2 盘.*22 g/);a.$('#upStart').click();await settle();
 const calls=writes(a),batch=JSON.parse(calls.find(c=>c.url==='/api/upload/batch').options.body.get('batch'));assert.deepEqual(batch.items.map(i=>[i.plate,i.grams,i.spoolId]),[[1,10,'white'],[7,12,'blue']]);assert.equal(calls.filter(c=>c.url==='/api/jobs/options').length,2);assert.equal(calls.some(c=>/dispatch|\/start/.test(c.url)),false);assert.equal(a.w.location.hash,'#queues');
});

test('unified busy printer can only queue, while missing material blocks submission',async t=>{
 const a=await unifiedPreparation(t,{busy:true});assert.equal(a.$('#upClearRow').hidden,true);assert.match(a.$('#upStart').textContent,/加入队列/);a.$('#upStart').click();await settle();assert.equal(writes(a).some(c=>c.url==='/api/jobs/dispatch'),false);
 const b=await unifiedPreparation(t);b.state.snapshot.spools.forEach(s=>s.mount=null);await b.Remote.refresh({renderPage:false});b.UploadPreparation.open(b.file);assert.equal(b.$('#upStart').disabled,true);assert.match(b.$('#upMaterials').textContent,/没有匹配/);
});

test('unified lost import reply survives leaving and rereads the original receipt without dispatch',async t=>{
 const a=await unifiedPreparation(t),base=a.w.fetch;let batch;
 a.w.fetch=async(url,o)=>{if(url==='/api/upload/batch'){a.state.calls.push({url,options:o});batch=JSON.parse(o.body.get('batch'));throw Error('lost reply')}if(url.startsWith('/api/jobs/import-receipt'))return reply(200,{ok:true,found:true,receipt:{requestId:batch.requestId,jobIds:[21]}});return base(url,o)};
 change(a.w,a.$('#upClear'),true);a.$('#upStart').click();await settle();assert.equal(a.$('#upRead').hidden,false);assert.equal(a.$('#upStart').disabled,true);a.$('#upCancel').click();assert.equal(a.w.location.hash,'#queues');
 await a.FileIntake.open(a.file);assert.equal(a.$('#upStart').disabled,true);a.$('#upRead').click();await settle();assert.equal(a.w.sessionStorage.getItem('layerhub.pendingImport.1'),null);assert.equal(writes(a).filter(c=>c.url==='/api/upload/batch').length,1);assert.equal(writes(a).some(c=>c.url==='/api/jobs/dispatch'),false);
});

test('unified lost dispatch reply never repeats dispatch when the receipt is reread',async t=>{
 const a=await unifiedPreparation(t),base=a.w.fetch;
 a.w.fetch=async(url,o)=>{if(url==='/api/jobs/dispatch'){a.state.calls.push({url,options:o});throw Error('lost start reply')}if(url.startsWith('/api/jobs/import-receipt'))return reply(200,{ok:true,found:true,receipt:{jobIds:[21]}});return base(url,o)};
 change(a.w,a.$('#upClear'),true);a.$('#upStart').click();await settle();assert.equal(a.$('#upRead').hidden,false);a.$('#upRead').click();await settle();assert.equal(writes(a).filter(c=>c.url==='/api/jobs/dispatch').length,1);assert.equal(a.w.location.hash,'#queues');
});

test('unified English preparation translates controls and never changes the original filename',async t=>{
 const a=await unifiedPreparation(t,{language:'en'});assert.match(a.$('#upStart').textContent,/Confirm and start/);const copy=a.$('#uploadPreparation').cloneNode(true);copy.querySelectorAll('.up-spool,#upPrinter,#upPrinterNote').forEach(n=>n.remove());assert.doesNotMatch(copy.textContent,/[\u3400-\u9fff]/u);assert.match(a.$('.up-file').textContent,/<img src=x>/);assert.equal(a.$('.up-file img'),null);a.$('#upCancel').click();assert.equal(a.w.location.hash,'#queues');
});


function submitForm(w,form){form.dispatchEvent(new w.Event('submit',{bubbles:true,cancelable:true}))}
function numberInput(w,el,value){el.value=String(value);el.dispatchEvent(new w.Event('input',{bubbles:true}));el.dispatchEvent(new w.Event('change',{bubbles:true}))}

function batchState(api,{size=[100,100,10],triangles=12,scale=1,rotation=[0,0,0]}={}){const options={x:138.25,y:128,scale,rotation,layer:.2,infill:20,material:'petg-basic',support:false,brim:false,color:'#FFFFFF',advanced:{wallLoops:3}},plate=api.plates.make('打印板 01',options);plate.modelId=5;plate.taskId=77;plate.options.copies=[{modelId:5,x:138.25,y:128,scale,rotation}];return {state:{plates:[plate],activePlate:0,activeCopy:0},models:[{id:5,filename:'bracket.stl',triangles,bounds:{size}}]}}

function selectedButtons(a){return [...a.w.document.querySelectorAll('[data-copy][aria-pressed="true"]')].map(b=>Number(b.dataset.copy))}
function pickObject(a,index,options={}){a.$('[data-copy="'+index+'"]').dispatchEvent(new a.w.MouseEvent('click',{bubbles:true,...options}))}

function objectSetting(a,key,mode,value){change(a.w,a.$('[data-object-mode="'+key+'"]'),mode);if(value!==undefined)numberInput(a.w,a.$('[data-object-value="'+key+'"]'),value)}
function applyObjectSettings(a){submitForm(a.w,a.$('#objectProcessForm'))}

function setModelSize(a,percent){a.$('#sliceResize').click();numberInput(a.w,a.$('#modelSizePercent'),percent);submitForm(a.w,a.$('#modelSizeForm'))}

function cameraViewer(){return {camera:{position:[425,-300,340],target:[128,128,16],zoom:1},views:[],loads:0,set(){},loadModels(){this.loads++},view(name){this.views.push(name);this.camera={position:[100,-100,90],target:[10,10,5],zoom:1}},finish(){},dispose(){},editing(){},clearPaths(){},paths(){},tool(){}}}
function historyKey(w,key){w.document.body.dispatchEvent(new w.KeyboardEvent('keydown',{key,ctrlKey:true,bubbles:true,cancelable:true}))}

const writes=a=>a.state.calls.filter(c=>c.options.method==='POST');

test('board keeps upload secondary, opens on demand and preserves draft inputs across filtering and refresh',async t=>{
 const a=await app(t),{w,$}=a;
 assert.equal($('#boardUpload').open,false);assert.equal($('.board-home .lower-grid'),null);assert.ok($('.board-device'));assert.equal(writes(a).length,0);
 $('[data-new-task]').click();assert.equal($('#boardUpload').open,true);$('#note').value='<draft note>';$('#note').dispatchEvent(new w.Event('input',{bubbles:true}));$('#pickPrinter').value='1';$('#pickPrinter').dispatchEvent(new w.Event('change',{bubbles:true}));
 $('[data-printer-filter="idle"]').click();assert.equal($('.board-device'),null);assert.equal($('#note').value,'<draft note>');assert.equal($('#pickPrinter').value,'1');assert.equal($('#boardUpload').open,true);
 await a.Remote.refresh();assert.equal($('#note').value,'<draft note>');assert.equal($('#pickPrinter').value,'1');assert.equal($('#boardUpload').open,true);assert.equal(writes(a).length,0);
 $('[data-printer-filter="all"]').click();$('[data-queue-printer="1"]').click();assert.equal(w.location.hash,'#queues');assert.ok($('.device-queue-panel'));assert.equal(writes(a).length,0);
});

test('board shows confirmation and warnings, isolates form state by account and never completes a print implicitly',async t=>{
 const a=await app(t),{w,$}=a,p=a.state.snapshot.printers[0];p.state='confirming';p.percent=100;p.warning='<img src=x onerror=alert(1)>';await a.Remote.refresh();
 assert.match($('.board-device').textContent,/打印完成 · 待登记/);assert.match($('.board-warning').textContent,/<img src=x/);assert.equal($('.board-warning img'),null);assert.ok($('.board-device [data-action="done"]'));assert.equal(writes(a).length,0);
 $('[data-new-task]').click();$('#note').value='private draft';$('#note').dispatchEvent(new w.Event('input',{bubbles:true}));a.state.snapshot.user={id:2,name:'另一位',username:'second',admin:false};await a.Remote.refresh();assert.equal($('#note').value,'');assert.equal($('#boardUpload').open,false);assert.equal($('.board-device [data-action="done"]'),null);assert.equal(writes(a).length,0);
});

test('mobile drawer keeps the desktop preference, closes with Escape and updates accessibility state',async t=>{
 const a=await app(t,{sidebarPreference:'1'}),{w,$}=a;
 w.innerWidth=390;w.dispatchEvent(new w.Event('resize'));assert.equal($('#menuBtn').getAttribute('aria-label'),'打开导航');$('#menuBtn svg path').dispatchEvent(new w.MouseEvent('click',{bubbles:true,composed:true}));assert.equal($('#sidebar').classList.contains('open'),true);assert.equal($('#menuBtn').getAttribute('aria-expanded'),'true');assert.equal(w.localStorage.getItem('layerhub.ui.sidebarCollapsed'),'1');
 w.document.body.dispatchEvent(new w.KeyboardEvent('keydown',{key:'Escape',bubbles:true}));assert.equal($('#sidebar').classList.contains('open'),false);assert.equal($('#menuBtn').getAttribute('aria-expanded'),'false');
 $('#menuBtn').click();w.document.body.click();assert.equal($('#sidebar').classList.contains('open'),false);assert.equal($('#menuBtn').getAttribute('aria-expanded'),'false');
 w.innerWidth=1280;w.dispatchEvent(new w.Event('resize'));assert.equal($('#menuBtn').getAttribute('aria-label'),'展开侧栏');assert.equal(writes(a).length,0);
});

test('schedule color cards show material, color code, stock and mount; search preserves an explicit choice',async t=>{
 const a=await app(t);a.state.snapshot.spools.push({...a.state.snapshot.spools[0],id:'private',owner:'someone-else',shared:false,label:'PRIVATE-DO-NOT-SHOW'});await a.Remote.refresh({renderPage:false});
 a.Remote.scheduleJob(a.data.jobs.find(j=>j.id===4));
 assert.equal(a.$('input[name="spoolId"][value="private"]'),null);
 assert.match(a.$('.schedule-spool-card').textContent,/白色.*PLA/);assert.match(a.$('.schedule-spool-card').textContent,/#EEEAE2/);assert.match(a.$('.schedule-spool-card').textContent,/可分配/);assert.match(a.$('.schedule-spool-card').textContent,/AMS/);
 assert.ok(a.$('.schedule-color-chip').style.background);assert.equal(a.$('input[name="spoolId"][value="short"]').disabled,true);
 choose(a.w,a.$('input[name="spoolId"][value="blue"]'));
 const search=a.$('[data-spool-search]');search.value='PETG';search.dispatchEvent(new a.w.Event('input',{bubbles:true}));
 assert.equal(new a.w.FormData(a.$('#scheduleForm')).get('spoolId'),'blue');assert.match(a.$('[data-spool-summary]').textContent,/蓝色/);
 a.$('#scheduleForm').requestSubmit();await settle();assert.equal(writes(a).length,1);assert.equal(JSON.parse(writes(a)[0].options.body).spoolId,'blue');
});

test('a color card selected before a quantity increase cannot reserve insufficient stock',async t=>{
 const a=await app(t);a.Remote.scheduleJob(a.data.jobs.find(j=>j.id===4));choose(a.w,a.$('input[name="spoolId"][value="blue"]'));
 const grams=a.$('#jobGrams');grams.value='900';grams.dispatchEvent(new a.w.Event('input',{bubbles:true}));
 assert.equal(a.$('input[name="spoolId"][value="blue"]').disabled,true);assert.match(a.$('[data-spool-summary]').textContent,/余量不足/);
 a.$('#scheduleForm').requestSubmit();await settle();assert.equal(writes(a).length,0);assert.ok(a.$('#scheduleError').textContent);
});

test('no matching registered material explains telemetry mismatch and offers the library rather than substituting PETG',async t=>{
 const a=await app(t);a.state.snapshot.spools.forEach(s=>s.material='PETG');a.state.snapshot.printers[0].equipment.feeds=[{material:'PETG',present:true,slot:'1'}];await a.Remote.refresh({renderPage:false});
 await a.PrinterControl.options(a.data.jobs.find(j=>j.id===4));a.$('[data-prepare-step="1"]').click();
 assert.match(a.$('.project-color-empty').textContent,/PLA/);assert.match(a.$('.project-color-empty').textContent,/PETG/);assert.match(a.$('.project-color-empty').textContent,/重新切片/);assert.ok(a.$('#prepareMaterials'));assert.equal(a.$('input[name="spool-0"]'),null);
 a.$('[data-prepare-step="2"]').click();assert.equal(a.$('#prepareSave').disabled,true);assert.equal(writes(a).length,0);
});

test('preparation rereads device and stock without discarding color choices or writing',async t=>{
 const a=await app(t);await a.PrinterControl.options(a.data.jobs.find(j=>j.id===4));choose(a.w,a.$('input[name="spool-0"][value="white"]'));
 a.$('[data-prepare-step="2"]').click();a.state.snapshot.spools.find(s=>s.id==='white').remaining=1;
 a.$('#prepareRecheck').click();await settle();
 assert.equal(a.$('input[name="spool-0"][value="white"]').checked,true);assert.match(a.$('#prepareChecklist').textContent,/余量不足/);assert.equal(a.$('#prepareSave').disabled,true);assert.equal(writes(a).length,0);
});

test('adapted X2D single-nozzle preparation explains the physical route and keeps unverified options off',async t=>{
 const a=await app(t);Object.assign(a.state.snapshot.printers[0],{modelId:'x2d',model:'X2D',state:'idle'});a.state.snapshot.jobs=a.state.snapshot.jobs.filter(j=>j.id===4);a.state.snapshot.capabilities.x2dSingleNozzlePrint=true;a.state.snapshot.capabilities.printStart=false;await a.Remote.refresh({renderPage:false});
 a.state.slice.model='Bambu Lab X2D';a.state.slice.plates[0].nozzle={logical:0,physical:1,diameter:'0.4',volume:'Standard'};
 await a.PrinterControl.options(a.data.jobs.find(j=>j.id===4));choose(a.w,a.$('input[name="spool-0"][value="white"]'));a.$('[data-prepare-step="2"]').click();
 assert.equal(a.$('input[name="flowCalibration"]').disabled,true);assert.equal(a.$('input[name="timelapse"]').disabled,true);assert.equal(a.$('#prepareSave').disabled,false);assert.match(a.$('#prepareChecklist').textContent,/X2D 单喷嘴打印验证/);assert.doesNotMatch(a.$('#prepareChecklist').textContent,/打印启动映射尚未开放/);
 a.PrinterControl.open(1);await settle();assert.equal(a.$('#controlSend').disabled,false);assert.equal(writes(a).length,0);
});

test('editing a draft during service loss cannot re-enable apply or send a setting',async t=>{
 const a=await app(t);a.PrinterControl.open(1);await settle();a.$('[data-equipment="fan:1"]').click();
 a.state.failWorkspace=true;await assert.rejects(a.Remote.refresh({renderPage:false}));
 a.$('[data-setting-preset="50"]').click();assert.equal(a.$('#applyEquipment').disabled,true);
 a.$('#equipmentForm').dispatchEvent(new a.w.Event('submit',{cancelable:true}));await settle();assert.equal(writes(a).length,0);
});

test('light switch submits once, waits for device state and never opens a settings dialog',async t=>{
 const a=await app(t);a.PrinterControl.open(1);await settle();
 let finish,calls=0;const original=a.w.fetch;
 a.w.fetch=(url,options)=>url==='/api/printers/settings'&&options.method==='POST'?(calls++,new Promise(resolve=>finish=()=>resolve(reply(200,{ok:true})))):original(url,options);
 const button=a.$('[data-equipment="chamber_light"]');assert.equal(button.getAttribute('role'),'switch');assert.equal(button.getAttribute('aria-checked'),'false');
 button.click();button.click();await settle();assert.equal(calls,1);assert.equal(a.$('#equipmentForm'),null);
 assert.equal(a.$('[data-equipment="chamber_light"]').getAttribute('aria-checked'),'false');assert.equal(a.$('[data-equipment="chamber_light"]').disabled,true);
 a.state.snapshot.printers[0].equipment.settings.find(s=>s.key==='chamber_light').value=1;finish();await settle();
 assert.equal(a.$('[data-equipment="chamber_light"]').getAttribute('aria-checked'),'true');assert.equal(calls,1);
});

test('control reads start concurrently and an unchanged refresh preserves controls, tabs and focus',async t=>{
 const a=await app(t);a.PrinterControl.open(1);await settle();
 const root=a.$('.control-shell'),tabs=a.$('.control-tabs'),light=a.$('[data-equipment="chamber_light"]'),history=a.$('.control-history');
 light.focus();const base=a.w.fetch;let release,commandsStarted=false;
 a.w.fetch=(url,o)=>url==='/api/workspace'?new Promise(resolve=>release=()=>resolve(reply(200,structuredClone(a.state.snapshot)))):url.startsWith('/api/printers/commands')?(commandsStarted=true,base(url,o)):base(url,o);
 const refreshing=a.PrinterControl.refresh();await settle();
 assert.equal(commandsStarted,true,'command read must not wait for the workspace response');
 assert.equal(a.$('.control-shell'),root);release();await refreshing;
 assert.equal(a.$('.control-tabs'),tabs);assert.equal(a.$('.control-history'),history);assert.equal(a.$('[data-equipment="chamber_light"]'),light);assert.equal(a.w.document.activeElement,light);
 assert.equal(writes(a).length,0);
});

test('light feedback is immediate, fast polling reads only, and device confirmation restores the same switch',async t=>{
 const a=await app(t,{language:'en'});a.PrinterControl.open(1);await settle();
 const base=a.w.fetch,timers=[];const timeout=a.w.setTimeout.bind(a.w);
 a.w.setTimeout=(fn,ms,...args)=>ms===350?(timers.push(fn),900000+timers.length):timeout(fn,ms,...args);
 let release,writesSent=0;
 a.w.fetch=(url,o)=>url==='/api/printers/settings'?(writesSent++,new Promise(resolve=>release=()=>resolve(reply(200,{ok:true})))):base(url,o);
 const light=a.$('[data-equipment="chamber_light"]'),camera=a.$('.printer-camera');
 light.click();light.click();
 assert.equal(a.$('[data-equipment="chamber_light"]'),light);assert.match(light.textContent,/Turning light on/);assert.equal(light.getAttribute('aria-busy'),'true');assert.equal(light.getAttribute('aria-checked'),'false');
 assert.equal(a.$('[data-equipment="fan:1"]').disabled,true,'same-printer physical commands remain serialized');
 a.state.commands=[{id:101,action:'set:chamber_light',state:'pending',created:Date.now(),user:'Admin',message:'target 1'}];release();await settle();
 assert.equal(writesSent,1);assert.ok(timers.length);assert.match(light.textContent,/Waiting for printer confirmation/);
 const next=timers.shift();a.state.commands[0].state='succeeded';a.state.snapshot.printers[0].equipment.settings.find(s=>s.key==='chamber_light').value=1;
 await next();await settle();assert.equal(light.getAttribute('aria-checked'),'true');assert.equal(light.disabled,false);assert.equal(light.getAttribute('aria-busy'),'false');assert.equal(writesSent,1);assert.equal(timers.length,0);assert.equal(a.$('.printer-camera'),camera);
});

test('fan slider and presets stay local until confirmation, and survive parent telemetry refresh',async t=>{
 const a=await app(t);a.PrinterControl.open(1);await settle();a.$('[data-equipment="fan:1"]').click();
 const dialog=a.$('#equipmentDialog'),input=a.$('#settingValue');input.value='42';input.dispatchEvent(new a.w.Event('input'));
 assert.equal(a.$('#settingRange').value,'42');a.$('[data-setting-preset="75"]').click();assert.equal(input.value,'75');assert.equal(writes(a).length,0);
 a.intervals.find(i=>i.ms===2000).fn();await settle();assert.equal(a.$('#equipmentDialog'),dialog);assert.equal(input.value,'75');
 a.$('#equipmentForm').requestSubmit();await settle();assert.equal(writes(a).length,1);
 assert.deepEqual(JSON.parse(writes(a)[0].options.body),{printerId:1,key:'fan:1',value:75});assert.ok(a.$('#controlRoot'));
});

test('temperature stepper enforces bounds and closing heat is an explicit confirmed zero target',async t=>{
 const a=await app(t);a.PrinterControl.open(1);await settle();a.$('[data-equipment="bed"]').click();
 a.$('[data-temperature-step="5"]').click();assert.equal(a.$('#settingValue').value,'55');
 a.$('#settingValue').value='99';a.$('[data-temperature-step="5"]').click();assert.equal(a.$('#settingValue').value,'100');
 a.$('[data-setting-preset="0"]').click();assert.equal(a.$('#settingValue').value,'0');assert.equal(writes(a).length,0);
 a.$('#equipmentForm').requestSubmit();await settle();assert.equal(writes(a).length,1);assert.equal(JSON.parse(writes(a)[0].options.body).value,0);
});

test('setting backdrop, close and Escape return to the same console and scroll position without writing',async t=>{
 for(const how of ['backdrop','close','escape','back']){
  const a=await app(t);a.PrinterControl.open(1);await settle();a.$('#modal').scrollTop=240;
  a.$('[data-equipment="fan:1"]').click();assert.ok(a.$('#equipmentForm'));
  if(how==='escape'){
   const event=new a.w.Event('cancel',{cancelable:true});a.$('#equipmentDialog').dispatchEvent(event);assert.equal(event.defaultPrevented,true);
  }else if(how==='close')a.$('[data-setting-close]').click();
  else if(how==='back')a.$('#equipmentBack').click();
  else a.$('#equipmentDialog').click();
  await settle();assert.equal(a.$('#modal').open,true);assert.equal(a.$('#controlRoot').dataset.printerId,'1');assert.equal(a.$('#modal').scrollTop,240);assert.equal(writes(a).length,0);
  a.$('[data-close]').click();assert.equal(a.$('#modal').open,false,'main console still closes normally');
 }
});

test('control tabs and inventory disclosure survive telemetry refresh without sending commands',async t=>{
 const a=await app(t);a.PrinterControl.open(1);await settle();
 a.$('#controlTab-materials').click();a.$('.control-inventory').open=true;await settle();
 a.intervals.find(i=>i.ms===2000).fn();await settle();
 assert.equal(a.$('#controlTab-materials').getAttribute('aria-selected'),'true');
 assert.equal(a.$('#controlPane-materials').hidden,false);assert.equal(a.$('#controlPane-overview').hidden,true);
 assert.equal(a.$('.control-inventory').open,true);
 a.$('.control-tabs').dispatchEvent(new a.w.KeyboardEvent('keydown',{key:'End',bubbles:true}));
 assert.equal(a.$('#controlPane-history').hidden,false);
 a.$('.control-tabs').dispatchEvent(new a.w.KeyboardEvent('keydown',{key:'Home',bubbles:true}));
 assert.equal(a.$('#controlPane-overview').hidden,false);assert.equal(writes(a).length,0);
});

test('leaving the monitor tab aborts video and does not restart it on refresh or return',async t=>{
 const a=await app(t);a.state.snapshot.capabilities.deviceMode='live';a.state.snapshot.printers[0].equipment.camera={supported:true,protocol:'rtsps'};
 Object.defineProperty(a.w.document,'hidden',{configurable:true,value:false});
 let cameraCalls=0,signal;const original=a.w.fetch;
 a.w.fetch=(url,options)=>{if(!url.startsWith('/api/printers/camera'))return original(url,options);cameraCalls++;signal=options.signal;return new Promise((resolve,reject)=>signal.addEventListener('abort',()=>reject(new a.w.DOMException('aborted','AbortError'))))};
 await a.Remote.refresh({renderPage:false});a.PrinterControl.open(1);await settle();a.$('[data-camera-toggle]').click();await settle();
 assert.equal(cameraCalls,1);assert.equal(signal.aborted,false);
 a.$('#controlTab-materials').click();await settle();assert.equal(signal.aborted,true);
 a.intervals.find(i=>i.ms===2000).fn();await settle();a.$('#controlTab-overview').click();await settle();
 assert.equal(a.$('[data-camera-toggle]').textContent,'开始观看');assert.equal(cameraCalls,1);assert.equal(writes(a).length,0);
});

test('unconfirmed device warnings stay visible outside the selected content tab',async t=>{
 const a=await app(t);a.state.commands=[{id:9,action:'set:chamber_light',state:'uncertain',created:Date.now(),user:'测试员',message:'待核对'}];
 a.PrinterControl.open(1);await settle();a.$('#controlTab-history').click();
 assert.match(a.$('#controlNotice').textContent,/未确认/);
 assert.equal(a.$('#controlNotice').closest('[role=tabpanel]'),null);
 assert.equal(a.$('[data-equipment="chamber_light"]').disabled,true);assert.equal(writes(a).length,0);
});

test('physical printer termination exposes a review path without requiring a failed cloud command',async t=>{
 const a=await app(t,{language:'en'});Object.assign(a.state.snapshot.printers[0],{state:'failed',rawState:'FAILED',file:''});
 await a.Remote.refresh({renderPage:false});a.PrinterControl.open(1);await settle();
 assert.ok(a.$('#controlResolve'));assert.match(a.$('#controlNotice').textContent,/printer reports that the job ended/);assert.equal(a.$('#controlStop').disabled,true);assert.equal(writes(a).length,0);
 a.$('#controlResolve').click();await settle();assert.ok(a.$('#confirmAction'));assert.equal(writes(a).length,0,'opening review must not stop or settle the printer');
 a.state.snapshot.printers[0].rawState='FINISH';a.state.snapshot.printers[0].state='confirming';a.state.snapshot.jobs[0].finishedAt=123;
 await a.Remote.refresh({renderPage:false});a.PrinterControl.open(1);await settle();assert.equal(a.$('#controlResolve'),null,'successful completion must not be presented as an aborted print');
});

test('service loss while browser remains online shows stale snapshot, disables physical controls, recovers without writes',async t=>{
 const a=await app(t);a.PrinterControl.open(1);await settle();
 assert.equal(a.$('[data-control-action="resume"]').disabled,false);
 a.state.failWorkspace=true;await assert.rejects(a.Remote.refresh({renderPage:false}));
 assert.equal(a.w.navigator.onLine,true);assert.equal(a.Remote.ready(),false);
 assert.equal(a.$('#offlineBanner').hidden,false);assert.match(a.$('#offlineBanner').textContent,/最后同步/);
 assert.equal(a.$('[data-control-action="resume"]').disabled,true);
 assert.match(a.$('.live-indicator').textContent,/服务链路不可用/);
 a.state.failWorkspace=false;await a.Remote.refresh({renderPage:false});
 a.PrinterControl.open(1);await settle();assert.equal(a.$('[data-control-action="resume"]').disabled,false);
 assert.equal(a.$('#offlineBanner').hidden,true);assert.equal(writes(a).length,0);
});
test('unknown command is held across refresh and reload; pending records block manual release',async t=>{
 const a=await app(t);a.state.failWrite=true;
 await assert.rejects(a.Remote.request('/api/printers/control',{printerId:1,jobId:1,action:'resume'}),e=>e.unconfirmed);
 assert.equal(a.Remote.needsReview(1),true);
 a.state.failWrite=false;await a.Remote.refresh({renderPage:false});
 await assert.rejects(a.Remote.request('/api/printers/control',{printerId:1,jobId:1,action:'resume'}),/待核对/);
 assert.equal(writes(a).length,1);
 const b=await app(t,{pendingReview:a.w.sessionStorage.getItem('layerhub.pendingReview')});
 assert.equal(b.Remote.needsReview(1),true);b.state.commands=[{state:'pending'}];
 await assert.rejects(b.Remote.reviewDevice(1),/仍在执行/);assert.equal(b.Remote.needsReview(1),true);
 b.state.commands=[];await b.Remote.reviewDevice(1);
 assert.equal(b.Remote.needsReview(1),true);b.$('#confirmAction').click();await settle();
 assert.equal(b.Remote.needsReview(1),false);assert.equal(writes(b).length,0);
});
test('independent printer telemetry age and service snapshot age both block controls',async t=>{
 const a=await app(t);a.state.snapshot.printers[0].updated-=120;
 await a.Remote.refresh({renderPage:false});a.PrinterControl.open(1);await settle();
 assert.equal(a.Remote.ready(),true);assert.match(a.$('.control-connection').textContent,/遥测已过期/);
 assert.equal(a.$('[data-control-action="resume"]').disabled,true);
 a.state.snapshot.printers[0].connected=false;await a.Remote.refresh({renderPage:false});a.PrinterControl.open(1);await settle();
 assert.match(a.$('.control-connection').textContent,/打印机离线/);
 const now=a.w.Date.now;a.w.Date.now=()=>now()+16000;a.Remote.paintConnection();
 assert.equal(a.Remote.ready(),false);assert.match(a.$('.live-indicator').textContent,/更新已过期/);
});

test('start and transfer confirmations track the printer after the control console is replaced',async t=>{
 const a=await app(t),job=a.state.snapshot.jobs.find(j=>j.id===4);
 job.printOptions={plate:1,useAMS:true,mapping:[{filament:0,spoolId:'white',grams:19}]};job.state='assigned';
 await a.Remote.refresh({renderPage:false});a.Remote.jobAction(4,'start');
 assert.equal(a.$('#realSend').dataset.printerId,'1');assert.equal(a.$('#realSend').disabled,false);
 a.state.snapshot.printers[0].connected=false;await a.Remote.refresh({renderPage:false});
 assert.equal(a.$('#realSend').disabled,true);a.$('#realSend').click();assert.equal(writes(a).length,0);
 await assert.rejects(a.Remote.request('/api/jobs/start',{jobId:4}),/离线/);
 a.state.snapshot.printers[0].connected=true;await a.Remote.refresh({renderPage:false});
 a.Remote.jobAction(4,'assign');assert.equal(a.$('#realSend').dataset.printerId,'1');
 a.state.failWorkspace=true;await assert.rejects(a.Remote.refresh({renderPage:false}));
 assert.equal(a.$('#realSend').disabled,true);a.$('#realSend').click();assert.equal(writes(a).length,0);
});
test('an uncertain record or state change during manual review keeps the hold in place',async t=>{
 const a=await app(t,{pendingReview:JSON.stringify([{printerId:1,at:Date.now(),path:'/api/printers/control'}])});
 a.state.commands=[{state:'uncertain'}];await assert.rejects(a.Remote.reviewDevice(1),/未确认结果/);
 a.state.commands=[];await a.Remote.reviewDevice(1);a.state.commands=[{state:'pending'}];
 a.$('#confirmAction').click();await settle();assert.equal(a.Remote.needsReview(1),true);
 assert.match(a.$('#toast').textContent,/状态已变化/);assert.equal(writes(a).length,0);
});
test('in-flight commands cannot be submitted twice, including when the service has not failed',async t=>{
 const a=await app(t),fetch=a.w.fetch;let finish;
 a.w.fetch=(url,options)=>url==='/api/printers/control'?new Promise(resolve=>{a.state.calls.push({url,options});finish=resolve}):fetch(url,options);
 const first=a.Remote.request('/api/printers/control',{printerId:1,jobId:1,action:'resume'});
 await assert.rejects(a.Remote.request('/api/printers/control',{printerId:1,jobId:1,action:'resume'}),/仍在提交/);
 assert.equal(writes(a).length,1);finish(reply(200,{ok:true}));await first;
 assert.equal(a.Remote.deviceBlocked(1),false);
});
test('lost setting confirmation opens the review notice and allows an explicit server-side resolution',async t=>{
 const a=await app(t);a.PrinterControl.open(1);await settle();
 a.state.failWrite=true;a.$('[data-equipment="chamber_light"]').click();await settle();
 assert.equal(a.Remote.needsReview(1),true);assert.equal(a.$('#connectionReviewNotice').hidden,false);
 a.state.failWrite=false;a.state.commands=[{state:'uncertain',action:'set:chamber_light',user:'测试员',created:new Date().toISOString(),message:'待核对'}];
 a.PrinterControl.open(1);await settle();a.$('#resolveSettings').click();
 assert.equal(a.$('#confirmAction').disabled,false);assert.equal(a.$('#confirmAction').dataset.review,'true');
 a.state.commands=[];a.$('#confirmAction').click();await settle();
 assert.equal(a.Remote.needsReview(1),false);assert.equal(writes(a).length,2);
 assert.equal(JSON.parse(writes(a)[1].options.body).key,'resolve_settings');
});

test('real X2D readings keep dual nozzles, estimated AMS stock and empty external sources distinct',async t=>{
 const a=await app(t),p=a.state.snapshot.printers[0],inventory=JSON.stringify(a.state.snapshot.spools);
 Object.assign(p,{modelId:'x2d',model:'X2D',state:'idle',equipment:{bed:27,bedTarget:0,extruders:[{name:'喷嘴 · 设备编号 0',temperature:31,target:0},{name:'喷嘴 · 设备编号 1',temperature:28,target:0}],settings:[],amsUnits:[{id:0,kind:'AMS 2 Pro',temperature:28.4,humidityIndex:1}],feeds:[...['FCE300FF','000000FF','001489FF','FFFFFFFF'].map((color,i)=>({slot:String(i+1),label:'AMS 2 Pro · 单元 0 / 槽 '+(i+1),kind:'AMS 2 Pro',material:'PETG',color,present:true,remaining:[73,90,100,100][i],remainingUnit:'%'})),...['external','external:1'].map(slot=>({slot,material:'',present:false,remaining:null,color:''}))]}});
 await a.Remote.refresh({renderPage:false});a.PrinterControl.open(1);await settle();
 assert.equal(a.w.document.querySelectorAll('.settings-thermal>.equipment-setting').length,3);
 assert.match(a.$('.settings-thermal').textContent,/31/);assert.match(a.$('.settings-thermal').textContent,/28/);
 assert.ok([...a.w.document.querySelectorAll('.settings-thermal>button')].every(b=>b.disabled));
 assert.match(a.$('.ams-unit-strip').textContent,/AMS 2 Pro/);assert.match(a.$('.ams-unit-strip').textContent,/湿度等级 1/);
 assert.match(a.$('[data-machine-slot="1"]').textContent,/PETG/);assert.match(a.$('[data-machine-slot="1"]').textContent,/73%/);
 assert.match(a.$('[data-machine-slot="external"]').textContent,/未装载/);
 assert.match(a.$('[data-machine-slot="external:1"]').textContent,/未装载/);
 assert.equal(JSON.stringify(a.state.snapshot.spools),inventory);assert.equal(writes(a).length,0);
});

test('camera is opt-in, survives telemetry redraw and aborts when its console closes',async t=>{
 const a=await app(t);a.state.snapshot.capabilities.deviceMode='live';a.state.snapshot.printers[0].equipment.camera={supported:true,protocol:'rtsps'};
 Object.defineProperty(a.w.document,'hidden',{configurable:true,value:false});
 let cameraCalls=0,signal;const original=a.w.fetch;
 a.w.fetch=(url,options)=>{if(!url.startsWith('/api/printers/camera'))return original(url,options);cameraCalls++;signal=options.signal;return new Promise((resolve,reject)=>{signal.addEventListener('abort',()=>reject(new a.w.DOMException('aborted','AbortError')))})};
 await a.Remote.refresh({renderPage:false});a.PrinterControl.open(1);await settle();
 assert.equal(cameraCalls,0);assert.equal(a.$('[data-camera-toggle]').disabled,false);
 a.$('[data-camera-toggle]').click();await settle();assert.equal(cameraCalls,1);assert.equal(signal.aborted,false);
 const panel=a.$('.printer-camera');a.intervals.find(i=>i.ms===2000).fn();await settle();
 assert.equal(a.$('.printer-camera'),panel);assert.equal(cameraCalls,1);assert.equal(signal.aborted,false);
 a.$('#modal').close();await settle();assert.equal(signal.aborted,true);assert.equal(writes(a).length,0);
});

test('failed camera access does not show an old image or issue control requests',async t=>{
 const a=await app(t);a.state.snapshot.capabilities.deviceMode='live';a.state.snapshot.printers[0].equipment.camera={supported:true,protocol:'rtsps'};
 Object.defineProperty(a.w.document,'hidden',{configurable:true,value:false});
 let count=0;const original=a.w.fetch;a.w.fetch=(url,options)=>{if(!url.startsWith('/api/printers/camera'))return original(url,options);count++;return Promise.resolve(reply(401,{ok:false,error:'请先登录'}))};
 await a.Remote.refresh({renderPage:false});a.PrinterControl.open(1);await settle();a.$('[data-camera-toggle]').click();await settle();
 assert.equal(count,1);assert.equal(a.$('.camera-viewport img').hidden,true);assert.equal(a.$('[data-camera-toggle]').textContent,'开始观看');assert.match(a.$('[data-camera-message]').textContent,/登录/);assert.equal(writes(a).length,0);
});

test('multipart video handles fragmented frames and capture downloads a separately requested JPEG',async t=>{
 const a=await app(t);a.state.snapshot.capabilities.deviceMode='live';a.state.snapshot.printers[0].equipment.camera={supported:true,protocol:'rtsps'};
 Object.defineProperty(a.w.document,'hidden',{configurable:true,value:false});a.w.TextDecoder=TextDecoder;
 const blobs=[],revoked=[],downloads=[];a.w.URL.createObjectURL=blob=>{blobs.push(blob);return 'blob:test-'+blobs.length};a.w.URL.revokeObjectURL=url=>revoked.push(url);
 a.w.HTMLAnchorElement.prototype.click=function(){downloads.push({name:this.download,url:this.href})};
 const jpeg=Buffer.from([255,216,10,20,255,217]),now=Date.now();
 const frame=n=>Buffer.concat([Buffer.from((n?'\r\n':'')+'--layerhub-frame\r\nContent-Length: 6\r\nContent-Type: image/jpeg\r\nX-Frame-Updated: '+(now+n)+'\r\n\r\n'),jpeg]);
 const all=Buffer.concat([frame(0),frame(1)]);let index=0,cancelled=false;const original=a.w.fetch;
 a.w.fetch=async(url,options)=>{
  if(!url.startsWith('/api/printers/camera'))return original(url,options);
  if(url.includes('capture=1'))return {ok:true,headers:new Map([['Content-Type','image/jpeg'],['X-Frame-Updated',String(now+2)]]),blob:async()=>new a.w.Blob([jpeg],{type:'image/jpeg'})};
  return {ok:true,headers:new Map([['Content-Type','multipart/x-mixed-replace; boundary=layerhub-frame']]),body:{getReader:()=>({read:()=>{
   if(index<all.length){const start=index;index=Math.min(all.length,index+7);return Promise.resolve({done:false,value:new Uint8Array(all.subarray(start,index))})}
   return new Promise((resolve,reject)=>options.signal.addEventListener('abort',()=>reject(new a.w.DOMException('aborted','AbortError'))));
  },cancel:async()=>{cancelled=true},releaseLock(){}})}};
 };
 await a.Remote.refresh({renderPage:false});a.PrinterControl.open(1);await settle();a.$('[data-camera-toggle]').click();await settle();
 assert.equal(a.$('.camera-viewport img').hidden,false);assert.equal(blobs.length,2);assert.equal(revoked.length,1);
 a.$('[data-camera-capture]').click();await settle();assert.equal(downloads.length,1);assert.match(downloads[0].name,/^LayerHub_.*\.jpg$/);assert.match(a.$('[data-camera-status]').textContent,/浏览器下载/);
 a.$('#modal').close();await settle();assert.equal(cancelled,true);assert.equal(a.$('.camera-viewport img').hidden,true);assert.equal(writes(a).length,0);
});

test('standalone capture cancels on console close without downloading an old fallback',async t=>{
 const a=await app(t);a.state.snapshot.capabilities.deviceMode='live';a.state.snapshot.printers[0].equipment.camera={supported:true,protocol:'rtsps'};
 Object.defineProperty(a.w.document,'hidden',{configurable:true,value:false});let signal,downloads=0;
 a.w.HTMLAnchorElement.prototype.click=()=>downloads++;const original=a.w.fetch;
 a.w.fetch=(url,options)=>{if(!url.includes('capture=1'))return original(url,options);signal=options.signal;return new Promise((resolve,reject)=>signal.addEventListener('abort',()=>reject(new a.w.DOMException('aborted','AbortError'))))};
 await a.Remote.refresh({renderPage:false});a.PrinterControl.open(1);await settle();a.$('[data-camera-capture]').click();await settle();assert.equal(a.$('[data-camera-capture]').disabled,true);
 a.$('#modal').close();await settle();assert.equal(signal.aborted,true);assert.equal(downloads,0);assert.equal(writes(a).length,0);
});

test('admin corrects a display name without sending password or access changes; refresh shows the result',async t=>{
 const a=await app(t);a.state.snapshot.capabilities.accountManagement=true;
 a.state.snapshot.members.push({id:2,name:'错别字',username:'maker',admin:false,disabled:false,role:'成员',color:'blue'});
 const original=a.w.fetch;a.w.fetch=async(url,options={})=>{
  if(url==='/api/users/update'){
   const input=JSON.parse(options.body);assert.deepEqual(Object.keys(input).sort(),['name','userId']);assert.equal(input.userId,2);
   a.state.snapshot.members.find(m=>m.id===2).name=input.name;
  }
  return original(url,options);
 };
 await a.Remote.refresh({renderPage:false});a.$('[data-page="team"]').click();
 assert.ok(a.$('[data-manage-member="1"]'));a.$('[data-manage-member="2"]').click();
 assert.equal(a.$('#memberNameForm input[name="name"]').value,'错别字');
 change(a.w,a.$('#memberNameForm input[name="name"]'),'  正确姓名  ');
 a.intervals.find(i=>i.ms===3000).fn();await settle();assert.equal(a.$('#memberNameForm input[name="name"]').value,'  正确姓名  ');
 a.$('#memberNameForm').dispatchEvent(new a.w.Event('submit',{bubbles:true,cancelable:true}));await settle();
 assert.equal(writes(a).length,1);assert.equal(JSON.parse(writes(a)[0].options.body).name,'正确姓名');assert.match(a.$('#pageContent').textContent,/正确姓名/);
 a.$('[data-manage-member="1"]').click();assert.ok(a.$('#memberNameForm'));assert.equal(a.$('#memberForm'),null);
 change(a.w,a.$('#memberNameForm input[name="name"]'),'  ');a.$('#memberNameForm').dispatchEvent(new a.w.Event('submit',{bubbles:true,cancelable:true}));await settle();
 assert.match(a.$('#memberNameError').textContent,/请输入/);assert.equal(writes(a).length,1);
 a.$('#modal').close();a.state.snapshot.user.admin=false;await a.Remote.refresh({renderPage:false});a.$('[data-page="team"]').click();assert.equal(a.$('[data-manage-member]'),null);
});

function discoveredFeed(a,overrides={}){
 a.state.snapshot.capabilities.deviceMode='live';a.state.snapshot.printers[0].state='idle';a.state.snapshot.spools=[];
 a.state.snapshot.printers[0].equipment.feeds=[{slot:'1',label:'AMS 2 Pro · 单元 0 / 槽 1',kind:'AMS 2 Pro',material:'PETG-CF',color:'FCE300FF',remaining:73,remainingUnit:'%',present:true,updated:new Date().toISOString(),...overrides}];
}
test('machine discovery prefills material/color/mount, requires stock confirmation, and saves a new spool once',async t=>{
 const a=await app(t);discoveredFeed(a);const original=a.w.fetch;
 a.w.fetch=async(url,options={})=>{if(url==='/api/spools'){const spool=JSON.parse(options.body);a.state.snapshot.spools.push({...spool,id:'registered-from-machine'})}return original(url,options)};
 await a.Remote.refresh({renderPage:false});a.$('[data-page="materials"]').click();
 assert.match(a.$('.machine-discovery').textContent,/1 个装载位置待登记/);a.$('[data-register-feed]').click();await settle();
 assert.equal(writes(a).length,0);const form=a.$('#spoolForm');assert.ok(form);assert.equal(form.elements.material.value,'PETG-CF');assert.equal(form.elements.color.value,'#fce300');
 assert.equal(form.elements.brand.value,'');assert.equal(form.elements.capacity.value,'');assert.equal(form.elements.remaining.value,'');assert.equal(form.elements.owner.value,'admin');
 assert.equal(a.$('#mountPrinter').value,'1');assert.equal(a.$('#mountSlot').value,'1');
 form.elements.brand.value='实际品牌';form.elements.capacity.value='1000';form.elements.capacity.dispatchEvent(new a.w.Event('input',{bubbles:true}));a.$('[data-estimate-remaining]').click();assert.equal(form.elements.remaining.value,'730');
 change(a.w,form.elements.owner,'team');assert.equal(a.$('#sharingField').hidden,true);
 form.dispatchEvent(new a.w.Event('submit',{bubbles:true,cancelable:true}));await settle();
 const saved=JSON.parse(writes(a)[0].options.body);assert.equal(writes(a).length,1);assert.equal(saved.id,'');assert.equal(saved.owner,'team');assert.equal(saved.shared,false);assert.equal(saved.capacity,1000);assert.equal(saved.remaining,730);assert.deepEqual(saved.mount,{printerId:1,slot:'1'});
 assert.equal(a.$('.machine-discovery'),null);assert.match(a.$('.filament-grid').textContent,/730/);
 a.PrinterControl.open(1);await settle();assert.equal(a.$('[data-register-feed]'),null);assert.ok(a.$('[data-machine-spool="registered-from-machine"]'));
});
test('private occupied slots, empty feeds and stale observations do not create duplicate or speculative stock',async t=>{
 const a=await app(t);discoveredFeed(a);a.state.snapshot.occupiedSlots=[{printerId:1,slot:'1'}];
 await a.Remote.refresh({renderPage:false});a.$('[data-page="materials"]').click();assert.equal(a.$('[data-register-feed]'),null);
 a.state.snapshot.occupiedSlots=[];a.state.snapshot.printers[0].equipment.feeds[0].present=false;await a.Remote.refresh();assert.equal(a.$('[data-register-feed]'),null);
 a.state.snapshot.printers[0].equipment.feeds[0].present=true;a.state.snapshot.printers[0].equipment.feeds[0].updated=new Date(Date.now()-120000).toISOString();await a.Remote.refresh();assert.equal(a.$('[data-register-feed]').disabled,true);assert.equal(writes(a).length,0);
});
test('discovery rechecks the machine before creating a draft and never overwrites existing inventory on telemetry refresh',async t=>{
 const a=await app(t);discoveredFeed(a);await a.Remote.refresh({renderPage:false});a.$('[data-page="materials"]').click();
 a.state.snapshot.printers[0].equipment.feeds[0].present=false;a.$('[data-register-feed]').click();await settle();assert.equal(a.$('#spoolForm'),null);assert.equal(writes(a).length,0);
 a.state.snapshot.printers[0].equipment.feeds[0].present=true;a.state.snapshot.spools=[{id:'old',label:'库存原记录',brand:'Brand',material:'PETG-CF',colorName:'黄',color:'#fce300',capacity:1000,remaining:400,owner:'admin',shared:false,state:'available',mount:{printerId:1,slot:'1'}}];
 await a.Remote.refresh({renderPage:false});a.PrinterControl.open(1);await settle();assert.equal(a.$('#controlRoot [data-register-feed]'),null);a.$('[data-machine-spool="old"]').click();assert.equal(a.$('#spoolForm').elements.remaining.value,'400');assert.equal(writes(a).length,0);
});
test('X2D nozzle editing addresses the selected nozzle explicitly and idle speed controls stay disabled',async t=>{
 const a=await app(t);discoveredFeed(a);const p=a.state.snapshot.printers[0];p.modelId='x2d';
 p.equipment.settings=[{key:'nozzle:0',label:'喷嘴 0 目标温度',value:0,min:0,max:300,unit:'°C',updated:new Date().toISOString()},{key:'nozzle:1',label:'喷嘴 1 目标温度',value:0,min:0,max:300,unit:'°C',updated:new Date().toISOString()},{key:'speed',label:'速度',value:2,min:1,max:4,unit:'',updated:new Date().toISOString()}];
 await a.Remote.refresh({renderPage:false});a.PrinterControl.open(1);await settle();assert.equal(a.$('[data-equipment="speed"]').disabled,true);
 a.$('[data-equipment="nozzle:1"]').click();change(a.w,a.$('#equipmentForm input[name="value"]'),'180');a.$('#equipmentForm').dispatchEvent(new a.w.Event('submit',{bubbles:true,cancelable:true}));await settle();
 assert.equal(writes(a).length,1);assert.deepEqual(JSON.parse(writes(a)[0].options.body),{printerId:1,key:'nozzle:1',value:180});
});

test('authorization refusal disables heating/fans with an explanation, while lamp and material registration stay available',async t=>{
 const a=await app(t);discoveredFeed(a);const p=a.state.snapshot.printers[0];p.modelId='x2d';p.equipment.controlIssue='authorization_required';
 p.equipment.settings=[{key:'nozzle:0',label:'喷嘴 0',value:0,min:0,max:300,unit:'°C',updated:new Date().toISOString()},{key:'fan:1',label:'模型风扇',value:0,min:0,max:100,unit:'%',updated:new Date().toISOString()},{key:'chamber_light',label:'机舱灯',value:1,min:0,max:1,unit:'',updated:new Date().toISOString()}];
 await a.Remote.refresh({renderPage:false});a.PrinterControl.open(1);await settle();assert.match(a.$('.equipment-authorization').textContent,/第三方控制/);
 assert.equal(a.$('[data-equipment="nozzle:0"]').disabled,true);assert.equal(a.$('[data-equipment="fan:1"]').disabled,true);assert.equal(a.$('[data-equipment="chamber_light"]').disabled,false);assert.equal(a.$('#controlRoot [data-register-feed]').disabled,false);assert.equal(writes(a).length,0);
});


test('email binding is optional at login and mail setup stays private to the account form',async t=>{
 const a=await app(t),{w,$,Remote,EmailUI,state}=a;
 Remote.login();assert.ok($('#realLogin [name="bindEmail"]'));assert.equal($('#realLogin [name="bindEmail"]').checked,false);
 $('#modal').close();
 let settings={ok:true,email:'',verified:false,enabled:false,available:true,pendingEmail:'',pendingState:'',resendAfter:0};
 const baseFetch=w.fetch;
 w.fetch=async(url,options={})=>{if(url.startsWith('/api/account/email')){state.calls.push({url,options});if(options.method==='POST'){const body=JSON.parse(options.body);if(url.endsWith('/request'))settings={...settings,pendingEmail:body.email,pendingState:'queued',resendAfter:60};if(url.endsWith('/verify'))settings={...settings,email:settings.pendingEmail,verified:true,enabled:true,pendingEmail:''};return reply(200,{ok:true})}return reply(200,settings)}return baseFetch(url,options)};
 await EmailUI.open();assert.equal($('#emailSend').disabled,false);
 $('#emailRequest [name="email"]').value='member@example.com';$('#emailRequest').dispatchEvent(new w.Event('submit',{bubbles:true,cancelable:true}));await settle();
 assert.match($('#emailDelivery').textContent,/等待发送/);assert.equal($('#emailSend').disabled,true);
 $('#emailVerify [name="code"]').value='123456';$('#emailVerify').dispatchEvent(new w.Event('submit',{bubbles:true,cancelable:true}));await settle();
 assert.equal($('#emailEnabled').checked,true);assert.match($('#modalContent').textContent,/member@example.com/);
 const verification=state.calls.find(c=>c.url.endsWith('/email/verify'));assert.deepEqual(JSON.parse(verification.options.body),{code:'123456'});
 assert.equal(state.calls.some(c=>c.url.includes('/printers/')&&c.options.method==='POST'),false);
});

test('unavailable mail service disables code requests without affecting workspace login',async t=>{
 const {w,$,EmailUI,Remote}=await app(t);
 w.fetch=async()=>reply(200,{ok:true,email:'',verified:false,enabled:false,available:false,pendingEmail:'',resendAfter:0});
 await EmailUI.open();assert.equal($('#emailSend').disabled,true);assert.match($('#modalContent').textContent,/可以继续使用工作空间/);assert.equal(Remote.ready(),true);
});


test('late email status response cannot replace a different dialog',async t=>{
 const {w,$,EmailUI,Remote}=await app(t);let resolve;
 w.fetch=()=>new Promise(r=>{resolve=r});
 const pending=EmailUI.open();Remote.account();resolve(reply(200,{ok:true,email:'',available:true}));await pending;
 assert.match($('#modalContent').textContent,/个人账户/);assert.equal($('#emailRequest'),null);
});


test('per-printer timing uses fresh remaining time, keeps queue order, and never invents duration',()=>{
 const c=vm.createContext({I18n:testLocale,});vm.runInContext(read('workspace-model.js')+'\nglobalThis.ws=Workspace;',c);const ws=c.ws;
 const now=Date.UTC(2026,9,23,2,0),p={id:1,enabled:true,connected:true,updated:now/1000,state:'printing',remaining:'20 分钟'};
 const data={serverTime:now,printers:[p,{...p,id:2,state:'idle'}],jobs:[{id:1,printer:1,state:'printing',estimate:150,order:1},{id:2,printer:1,state:'queued',estimate:30,order:2},{id:3,printer:1,state:'queued',estimate:40,order:3},{id:4,printer:2,state:'queued',estimate:15,order:4}]};
 let q=ws.queue(data,1);assert.equal(q.rows[0].startAt,now+20*60000);assert.equal(q.rows[0].endAt,now+50*60000);assert.equal(q.rows[1].endAt,now+90*60000);assert.equal(ws.queue(data,2).rows[0].start,0);
 p.state='pause';q=ws.queue(data,1);assert.equal(q.rows[0].startAt,null);assert.match(q.reason,/暂停/);
 p.state='printing';p.remaining='未知';q=ws.queue(data,1);assert.equal(q.total,null);assert.match(q.reason,/未知/);
 p.remaining='20 分钟';p.updated-=91;q=ws.queue(data,1);assert.equal(q.rows[0].endAt,null);assert.match(q.reason,/过期/);
 p.updated=now/1000;p.state='idle';data.jobs[0].state='done';data.jobs[1].estimate=0;q=ws.queue(data,1);assert.equal(q.rows[0].endAt,null);assert.equal(q.rows[1].startAt,null);assert.equal(q.total,null);
});

test('notification center escapes content, is read-only until chosen, and keeps mail delivery claims precise',async t=>{
 const a=await app(t),{w,$,TeamTools,state}=a;let readOne=false;
 const original=w.fetch;w.fetch=async(url,options={})=>{
  if(url.startsWith('/api/notifications')){state.calls.push({url,options});if(options.method==='POST'){readOne=true;state.snapshot.notificationSummary={unread:0,latestId:7};return reply(200,{ok:true})}return reply(200,{ok:true,summary:{unread:readOne?0:1,latestId:7},nextBefore:0,notifications:[{id:7,title:'任务已确认完成',body:'<img src=x onerror=bad()>',category:'completed',created:1,read:readOne,delivery:'sent',entityType:'job',entityId:'4'}],deliveries:[{kind:'notification',state:'sent',created:1,attempts:1}]})}
  return original(url,options);
 };
 await TeamTools.notifications();assert.equal($('.notice-card p img'),null);assert.match($('.notice-card p').textContent,/<img/);assert.equal(writes(a).length,0);assert.match($('#notificationBtn').getAttribute('aria-label'),/1 条未读/);
 $('[data-notice-read="7"]').click();await settle();assert.equal(readOne,true);assert.equal($('#notificationBtn i').hidden,true);assert.equal(writes(a).length,1);
 $('[data-notice-tab="mail"]').click();await settle();assert.match($('.notice-list').textContent,/邮件服务已接收/);assert.doesNotMatch($('.notice-list').textContent,/已经送达收件箱/);
});

test('notification preference conflicts keep the form and selected categories for correction',async t=>{
 const a=await app(t),{w,$,TeamTools,state}=a;const base=w.fetch;
 w.fetch=async(url,options={})=>{if(url==='/api/account/notification-preferences'){state.calls.push({url,options});return options.method==='POST'?reply(409,{ok:false,error:'通知偏好已在其他窗口修改'}):reply(200,{ok:true,preferences:{started:true,completed:true,attention:true,devices:true,stock:true,revision:4}})}return base(url,options)};
 await TeamTools.preferences();$('#notificationPreferences [name="completed"]').checked=false;$('#notificationPreferences').dispatchEvent(new w.Event('submit',{bubbles:true,cancelable:true}));await settle();
 assert.match($('#preferencesError').textContent,/其他窗口/);assert.equal($('#notificationPreferences [name="completed"]').checked,false);
 const saved=writes(a).find(c=>c.url==='/api/account/notification-preferences');assert.equal(JSON.parse(saved.options.body).revision,4);assert.equal(JSON.parse(saved.options.body).completed,false);
 assert.equal($('#notificationPreferences [name="started"]').checked,true);assert.equal(JSON.parse(saved.options.body).started,true);assert.match($('#notificationPreferences').textContent,/打印机确认开始打印/);
});

test('stock reminder uses available inventory and concurrent submits issue only one settings write',async t=>{
 const a=await app(t),{w,$,Remote,TeamTools,state}=a;const spool=state.snapshot.spools[0];spool.remaining=100;spool.lowThreshold=80;spool.alertRevision=2;spool.revision=5;
 await Remote.refresh({renderPage:false});assert.equal(TeamTools.isLow(w.testing.data.spools[0]),true);
 let resolve;const base=w.fetch;w.fetch=async(url,options={})=>{if(url==='/api/spools/alert'){state.calls.push({url,options});return new Promise(r=>{resolve=r})}return base(url,options)};
 TeamTools.stockForm(w.testing.data.spools[0]);const form=$('#stockAlertForm');form.elements.threshold.value='60';form.dispatchEvent(new w.Event('submit',{bubbles:true,cancelable:true}));form.dispatchEvent(new w.Event('submit',{bubbles:true,cancelable:true}));await settle();
 const changes=writes(a).filter(c=>c.url==='/api/spools/alert');assert.equal(changes.length,1);assert.deepEqual(JSON.parse(changes[0].options.body),{id:'white',threshold:60,revision:2});
 resolve(reply(200,{ok:true}));await settle();assert.equal(state.snapshot.spools[0].remaining,100);assert.equal(state.calls.some(c=>c.url.includes('/printers/')&&c.options.method==='POST'),false);
});

test('late notification responses cannot replace another dialog',async t=>{
 const {w,$,TeamTools,Remote}=await app(t);let resolve;w.fetch=()=>new Promise(r=>{resolve=r});const pending=TeamTools.notifications();Remote.account();resolve(reply(200,{notifications:[],deliveries:[],summary:{unread:0,latestId:0}}));await pending;assert.match($('#modalContent').textContent,/个人账户/);
});


test('service loss immediately replaces stale queue timestamps with an unknown-time notice',async t=>{
 const a=await app(t);a.w.location.hash='#queues';await settle();
 assert.ok(a.$('[data-queue-job-time]'));
 a.state.failWorkspace=true;await assert.rejects(a.Remote.refresh({renderPage:false}));
 assert.match(a.$('[data-queue-job-time]').textContent,/服务状态不可用/);assert.match(a.$('[data-queue-summary]').textContent,/时间待定/);
});

test('one print confirmation dispatches once; a lost acknowledgement blocks repeats',async t=>{
 const a=await app(t),{w,$,state}=a;const job=state.snapshot.jobs.find(j=>j.id===4);job.printOptions={plate:1,useAMS:true,mapping:[{filament:0,spoolId:'white',grams:19}]};await a.Remote.refresh({renderPage:false});a.Remote.jobAction(4,'assign');assert.match($('#modal').textContent,/确认并打印/);assert.match($('#modal').textContent,/自动传输/);assert.equal(writes(a).length,0);state.failWrite=true;$('#realSend').click();$('#realSend').click();await settle();const calls=writes(a);assert.equal(calls.length,1);assert.equal(calls[0].url,'/api/jobs/dispatch');assert.equal(a.Remote.needsReview(1),true);assert.equal($('#realSend').disabled,true);
});

async function readySlice(t,{unknownPromote=false}={}){
 const a=await app(t),{w,state,$}=a,original=w.fetch;
 state.snapshot.printers[0].modelId='x2d';state.snapshot.spools[0].material='PETG';await a.Remote.refresh({renderPage:false});
 const task={id:22,modelId:5,filename:'直接打印.stl',state:'succeeded',updated:1,jobId:0,options:{x:138,y:128,scale:1,rotation:[0,0,0],layer:.2,infill:15,support:false,brim:false,color:'#FFFFFF'},info:{plates:[{id:1,seconds:968,filaments:[{grams:3,usedGrams:2.04,color:'#FFFFFF'}]}]}};
 w.fetch=async(url,options={})=>{
  if(url==='/api/slicing')return reply(200,{ok:true,ready:true,models:[],tasks:[task]});
  if(url==='/api/slicing/promote'){state.calls.push({url,options});if(unknownPromote)throw Error('lost acknowledgement');task.jobId=9;state.snapshot.jobs.push({id:9,userId:1,filename:'直接打印.gcode.3mf',state:'queued',printer:1,spoolId:'white',grams:3,estimate:17,printOptions:{plate:1}});return reply(200,{ok:true,jobId:9})}
  return original(url,options);
 };$('[data-page="slicer"]').click();await settle();$('[data-slice-task="22"]').click();$('#slicePromote').click();await settle();choose(w,$('[name="spool"][value="white"]'));return a;
}


test('display preferences are account scoped, validated and never written to the service',async t=>{
 const a=await app(t),{w,$,state}=a,html=w.document.documentElement;assert.equal(html.dataset.uiSize,'comfortable');$('#accountBtn').click();$('#appearanceSettings').click();const form=$('#appearanceForm');change(w,form.elements.size,'large');change(w,form.elements.contrast,'strong');change(w,form.elements.weight,'medium');assert.equal(html.dataset.uiSize,'comfortable');assert.equal($('#appearancePreview').dataset.uiSize,'large');submitForm(w,form);assert.equal(html.dataset.uiSize,'large');assert.equal(html.dataset.uiContrast,'strong');assert.equal(html.dataset.uiWeight,'medium');assert.equal(writes(a).length,0);
 state.snapshot.user.id=2;await a.Remote.refresh();assert.equal(html.dataset.uiSize,'comfortable');state.snapshot.user.id=1;await a.Remote.refresh();assert.equal(html.dataset.uiSize,'large');
 $('#accountBtn').click();$('#appearanceSettings').click();$('#appearanceReset').click();submitForm(w,$('#appearanceForm'));assert.equal(html.dataset.uiSize,'comfortable');assert.equal(html.dataset.uiContrast,'clear');
 const b=await app(t,{appearancePreference:'{"size":"huge; background:red","font":"url(evil)","weight":"medium"}'});assert.equal(b.w.document.documentElement.dataset.uiSize,'comfortable');assert.equal(b.w.document.documentElement.dataset.uiFont,'system');assert.equal(b.w.document.documentElement.dataset.uiWeight,'medium');
});

test('font choices are grouped and webfonts load only when chosen; previews keep saved appearance intact',async t=>{
 const a=await app(t),{w,$}=a;assert.equal(w.document.querySelectorAll('link[data-reading-font]').length,0);$('#accountBtn').click();$('#appearanceSettings').click();const form=$('#appearanceForm');assert.equal(form.elements.font.options.length,10);assert.equal(form.elements.font.querySelectorAll('optgroup').length,3);
 for(const [id,path] of [['wenkai','lxgw-wenkai'],['notoserif','noto-serif-sc'],['xiaowei','zcool-xiaowei'],['kuaile','zcool-kuaile'],['noto','noto-sans-sc']]){change(w,form.elements.font,id);assert.equal($('#appearancePreview').dataset.uiFont,id);assert.equal(w.document.documentElement.dataset.uiFont,'system');assert.equal(w.document.querySelector(`link[data-reading-font="${id}"]`).getAttribute('href'),`./assets/fonts/${path}/font.css`);assert.match($('#appearanceFontHint').textContent,/本服务器按需加载/)}
 const count=w.document.querySelectorAll('link[data-reading-font]').length;change(w,form.elements.font,'wenkai');assert.equal(w.document.querySelectorAll('link[data-reading-font]').length,count);submitForm(w,form);assert.equal(w.document.documentElement.dataset.uiFont,'wenkai');assert.equal(writes(a).length,0);
 $('#accountBtn').click();$('#appearanceSettings').click();change(w,$('#appearanceForm').elements.font,'kaiti');assert.match($('#appearanceFontHint').textContent,/电脑已安装/);assert.equal(w.document.querySelectorAll('link[data-reading-font]').length,count);$('#appearanceReset').click();assert.equal($('#appearancePreview').dataset.uiFont,'system');assert.equal(w.document.documentElement.dataset.uiFont,'wenkai');
});
test('saved webfont and old serif preferences remain usable after upgrading',async t=>{
 const a=await app(t,{appearancePreference:'{"font":"wenkai"}'});assert.equal(a.w.document.documentElement.dataset.uiFont,'wenkai');assert.equal(a.w.document.querySelectorAll('link[data-reading-font]').length,1);
 const b=await app(t,{appearancePreference:'{"font":"serif"}'});assert.equal(b.w.document.documentElement.dataset.uiFont,'serif');assert.equal(b.w.document.querySelectorAll('link[data-reading-font]').length,0);
});


async function betaIntakeApp(t,{language='zh-CN',multi=false}={}){
 const a=await app(t,{language}),{w,state}=a,base=w.fetch;
 const file=new w.File(['fixture'],'<img src=x>.3mf');
 const review={schema:2,kind:'sliced-3mf',sha256:'a'.repeat(64),bytes:7,sliced:{available:true,info:{model:'X2D',plates:[{id:1,seconds:3701,filaments:[{material:'PETG',color:'#FFFFFF',usedGrams:12.1,grams:13,index:0}]},...(multi?[plate(2,'#3159ed')]:[])]}},reference:{printer:'<img src=x> X2D',layer:'0.20',materials:['PETG']}};
 w.fetch=(url,options={})=>{if(url==='/api/files/inspect'){state.calls.push({url,options});return Promise.resolve(reply(200,{ok:true,review}))}return base(url,options)};
 return {...a,file,review};
}
test('beta removes editor navigation and payloads and sends retired bookmarks to the dashboard',async t=>{
 const a=await app(t),{$,w}=a;assert.equal($('[data-page="slicer"]'),null);assert.equal($('[data-go="slicer"]'),null);assert.equal($('#fileInput').accept,'.3mf');assert.equal(w.document.querySelector('script[src*="slicer-"]'),null);assert.equal(w.document.querySelector('script[src*="team-library"]'),null);
 w.location.hash='slicer';await settle();assert.ok($('#boardRoot'));assert.equal(w.location.hash,'#board');assert.equal(writes(a).length,0);
});
test('beta verifies a sliced file inline, preserves colors and prefills only confirmed single-plate estimates',async t=>{
 const a=await betaIntakeApp(t),{$,w}=a;await a.FileIntake.open(a.file);assert.ok($('#uploadPreparation'));assert.match($('#fileReview').textContent,/已校验切片/);assert.match($('#fileReview').textContent,/#FFFFFF/);assert.match($('#fileReview').textContent,/12.10 g/);assert.equal($('#fileReview img'),null);assert.match($('#pickedName').textContent,/<img src=x>/);
 a.Remote.upload();const form=$('#uploadScheduleForm');assert.equal(form.elements.estimate.value,'62');assert.equal(form.elements.grams.value,'13');assert.equal(form.querySelector('[name=spoolId]:checked'),null);assert.equal(writes(a).length,1);assert.equal(writes(a)[0].url,'/api/files/inspect');
});
test('beta asks which plates to arrange instead of choosing the first or showing blank estimates',async t=>{
 const a=await betaIntakeApp(t,{multi:true}),{$}=a;await a.FileIntake.open(a.file);assert.equal(a.w.document.querySelectorAll('.intake-plate').length,2);assert.equal(a.FileIntake.estimates(a.file).estimate,null);a.Remote.upload();assert.equal($('#uploadScheduleForm'),null);assert.ok($('#intakeChoose'));$('#intakeChooseNext').click();assert.match($('#intakeChooseError').textContent,/请先选择/);assert.equal(writes(a).length,1);
});
test('beta English intake and errors explain the external slicing workflow without an editor choice',async t=>{
 const a=await betaIntakeApp(t,{language:'en'}),{$,w}=a;await a.FileIntake.open(a.file);assert.match($('#fileReview').textContent,/Sliced 3MF verified/);assert.doesNotMatch($('#fileReview').textContent,/已校验|导入|工作台/);assert.equal($('#intakeGeometry'),null);
 w.fetch=async()=>reply(400,{ok:false,error:'Beta 仅接受已切片的 3MF 文件，请先在 Bambu Studio 完成切片并导出切片盘文件'});await a.FileIntake.open(new w.File(['model'],'unsliced.3mf'));assert.match($('#modal').textContent,/Beta accepts sliced 3MF files only/);assert.match($('#modal').textContent,/STL, unsliced projects and raw G-code are not accepted/);
});
test('beta cancellation and stale account responses cannot select files or create a task',async t=>{
 const a=await betaIntakeApp(t),{$,w}=a;let finish,options;w.fetch=(url,o)=>{options=o;return new Promise(r=>finish=()=>r(reply(200,{ok:true,review:a.review})))};
 const pending=a.FileIntake.open(a.file);$('#modal [data-cancel]').click();assert.equal(options.signal.aborted,true);finish();await pending;assert.equal($('#fileReview')?.textContent||'','');assert.equal($('#modal').open,false);assert.equal(w.location.hash,'#queues');
 const second=a.FileIntake.open(a.file);a.data.user.id=2;finish();await second;assert.equal(a.FileIntake.forFile(a.file),null);assert.equal($('#fileReview')?.textContent||'','');
});
test('beta inspection report is local and no geometry or execution action is offered',async t=>{
 const a=await betaIntakeApp(t),{$,w}=a;let downloads=0;w.URL.createObjectURL=()=>{downloads++;return 'blob:test'};w.URL.revokeObjectURL=()=>{};w.HTMLAnchorElement.prototype.click=function(){assert.equal(this.download,'layerhub-sliced-3mf-review.json')};await a.FileIntake.open(a.file);$('#fileReviewExport').click();assert.equal(downloads,1);assert.equal(writes(a).length,1);assert.equal($('#intakeSlice'),null);assert.equal($('#intakeGeometry'),null);
});

async function multiImportApp(t,options={}){
 const a=await betaIntakeApp(t,{multi:true,...options});
 a.review.sliced.info.model='P1S';a.review.sliced.info.plates=[{...plate(1,'#FFFFFF',10),seconds:61},{...plate(7,'#4778BC',12),seconds:181}];
 await a.FileIntake.open(a.file);return a;
}
async function chooseBatchSpool(a,plateId,spool){
 a.$(`[data-batch-spool="${plateId}"]`).click();const form=a.$('#batchSpoolForm');
 choose(a.w,form.querySelector(`[name=spoolId][value="${spool}"]`));submitForm(a.w,form);await settle();
}
async function openFilledBatch(a){
 a.$('[data-select-batch]').click();a.Remote.upload();
 await chooseBatchSpool(a,1,'white');await chooseBatchSpool(a,7,'blue');
}
test('multi-plate selection preserves a noncontiguous plate ID and its own estimates in single upload',async t=>{
 const a=await multiImportApp(t),{$,w}=a;choose(w,$('[data-import-plate="7"]'));a.Remote.upload();
 const form=$('#uploadScheduleForm');assert.equal(form.elements.estimate.value,'4');assert.equal(form.elements.grams.value,'12');
 let sent;w.XMLHttpRequest=class{constructor(){this.upload={}}open(){}setRequestHeader(){}send(body){sent=body}};
 choose(w,form.querySelector('[name=spoolId][value=blue]'));submitForm(w,form);
 assert.equal(JSON.parse(sent.get('schedule')).plate,7);assert.equal(JSON.parse(sent.get('schedule')).grams,12);
});
test('batch queue reviews colors per plate, reserves in one request and never sends a printer command',async t=>{
 const a=await multiImportApp(t),{$,w,state}=a;await openFilledBatch(a);assert.equal($('.batch-spool-dialog'),null);
 assert.match($('[data-batch-spool="1"]').textContent,/白色/);assert.match($('[data-batch-spool="7"]').textContent,/蓝色/);
 const base=w.fetch;let payload;
 w.fetch=async(url,o)=>{if(url==='/api/upload/batch'){state.calls.push({url,options:o});payload=JSON.parse(o.body.get('batch'));return reply(201,{ok:true,receipt:{requestId:payload.requestId,jobIds:[21,22]}})}return base(url,o)};
 submitForm(w,$('#batchQueueForm'));submitForm(w,$('#batchQueueForm'));await settle();
 assert.deepEqual(payload.items.map(x=>x.plate),[1,7]);assert.equal(payload.items[1].grams,12);
 assert.equal(state.calls.filter(c=>c.url==='/api/upload/batch').length,1);assert.equal(state.calls.some(c=>/dispatch|\/start|settings\/apply/.test(c.url)),false);
 assert.equal($('#modal').open,false);assert.equal(w.sessionStorage.getItem('layerhub.pendingImport.1'),null);
});
test('batch picker filters material and subtracts other plate drafts before offering a spool',async t=>{
 const a=await multiImportApp(t),{$,w,state}=a;state.snapshot.spools.find(s=>s.id==='white').remaining=60;await a.Remote.refresh();
 await openFilledBatch(a);$('[data-batch-spool="7"]').click();const form=$('#batchSpoolForm');
 assert.equal(form.querySelector('[name=spoolId][value=petg]'),null);assert.equal(form.querySelector('[name=spoolId][value=white]').disabled,true);
 form.querySelector('[data-back]').click();numberInput(w,$('[name=grams-1]'),900);submitForm(w,$('#batchQueueForm'));await settle();
 assert.match($('#batchError').textContent,/合计预留/);assert.equal(state.calls.some(c=>c.url==='/api/upload/batch'),false);
});
test('unknown batch response preserves the same ID and performs no automatic upload retry',async t=>{
 const a=await multiImportApp(t),{$,w,state}=a;await openFilledBatch(a);const base=w.fetch;let seen;
 w.fetch=async(url,o)=>{if(url==='/api/upload/batch'){state.calls.push({url,options:o});seen=JSON.parse(o.body.get('batch'));throw Error('lost acknowledgement')}if(url.startsWith('/api/jobs/import-receipt'))return reply(200,{ok:true,found:true,receipt:{requestId:seen.requestId,jobIds:[31,32]}});return base(url,o)};
 submitForm(w,$('#batchQueueForm'));await settle();assert.match($('#batchError').textContent,/未确认/);assert.equal($('#batchSubmit').disabled,false);assert.equal($('[name=grams-1]').disabled,true);
 const pending=JSON.parse(w.sessionStorage.getItem('layerhub.pendingImport.1'));assert.equal(pending.requestId,seen.requestId);
 assert.equal(state.calls.filter(c=>c.url==='/api/upload/batch').length,1);$('#modal').close();a.Remote.upload();assert.equal($('[name=grams-1]').disabled,true);assert.match($('[data-batch-spool="7"]').textContent,/蓝色/);$('#batchRead').click();await settle();assert.equal($('#modal').open,false);assert.equal(w.sessionStorage.getItem('layerhub.pendingImport.1'),null);
});
test('correcting a rejected batch keeps its original request ID and a replay cannot create duplicates',async t=>{
 const a=await multiImportApp(t),{$,w,state}=a;await openFilledBatch(a);const base=w.fetch,requests=[];
 w.fetch=async(url,o)=>{if(url==='/api/upload/batch'){requests.push(JSON.parse(o.body.get('batch')));return requests.length===1?reply(409,{ok:false,error:'库存变化，请重新选择'}):reply(200,{ok:true,receipt:{jobIds:[41,42]}})}if(url.startsWith('/api/jobs/import-receipt'))return reply(200,{ok:true,found:false});return base(url,o)};
 submitForm(w,$('#batchQueueForm'));await settle();assert.equal($('[name=grams-1]').disabled,false);
 numberInput(w,$('[name=grams-1]'),11);submitForm(w,$('#batchQueueForm'));await settle();assert.equal(requests.length,2);assert.equal(requests[0].requestId,requests[1].requestId);assert.equal(requests[1].items[0].grams,11);
});
test('English batch import has translated controls and A1 bare preparation locks AMS off',async t=>{
 const a=await multiImportApp(t,{language:'en'}),{$,w,state}=a;await openFilledBatch(a);
 // User-created spool names intentionally retain their original language.
 assert.match($('#batchQueueRoot').textContent,/Each plate becomes a separate job/);assert.equal($('#batchSubmit').textContent,'Add batch to queue');assert.match($('#batchTotals').textContent,/22 g reserved/);
 $('#modal').close();const p=state.snapshot.printers[0];p.modelId='a1';p.model='A1';p.configuration='bare';state.slice={model:'A1',plates:[plate(1,'#FFFFFF',10),plate(7,'#222222',12)]};state.snapshot.jobs[1].selectedPlate=7;await a.Remote.refresh();a.PrinterControl.options(a.w.testing.data.jobs.find(j=>j.id===4));await settle();
 assert.equal($('#projectPlate').value,'7');assert.equal($('#projectAMS').disabled,true);assert.equal($('#projectAMS').checked,false);assert.match($('#projectAMS').closest('label').textContent,/external spool holder/);assert.equal(writes(a).filter(c=>c.url!=='/api/files/inspect').length,0);
});

test('batch upload requires a saved recovery ID before any upload can be sent',async t=>{
 const a=await multiImportApp(t),{$,w,state}=a;await openFilledBatch(a);const original=w.Storage.prototype.setItem;
 w.Storage.prototype.setItem=function(k,v){if(k.startsWith('layerhub.pendingImport.'))throw new w.DOMException('storage blocked');return original.call(this,k,v)};
 submitForm(w,$('#batchQueueForm'));await settle();assert.match($('#batchError').textContent,/无法保存导入恢复编号/);assert.equal($('[name=grams-1]').disabled,false);assert.equal(state.calls.some(c=>c.url==='/api/upload/batch'),false);
});

test('plate IDs stay visible in task rows, the control console and the final print confirmation',async t=>{
 const a=await app(t),{$,state}=a;state.snapshot.jobs.forEach(j=>{j.selectedPlate=7;j.size=.1});await a.Remote.refresh();$('[data-page=mine]').click();
 assert.match($('.job-plate-label').textContent,/打印盘 7/);a.PrinterControl.open(1);await settle();assert.match($('.control-filename').textContent,/打印盘 7/);
 a.PrintFlow.open(a.w.testing.data.jobs.find(j=>j.id===4));assert.match($('.print-summary').textContent,/打印盘 7/);assert.equal(writes(a).length,0);
});

async function threePlatePlan(t){
 const a=await betaIntakeApp(t,{multi:true});
 a.review.sliced.info.plates=[
  {id:1,seconds:1461,filaments:[{index:2,material:'PETG',color:'#FFFFFF',usedGrams:.99,grams:1}]},
  {id:2,seconds:21267,filaments:[{index:1,material:'PETG',color:'#FFFF00',usedGrams:139.95,grams:140}]},
  {id:3,seconds:2440,filaments:[{index:0,material:'PETG',color:'#000000',usedGrams:2.11,grams:3}]}
 ];
 await a.FileIntake.open(a.file);return a;
}
test('a three-plate file explains three separate print runs outside collapsed details',async t=>{
 const a=await threePlatePlan(t),{$}=a,plan=$('.intake-print-plan');
 assert.match(plan.textContent,/共 3 个打印盘，全部打印需 3 次/);assert.equal($('#fileReview details').contains(plan),false);
 $('#fileReview details').open=false;assert.equal(plan.hidden,false);assert.match(plan.textContent,/每个打印盘对应一次独立打印/);
 assert.equal(a.FileIntake.selection(a.file).length,0);assert.equal(writes(a).length,1);assert.equal(writes(a)[0].url,'/api/files/inspect');
});
test('selected print runs track a subset of the file and clear without creating jobs',async t=>{
 const a=await threePlatePlan(t),{$,w}=a;assert.match($('[data-plan-selection]').textContent,/勾选/);
 choose(w,$('[data-import-plate="1"]'));assert.match($('[data-plan-selection]').textContent,/已选 1 盘.*1 次打印/);
 choose(w,$('[data-import-plate="3"]'));assert.match($('[data-plan-selection]').textContent,/已选 2 盘.*2 次打印/);
 assert.match($('.intake-print-plan>strong').textContent,/共 3 个打印盘/);$('[data-clear-plates]').click();assert.match($('[data-plan-selection]').textContent,/勾选/);assert.equal(writes(a).length,1);
});
test('multiple materials on one plate still mean one print run',async t=>{
 const a=await betaIntakeApp(t);a.review.sliced.info.plates[0].filaments.push({material:'PETG',color:'#FFFF00',grams:2,index:1},{material:'PETG',color:'#000000',grams:3,index:2});await a.FileIntake.open(a.file);
 assert.match(a.$('.intake-print-plan>strong').textContent,/共 1 个打印盘，全部打印需 1 次/);assert.match(a.$('[data-plan-selection]').textContent,/已选 1 盘.*1 次打印/);
});
test('English intake and batch review explain the number of separate runs without starting any',async t=>{
 const a=await multiImportApp(t,{language:'en'}),{$}=a;assert.match($('.intake-print-plan>strong').textContent,/2 plates.*2 separate print runs/);
 assert.doesNotMatch($('.intake-print-plan').textContent,/[\u3400-\u9fff]/u);await openFilledBatch(a);
 assert.match($('.batch-run-summary').textContent,/2 separate print jobs/);assert.match($('.batch-run-summary').textContent,/Confirm each start individually/);assert.equal(writes(a).length,1);
});

test('next from a real three-plate plan leads to per-plate time and consumption instead of empty totals',async t=>{
 const a=await threePlatePlan(t),{$}=a;a.Remote.upload();
 assert.equal($('#intakeChoose [data-import-plate]:checked'),null);assert.equal($('#batchQueueForm'),null);
 $('#intakeChoose [data-choose-all]').click();assert.match($('#intakeChoose [data-plan-selection]').textContent,/已选 3 盘.*3 次打印/);
 $('#intakeChooseNext').click();assert.ok($('#batchQueueForm'));
 assert.deepEqual([1,2,3].map(id=>[$(`[name=estimate-${id}]`).value,$(`[name=grams-${id}]`).value]),[['25','1'],['355','140'],['41','3']]);
 assert.match($('#batchTotals').textContent,/3 盘.*421 分钟.*144 g/);assert.equal(writes(a).length,1);assert.equal(writes(a)[0].url,'/api/files/inspect');
});

test('choosing only the yellow plate carries its own quantity, material and color into the next step',async t=>{
 const a=await threePlatePlan(t),{$,w}=a;a.Remote.upload();choose(w,$('#intakeChoose [data-import-plate="2"]'));$('#intakeChooseNext').click();
 const form=$('#uploadScheduleForm');assert.equal(form.elements.estimate.value,'355');assert.equal(form.elements.grams.value,'140');
 assert.match($('.intake-schedule-plate').textContent,/打印盘 2/);assert.match($('.intake-schedule-plate').textContent,/#FFFF00.*139.95 g/);
 assert.equal(form.querySelector('[name=spoolId][value=white]'),null);assert.ok(form.querySelector('[name=spoolId][value=petg]'));assert.equal(form.querySelector('[name=spoolId]:checked'),null);
 assert.equal(a.FileIntake.selection(a.file)[0].id,2);assert.equal(writes(a).length,1);
});

test('plate chooser cancels without writes and stale account callbacks cannot continue an upload',async t=>{
 const a=await threePlatePlan(t),{$,w}=a;a.Remote.upload();choose(w,$('#intakeChoose [data-import-plate="3"]'));$('#modal [data-cancel]').click();
 assert.equal($('#modal').open,false);assert.equal(a.FileIntake.selection(a.file)[0].id,3);a.Remote.upload();assert.equal($('#uploadScheduleForm [name=grams]').value,'3');
 $('#modal').close();a.FileIntake.select(a.file,[]);a.Remote.upload();const next=$('#intakeChooseNext');a.data.user.id=2;next.click();assert.equal($('#uploadScheduleForm'),null);assert.equal(writes(a).length,1);
});

test('English plate chooser explains per-plate values and does not introduce untranslated instructions',async t=>{
 const a=await multiImportApp(t,{language:'en'}),{$}=a;a.Remote.upload();
 assert.match($('#modal .modal-header h2').textContent,/Choose plates/);assert.match($('#intakeChoose').textContent,/time and material/);
 assert.doesNotMatch($('#intakeChoose').textContent,/[\u3400-\u9fff]/u);assert.match($('#intakeChooseNext').textContent,/Review each plate/);assert.equal(writes(a).length,1);
});

async function queueOrderApp(t,options={}){
 const a=await app(t,options),{state}=a;
 state.snapshot.capabilities.queueOrder=true;state.snapshot.printers[0].state='idle';
 state.snapshot.jobs=[1,2,3].map((id,i)=>({id,userId:1,user:state.snapshot.user.name,filename:'three-plates.gcode.3mf',selectedPlate:id,printer:1,state:'queued',order:i+1,grams:[1,140,3][i],estimate:[25,355,41][i],spoolId:'white',created:'今天'}));
 await a.Remote.refresh();a.$('[data-page=queues]').click();return a;
}

test('queue order controls show the next plate and keep reordering separate from printing',async t=>{
 const a=await queueOrderApp(t),{$,w,state}=a,base=w.fetch;
 assert.match($('[data-queue-entry="1"] .queue-next-label').textContent,/下一项/);assert.equal($('[data-queue-job="1"][data-queue-move=up]').disabled,true);assert.equal($('[data-queue-job="3"][data-queue-move=down]').disabled,true);
 assert.equal($('[data-queue-entry="1"] [data-action=assign]').disabled,false);assert.equal($('[data-queue-entry="3"] [data-action=assign]').disabled,true);
 w.fetch=async(url,o={})=>{if(url==='/api/jobs/reorder'){state.calls.push({url,options:o});const inx=JSON.parse(o.body);assert.equal(inx.move,'up');assert.deepEqual(inx.expected.map(x=>x.id),[1,2,3]);state.snapshot.jobs[1].order=3;state.snapshot.jobs[2].order=2;return reply(200,{ok:true})}return base(url,o)};
 $('[data-queue-job="3"][data-queue-move=up]').click();await settle();
 assert.deepEqual([...w.document.querySelectorAll('[data-queue-entry]')].map(r=>Number(r.dataset.queueEntry)),[1,3,2]);assert.equal(writes(a).length,1);assert.equal(writes(a)[0].url,'/api/jobs/reorder');
 assert.equal(state.calls.some(c=>/dispatch|\/start|\/send/.test(c.url)),false);assert.equal(state.snapshot.jobs[2].grams,3);
});

test('setting the next task requires review and double clicks do not submit twice',async t=>{
 const a=await queueOrderApp(t),{$,w,state}=a;let finish;const base=w.fetch;
 w.fetch=async(url,o={})=>{if(url==='/api/jobs/reorder'){state.calls.push({url,options:o});return new Promise(r=>{finish=()=>r(reply(200,{ok:true}))})}return base(url,o)};
 $('[data-queue-job="3"][data-queue-move=first]').click();assert.match($('#modal').textContent,/不会立即开始打印/);assert.equal(writes(a).length,0);$('#modal [data-cancel]').click();assert.equal(writes(a).length,0);
 $('[data-queue-job="3"][data-queue-move=first]').click();$('#queueMoveConfirm').click();$('#queueMoveConfirm').click();await settle();assert.equal(writes(a).length,1);finish();await settle();assert.equal($('#modal').open,false);
});

test('member queue controls cannot overtake another owner and prepared jobs stay pinned',async t=>{
 const a=await queueOrderApp(t),{$,state}=a;state.snapshot.user.admin=false;state.snapshot.jobs[0].userId=2;state.snapshot.jobs[0].user='其他成员';await a.Remote.refresh();
 assert.equal($('[data-queue-entry="1"] .queue-order-tools'),null);assert.equal($('[data-queue-job="2"][data-queue-move=up]').disabled,true);assert.equal($('[data-queue-job="3"][data-queue-move=first]').disabled,true);assert.equal($('[data-queue-job="3"][data-queue-move=up]').disabled,false);
 state.snapshot.jobs[0].state='assigned';await a.Remote.refresh();assert.ok($('[data-queue-entry="1"] .queue-next-label'));assert.equal($('[data-queue-entry="1"] .queue-order-tools'),null);assert.equal(writes(a).length,0);
});

test('an unknown reorder response blocks repeat moves until an explicit queue reread',async t=>{
 const a=await queueOrderApp(t),{$,w,state}=a,base=w.fetch;
 w.fetch=async(url,o={})=>{if(url==='/api/jobs/reorder'){state.calls.push({url,options:o});throw Error('ack lost')}return base(url,o)};
 $('[data-queue-job="3"][data-queue-move=up]').click();await settle();assert.match($('[data-queue-order-error]').textContent,/重新读取/);assert.equal($('[data-queue-job="3"][data-queue-move=up]').disabled,true);$('[data-queue-job="2"][data-queue-move=down]').click();await settle();assert.equal(writes(a).length,1);
 $('[data-queue-reread]').click();await settle();assert.equal($('[data-queue-job="3"][data-queue-move=up]').disabled,false);assert.equal(writes(a).length,1);
});

test('English queue order instructions and confirmation remain localized',async t=>{
 const a=await queueOrderApp(t,{language:'en'}),{$}=a;assert.doesNotMatch($('.queue-order-note').textContent,/[\u3400-\u9fff]/u);assert.match($('.queue-order-note').textContent,/one at a time/);
 $('[data-queue-job="3"][data-queue-move=first]').click();assert.match($('#modal').textContent,/does not start/);assert.match($('#queueMoveConfirm').textContent,/Confirm/);assert.equal(writes(a).length,0);
});

test('orphan completion form explains empty inventory and offline printers without allowing submission',async t=>{
 for(const mode of ['empty','offline']){
 const a=await app(t),p=a.state.snapshot.printers[0];a.state.snapshot.jobs=[];p.completionReview={jobId:16,commandId:83};p.state=mode==='offline'?'offline':'confirming';if(mode==='empty')a.state.snapshot.spools=[];else p.connected=false;
 await a.Remote.refresh();a.PrinterControl.open(1);await settle();a.$('#settleMissingTask').click();const f=a.$('#orphanSettlement');assert.equal(a.$('[form="orphanSettlement"]').disabled,true);assert.match(f.textContent,mode==='empty'?/没有可登记用量的耗材/:/打印机当前离线/);submitForm(a.w,f);await settle();assert.equal(writes(a).length,0);
 }
});

test('Studio setup keeps the executable path local, downloads setup and opens only a fixed protocol',async t=>{
 const a=await app(t),{$,w}=a,links=[],blobs=[];w.URL.createObjectURL=b=>{blobs.push(b);return 'blob:studio'};w.URL.revokeObjectURL=()=>{};w.HTMLAnchorElement.prototype.click=function(){links.push({href:this.href,download:this.download})};
 $('#studioShortcut').click();assert.ok($('#studioSetup'));assert.equal(writes(a).length,0);const f=$('#studioSetup');f.elements.path.value='"F:\\软件\\Bambu Studio\\bambu-studio.exe"';submitForm(w,f);await settle();assert.equal(links[0].download,'LayerHub-Bambu-Studio.reg');assert.equal(blobs.length,1);assert.equal(w.localStorage.getItem('layerhub.local.bambuStudio.path'),'F:\\软件\\Bambu Studio\\bambu-studio.exe');assert.equal(writes(a).length,0);
 $('#studioTry').click();assert.equal(links.at(-1).href,'layerhub-studio://open');assert.doesNotMatch(links.at(-1).href,/F:|软件|exe/);assert.match($('#toast').textContent,/已请求/);assert.doesNotMatch($('#toast').textContent,/已成功/);
 $('#studioRemove').click();assert.equal(links.at(-1).download,'LayerHub-Remove-Bambu-Studio.reg');$('#studioForget').click();assert.equal(w.localStorage.getItem('layerhub.local.bambuStudio.path'),null);assert.equal($('#studioTry').disabled,true);assert.equal(writes(a).length,0);
});

test('Studio rejects command lines, UNC executables and injected registry content without downloading',async t=>{
 const a=await app(t),{$,w}=a;let downloads=0;w.URL.createObjectURL=()=>{downloads++;return 'blob:test'};w.HTMLAnchorElement.prototype.click=()=>{};$('#studioShortcut').click();const f=$('#studioSetup');
 for(const path of ['cmd.exe','C:\\Windows\\notepad.exe','C:\\Bambu\\bambu-studio.exe --debug','\\\\host\\share\\bambu-studio.exe','C:\\x%APPDATA%\\bambu-studio.exe','C:\\x"\\bambu-studio.exe','C:\\..\\bambu-studio.exe']){f.elements.path.value=path;submitForm(w,f);assert.match($('#studioError').textContent,/完整/)}assert.equal(downloads,0);assert.equal(w.localStorage.getItem('layerhub.local.bambuStudio.path'),null);assert.equal(writes(a).length,0);
});

test('Studio setup translates its instructions and does not require a NAS connection to show local settings',async t=>{
 const a=await app(t,{language:'en'});a.$('#studioShortcut').click();assert.match(a.$('#modal').textContent,/Local Bambu Studio/);assert.doesNotMatch(a.$('#modal').textContent,/[\u3400-\u9fff]/u);assert.equal(writes(a).length,0);
});
