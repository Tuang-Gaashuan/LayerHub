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
const reply=(status,value)=>({status,ok:status>=200&&status<300,json:async()=>value});
const tick=()=>new Promise(resolve=>setImmediate(resolve));
async function settle(){for(let i=0;i<8;i++)await tick()}

test('English navigation, account and material pages preserve user-created names',async t=>{
 const a=await app(t,{language:'en'}),{w,$,state}=a;
 assert.equal(w.document.documentElement.lang,'en');assert.equal(w.I18n.locale(),'en-US');
 assert.equal($('[data-page="slicer"]').textContent.trim(),'Model workbench');
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

test('English workbench labels and undo preserve object geometry and camera',async t=>{
 const viewer=cameraViewer(),a=await multiWorkbench(t,{viewer,language:'en'}),{$,w}=a;
 assert.match($('#sliceWorkbench').textContent,/Print configuration/);assert.match($('#sliceWorkbench').textContent,/Generate supports/);
 $('[data-model="5"]').click();await settle();const camera=structuredClone(viewer.camera),before=workbenchState(a);
 $('#sliceDuplicate').click();$('#sliceUndo').click();await settle();assert.deepEqual(workbenchState(a),before);assert.deepEqual(viewer.camera,camera);
 $('#sliceObjectProcess').click();await settle();assert.match($('#modal').textContent,/Selected-object process/);assert.match($('#modal').textContent,/Inherit plate/);
 assert.equal(writes(a).filter(c=>/jobs\/|slicing\/tasks/.test(c.url)).length,0);
});

test('language changes flush acknowledged drafts before saving the account preference and reloading',async t=>{
 const a=await multiWorkbench(t),{$,w,state}=a;$('[data-model="5"]').click();await settle();numberInput(w,$('[data-place="x"]'),149);
 const before=workbenchState(a);let reloaded=false;
 await w.I18n.change('en',{reload:()=>{reloaded=true}});
 const calls=writes(a),draft=calls.findIndex(c=>c.url==='/api/slicing/draft'),language=calls.findIndex(c=>c.url==='/api/account/language');
 assert.ok(draft>=0&&language>draft);assert.equal(JSON.parse(calls[language].options.body).language,'en');assert.equal(w.localStorage.getItem('layerhub.language'),'en');assert.equal(reloaded,true);assert.deepEqual(workbenchState(a),before);
 assert.equal(state.calls.filter(c=>/jobs\/dispatch|jobs\/start|slicing\/tasks/.test(c.url)).length,0);
});

test('unconfirmed draft saves block language reload and retain all local model edits',async t=>{
 const a=await multiWorkbench(t),{$,w,state}=a;$('[data-model="5"]').click();await settle();numberInput(w,$('[data-place="x"]'),157);
 const before=workbenchState(a),base=w.fetch;w.fetch=(url,options)=>url==='/api/slicing/draft'?Promise.reject(Error('unavailable')):base(url,options);let reloaded=false;
 await assert.rejects(w.I18n.change('en',{reload:()=>{reloaded=true}}),/草稿/);
 assert.equal(reloaded,false);assert.equal(w.localStorage.getItem('layerhub.language'),'zh-CN');assert.deepEqual(workbenchState(a),before);assert.equal(state.calls.filter(c=>c.url==='/api/account/language').length,0);
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

test('per-board material selection is undoable and old projects keep PETG Basic as the default',async t=>{
 const a=await multiWorkbench(t),{w,$,state}=a;
 assert.equal($('#sliceMaterial').value,'petg-basic');change(w,$('#sliceMaterial'),'pla-basic');
 assert.match($('#sliceMaterialHint').textContent,/待实机验证/);$('#slicePlateAdd').click();
 assert.equal($('#sliceMaterial').value,'pla-basic');change(w,$('#sliceMaterial'),'petg-hf');
 $('[data-plate="0"]').click();await settle();assert.equal($('#sliceMaterial').value,'pla-basic');$('[data-plate="1"]').click();await settle();assert.equal($('#sliceMaterial').value,'petg-hf');
 // Each board keeps its own profile through the serialized project state.
 $('#sliceUndo').click();assert.equal($('#sliceMaterial').value,'pla-basic');
 assert.equal(state.calls.filter(c=>c.url==='/api/jobs/dispatch').length,0);
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

test('lay-flat review only applies a checked candidate to one instance, is undoable and retains camera and overrides',async t=>{
 const viewer=cameraViewer(),a=await multiWorkbench(t,{viewer}),{w,$,state}=a;$('[data-model="5"]').click();await settle();$('#sliceDuplicate').click();await settle();const before=workbenchState(a),camera=structuredClone(viewer.camera);
 $('#sliceFace').click();assert.equal($('.face-review').hidden,false);assert.equal($('#faceApply').disabled,true);viewer.callbacks.onFace({rotation:[90,0,0],area:200,size:[20,10,20]});assert.deepEqual(workbenchState(a),before);assert.equal($('#faceApply').disabled,false);$('#faceApply').click();assert.deepEqual(workbenchState(a).copies[0],before.copies[0]);assert.deepEqual([...workbenchState(a).copies[1].rotation],[90,0,0]);assert.deepEqual(viewer.camera,camera);
 historyKey(w,'z');await settle();assert.deepEqual(workbenchState(a).copies,before.copies);historyKey(w,'y');await settle();assert.deepEqual([...workbenchState(a).copies[1].rotation],[90,0,0]);assert.deepEqual(viewer.camera,camera);assert.equal(state.calls.some(c=>c.url==='/api/slicing/tasks'),false);
});
test('lay-flat cancellation, invalid faces, stale selection and late callbacks never apply a pose',async t=>{
 const viewer=cameraViewer(),a=await multiWorkbench(t,{viewer}),{w,$}=a;$('[data-model="5"]').click();await settle();$('#sliceDuplicate').click();await settle();const before=workbenchState(a);$('#sliceFace').click();viewer.callbacks.onFace({error:'其他部位会先接触打印板'});assert.equal($('#faceApply').disabled,true);assert.match($('#faceReviewStatus').textContent,/先接触/);$('#faceCancel').click();viewer.callbacks.onFace({rotation:[90,0,0],area:20,size:[20,10,20]});assert.deepEqual(workbenchState(a),before);
 $('#sliceFace').click();viewer.callbacks.onFace({rotation:[90,0,0],area:20,size:[20,10,20]});$('[data-copy="0"]').click();assert.equal($('.face-review').hidden,true);$('#faceApply').click();assert.deepEqual(workbenchState(a).copies,before.copies);
 $('#sliceFace').click();w.document.body.dispatchEvent(new w.KeyboardEvent('keydown',{key:'Escape',bubbles:true}));assert.equal($('.face-review').hidden,true);assert.deepEqual(workbenchState(a).copies,before.copies);
 $('[data-page="team"]').click();viewer.callbacks.onFace({rotation:[90,0,0],area:20,size:[20,10,20]});assert.equal($('.face-review'),null);
});
test('extended object fields preserve inherited and mixed parameters and store project format four',async t=>{
 const a=await multiWorkbench(t),{w,$,state}=a;$('[data-model="5"]').click();$('#sliceDuplicate').click();$('#sliceObjectProcess').click();
 for(const [key,value] of [['topThickness','.65'],['bottomThickness','1.2'],['pattern','gyroid']]){change(w,$(`[data-object-mode="${key}"]`),'custom');const el=$(`[data-object-value="${key}"]`);if(el.tagName==='SELECT')change(w,el,value);else numberInput(w,el,value)}
 $('#objectProcessApply').click();const before=workbenchState(a);assert.deepEqual(JSON.parse(JSON.stringify(before.copies[1].process)),{topThickness:.65,bottomThickness:1.2,pattern:'gyroid'});assert.equal(before.copies[0].process,undefined);
 $('#sliceSelectAll').click();$('#sliceObjectProcess').click();change(w,$('[data-object-mode="walls"]'),'custom');numberInput(w,$('[data-object-value="walls"]'),4);$('#objectProcessApply').click();const after=workbenchState(a);assert.equal(after.copies[0].process.walls,4);assert.equal(after.copies[1].process.topThickness,.65);assert.equal(after.copies[0].process.topThickness,undefined);
 $('#draftSaveNow').click();await settle();const draft=state.calls.filter(c=>c.url==='/api/slicing/draft'&&c.options.method==='POST').at(-1);assert.equal(JSON.parse(draft.options.body).formatVersion,4);
 $('#sliceSelectOnly').click();$('#sliceObjectProcess').click();change(w,$('[data-object-mode="topThickness"]'),'custom');numberInput(w,$('[data-object-value="topThickness"]'),'3.1');assert.equal($('#objectProcessApply').disabled,true);assert.match($('#objectProcessError').textContent,/0.12–3/);
});
test('print preparation uses saved object process and count rather than current editing parameters',async t=>{
 const a=await multiWorkbench(t),{w,$,tasks,state}=a;$('[data-model="5"]').click();numberInput(w,$('#sliceWalls'),6);
 tasks.push({id:7,modelId:5,filename:'saved.stl',state:'succeeded',updated:1,options:{x:138.25,y:128,rotation:[0,0,0],scale:1,layer:.2,infill:15,color:'#FFFFFF',material:'petg-basic',support:false,brim:false,copies:[{modelId:5,instanceId:'a',x:80,y:128,rotation:[0,0,0],scale:1},{modelId:5,instanceId:'b',x:170,y:128,rotation:[0,0,0],scale:1,process:{walls:4,topThickness:2,bottomThickness:1.6,pattern:'gyroid'}}]},info:{model:'X2D',plates:[{id:1,seconds:600,filaments:[{index:0,usedGrams:4.5,grams:5,color:'#FFFFFF',material:'PETG'}]}]}});
 $('#sliceRefresh').click();await settle();$('[data-slice-task="7"]').click();await settle();$('#slicePromote').click();await settle();assert.match($('.slice-queue-process').textContent,/2 个对象/);assert.match($('.slice-queue-process').textContent,/墙数 4/);assert.match($('.slice-queue-process').textContent,/2 mm/);assert.match($('.slice-queue-process').textContent,/Gyroid/);assert.equal(state.calls.some(c=>c.url==='/api/slicing/promote'||c.url==='/api/jobs/dispatch'),false);
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

async function teamLibraryApp(t){
 const a=await multiWorkbench(t),{w,$,state}=a,base=w.fetch;state.snapshot.capabilities.teamProjects=true;await a.Remote.refresh({renderPage:false});
 const options={x:138.25,y:128,rotation:[0,0,0],scale:1,layer:.2,infill:15,color:'#FFFFFF',material:'petg-basic',support:false,brim:false,copies:[{modelId:5,instanceId:'original',x:138.25,y:128,rotation:[0,0,0],scale:1}]};
 const project={id:8,name:'私人支架',revision:3,modelId:5,options,activeCopy:0,formatVersion:4,plates:[{id:'private',name:'支架盘',modelId:5,options,activeCopy:0}],activePlate:0};
 const shared={id:2,author:{id:1,username:'author',name:'<img src=x>'},title:'<img src=x>共享支架',description:'用途说明',latest:1,withdrawn:false,authorDisabled:false,updated:123,canPublish:true,canManage:true,canRestore:true};
 const version={id:21,number:1,title:shared.title,description:shared.description,note:'首版说明',created:123,state:project,sources:[{id:1,filename:'<img src=x>.stl',triangles:12,bounds:{size:[20,20,10]}}]};let copy=null;
 w.fetch=async(url,opts={})=>{
  if(url==='/api/slicing/projects')return reply(200,{ok:true,projects:[project,...(copy?[copy]:[])],draft:null,draftRevision:0});
  if(url.startsWith('/api/slicing/project?id='))return reply(200,{ok:true,project:Number(url.split('=').at(-1))===8?project:copy});
  if(url.startsWith('/api/team-projects/detail')){state.calls.push({url,options:opts});return reply(200,{ok:true,project:structuredClone(shared),version:structuredClone(version),versions:[{id:21,number:1,note:'首版说明',created:123}]})}
  if(url.startsWith('/api/team-projects?'))return reply(200,{ok:true,projects:[structuredClone(shared)],nextCursor:0});
  if(url==='/api/team-projects/publish'||url==='/api/team-projects/copy'||url==='/api/team-projects/visibility'){
   state.calls.push({url,options:opts});if(state.failWrite)throw Error('ack lost');
   if(url.endsWith('/publish'))return reply(200,{ok:true,versionId:21,sharedId:2});
   if(url.endsWith('/visibility')){shared.withdrawn=JSON.parse(opts.body).withdrawn;return reply(200,{ok:true})}
   copy={...structuredClone(project),id:9,name:JSON.parse(opts.body).name,revision:1};return reply(200,{ok:true,project:copy});
  }
  return base(url,opts);
 };
 $('[data-page="board"]').click();$('[data-page="slicer"]').click();await settle();return {...a,project,shared,version};
}

test('team library is on demand and publication requires an explicit snapshot agreement',async t=>{
 const a=await teamLibraryApp(t),{w,$}=a;assert.ok($('#projectTeamBrowse'));assert.equal($('#teamLibraryPanel'),null);$('#projectBrowse').click();await settle();$('[data-project-share="8"]').click();await settle();assert.match($('#teamPublishForm').textContent,/已保存版本 r3/);const form=$('#teamPublishForm');assert.equal(form.querySelector('[type=submit]').disabled,true);submitForm(w,form);await settle();assert.equal(writes(a).filter(c=>c.url.startsWith('/api/team-projects')).length,0);
 change(w,form.elements.agree,true);submitForm(w,form);submitForm(w,form);await settle();const calls=writes(a).filter(c=>c.url==='/api/team-projects/publish');assert.equal(calls.length,1);const inData=JSON.parse(calls[0].options.body);assert.equal(inData.projectId,8);assert.equal(inData.revision,3);assert.equal(inData.acknowledge,'team-models-and-settings');assert.equal(inData.sharedId,0);assert.match($('#teamLibraryPanel').textContent,/版本|更新说明/);assert.equal($('#teamLibraryPanel img'),null);
});

test('team copy stays independent and leaves editing untouched until the user explicitly opens it',async t=>{
 const a=await teamLibraryApp(t),{w,$}=a;$('[data-model="5"]').click();const before=JSON.stringify(workbenchState(a));$('#projectTeamBrowse').click();await settle();$('[data-team-detail="2"]').click();await settle();assert.equal($('#teamLibraryPanel img'),null);$('#teamCopy').click();const form=$('#teamCopyForm');form.elements.name.value='我的独立副本';submitForm(w,form);submitForm(w,form);await settle();assert.ok($('#teamOpenCopy'));assert.equal(JSON.stringify(workbenchState(a)),before);assert.equal(writes(a).filter(c=>c.url==='/api/team-projects/copy').length,1);assert.equal(writes(a).some(c=>c.url==='/api/slicing/tasks'||c.url==='/api/jobs/dispatch'),false);
 $('#teamOpenCopy').click();await settle();assert.match($('#modalContent').textContent,/替换当前摆放/);$('[data-cancel]').click();assert.equal(JSON.stringify(workbenchState(a)),before);
});

test('shared revisions are chosen explicitly and unknown publish acknowledgement blocks resubmission',async t=>{
 const a=await teamLibraryApp(t),{w,$,state}=a;$('#projectBrowse').click();await settle();$('[data-project-share="8"]').click();await settle();const form=$('#teamPublishForm');change(w,form.elements.series,'2');assert.equal(form.elements.note.value,'');form.elements.note.value='第二版';change(w,form.elements.agree,true);state.failWrite=true;submitForm(w,form);await settle();assert.match($('#teamPublishError').textContent,/核对/);submitForm(w,form);await settle();const calls=writes(a).filter(c=>c.url==='/api/team-projects/publish');assert.equal(calls.length,1);assert.equal(JSON.parse(calls[0].options.body).expectedVersion,1);assert.equal(form.querySelector('[type=submit]').disabled,true);
});

test('withdrawn team entries cannot copy and late library reads do not replace another dialog',async t=>{
 const a=await teamLibraryApp(t),{w,$}=a;a.shared.withdrawn=true;$('#projectTeamBrowse').click();await settle();$('[data-team-detail="2"]').click();await settle();assert.equal($('#teamCopy').disabled,true);assert.match($('#teamLibraryPanel').textContent,/当前不能复制/);a.shared.withdrawn=false;
 $('#teamLibraryBack').click();await settle();let resolve;w.fetch=()=>new Promise(r=>resolve=r);$('[data-team-detail="2"]').click();a.Remote.account();resolve(reply(200,{ok:true,project:a.shared,version:a.version,versions:[]}));await settle();assert.match($('#modalContent').textContent,/个人账户/);assert.equal($('#teamLibraryPanel'),null);
});

test('unknown team copy acknowledgement blocks duplicate copies and offers only a private project lookup',async t=>{
 const a=await teamLibraryApp(t),{w,$,state}=a;$('#projectTeamBrowse').click();await settle();$('[data-team-detail="2"]').click();await settle();$('#teamCopy').click();const form=$('#teamCopyForm');state.failWrite=true;submitForm(w,form);await settle();assert.match($('#teamCopyError').textContent,/核对/);assert.equal(form.elements.name.disabled,true);submitForm(w,form);await settle();assert.equal(writes(a).filter(c=>c.url==='/api/team-projects/copy').length,1);assert.equal(form.querySelector('[type=submit]').disabled,true);assert.ok([...form.querySelectorAll('button')].find(b=>b.textContent==='查看我的工程'));
});

test('cancelled sharing and copies stay inert after switching away from the workbench',async t=>{
 const a=await teamLibraryApp(t),{w,$}=a;$('#projectBrowse').click();await settle();$('[data-project-share="8"]').click();await settle();const form=$('#teamPublishForm');change(w,form.elements.agree,true);a.Remote.account();submitForm(w,form);await settle();assert.equal(writes(a).filter(c=>c.url==='/api/team-projects/publish').length,0);
});

test('failed team filtering clears the previous cursor so unrelated pages cannot be appended',async t=>{
 const a=await teamLibraryApp(t),{w,$}=a;$('#projectTeamBrowse').click();await settle();assert.equal($('#teamLibraryList').children.length,1);w.fetch=async()=>{throw Error('offline')};change(w,$('#teamSearch').elements.scope,'mine');await settle();assert.equal($('#teamLibraryList').children.length,0);assert.equal($('#teamLibraryMore').hidden,true);assert.match($('#teamLibraryError').textContent,/连接|网络|offline|服务/);
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
 if(viewer)w.testViewer=viewer;
 const source=scripts.join('\n');
 w.eval((viewer?source.replace("import('./slicer-viewer.js')","Promise.resolve({createViewer:(host,callbacks)=>{window.testViewer.callbacks=callbacks;return window.testViewer}})"):source)+'\nwindow.testing={Remote,PrinterControl,EquipmentUI,Finish,EmailUI,TeamTools,Workspace,ToolpathUI,get workbench(){return SlicerWorkbench.options()},get data(){return data}};');
 await settle();assert.equal(w.testing.Remote.ready(),true);
 return {w,...w.testing,state,intervals,$:selector=>w.document.querySelector(selector)};
}
function change(w,el,value){if(el.type==='checkbox')el.checked=value;else el.value=value;el.dispatchEvent(new w.Event('change',{bubbles:true}))}
function choose(w,el){el.checked=true;el.dispatchEvent(new w.Event('change',{bubbles:true}))}

async function multiWorkbench(t,{recovery=null,deferSlice=false,viewer,language='zh-CN'}={}){
 const a=await app(t,{viewer,language}),{w,state,$}=a,original=w.fetch;
 const models=[{id:5,filename:'cube.stl',triangles:12,bounds:{size:[20,20,10]}},{id:6,filename:'box.stl',triangles:12,bounds:{size:[40,12,18]}}],tasks=[];
 let resolveSlice;
 w.fetch=async(url,options={})=>{
  if(url==='/api/slicing')return reply(200,{ok:true,ready:true,objectProcessVersion:2,models,tasks});
  if(url==='/api/slicing/projects')return reply(200,{ok:true,projects:[],draft:recovery,draftRevision:recovery?4:0});
  if(url==='/api/slicing/tasks'){state.calls.push({url,options});const body=JSON.parse(options.body),id=tasks.length+1;tasks.push({id,modelId:body.modelId,options:body.options,filename:'synthetic.stl',state:'queued',updated:1});if(deferSlice)return new Promise(r=>resolveSlice=()=>r(reply(200,{ok:true,taskId:id})));return reply(200,{ok:true,taskId:id})}
  if(url==='/api/slicing/draft'){state.calls.push({url,options});return reply(200,{ok:true,revision:JSON.parse(options.body).revision+1})}
  return original(url,options);
 };
 $('[data-page="slicer"]').click();await settle();return {...a,models,tasks,resolve:()=>resolveSlice()};
}
function submitForm(w,form){form.dispatchEvent(new w.Event('submit',{bubbles:true,cancelable:true}))}
function numberInput(w,el,value){el.value=String(value);el.dispatchEvent(new w.Event('input',{bubbles:true}));el.dispatchEvent(new w.Event('change',{bubbles:true}))}
function batchPlanner(){const context=vm.createContext({I18n:testLocale,crypto:require('node:crypto').webcrypto});vm.runInContext(read('slicer-copies.js')+read('slicer-plates.js')+read('slicer-batch.js')+';globalThis.batch=SlicerBatch;globalThis.plates=SlicerPlates;',context);return context}
function batchState(api,{size=[100,100,10],triangles=12,scale=1,rotation=[0,0,0]}={}){const options={x:138.25,y:128,scale,rotation,layer:.2,infill:20,material:'petg-basic',support:false,brim:false,color:'#FFFFFF',advanced:{wallLoops:3}},plate=api.plates.make('打印板 01',options);plate.modelId=5;plate.taskId=77;plate.options.copies=[{modelId:5,x:138.25,y:128,scale,rotation}];return {state:{plates:[plate],activePlate:0,activeCopy:0},models:[{id:5,filename:'bracket.stl',triangles,bounds:{size}}]}}

test('batch planner fills new plates without resizing or touching existing plate results',()=>{
 const api=batchPlanner(),v=batchState(api),before=JSON.stringify(v),result=api.batch.plan(v.state,v.models,9);
 assert.deepEqual(Array.from(result.added,p=>p.count),[4,4,1]);assert.equal(result.activePlate,1);assert.equal(JSON.stringify(v),before);assert.equal(JSON.stringify(result.plates[0]),JSON.stringify(v.state.plates[0]));
 for(const p of result.plates.slice(1)){assert.equal(p.taskId,0);assert.equal(p.options.infill,20);assert.equal(p.options.advanced.wallLoops,3);assert.ok(p.options.copies.every(o=>o.scale===1&&o.modelId===5));const boxes=p.options.copies.map(o=>({min:[o.x-50,o.y-50,0],max:[o.x+50,o.y+50,10]}));const issues=vm.runInContext('SlicerCopies',api).inspectBounds(boxes,p.options);assert.equal(issues.length,0)}
 assert.equal(new Set(result.plates.map(p=>p.id)).size,4);assert.equal(new Set(result.plates.map(p=>p.name)).size,4);
});

test('batch planner obeys mesh, support margins, plate count and project limits atomically',()=>{
 const api=batchPlanner(),v=batchState(api,{triangles:60000});assert.deepEqual(Array.from(api.batch.plan(v.state,v.models,5).added,p=>p.count),[2,2,1]);
 const margins=batchState(api);margins.state.plates[0].options.support=true;margins.state.plates[0].options.brim=true;assert.deepEqual(Array.from(api.batch.plan(margins.state,margins.models,5).added,p=>p.count),[2,2,1]);
 const before=JSON.stringify(v);for(const n of [0,1.5,256,NaN,-1])assert.throws(()=>api.batch.plan(v.state,v.models,n));assert.throws(()=>api.batch.plan(v.state,v.models,15),/只剩 7/);assert.equal(JSON.stringify(v),before);
 const full=batchState(api);while(full.state.plates.length<8)full.state.plates.push(api.plates.make('空盘',full.state.plates[0].options));assert.throws(()=>api.batch.plan(full.state,full.models,1),/已有 8/);
 const crowded=batchState(api,{size:[10,10,10]});crowded.state.plates[0].options.copies=Array.from({length:64},()=>({...crowded.state.plates[0].options.copies[0]}));assert.throws(()=>api.batch.plan(crowded.state,crowded.models,193),/最多 256/);
 const huge=batchState(api,{size:[300,100,10]});assert.throws(()=>api.batch.plan(huge.state,huge.models,1),/不会自动缩小/);
});

test('batch uses the selected object on the active plate rather than the legacy anchor selection',()=>{
 const api=batchPlanner(),v=batchState(api),other=api.plates.make('打印板 02',v.state.plates[0].options);
 other.modelId=5;other.options.copies=[{modelId:5,x:80,y:80,scale:1,rotation:[0,0,0]},{modelId:6,x:150,y:150,scale:1.2,rotation:[0,0,90]}];other.activeCopy=1;v.state.plates.push(other);v.state.activePlate=1;v.models.push({id:6,filename:'selected.stl',triangles:12,bounds:{size:[40,12,18]}});
 const result=api.batch.plan(v.state,v.models,2);assert.equal(result.filename,'selected.stl');assert.equal(result.plates[2].modelId,6);assert.equal(result.plates[2].options.scale,1.2);assert.equal(result.plates[2].options.modelId,undefined);assert.ok(result.plates[2].options.copies.every(p=>p.modelId===6&&p.scale===1.2&&p.rotation[2]===90));assert.equal(result.plates[0].taskId,77);
});

test('batch preview cancellation and invalid inputs create no plates or commands',async t=>{
 const a=await multiWorkbench(t),{w,$,models,state}=a;assert.equal($('#sliceBatch').disabled,true);models[0].bounds.size=[100,100,10];$('[data-model="5"]').click();await settle();$('#sliceBatch').click();numberInput(w,$('#sliceBatchAmount'),9);
 assert.match($('#sliceBatchPreview').textContent,/分为 3 盘/);assert.equal(w.document.querySelectorAll('[data-plate]').length,1);
 for(const value of ['','0','1.5','256']){numberInput(w,$('#sliceBatchAmount'),value);assert.equal($('#sliceBatchApply').disabled,true);submitForm(w,$('#sliceBatchForm'));assert.equal(w.document.querySelectorAll('[data-plate]').length,1)}
 numberInput(w,$('#sliceBatchAmount'),9);const stale=$('#sliceBatchForm');$('[data-cancel]').click();submitForm(w,stale);assert.equal(w.document.querySelectorAll('[data-plate]').length,1);assert.equal(state.calls.filter(c=>c.url.startsWith('/api/jobs/')||c.url==='/api/slicing/tasks').length,0);
});

test('batch apply is a single undoable project edit, preserves camera and sends only explicit later slicing',async t=>{
 const viewer=cameraViewer(),a=await multiWorkbench(t,{viewer}),{w,$,models,state}=a;models[0].bounds.size=[100,100,10];$('[data-model="5"]').click();await settle();viewer.camera={position:[400,110,90],target:[100,100,5],zoom:1.7};const camera=structuredClone(viewer.camera);
 $('#sliceBatch').click();numberInput(w,$('#sliceBatchAmount'),9);submitForm(w,$('#sliceBatchForm'));await settle();assert.equal(w.document.querySelectorAll('[data-plate]').length,4);assert.equal(w.document.querySelectorAll('[data-copy]').length,4);assert.deepEqual(viewer.camera,camera);
 $('#sliceUndo').click();await settle();assert.equal(w.document.querySelectorAll('[data-plate]').length,1);assert.equal(w.document.querySelectorAll('[data-copy]').length,1);assert.deepEqual(viewer.camera,camera);$('#sliceRedo').click();await settle();assert.equal(w.document.querySelectorAll('[data-plate]').length,4);
 $('#sliceCompute').click();await settle();const request=state.calls.find(c=>c.url==='/api/slicing/tasks');assert.equal(JSON.parse(request.options.body).options.copies.length,4);assert.equal(state.calls.filter(c=>c.url.startsWith('/api/jobs/')).length,0);
});

test('batch rejects a preview after selection or process changes and ignores a dismissed page',async t=>{
 const a=await multiWorkbench(t),{w,$}=a;$('[data-model="5"]').click();await settle();$('#sliceDuplicate').click();await settle();$('#sliceBatch').click();$('[data-copy="0"]').click();submitForm(w,$('#sliceBatchForm'));assert.match($('#sliceBatchError').textContent,/已变化/);assert.equal(w.document.querySelectorAll('[data-plate]').length,1);
 $('[data-cancel]').click();$('#sliceBatch').click();change(w,$('#sliceSupport'),true);submitForm(w,$('#sliceBatchForm'));assert.match($('#sliceBatchError').textContent,/已变化/);$('[data-cancel]').click();$('#sliceBatch').click();const stale=$('#sliceBatchForm');$('[data-page="board"]').click();submitForm(w,stale);assert.equal($('#sliceWorkbench'),null);
});

test('batch retains legacy scaled rotations, inherited material and every plate in project draft',async t=>{
 const options={x:100,y:120,rotation:[0,0,90],scale:1.4,layer:.16,infill:25,support:true,brim:false,color:'#225599',material:'pla-basic'};
 const a=await multiWorkbench(t,{recovery:{modelId:5,options,activeCopy:0}}),{w,$,state,intervals}=a;$('#draftRecover').click();await settle();$('#sliceBatch').click();numberInput(w,$('#sliceBatchAmount'),3);assert.match($('#sliceBatchPreview').textContent,/140%/);submitForm(w,$('#sliceBatchForm'));await settle();assert.equal($('#sliceMaterial').value,'pla-basic');assert.equal($('[data-rotate="2"]').value,'90');assert.match($('#sliceScaleSummary').textContent,/140%/);
 $("#draftSaveNow").click();await settle();const writes=state.calls.filter(c=>c.url==='/api/slicing/draft'),body=JSON.parse(writes.at(-1).options.body);assert.equal(body.plates.length,2);assert.equal(body.plates[1].options.copies.length,3);assert.equal(body.plates[1].options.material,'pla-basic');assert.equal(body.plates[0].options.copies.length,1);
});

function selectedButtons(a){return [...a.w.document.querySelectorAll('[data-copy][aria-pressed="true"]')].map(b=>Number(b.dataset.copy))}
function pickObject(a,index,options={}){a.$('[data-copy="'+index+'"]').dispatchEvent(new a.w.MouseEvent('click',{bubbles:true,...options}))}
test('multi selection supports additive clicks, ranges, deselection and Ctrl+A without resizing',async t=>{
 const a=await multiWorkbench(t),{w,$}=a;$('[data-model="5"]').click();$('#sliceDuplicate').click();$('#sliceDuplicate').click();pickObject(a,0);pickObject(a,2,{shiftKey:true});assert.deepEqual(selectedButtons(a),[0,1,2]);assert.equal($('[data-place="x"]').disabled,true);assert.equal($('#sliceResize').disabled,true);assert.equal($('#sliceBatch').disabled,true);assert.equal($('[data-transform="rotate"]').disabled,true);
 pickObject(a,1,{ctrlKey:true});assert.deepEqual(selectedButtons(a),[0,2]);pickObject(a,0);assert.deepEqual(selectedButtons(a),[0]);assert.equal($('#sliceResize').disabled,false);historyKey(w,'a');assert.deepEqual(selectedButtons(a),[0,1,2]);assert.match($('#sliceRotateScope').textContent,/3/);
 pickObject(a,1);$('[data-place="x"]').dispatchEvent(new w.KeyboardEvent('keydown',{key:'a',ctrlKey:true,bubbles:true,cancelable:true}));assert.deepEqual(selectedButtons(a),[1]);
});
test('touch-friendly multi mode and explicit select-all keep numeric inputs protected',async t=>{
 const a=await multiWorkbench(t),{$}=a;$('[data-model="5"]').click();$('#sliceDuplicate').click();$('#sliceMultiToggle').click();pickObject(a,0);assert.deepEqual(selectedButtons(a),[0,1]);assert.equal($('#sliceMultiToggle').getAttribute('aria-pressed'),'true');$('#sliceSelectOnly').click();assert.equal(selectedButtons(a).length,1);assert.equal($('#sliceMultiToggle').getAttribute('aria-pressed'),'false');$('#sliceSelectAll').click();assert.deepEqual(selectedButtons(a),[0,1]);
});
test('quarter turns affect only selected placements, preserve size/camera and form one undo step',async t=>{
 const viewer=cameraViewer(),a=await multiWorkbench(t,{viewer}),{w,$,state}=a;$('[data-model="5"]').click();await settle();$('#sliceDuplicate').click();await settle();$('#sliceDuplicate').click();await settle();pickObject(a,0);pickObject(a,2,{ctrlKey:true});const camera=structuredClone(viewer.camera);
 $('[data-quarter="0"]').click();pickObject(a,0);assert.equal($('[data-rotate="0"]').value,'90');assert.match($('#sliceScaleSummary').textContent,/100%/);pickObject(a,1);assert.equal($('[data-rotate="0"]').value,'0');pickObject(a,2);assert.equal($('[data-rotate="0"]').value,'90');assert.deepEqual(viewer.camera,camera);
 $('#sliceUndo').click();await settle();assert.equal($('[data-rotate="0"]').value,'');assert.equal($('[data-rotate="0"]').disabled,true);pickObject(a,0);assert.equal($('[data-rotate="0"]').value,'0');$('#sliceRedo').click();await settle();assert.equal($('[data-rotate="0"]').value,'90');assert.deepEqual(viewer.camera,camera);assert.equal(state.calls.filter(c=>c.url.startsWith('/api/jobs/')||c.url==='/api/slicing/tasks').length,0);
 for(let i=0;i<3;i++)$('[data-quarter="0"]').click();assert.equal($('[data-rotate="0"]').value,'0');
});
test('multi copy duplicates the selected sources once and batch delete is confirmed and undoable',async t=>{
 const a=await multiWorkbench(t),{w,$}=a;$('[data-model="5"]').click();$('[data-add-model="6"]').click();pickObject(a,0);pickObject(a,1,{ctrlKey:true});$('#sliceDuplicate').click();assert.equal(w.document.querySelectorAll('[data-copy]').length,4);assert.deepEqual(selectedButtons(a),[2,3]);$('#sliceRemoveCopy').click();assert.match($('#modal h2').textContent,/2/);assert.equal(w.document.querySelectorAll('[data-copy]').length,4);$('[data-cancel]').click();assert.equal(w.document.querySelectorAll('[data-copy]').length,4);$('#sliceRemoveCopy').click();$('#confirmAction').click();await settle();assert.equal(w.document.querySelectorAll('[data-copy]').length,2);$('#sliceUndo').click();await settle();assert.equal(w.document.querySelectorAll('[data-copy]').length,4);assert.deepEqual(selectedButtons(a),[2,3]);
});
test('selected cross-plate transfer is atomic, inherits target process and can be undone once',async t=>{
 const a=await multiWorkbench(t),{w,$,state}=a;$('[data-model="5"]').click();$('[data-add-model="6"]').click();$('#slicePlateAdd').click();$('[data-model="5"]').click();change(w,$('#sliceSupport'),true);$('[data-plate="0"]').click();await settle();historyKey(w,'a');$('#slicePlateMove').click();const form=$('#plateMoveForm');form.elements.target.value='1';submitForm(w,form);await settle();assert.equal(w.document.querySelectorAll('[data-copy]').length,3);assert.equal($('#sliceSupport').checked,true);$('[data-plate="0"]').click();await settle();assert.equal(w.document.querySelectorAll('[data-copy]').length,0);$('#sliceUndo').click();await settle();assert.equal(w.document.querySelectorAll('[data-copy]').length,2);assert.equal(state.calls.filter(c=>c.url.startsWith('/api/jobs/')).length,0);
});
test('multi rotation draft persists exact per-object transforms and never stores selection UI fields',async t=>{
 const a=await multiWorkbench(t),{w,$,state}=a;$('[data-model="5"]').click();$('[data-add-model="6"]').click();historyKey(w,'a');$('[data-quarter="1"]').click();$('#draftSaveNow').click();await settle();const body=JSON.parse(state.calls.filter(c=>c.url==='/api/slicing/draft').at(-1).options.body);assert.ok(body.plates[0].options.copies.every(p=>p.rotation[1]===90&&p.scale===1));assert.equal(body.selection,undefined);assert.equal(body.plates[0].options.selection,undefined);$('#sliceCompute').click();await settle();const payload=JSON.parse(state.calls.find(c=>c.url==='/api/slicing/tasks').options.body);assert.ok(payload.options.copies.every(p=>p.rotation[1]===90));
});

const workbenchState=a=>JSON.parse(JSON.stringify(a.w.testing.workbench));
test('box results replace or add selections without changing geometry and ignore late callbacks after leaving',async t=>{
 const viewer=cameraViewer(),a=await multiWorkbench(t,{viewer}),{w,$}=a;$('[data-model="5"]').click();await settle();$('#sliceDuplicate').click();await settle();$('#sliceDuplicate').click();await settle();const before=workbenchState(a);
 $('[data-transform="box"]').click();viewer.callbacks.onSelectMany([0,1],{additive:false});assert.deepEqual(selectedButtons(a),[0,1]);viewer.callbacks.onSelectMany([2],{additive:true});assert.deepEqual(selectedButtons(a),[0,1,2]);viewer.callbacks.onSelectMany([],{additive:false});assert.deepEqual(selectedButtons(a),[0,1,2]);assert.deepEqual(workbenchState(a).copies,before.copies);assert.equal($('[data-transform="translate"]').disabled,false);assert.equal($('[data-transform="rotate"]').disabled,true);
 $('[data-page="board"]').click();viewer.callbacks.onSelectMany([0],{additive:false});assert.equal($('#sliceWorkbench'),null);
});
test('a grouped drag preserves relative transforms and untouched models, is one undo step and keeps the camera',async t=>{
 const viewer=cameraViewer(),a=await multiWorkbench(t,{viewer}),{w,$,state}=a;$('[data-model="5"]').click();await settle();$('[data-add-model="6"]').click();await settle();$('#sliceDuplicate').click();await settle();viewer.callbacks.onSelectMany([0,1],{additive:false});const before=workbenchState(a).copies,cx=(before[0].x+before[1].x)/2,cy=(before[0].y+before[1].y)/2,camera=structuredClone(viewer.camera);
 $('[data-transform="translate"]').click();viewer.callbacks.onGesture(true);viewer.callbacks.onTransform({x:cx+10,y:cy+5});viewer.callbacks.onTransform({x:cx+20,y:cy+15});viewer.callbacks.onGesture(false);const after=workbenchState(a).copies;
 for(let i=0;i<2;i++){assert.equal(after[i].x-before[i].x,20);assert.equal(after[i].y-before[i].y,15);assert.deepEqual(after[i].rotation,before[i].rotation);assert.equal(after[i].scale,before[i].scale)}assert.deepEqual(after[2],before[2]);assert.deepEqual(viewer.camera,camera);
 $('#sliceUndo').click();await settle();assert.deepEqual(workbenchState(a).copies,before);$('#sliceRedo').click();await settle();assert.deepEqual(workbenchState(a).copies,after);assert.deepEqual(viewer.camera,camera);$('#draftSaveNow').click();await settle();const body=JSON.parse(state.calls.filter(c=>c.url==='/api/slicing/draft').at(-1).options.body);assert.deepEqual(body.plates[0].options.copies,after);assert.equal(state.calls.filter(c=>c.url.startsWith('/api/jobs/')).length,0);
});
test('grouped movement rejects nonfinite and out-of-editor positions atomically; box shortcuts ignore fields',async t=>{
 const viewer=cameraViewer(),a=await multiWorkbench(t,{viewer}),{w,$}=a;$('[data-model="5"]').click();await settle();$('#sliceDuplicate').click();await settle();$('#sliceSelectAll').click();$('[data-transform="translate"]').click();const before=workbenchState(a).copies;
 viewer.callbacks.onTransform({x:999,y:999});assert.deepEqual(workbenchState(a).copies,before);viewer.callbacks.onTransform({x:Infinity,y:10});assert.deepEqual(workbenchState(a).copies,before);
 w.document.body.dispatchEvent(new w.KeyboardEvent('keydown',{key:'b',bubbles:true}));assert.equal($('[data-transform="box"]').getAttribute('aria-pressed'),'true');$('[data-transform="select"]').click();$('[data-place="x"]').dispatchEvent(new w.KeyboardEvent('keydown',{key:'b',bubbles:true}));assert.equal($('[data-transform="select"]').getAttribute('aria-pressed'),'true');
});

function objectSetting(a,key,mode,value){change(a.w,a.$('[data-object-mode="'+key+'"]'),mode);if(value!==undefined)numberInput(a.w,a.$('[data-object-value="'+key+'"]'),value)}
function applyObjectSettings(a){submitForm(a.w,a.$('#objectProcessForm'))}
test('single-object process applies only to its stable instance and persists without polluting plate fields',async t=>{
 const viewer=cameraViewer(),a=await multiWorkbench(t,{viewer}),{w,$,state}=a;$('[data-model="5"]').click();await settle();$('#sliceDuplicate').click();await settle();const before=workbenchState(a),camera=structuredClone(viewer.camera);assert.notEqual(before.copies[0].instanceId,before.copies[1].instanceId);
 $('#sliceObjectProcess').click();assert.equal($('#objectProcessApply').disabled,true);objectSetting(a,'walls','custom',4);objectSetting(a,'infill','custom',30);assert.match($('#objectProcessPreview').textContent,/尚未应用/);assert.equal(workbenchState(a).copies[1].process,undefined);applyObjectSettings(a);let after=workbenchState(a);assert.equal(after.copies[0].process,undefined);assert.deepEqual(after.copies[1].process,{walls:4,infill:30});assert.deepEqual(after.copies.map(p=>p.instanceId),before.copies.map(p=>p.instanceId));assert.deepEqual(viewer.camera,camera);
 $('#draftSaveNow').click();await settle();const body=JSON.parse(state.calls.filter(c=>c.url==='/api/slicing/draft').at(-1).options.body);assert.equal(body.options.process,undefined);assert.equal(body.options.instanceId,undefined);assert.deepEqual(body.plates[0].options.copies[1].process,{walls:4,infill:30});$('#sliceUndo').click();await settle();assert.equal(workbenchState(a).copies[1].process,undefined);$('#sliceRedo').click();await settle();assert.deepEqual(workbenchState(a).copies[1].process,{walls:4,infill:30});
});
test('multi-object process changes only explicitly selected fields, keeps mixed values, and restores inheritance after confirmation',async t=>{
 const a=await multiWorkbench(t),{w,$}=a;$('[data-model="5"]').click();$('#sliceDuplicate').click();
 for(const [i,walls,infill]of [[0,4,30],[1,6,10]]){pickObject(a,i);$('#sliceObjectProcess').click();objectSetting(a,'walls','custom',walls);objectSetting(a,'infill','custom',infill);applyObjectSettings(a)}$('#sliceSelectAll').click();$('#sliceObjectProcess').click();assert.equal($('[data-object-mode="walls"]').value,'keep');assert.match($('#objectProcessForm').textContent,/多个设置/);objectSetting(a,'infill','custom',20);applyObjectSettings(a);assert.deepEqual(workbenchState(a).copies.map(p=>p.process),[{walls:4,infill:20},{walls:6,infill:20}]);
 $('#sliceObjectProcess').click();$('#objectProcessInheritAll').click();assert.ok(workbenchState(a).copies.every(p=>p.process));applyObjectSettings(a);assert.ok(workbenchState(a).copies.every(p=>p.process===undefined));$('#sliceUndo').click();await settle();assert.deepEqual(workbenchState(a).copies.map(p=>p.process),[{walls:4,infill:20},{walls:6,infill:20}]);
});
test('object process cancel, invalid input, stale selection and stale dialogs never change parameters',async t=>{
 const a=await multiWorkbench(t),{w,$,state}=a;$('[data-model="5"]').click();$('#sliceDuplicate').click();$('#sliceObjectProcess').click();objectSetting(a,'walls','custom',7);assert.equal($('#objectProcessApply').disabled,true);applyObjectSettings(a);assert.equal(workbenchState(a).copies[1].process,undefined);objectSetting(a,'walls','custom',4);const form=$('#objectProcessForm');$('[data-cancel]').click();submitForm(w,form);assert.equal(workbenchState(a).copies[1].process,undefined);
 $('#sliceObjectProcess').click();objectSetting(a,'walls','custom',4);pickObject(a,0);applyObjectSettings(a);assert.match($('#objectProcessError').textContent,/已变化/);assert.ok(workbenchState(a).copies.every(p=>!p.process));$('[data-cancel]').click();$('#sliceObjectProcess').click();const stale=$('#objectProcessForm');$('[data-page="board"]').click();submitForm(w,stale);assert.equal($('#sliceWorkbench'),null);assert.equal(state.calls.filter(c=>c.url==='/api/slicing/tasks'||c.url.startsWith('/api/jobs/')).length,0);
});
test('copy and new-plate copies inherit overrides but get fresh identities; cross-plate movement preserves identity',async t=>{
 const a=await multiWorkbench(t),{w,$}=a;$('[data-model="5"]').click();$('#sliceObjectProcess').click();objectSetting(a,'walls','custom',4);applyObjectSettings(a);const original=workbenchState(a).copies[0].instanceId;$('#sliceDuplicate').click();let copies=workbenchState(a).copies;assert.notEqual(copies[1].instanceId,original);assert.deepEqual(copies[1].process,{walls:4});const moved=copies[1].instanceId;$('#slicePlateMove').click();$('#plateMoveForm [name="target"]').value='new';submitForm(w,$('#plateMoveForm'));await settle();assert.equal(workbenchState(a).copies[0].instanceId,moved);assert.deepEqual(workbenchState(a).copies[0].process,{walls:4});assert.equal(workbenchState(a).process,undefined);
 $('#sliceBatch').click();numberInput(w,$('#sliceBatchAmount'),3);submitForm(w,$('#sliceBatchForm'));await settle();copies=workbenchState(a).copies;assert.equal(new Set(copies.map(p=>p.instanceId)).size,3);assert.ok(copies.every(p=>p.instanceId!==moved&&p.process.walls===4));assert.equal(workbenchState(a).instanceId,undefined);
});
test('legacy projects acquire IDs on editing without implicit overrides, and slicing declares its schema version',async t=>{
 const legacy={modelId:5,options:{x:100,y:120,scale:1,rotation:[0,0,0],layer:.2,infill:15,support:false,brim:false,color:'#FFFFFF'},activeCopy:0};const a=await multiWorkbench(t,{recovery:legacy}),{$,state}=a;$('#draftRecover').click();await settle();const copies=workbenchState(a).copies;assert.ok(copies[0].instanceId);assert.equal(copies[0].process,undefined);$('#sliceCompute').click();await settle();const body=JSON.parse(state.calls.find(c=>c.url==='/api/slicing/tasks').options.body);assert.equal(body.clientVersion,2);assert.equal(body.options.copies[0].instanceId,copies[0].instanceId);
});

function setModelSize(a,percent){a.$('#sliceResize').click();numberInput(a.w,a.$('#modelSizePercent'),percent);submitForm(a.w,a.$('#modelSizeForm'))}

test('model sizing is secondary and edits are a preview until explicit apply; S cannot arm scaling',async t=>{
 const a=await multiWorkbench(t),{w,$,state}=a;
 assert.equal($('#sliceResize').disabled,true);assert.equal($('[data-place="scale"]'),null);assert.equal($('[data-transform="scale"]'),null);
 $('[data-model="5"]').click();w.document.body.dispatchEvent(new w.KeyboardEvent('keydown',{key:'s',bubbles:true}));assert.equal($('[data-transform="select"]').getAttribute('aria-pressed'),'true');assert.equal($('#modal').open,false);
 $('#sliceResize').click();assert.equal($('#modelSizePercent').value,'100');assert.match($('.model-size-original').textContent,/20 × 20 × 10 mm/);numberInput(w,$('#modelSizePercent'),150);assert.equal($('[data-size-axis="0"]').textContent,'30 mm');assert.match($('#sliceScaleSummary').textContent,/100%/);
 const staleForm=$('#modelSizeForm');$('[data-cancel]').click();submitForm(w,staleForm);assert.match($('#sliceScaleSummary').textContent,/100%/);assert.equal(state.calls.filter(c=>c.url.startsWith('/api/jobs/')||c.url==='/api/slicing/tasks').length,0);
});

test('model size apply affects one object, preserves placement and camera, and reset is confirmed and undoable',async t=>{
 const viewer=cameraViewer(),a=await multiWorkbench(t,{viewer}),{w,$,state}=a;$('[data-model="5"]').click();await settle();$('#sliceDuplicate').click();await settle();
 const x=$('[data-place="x"]').value,y=$('[data-place="y"]').value;viewer.camera={position:[300,160,90],target:[100,90,4],zoom:1.7};const camera=structuredClone(viewer.camera);
 setModelSize(a,150);assert.match($('#sliceScaleSummary').textContent,/150%/);assert.equal($('[data-place="x"]').value,x);assert.equal($('[data-place="y"]').value,y);assert.deepEqual(viewer.camera,camera);
 $('[data-copy="0"]').click();assert.match($('#sliceScaleSummary').textContent,/100%/);$('[data-copy="1"]').click();assert.match($('#sliceScaleSummary').textContent,/150%/);
 $('#sliceResize').click();$('#modelSizeReset').click();assert.equal($('#modelSizePercent').value,'100');assert.match($('#sliceScaleSummary').textContent,/150%/);submitForm(w,$('#modelSizeForm'));assert.match($('#sliceScaleSummary').textContent,/100%/);
 $('#sliceUndo').click();await settle();assert.match($('#sliceScaleSummary').textContent,/150%/);assert.deepEqual(viewer.camera,camera);$('#sliceRedo').click();await settle();assert.match($('#sliceScaleSummary').textContent,/100%/);
 assert.equal(state.calls.filter(c=>c.url.startsWith('/api/jobs/')||c.url==='/api/slicing/tasks').length,0);
});

test('sizing rejects invalid values and a stale selected object without changing either object',async t=>{
 const a=await multiWorkbench(t),{w,$}=a;$('[data-model="5"]').click();$('#sliceDuplicate').click();$('#sliceResize').click();
 for(const value of ['','0','1001','-5']){numberInput(w,$('#modelSizePercent'),value);assert.equal($('#modelSizeApply').disabled,true);submitForm(w,$('#modelSizeForm'));assert.match($('#sliceScaleSummary').textContent,/100%/)}
 numberInput(w,$('#modelSizePercent'),175);$('[data-copy="0"]').click();submitForm(w,$('#modelSizeForm'));assert.match($('#modelSizeError').textContent,/选中对象或摆放已变化/);assert.match($('#sliceScaleSummary').textContent,/100%/);
});

test('legacy scaled projects keep their dimensions until an explicit size change',async t=>{
 const options={x:100,y:120,rotation:[0,0,45],scale:1.4,layer:.2,infill:15,support:false,brim:false,color:'#FFFFFF'};
 const a=await multiWorkbench(t,{recovery:{modelId:5,options,activeCopy:0}}),{$}=a;$('#draftRecover').click();await settle();assert.match($('#sliceScaleSummary').textContent,/140%/);$('#sliceResize').click();assert.equal($('#modelSizePercent').value,'140');assert.equal($('[data-size-axis="0"]').textContent,'28 mm');assert.match($('.model-size-root .form-help').textContent,/模型原始方向/);
});

function cameraViewer(){return {camera:{position:[425,-300,340],target:[128,128,16],zoom:1},views:[],loads:0,set(){},loadModels(){this.loads++},view(name){this.views.push(name);this.camera={position:[100,-100,90],target:[10,10,5],zoom:1}},finish(){},dispose(){},editing(){},clearPaths(){},paths(){},tool(){}}}
function historyKey(w,key){w.document.body.dispatchEvent(new w.KeyboardEvent('keydown',{key,ctrlKey:true,bubbles:true,cancelable:true}))}

test('Ctrl+Z, Ctrl+Y and toolbar history preserve the current orbit, pan and distance through asynchronous model reloads',async t=>{
 const viewer=cameraViewer(),a=await multiWorkbench(t,{viewer}),{w,$,state}=a;
 $('[data-model="5"]').click();await settle();assert.equal(viewer.views.length,1);
 numberInput(w,$('[data-place="x"]'),145);viewer.camera={position:[410,-125,245],target:[100,90,18],zoom:1.4};const current=structuredClone(viewer.camera);
 historyKey(w,'z');await settle();assert.equal($('[data-place="x"]').value,'138.25');assert.deepEqual(viewer.camera,current);assert.equal(viewer.views.length,1);
 viewer.camera={position:[380,-20,190],target:[85,110,10],zoom:1.8};const newer=structuredClone(viewer.camera);
 historyKey(w,'y');await settle();assert.equal($('[data-place="x"]').value,'145');assert.deepEqual(viewer.camera,newer);
 $('#sliceUndo').click();await settle();$('#sliceRedo').click();await settle();assert.deepEqual(viewer.camera,newer);assert.ok(viewer.loads>=5);
 $('[data-view="fit"]').click();assert.equal(viewer.views.at(-1),'fit');assert.notDeepEqual(viewer.camera,newer);
 assert.equal(state.calls.filter(c=>c.url==='/api/slicing/tasks'||c.url.startsWith('/api/jobs/')).length,0);
});

test('history keeps the camera while restoring copied objects and an empty plate',async t=>{
 const viewer=cameraViewer(),a=await multiWorkbench(t,{viewer}),{w,$}=a;$('[data-model="5"]').click();await settle();$('#sliceDuplicate').click();await settle();assert.equal(w.document.querySelectorAll('[data-copy]').length,2);
 viewer.camera={position:[500,100,160],target:[130,120,12],zoom:1.2};const current=structuredClone(viewer.camera),fits=viewer.views.length;
 historyKey(w,'z');await settle();assert.equal(w.document.querySelectorAll('[data-copy]').length,1);assert.deepEqual(viewer.camera,current);
 historyKey(w,'z');await settle();assert.equal(w.document.querySelectorAll('[data-copy]').length,0);assert.deepEqual(viewer.camera,current);
 historyKey(w,'y');await settle();historyKey(w,'y');await settle();assert.equal(w.document.querySelectorAll('[data-copy]').length,2);assert.deepEqual(viewer.camera,current);assert.equal(viewer.views.length,fits);
});

test('a late auto-frame request cannot reset the camera after a newer history operation',async t=>{
 const viewer=cameraViewer(),a=await multiWorkbench(t,{viewer}),{w,$}=a;$('[data-model="5"]').click();await settle();const original=w.fetch;let release,hold=true;
 w.fetch=async(url,opts={})=>{if(url.startsWith('/api/slicing/model?')&&hold){hold=false;return new Promise(r=>release=()=>r({ok:true,arrayBuffer:async()=>new ArrayBuffer(84)}))}return original(url,opts)};
 $('#sliceDuplicate').click();await settle();assert.ok(release);viewer.camera={position:[350,220,180],target:[110,100,9],zoom:2};const current=structuredClone(viewer.camera),fits=viewer.views.length;
 historyKey(w,'z');await settle();assert.deepEqual(viewer.camera,current);release();await settle();assert.deepEqual(viewer.camera,current);assert.equal(viewer.views.length,fits);assert.equal(w.document.querySelectorAll('[data-copy]').length,1);
});

async function importWorkbench(t,{lostCommit=false,deferReview=false,rejectReview=false}={}){
 const a=await multiWorkbench(t),{w,$,state}=a,original=w.fetch;let resolveReview,commits=0;
 const setup={x:100,y:120,rotation:[0,0,0],scale:1,layer:.2,infill:15,color:'#FFFFFF',support:false,brim:false};
 const project={id:7,name:'导入工程',revision:1,modelId:5,options:{...setup,copies:[{modelId:5,x:100,y:120,rotation:[0,0,0],scale:1}]},activeCopy:0,plates:[{id:'import-one',name:'第一盘',modelId:5,activeCopy:0,options:{...setup,copies:[{modelId:5,x:100,y:120,rotation:[0,0,0],scale:1}]}},{id:'import-two',name:'第二盘',modelId:6,activeCopy:0,options:{...setup,copies:[{modelId:6,x:140,y:120,rotation:[0,0,0],scale:1}]}},{id:'import-empty',name:'备用盘',modelId:0,activeCopy:0,options:{...setup,copies:[]}}]};
 const review={sha256:'a'.repeat(64),objects:2,skipped:1,hasGcode:true,plates:[{name:'<img src=x>',objects:1,triangles:12},{name:'第二盘',objects:1,triangles:12},{name:'备用盘',objects:0,triangles:0}],sources:[{name:'<private>',size:[20,20,10],triangles:12},{name:'box',size:[12,40,18],triangles:12}]};
 w.fetch=async(url,options={})=>{
  if(url==='/api/slicing/import-3mf'){state.calls.push({url,options});const mode=options.body.get('mode');if(mode==='review'){if(rejectReview)return reply(400,{ok:false,error:'该文件只有切片路径，请另存工程 3MF'});if(deferReview)return new Promise(r=>resolveReview=()=>r(reply(200,{ok:true,review})));return reply(200,{ok:true,review})}commits++;if(lostCommit&&commits===1)throw Error('connection lost after commit');return reply(201,{ok:true,project,replayed:commits>1})}
  if(url==='/api/slicing/project?id=7')return reply(200,{ok:true,project});return original(url,options);
 };
 const chooseFiles=files=>{Object.defineProperty($('#sliceInput'),'files',{value:files,configurable:true});$('#sliceInput').dispatchEvent(new w.Event('change',{bubbles:true}))};
 return {...a,chooseFiles,file:new w.File(['fixture'],'工程.3mf'),resolve:()=>resolveReview(),project};
}
test('3MF import review escapes names, explains replacement settings and requires explicit agreement before committing',async t=>{
 const a=await importWorkbench(t),{w,$}=a;a.chooseFiles([a.file]);await settle();assert.match($('#modalContent').textContent,/重新排版/);assert.match($('#modalContent').textContent,/温度和自定义工艺不继承/);assert.match($('#modalContent').textContent,/G-code 不会导入或执行/);assert.match($('#modalContent').textContent,/跳过 1/);assert.match($('#modalContent').textContent,/<img src=x>/);assert.equal($('#import3mfPanel img'),null);assert.equal($('#import3mfCommit').disabled,true);assert.equal(writes(a).filter(c=>c.url==='/api/slicing/import-3mf').length,1);
 submitForm(w,$('#import3mfPanel'));await settle();assert.equal(writes(a).length,1);choose(w,$('#import3mfAgree'));submitForm(w,$('#import3mfPanel'));await settle();const commits=writes(a).filter(c=>c.url==='/api/slicing/import-3mf'&&c.options.body.get('mode')==='commit');assert.equal(commits.length,1);assert.equal(commits[0].options.body.get('acknowledge'),'geometry-only');assert.equal(commits[0].options.body.get('sha256'),'a'.repeat(64));assert.equal(w.document.querySelectorAll('[data-plate]').length,3);assert.equal(writes(a).some(c=>/slicing\/(tasks|promote)|jobs\/(start|send)/.test(c.url)),false);
});
test('3MF review cancellation leaves the current plate and numeric edits untouched',async t=>{
 const a=await importWorkbench(t),{w,$}=a;$('[data-model="5"]').click();numberInput(w,$('[data-place="x"]'),155);a.chooseFiles([a.file]);await settle();$('#modal [data-cancel]').click();await settle();assert.equal($('#modal').open,false);assert.equal($('[data-place="x"]').value,'155');assert.equal(w.document.querySelectorAll('[data-plate]').length,1);assert.equal(writes(a).filter(c=>c.url==='/api/slicing/import-3mf'&&c.options.body.get('mode')==='commit').length,0);
});
test('an uncertain 3MF import retries with the same receipt key and never automatically resubmits',async t=>{
 const a=await importWorkbench(t,{lostCommit:true}),{w,$}=a;a.chooseFiles([a.file]);await settle();choose(w,$('#import3mfAgree'));submitForm(w,$('#import3mfPanel'));await settle();assert.match($('#import3mfError').textContent,/没有收到导入确认/);assert.match($('#import3mfCommit').textContent,/核对并重试/);const first=writes(a).filter(c=>c.url==='/api/slicing/import-3mf'&&c.options.body.get('mode')==='commit');assert.equal(first.length,1);submitForm(w,$('#import3mfPanel'));submitForm(w,$('#import3mfPanel'));await settle();const calls=writes(a).filter(c=>c.url==='/api/slicing/import-3mf'&&c.options.body.get('mode')==='commit');assert.equal(calls.length,2);assert.equal(calls[0].options.body.get('requestKey'),calls[1].options.body.get('requestKey'));assert.equal(w.document.querySelectorAll('[data-plate]').length,3);
});
test('a late 3MF review cannot replace another page or create a project after leaving',async t=>{
 const a=await importWorkbench(t,{deferReview:true}),{w,$}=a;a.chooseFiles([a.file]);await settle();const reviewCall=writes(a).find(c=>c.url==='/api/slicing/import-3mf');$('[data-page="board"]').click();await settle();assert.equal(reviewCall.options.signal.aborted,true);a.resolve();await settle();assert.equal($('#import3mfPanel'),null);assert.equal($('#modal').open,false);assert.equal(writes(a).filter(c=>c.url==='/api/slicing/import-3mf').length,1);
});
test('3MF geometry rejection explains how to obtain an editable project and does not affect the current model',async t=>{
 const a=await importWorkbench(t,{rejectReview:true}),{w,$}=a;$('[data-model="5"]').click();a.chooseFiles([a.file]);await settle();assert.match($('#modalContent').textContent,/只有切片路径/);assert.match($('#modalContent').textContent,/另存包含模型的工程/);assert.equal($('#import3mfCommit'),null);assert.equal(w.document.querySelectorAll('[data-copy]').length,1);assert.equal(writes(a).filter(c=>c.url==='/api/slicing/import-3mf').length,1);
});
test('3MF import enforces one project at a time and the file size limit before uploading',async t=>{
 const a=await importWorkbench(t),{w,$}=a;assert.match($('#sliceInput').accept,/\.3mf/);a.chooseFiles([a.file,new w.File(['stl'],'a.stl')]);await settle();assert.match($('#sliceMessage').textContent,/单独导入/);a.chooseFiles([{name:'big.3mf',size:33*1024*1024}]);await settle();assert.match($('#toast').textContent,/32 MB/);assert.equal(writes(a).filter(c=>c.url==='/api/slicing/import-3mf').length,0);
});
test('multiple boards preserve independent process and object settings; saves include empty active board and all sources',async t=>{
 const a=await multiWorkbench(t),{w,$}=a;$('[data-model="5"]').click();numberInput(w,$('[data-place="x"]'),100);change(w,$('#sliceProcessPreset'),'strong');
 $('#slicePlateAdd').click();assert.equal($('#sliceCompute').disabled,true);assert.equal(w.document.querySelectorAll('[data-copy]').length,0);$('[data-model="6"]').click();numberInput(w,$('[data-place="x"]'),170);change(w,$('#sliceProcessPreset'),'draft');
 $('[data-plate="0"]').click();assert.equal($('[data-place="x"]').value,'100');assert.equal($('#sliceWalls').value,'4');assert.match($('#sliceCopies').textContent,/cube/);
 $('[data-plate="1"]').click();assert.equal($('[data-place="x"]').value,'170');assert.equal($('#sliceWalls').value,'2');assert.match($('#sliceCopies').textContent,/box/);
 $('#slicePlateAdd').click();$('#draftSaveNow').click();await settle();const saved=JSON.parse(writes(a).at(-1).options.body);assert.equal(saved.activePlate,2);assert.equal(saved.plates.length,3);assert.equal(saved.plates[2].modelId,0);assert.deepEqual(saved.plates.map(p=>p.options.copies.map(o=>o.modelId)),[[5],[6],[]]);assert.equal(saved.modelId,5);assert.equal(saved.plates[0].options.advanced.walls,4);assert.equal(writes(a).every(c=>c.url==='/api/slicing/draft'),true);
});
test('moving the last object leaves an empty board, preserves destination process and can undo the entire move',async t=>{
 const a=await multiWorkbench(t),{w,$}=a;$('[data-model="5"]').click();$('#slicePlateAdd').click();$('[data-model="6"]').click();change(w,$('#sliceProcessPreset'),'strong');$('[data-plate="0"]').click();
 $('#slicePlateMove').click();$('#plateMoveForm [name=target]').value='1';submitForm(w,$('#plateMoveForm'));assert.equal($('#modal').open,false);assert.equal(w.document.querySelectorAll('[data-copy]').length,2);assert.equal($('#sliceWalls').value,'4');assert.equal($('[data-plate="1"]').getAttribute('aria-selected'),'true');
 $('#draftSaveNow').click();await settle();const saved=JSON.parse(writes(a).at(-1).options.body);assert.equal(saved.plates.every(p=>p.options.modelId===undefined),true);assert.deepEqual(saved.plates.map(p=>p.modelId),[0,6]);
 $('[data-plate="0"]').click();assert.equal(w.document.querySelectorAll('[data-copy]').length,0);assert.equal($('#sliceCompute').disabled,true);$('#sliceUndo').click();assert.equal(w.document.querySelectorAll('[data-copy]').length,1);assert.match($('#sliceCopies').textContent,/cube/);$('[data-plate="1"]').click();assert.equal(w.document.querySelectorAll('[data-copy]').length,1);assert.match($('#sliceCopies').textContent,/box/);assert.equal(writes(a).every(c=>c.url==='/api/slicing/draft'),true);
});
test('failed cross-board packing is atomic and cannot lose either board',async t=>{
 const a=await multiWorkbench(t),{w,$,models}=a;$('[data-model="5"]').click();$('#slicePlateAdd').click();$('[data-model="6"]').click();$('[data-plate="0"]').click();models[1].bounds.size=[300,12,18];
 $('#slicePlateMove').click();$('#plateMoveForm [name=target]').value='1';submitForm(w,$('#plateMoveForm'));assert.match($('#plateMoveForm [role=alert]').textContent,/原布局已保留/);assert.equal(w.document.querySelectorAll('[data-copy]').length,1);assert.equal($('[data-plate="0"]').getAttribute('aria-selected'),'true');$('#modal').close();$('[data-plate="1"]').click();assert.equal(w.document.querySelectorAll('[data-copy]').length,1);assert.equal(writes(a).length,0);
});
test('a delayed slice submission stays attached to its origin board and editing another board cannot invalidate it',async t=>{
 const a=await multiWorkbench(t,{deferSlice:true}),{w,$}=a;$('[data-model="5"]').click();$('#slicePlateAdd').click();$('[data-model="6"]').click();$('[data-plate="0"]').click();$('#sliceCompute').click();await settle();$('[data-plate="1"]').click();numberInput(w,$('[data-place="x"]'),175);a.resolve();await settle();assert.equal($('#sliceWorkbench').dataset.mode,'prepare');assert.match($('#sliceCopies').textContent,/box/);
 $('#draftSaveNow').click();await settle();let saved=JSON.parse(writes(a).at(-1).options.body);assert.equal(saved.plates[0].taskId,1);assert.equal(saved.plates[1].taskId||0,0);assert.equal(saved.activePlate,1);
 $('[data-plate="0"]').click();await settle();assert.match($('#sliceResult').textContent,/等待切片/);assert.equal($('#sliceWorkbench').dataset.mode,'preview');$('#studioPrepare').click();numberInput(w,$('[data-place="x"]'),110);$('#draftSaveNow').click();await settle();saved=JSON.parse(writes(a).at(-1).options.body);assert.equal(saved.plates[0].taskId||0,0);assert.equal(writes(a).filter(c=>c.url==='/api/slicing/tasks').length,1);
});
test('edits made while a slice is being submitted cannot acquire its outdated result',async t=>{
 const a=await multiWorkbench(t,{deferSlice:true}),{w,$}=a;$('[data-model="5"]').click();$('#sliceCompute').click();await settle();numberInput(w,$('[data-place="x"]'),150);a.resolve();await settle();$('#draftSaveNow').click();await settle();const saved=JSON.parse(writes(a).at(-1).options.body);assert.equal(saved.plates[0].taskId||0,0);assert.equal(saved.plates[0].options.copies[0].x,150);assert.equal($('#sliceWorkbench').dataset.mode,'prepare');
});
test('board names are escaped, empty board deletion is undoable and source deletion checks inactive boards',async t=>{
 const a=await multiWorkbench(t),{w,$}=a;$('[data-model="5"]').click();$('#slicePlateAdd').click();$('#slicePlateRename').click();$('#plateNameForm [name=plateName]').value='<img src=x>';submitForm(w,$('#plateNameForm'));assert.equal($('#slicePlates img'),null);assert.match($('#slicePlates').textContent,/<img src=x>/);
 $('[data-delete-model="5"]').click();assert.match($('#sliceMessage').textContent,/工程中的打印板/);assert.equal(writes(a).length,0);$('#slicePlateDelete').click();assert.equal(w.document.querySelectorAll('[data-plate]').length,1);$('#sliceUndo').click();assert.equal(w.document.querySelectorAll('[data-plate]').length,2);assert.match($('#slicePlates').textContent,/<img src=x>/);
 for(let i=2;i<8;i++)$('#slicePlateAdd').click();assert.equal(w.document.querySelectorAll('[data-plate]').length,8);assert.equal($('#slicePlateAdd').disabled,true);
});
test('multi-board draft recovery retains every board and active empty board without issuing slices or prints',async t=>{
 const options={x:100,y:120,scale:1,rotation:[0,0,0],layer:.2,infill:15,color:'#FFFFFF',brim:false,support:false};const recovery={modelId:5,options,activeCopy:0,activePlate:2,plates:[{id:'one',name:'外壳',modelId:5,options:{...options,copies:[{modelId:5,x:100,y:120,scale:1,rotation:[0,0,0]}]},activeCopy:0},{id:'two',name:'底座',modelId:6,options:{...options,layer:.16,copies:[{modelId:6,x:160,y:120,scale:1,rotation:[0,0,0]}]},activeCopy:0},{id:'empty',name:'备用盘',modelId:0,options:{...options,copies:[]},activeCopy:0}]};
 const a=await multiWorkbench(t,{recovery}),{w,$}=a;$('#draftRecover').click();await settle();assert.equal(w.document.querySelectorAll('[data-plate]').length,3);assert.equal($('[data-plate="2"]').getAttribute('aria-selected'),'true');assert.equal($('#sliceCompute').disabled,true);$('[data-plate="1"]').click();assert.equal($('[data-place="x"]').value,'160');assert.equal($('[data-layer="0.16"]').classList.contains('active'),true);$('#draftSaveNow').click();await settle();const saved=JSON.parse(writes(a).at(-1).options.body);assert.equal(saved.plates.length,3);assert.equal(saved.plates[1].modelId,6);assert.equal(writes(a).every(c=>c.url==='/api/slicing/draft'),true);
});
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

test('sidebar collapse is explicit, persists across pages and initial loads, and never writes business data',async t=>{
 const a=await app(t),{w,$}=a;
 assert.equal(w.document.documentElement.classList.contains('nav-collapsed'),false);assert.equal($('#menuBtn').getAttribute('aria-expanded'),'true');
 $('#menuBtn').click();assert.equal(w.document.documentElement.classList.contains('nav-collapsed'),true);assert.equal(w.localStorage.getItem('layerhub.ui.sidebarCollapsed'),'1');assert.equal($('#menuBtn').getAttribute('aria-label'),'展开侧栏');assert.equal($('[data-page="slicer"]').getAttribute('aria-label'),'模型工作台');
 $('[data-page="slicer"]').click();await settle();assert.equal(w.document.documentElement.classList.contains('nav-collapsed'),true);$('[data-page="board"]').click();assert.equal(w.document.documentElement.classList.contains('nav-collapsed'),true);
 const reloaded=await app(t,{sidebarPreference:'1'});assert.equal(reloaded.w.document.documentElement.classList.contains('nav-collapsed'),true);assert.equal(reloaded.$('#menuBtn').getAttribute('aria-expanded'),'false');
 $('#menuBtn').click();assert.equal(w.document.documentElement.classList.contains('nav-collapsed'),false);assert.equal(w.localStorage.getItem('layerhub.ui.sidebarCollapsed'),'0');assert.equal(writes(a).length,0);
});

test('mobile drawer keeps the desktop preference, closes with Escape and updates accessibility state',async t=>{
 const a=await app(t,{sidebarPreference:'1'}),{w,$}=a;
 w.innerWidth=390;w.dispatchEvent(new w.Event('resize'));assert.equal($('#menuBtn').getAttribute('aria-label'),'打开导航');$('#menuBtn svg path').dispatchEvent(new w.MouseEvent('click',{bubbles:true,composed:true}));assert.equal($('#sidebar').classList.contains('open'),true);assert.equal($('#menuBtn').getAttribute('aria-expanded'),'true');assert.equal(w.localStorage.getItem('layerhub.ui.sidebarCollapsed'),'1');
 w.document.body.dispatchEvent(new w.KeyboardEvent('keydown',{key:'Escape',bubbles:true}));assert.equal($('#sidebar').classList.contains('open'),false);assert.equal($('#menuBtn').getAttribute('aria-expanded'),'false');
 $('#menuBtn').click();w.document.body.click();assert.equal($('#sidebar').classList.contains('open'),false);assert.equal($('#menuBtn').getAttribute('aria-expanded'),'false');
 w.innerWidth=1280;w.dispatchEvent(new w.Event('resize'));assert.equal($('#menuBtn').getAttribute('aria-label'),'展开侧栏');assert.equal(writes(a).length,0);
});

test('model workbench preserves editing across refresh and only slices after explicit submission',async t=>{
 const a=await app(t),{w,$,state}=a;const original=w.fetch;
 const model={id:5,filename:'<private>.stl',triangles:12,bounds:{min:[-10,-10,0],max:[10,10,10],size:[20,20,10]}};
 w.fetch=async(url,options={})=>{if(url==='/api/slicing'){state.calls.push({url,options});return reply(200,{ok:true,ready:true,models:[model],tasks:[]})}if(url==='/api/slicing/tasks'){state.calls.push({url,options});return reply(200,{ok:true,taskId:8})}return original(url,options)};
 $('[data-page="slicer"]').click();await settle();assert.match($('#sliceModels').textContent,/<private>/);assert.equal($('#sliceModels private'),null);assert.equal(writes(a).length,0);
 $('[data-model="5"]').click();await settle();const input=$('[data-place="x"]');input.value='151.5';input.dispatchEvent(new w.Event('input',{bubbles:true}));$('[data-layer="0.16"]').click();
 await a.Remote.refresh({renderPage:false});assert.equal($('[data-place="x"]').value,'151.5');assert.equal(writes(a).length,0);
 $('#sliceCompute').click();await settle();const call=writes(a).find(c=>c.url==='/api/slicing/tasks');assert.ok(call);const payload=JSON.parse(call.options.body);assert.equal(payload.modelId,5);assert.equal(payload.options.x,151.5);assert.equal(payload.options.layer,.16);assert.equal(writes(a).some(c=>/jobs\/(start|send)|slicing\/promote/.test(c.url)),false);
});

test('copies keep independent placement, shortcuts and failed packing preserve the plate without device writes',async t=>{
 const a=await app(t),{w,$,state}=a,original=w.fetch;
 const model={id:5,filename:'cube.stl',triangles:12,bounds:{min:[-40,-40,0],max:[40,40,10],size:[80,80,10]}};
 w.fetch=async(url,options={})=>{if(url==='/api/slicing')return reply(200,{ok:true,ready:true,models:[model],tasks:[]});if(url==='/api/slicing/tasks'){state.calls.push({url,options});return reply(200,{ok:true,taskId:9})}return original(url,options)};
 $('[data-page="slicer"]').click();await settle();$('[data-model="5"]').click();await settle();$('#sliceDuplicate').click();assert.equal($('#sliceCopyCount').value,'2');
 $('[data-copy="1"]').click();const first=$('[data-copy="0"]');const x=$('[data-place="x"]');x.value='180';x.dispatchEvent(new w.Event('input',{bubbles:true}));first.click();assert.notEqual(Number(x.value),180);
 w.document.body.dispatchEvent(new w.KeyboardEvent('keydown',{key:'d',ctrlKey:true,bubbles:true,cancelable:true}));assert.equal($('#sliceCopyCount').value,'3');
 $('#sliceCopyCount').value='64';$('#sliceApplyCount').click();assert.match($('#sliceMessage').textContent,/放不下/);assert.equal($('[data-copy="3"]'),null);
 $('#sliceArrange').click();assert.equal(writes(a).length,0);$('#sliceCompute').click();await settle();const body=JSON.parse(writes(a)[0].options.body);assert.equal(body.options.copies.length,3);assert.equal(writes(a).length,1);assert.equal(writes(a)[0].url,'/api/slicing/tasks');
});

test('mixed objects retain independent source IDs through copies, quantity, reanchoring and undo',async t=>{
 const a=await app(t),{w,$,state}=a,original=w.fetch;
 const models=[{id:5,filename:'cube.stl',triangles:12,bounds:{size:[20,20,10]}},{id:6,filename:'long-box.stl',triangles:12,bounds:{size:[40,12,18]}},{id:7,filename:'oversized.stl',triangles:12,bounds:{size:[300,300,300]}}];
 w.fetch=async(url,options={})=>{if(url==='/api/slicing')return reply(200,{ok:true,ready:true,models,tasks:[]});if(url==='/api/slicing/tasks'){state.calls.push({url,options});return reply(200,{ok:true,taskId:9})}return original(url,options)};
 $('[data-page="slicer"]').click();await settle();$('[data-model="5"]').click();$('[data-add-model="6"]').click();await settle();
 assert.match($('#sliceCopies').textContent,/cube/);assert.match($('#sliceCopies').textContent,/long-box/);assert.equal($('#sliceCopyCount').value,'1');
 const x=$('[data-place="x"]');x.value='160';x.dispatchEvent(new w.Event('input',{bubbles:true}));$('[data-copy="0"]').click();assert.notEqual(x.value,'160');$('[data-copy="1"]').click();assert.equal(x.value,'160');
 $('#sliceDuplicate').click();assert.equal($('#sliceCopyCount').value,'2');$('#sliceCopyCount').value='3';$('#sliceApplyCount').click();assert.equal(w.document.querySelectorAll('[data-copy]').length,4);
 $('[data-add-model="7"]').click();assert.match($('#sliceMessage').textContent,/超出/);assert.equal(w.document.querySelectorAll('[data-copy]').length,4);
 $('[data-copy="0"]').click();$('#sliceRemoveCopy').click();assert.equal(w.document.querySelectorAll('[data-copy]').length,3);$('#sliceUndo').click();assert.equal(w.document.querySelectorAll('[data-copy]').length,4);
 assert.equal(writes(a).length,0);$('#sliceCompute').click();await settle();const body=JSON.parse(writes(a)[0].options.body);assert.equal(body.modelId,5);assert.deepEqual(body.options.copies.map(p=>p.modelId),[5,6,6,6]);assert.equal(body.options.modelId,undefined);
 $('#sliceRemoveCopy').click();$('#sliceCompute').click();await settle();const second=JSON.parse(writes(a).at(-1).options.body);assert.equal(second.modelId,6);assert.deepEqual(second.options.copies.map(p=>p.modelId),[6,6,6]);
 assert.equal(writes(a).every(c=>c.url==='/api/slicing/tasks'),true);
});

test('mixed project recovery keeps all sources and saves the whole plate without starting a print',async t=>{
 const a=await app(t),{w,$,state}=a,original=w.fetch;
 const models=[{id:5,filename:'a.stl',triangles:12,bounds:{size:[20,20,10]}},{id:6,filename:'b.stl',triangles:12,bounds:{size:[40,12,18]}}];
 const options={x:70,y:90,scale:1,rotation:[0,0,0],layer:.2,infill:15,color:'#FFFFFF',brim:false,support:false,copies:[{modelId:5,x:70,y:90,scale:1,rotation:[0,0,0]},{modelId:6,x:120,y:90,scale:1,rotation:[0,0,0]}]};
 w.fetch=async(url,req={})=>{if(url==='/api/slicing')return reply(200,{ok:true,ready:true,models,tasks:[]});if(url==='/api/slicing/projects')return reply(200,{ok:true,projects:[],draft:{modelId:5,options,activeCopy:1,revision:2},draftRevision:2});if(url==='/api/slicing/draft'){state.calls.push({url,options:req});return reply(200,{ok:true,revision:3})}return original(url,req)};
 $('[data-page="slicer"]').click();await settle();$('#draftRecover').click();await settle();assert.match($('#sliceCopies').textContent,/a/);assert.match($('#sliceCopies').textContent,/b/);assert.equal($('[data-place="x"]').value,'120');
 $('[data-place="x"]').value='150';$('[data-place="x"]').dispatchEvent(new w.Event('input',{bubbles:true}));$('#draftSaveNow').click();await settle();const saved=JSON.parse(writes(a)[0].options.body);assert.deepEqual(saved.options.copies.map(p=>p.modelId),[5,6]);assert.equal(saved.options.copies[0].x,70);assert.equal(saved.options.copies[1].x,150);assert.equal(saved.activeCopy,1);assert.equal(saved.modelId,5);assert.equal(writes(a).length,1);
});

test('workbench queue confirmation requires an explicit roll and does not send or start printing',async t=>{
 const a=await app(t),{w,$,state}=a;const original=w.fetch;state.snapshot.printers[0].modelId='x2d';state.snapshot.spools[0].material='PETG';await a.Remote.refresh({renderPage:false});
 const task={id:2,modelId:5,filename:'test.stl',state:'succeeded',updated:1,jobId:0,options:{x:138.25,y:128,scale:1,rotation:[0,0,0],layer:.2,infill:15,support:false,brim:false,color:'#FFFFFF'},info:{plates:[{id:1,seconds:968,filaments:[{grams:3,usedGrams:2.04,color:'#FFFFFF'}]}]}};
 w.fetch=async(url,options={})=>{if(url==='/api/slicing'){state.calls.push({url,options});return reply(200,{ok:true,ready:true,models:[],tasks:[task]})}return original(url,options)};
 $('[data-page="slicer"]').click();await settle();$('[data-slice-task="2"]').click();$('#slicePromote').click();await settle();const form=$('#sliceQueueForm');assert.ok(form);assert.match($('#sliceSpools').textContent,/#EEEAE2/);
 form.dispatchEvent(new w.Event('submit',{bubbles:true,cancelable:true}));await settle();assert.match($('#sliceQueueError').textContent,/明确选择/);assert.equal(writes(a).length,0);
 const roll=$('[name="spool"][value="white"]');roll.checked=true;roll.dispatchEvent(new w.Event('change',{bubbles:true}));form.dispatchEvent(new w.Event('submit',{bubbles:true,cancelable:true}));await settle();assert.equal(writes(a).length,1);assert.equal(writes(a)[0].url,'/api/slicing/promote');assert.equal(JSON.parse(writes(a)[0].options.body).spoolId,'white');
});

test('canvas tools are explicit, shortcuts ignore numeric fields and resetting affects only the selected object',async t=>{
 const a=await app(t),{w,$,state}=a,original=w.fetch;
 const models=[{id:5,filename:'cube.stl',triangles:12,bounds:{size:[20,20,10]}},{id:6,filename:'box.stl',triangles:12,bounds:{size:[40,12,18]}}];
 w.fetch=async(url,options={})=>{if(url==='/api/slicing')return reply(200,{ok:true,ready:true,models,tasks:[]});if(url==='/api/slicing/tasks'){state.calls.push({url,options});return reply(200,{ok:true,taskId:9})}return original(url,options)};
 $('[data-page="slicer"]').click();await settle();assert.equal($('[data-transform="translate"]').disabled,true);$('[data-model="5"]').click();$('[data-add-model="6"]').click();await settle();
 $('[data-transform="rotate"]').click();assert.equal($('[data-transform="rotate"]').getAttribute('aria-pressed'),'true');assert.equal($('.studio-process').hidden,false);assert.equal(writes(a).length,0);
 w.document.body.dispatchEvent(new w.KeyboardEvent('keydown',{key:'s',bubbles:true}));assert.equal($('[data-transform="scale"]'),null);assert.equal($('[data-transform="rotate"]').getAttribute('aria-pressed'),'true');
 $('[data-place="x"]').dispatchEvent(new w.KeyboardEvent('keydown',{key:'m',bubbles:true}));assert.equal($('[data-transform="rotate"]').getAttribute('aria-pressed'),'true');
 setModelSize(a,140);numberInput(w,$('[data-rotate="2"]'),45);
 const x=$('[data-place="x"]').value,y=$('[data-place="y"]').value;$('#sliceReset').click();assert.match($('#sliceScaleSummary').textContent,/140%/);assert.equal($('[data-rotate="2"]').value,'0');assert.equal($('[data-place="x"]').value,x);assert.equal($('[data-place="y"]').value,y);
 $('#sliceUndo').click();assert.match($('#sliceScaleSummary').textContent,/140%/);assert.equal($('[data-rotate="2"]').value,'45');$('[data-copy="0"]').click();assert.match($('#sliceScaleSummary').textContent,/100%/);assert.equal($('[data-rotate="2"]').value,'0');
 const undoDisabled=$('#sliceUndo').disabled;$('#sliceGround').click();assert.equal($('#sliceUndo').disabled,undoDisabled);assert.equal(writes(a).length,0);$('#sliceCompute').click();await settle();const body=JSON.parse(writes(a)[0].options.body);assert.equal(body.options.copies[1].scale,1.4);assert.equal(body.options.copies[0].scale,1);assert.equal(body.options.copies[1].rotation[2],45);assert.equal(writes(a).length,1);
});

test('bounds inspection highlights every close object and distinguishes bed edges and extra support clearance',()=>{
 const c=vm.createContext({I18n:testLocale,});vm.runInContext(read('slicer-copies.js')+'\nthis.inspect=SlicerCopies.inspectBounds;',c);
 const box=(x,y=50,w=20)=>({min:[x,y,0],max:[x+w,y+20,10]});
 assert.equal(c.inspect([box(40),box(65)],{}).length,0);
 let issues=c.inspect([box(40),box(64)],{});assert.deepEqual(Array.from(issues,i=>i.index),[0,1]);assert.equal(issues[0].reasons[0].peer,1);assert.equal(issues[1].reasons[0].peer,0);
 issues=c.inspect([box(40),box(65)],{support:true});assert.equal(issues.length,2);
 issues=c.inspect([box(19),box(120)],{});assert.equal(issues.length,1);assert.equal(issues[0].index,0);assert.equal(issues[0].reasons[0].kind,'bounds');
 assert.equal(c.inspect([box(24)],{brim:true}).length,1);assert.equal(c.inspect([box(26)],{brim:true}).length,0);
 const high=box(40);high.max[2]=257;assert.equal(c.inspect([high],{})[0].reasons[0].kind,'bounds');
});

test('process presets retain placement, color and support preferences, and undo restores independent advanced state',async t=>{
 const a=await app(t),{w,$,state}=a,original=w.fetch;
 const model={id:5,filename:'process.stl',triangles:12,bounds:{size:[20,20,10]}};
 w.fetch=async(url,options={})=>{if(url==='/api/slicing')return reply(200,{ok:true,ready:true,models:[model],tasks:[]});if(url==='/api/slicing/tasks'){state.calls.push({url,options});return reply(200,{ok:true,taskId:9})}return original(url,options)};
 $('[data-page="slicer"]').click();await settle();$('[data-model="5"]').click();await settle();assert.equal($('#sliceAdvanced').open,false);assert.equal($('#sliceSupportOptions').disabled,true);
 const input=(selector,value)=>{const el=$(selector);el.value=value;el.dispatchEvent(new w.Event('input',{bubbles:true}));el.dispatchEvent(new w.Event('change',{bubbles:true}))};
 input('[data-place="x"]','150');input('#sliceColor','#00FF00');$('#sliceSupport').checked=true;$('#sliceSupport').dispatchEvent(new w.Event('change',{bubbles:true}));assert.equal($('#sliceSupportOptions').disabled,false);
 change(w,$('#sliceSupportType'),'normal');input('#sliceSupportAngle','45');$('#slicePlateOnly').checked=true;$('#slicePlateOnly').dispatchEvent(new w.Event('change',{bubbles:true}));change(w,$('#sliceProcessPreset'),'strong');
 assert.equal($('#sliceWalls').value,'4');assert.equal($('#sliceTopThickness').value,'1.2');assert.equal($('#slicePattern').value,'gyroid');assert.equal($('#sliceInfill').value,'30');assert.equal($('#sliceSupportType').value,'normal');assert.equal($('#sliceSupportAngle').value,'45');assert.equal($('#slicePlateOnly').checked,true);assert.equal($('[data-place="x"]').value,'150');assert.equal($('#sliceColor').value.toUpperCase(),'#00FF00');assert.equal(writes(a).length,0);
 input('#sliceWalls','5');assert.equal($('#sliceProcessPreset').value,'custom');$('#sliceUndo').click();assert.equal($('#sliceWalls').value,'4');$('#sliceUndo').click();assert.equal($('#sliceWalls').value,'2');$('#sliceRedo').click();assert.equal($('#sliceWalls').value,'4');
 $('#sliceCompute').click();await settle();const body=JSON.parse(writes(a)[0].options.body);assert.deepEqual(body.options.advanced,{walls:4,topThickness:1.2,bottomThickness:1.2,pattern:'gyroid',supportType:'normal',supportAngle:45,plateOnly:true});assert.equal(body.options.x,150);assert.equal(body.options.support,true);assert.equal(writes(a).length,1);
});

test('invalid advanced options block slicing and default reset remains undoable without moving the model',async t=>{
 const a=await app(t),{w,$,state}=a,original=w.fetch;const model={id:5,filename:'process.stl',triangles:12,bounds:{size:[20,20,10]}};
 w.fetch=async(url,options={})=>url==='/api/slicing'?reply(200,{ok:true,ready:true,models:[model],tasks:[]}):original(url,options);
 $('[data-page="slicer"]').click();await settle();$('[data-model="5"]').click();await settle();change(w,$('#sliceProcessPreset'),'strong');const walls=$('#sliceWalls');walls.value='0';walls.dispatchEvent(new w.Event('input',{bubbles:true}));assert.equal($('#sliceCompute').disabled,true);assert.match($('#sliceProcessError').textContent,/1–6/);assert.match($('#sliceService').textContent,/待修正/);$('#sliceCompute').click();assert.equal(writes(a).length,0);
 $('#sliceProcessReset').click();assert.equal($('#sliceCompute').disabled,false);assert.equal($('#sliceWalls').value,'2');assert.equal($('#sliceSupport').checked,false);$('#sliceUndo').click();assert.equal($('#sliceWalls').value,'0');assert.equal($('#sliceCompute').disabled,true);
});

test('advanced draft recovery retains custom options, while old fine-layer drafts preserve their inherited bottom layers',async t=>{
 const a=await app(t),{w,$,state}=a,original=w.fetch;
 const model={id:5,filename:'saved.stl',triangles:12,bounds:{size:[20,20,10]}};
 const options={x:100,y:120,scale:1,rotation:[0,0,0],layer:.16,infill:20,support:true,brim:false,color:'#FFFFFF',advanced:{walls:3,topThickness:1.4,bottomThickness:.8,pattern:'gyroid',supportType:'normal',supportAngle:45,plateOnly:true}};
 let saved;
 w.fetch=async(url,req={})=>{if(url==='/api/slicing')return reply(200,{ok:true,ready:true,models:[model],tasks:[]});if(url==='/api/slicing/projects')return reply(200,{ok:true,projects:[],draft:{modelId:5,options,activeCopy:0,revision:2},draftRevision:2});if(url==='/api/slicing/draft'){state.calls.push({url,options:req});saved=JSON.parse(req.body);return reply(200,{ok:true,revision:3})}return original(url,req)};
 $('[data-page="slicer"]').click();await settle();$('#draftRecover').click();await settle();assert.equal($('#sliceWalls').value,'3');assert.equal($('#sliceSupportType').value,'normal');assert.equal($('#sliceSupportAngle').value,'45');assert.equal($('#sliceProcessPreset').value,'custom');$('#sliceProcessReset').click();$('#sliceUndo').click();assert.equal($('#sliceTopThickness').value,'1.4');
 const infill=$('#sliceInfill');infill.value='25';infill.dispatchEvent(new w.Event('input',{bubbles:true}));$('#draftSaveNow').click();await settle();assert.deepEqual(saved.options.advanced,options.advanced);assert.equal(saved.options.layer,.16);
 const c=vm.createContext({I18n:testLocale,});vm.runInContext(read('slicer-process.js')+'\nthis.process=SlicerProcess;',c);assert.equal(c.process.effective({layer:.12}).bottomThickness,.36);assert.equal(c.process.effective({layer:.16}).bottomThickness,.48);
});

test('workbench undo and redo restore copies and settings, invalidate results and never write tasks',async t=>{
 const a=await app(t),{w,$,state}=a,original=w.fetch;const model={id:5,filename:'cube.stl',triangles:12,bounds:{min:[-10,-10,0],max:[10,10,10],size:[20,20,10]}};
 w.fetch=async(url,options={})=>url==='/api/slicing'?reply(200,{ok:true,ready:true,models:[model],tasks:[]}):original(url,options);
 $('[data-page="slicer"]').click();await settle();$('[data-model="5"]').click();await settle();$('#sliceDuplicate').click();assert.equal($('#sliceCopyCount').value,'2');$('#sliceUndo').click();assert.equal($('#sliceCopyCount').value,'1');$('#sliceRedo').click();assert.equal($('#sliceCopyCount').value,'2');
 $('[data-layer="0.16"]').click();$('#sliceUndo').click();assert.equal($('[data-layer="0.2"]').classList.contains('active'),true);$('#sliceRedo').click();assert.equal($('[data-layer="0.16"]').classList.contains('active'),true);
 const x=$('[data-place="x"]');x.value='140';x.dispatchEvent(new w.Event('input',{bubbles:true}));x.value='145';x.dispatchEvent(new w.Event('input',{bubbles:true}));x.dispatchEvent(new w.Event('change',{bubbles:true}));$('#sliceUndo').click();assert.notEqual(x.value,'140');assert.notEqual(x.value,'145');
 $('#sliceRemoveCopy').click();assert.equal($('#sliceRedo').disabled,true);assert.equal(writes(a).length,0);
});

test('path preview is read-only, layer changes discard stale responses, and close aborts loading',async t=>{
 const a=await app(t),{w,$}=a,api=a.ToolpathUI;const node=w.document.createElement('section');w.document.body.append(node);let release,layerSignal;const calls=[],frames=[];const scene={paths:(segments)=>frames.push(segments),clearPaths(){},view(){}};
 w.fetch=async(url,options={})=>{calls.push({url,options});if(!url.includes('&layer='))return reply(200,{ok:true,layers:[{number:1,z:.2},{number:2,z:.4}]});if(url.endsWith('layer=1')){layerSignal=options.signal;return new Promise(resolve=>release=()=>resolve(reply(200,{ok:true,z:.2,segments:[[1,1,.2,2,2,.2,1]]})))}return reply(200,{ok:true,z:.4,segments:[[3,3,.4,4,4,.4,4]]})};
 const open=api.open(node,scene,5);await settle();$('#pathNext').click();await settle();assert.equal(layerSignal.aborted,true);release();await open;await settle();assert.match($('#pathStatus').textContent,/第 2/);assert.equal(frames.at(-1)[0][6],4);
 $('#pathClose').click();assert.equal(node.textContent,'');assert.equal(calls.every(c=>!c.options.method||c.options.method==='GET'),true);
 const late=api.open(node,scene,5);await settle();api.close();release();await late;assert.equal(node.textContent,'');
});

test('edit history groups input, keeps immutable snapshots, clears redo and bounds history',()=>{
 const c=vm.createContext({I18n:testLocale,});vm.runInContext(read('slicer-history.js')+'\nthis.h=SlicerHistory.create(2)',c);const h=c.h;
 h.reset({x:0});h.record({x:1},'drag');h.record({x:2},'drag');assert.equal(h.undo().x,0);assert.equal(h.redo().x,2);h.end();h.record({x:3});h.record({x:4});h.record({x:5});assert.equal(h.undo().x,4);assert.equal(h.undo().x,3);assert.equal(h.undo(),null);h.record({x:8});assert.equal(h.canRedo,false);
});

test('projects recover a saved plate without slicing; explicit save and draft writes retain complete placement',async t=>{
 const a=await app(t),{w,$,state}=a,original=w.fetch;const options={x:100,y:120,rotation:[0,0,0],scale:1,layer:.16,infill:20,support:false,brim:false,color:'#FFFFFF',copies:[{x:70,y:120,rotation:[0,0,0],scale:1},{x:130,y:120,rotation:[0,0,45],scale:1}]};
 const model={id:5,filename:'saved.stl',triangles:12,bounds:{min:[-10,-10,0],max:[10,10,10],size:[20,20,10]}};
 const saved={revision:4,modelId:5,options,activeCopy:1};let stored=null;
 w.fetch=async(url,req={})=>{if(url==='/api/slicing')return reply(200,{ok:true,ready:true,models:[model],tasks:[]});if(url==='/api/slicing/projects')return reply(200,{ok:true,projects:[],draft:stored||saved,draftRevision:stored?.revision||4});if(url==='/api/slicing/draft'){state.calls.push({url,options:req});const body=JSON.parse(req.body);stored={...body,revision:body.revision+1};return reply(200,{ok:true,revision:stored.revision})}if(url==='/api/slicing/project'){state.calls.push({url,options:req});const body=JSON.parse(req.body);return reply(200,{ok:true,project:{...body,id:8,revision:1,filename:'saved.stl',updated:Date.now()}})}return original(url,req)};
 $('[data-page="slicer"]').click();await settle();assert.equal($('#draftRecovery').hidden,false);assert.equal(writes(a).length,0);$('#draftRecover').click();await settle();assert.equal($('#sliceCopyCount').value,'2');assert.equal($('[data-rotate="2"]').value,'45');assert.equal($('#draftRecovery').hidden,true);assert.equal(writes(a).length,0);
 const input=$('[data-place="x"]');input.value='145';input.dispatchEvent(new w.Event('input',{bubbles:true}));$('#draftSaveNow').click();await settle();assert.equal(stored.options.copies[1].x,145);assert.equal(stored.revision,5);
 $('#projectSave').click();const form=$('#slicerProjectName');form.elements.projectName.value='<my plate>';form.dispatchEvent(new w.Event('submit',{bubbles:true,cancelable:true}));await settle();assert.match($('#projectCurrent').textContent,/<my plate>/);assert.equal($('#projectCurrent my'),null);$('[data-copy="0"]').click();assert.doesNotMatch($('#projectCurrent').textContent,/未保存更改/);assert.equal(writes(a).filter(c=>/slicing\/tasks|jobs\//.test(c.url)).length,0);
});

test('draft conflict preserves local edits and pauses automatic retry',async t=>{
 const a=await app(t),{w,$,state}=a,original=w.fetch;const model={id:5,filename:'cube.stl',triangles:12,bounds:{min:[-10,-10,0],max:[10,10,10],size:[20,20,10]}};
 w.fetch=async(url,req={})=>{if(url==='/api/slicing')return reply(200,{ok:true,ready:true,models:[model],tasks:[]});if(url==='/api/slicing/projects')return reply(200,{ok:true,projects:[],draft:null,draftRevision:3});if(url==='/api/slicing/draft'){state.calls.push({url,options:req});return reply(409,{ok:false,error:'草稿已被其他页面更新'})}return original(url,req)};
 $('[data-page="slicer"]').click();await settle();$('[data-model="5"]').click();await settle();const x=$('[data-place="x"]');x.value='155';x.dispatchEvent(new w.Event('input',{bubbles:true}));$('#draftSaveNow').click();await settle();assert.equal(x.value,'155');assert.match($('#draftStatus').textContent,/版本冲突/);assert.equal($('#draftSaveNow').disabled,true);assert.equal(writes(a).length,1);
 x.value='156';x.dispatchEvent(new w.Event('input',{bubbles:true}));assert.equal(writes(a).length,1);assert.equal($('#draftReload').hidden,false);
});

test('project list and deletion review escape stored project names without writing',async t=>{
 const a=await app(t),{w,$}=a,original=w.fetch;const p={id:7,name:'<img src=x onerror=alert(1)>',filename:'<source>.stl',modelId:1,revision:1,updated:1,options:{copies:[]}};
 w.fetch=async(url,req={})=>url==='/api/slicing/projects'?reply(200,{ok:true,projects:[p],draft:null,draftRevision:0}):original(url,req);
 $('[data-page="slicer"]').click();await settle();$('#projectBrowse').click();await settle();assert.match($('#modal').textContent,/<img src=x/);assert.equal($('#modal img'),null);$('[data-project-delete="7"]').click();assert.match($('#modal').textContent,/<img src=x/);assert.equal($('#modal img'),null);assert.equal(writes(a).length,0);
});

test('studio panels and prepare-preview navigation preserve edits and never slice or print implicitly',async t=>{
 const a=await app(t),{w,$}=a,original=w.fetch;
 const model={id:5,filename:'layout.stl',triangles:12,bounds:{min:[-10,-10,0],max:[10,10,10],size:[20,20,10]}};
 w.fetch=async(url,req={})=>url==='/api/slicing'?reply(200,{ok:true,ready:true,models:[model],tasks:[]}):original(url,req);
 $('[data-page="slicer"]').click();await settle();assert.equal($('#studioAssets').hidden,true);assert.equal($('#sliceCenter').disabled,true);assert.equal(w.document.querySelectorAll('#sliceCompute').length,1);
 $('#studioLibrary').click();assert.equal($('#studioAssets').hidden,false);$('[data-model="5"]').click();await settle();assert.equal($('#studioAssets').hidden,true);
 $('#studioPlacement').click();assert.equal($('.studio-objects').hidden,false);const x=$('[data-place="x"]');x.value='153';x.dispatchEvent(new w.Event('input',{bubbles:true}));$('#studioProcess').click();assert.equal($('.studio-process').hidden,false);$('#studioObjects').click();assert.equal(x.value,'153');
 $('#studioPreview').click();assert.equal($('#sliceWorkbench').dataset.mode,'preview');assert.equal($('.slice-settings').hidden,true);assert.equal($('.studio-preview-inspector').hidden,false);$('#studioLibrary').click();$('#studioLibrary').click();assert.equal(x.value,'153');
 await a.Remote.refresh({renderPage:false});assert.equal($('#sliceWorkbench').dataset.mode,'preview');assert.equal(x.value,'153');$('#studioPrepare').click();assert.equal($('#sliceWorkbench').dataset.mode,'prepare');assert.equal($('.studio-objects').hidden,false);assert.equal(writes(a).length,0);
 $('[data-page="board"]').click();assert.equal(w.document.body.classList.contains('slicer-workspace'),false);
});

test('opening slice history uses the preview inspector; returning to editing retains unsubmitted placement',async t=>{
 const a=await app(t),{w,$}=a,original=w.fetch;
 const model={id:5,filename:'cube.stl',triangles:12,bounds:{min:[-10,-10,0],max:[10,10,10],size:[20,20,10]}};
 const task={id:2,modelId:5,filename:'cube.stl',state:'succeeded',updated:1,jobId:0,options:{x:138.25,y:128,scale:1,rotation:[0,0,0],layer:.2,infill:15,support:false,brim:false,color:'#FFFFFF'},info:{plates:[{id:1,seconds:968,filaments:[{grams:3,usedGrams:2.04,color:'#FFFFFF'}]}]}};
 w.fetch=async(url,req={})=>url==='/api/slicing'?reply(200,{ok:true,ready:true,models:[model],tasks:[task]}):original(url,req);
 $('[data-page="slicer"]').click();await settle();$('[data-model="5"]').click();await settle();const x=$('[data-place="x"]');x.value='147';x.dispatchEvent(new w.Event('input',{bubbles:true}));
 $('#studioLibrary').click();$('[data-slice-task="2"]').click();assert.equal($('#sliceWorkbench').dataset.mode,'preview');assert.equal($('#studioAssets').hidden,true);assert.ok($('.studio-preview-inspector #slicePromote'));assert.equal(x.value,'147');
 $('#studioPrepare').click();assert.equal(x.value,'147');$('#studioPreview').click();assert.ok($('#slicePromote'));assert.equal(writes(a).length,0);
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

test('missing material has a direct fix and library round trip keeps the preparation choices',async t=>{
 const a=await app(t);await a.PrinterControl.options(a.data.jobs.find(j=>j.id===4));a.$('[data-prepare-step="2"]').click();
 a.$('[data-prepare-fix="mapping"]').click();assert.equal(a.$('[data-prepare-pane="1"]').hidden,false);
 change(a.w,a.$('#projectPlate'),'2');choose(a.w,a.$('input[name="spool-0"][value="blue"]'));change(a.w,a.$('input[name="grams-0"]'),'24');
 a.$('input[name="flowCalibration"]').checked=true;a.$('#prepareMaterials').click();
 assert.equal(a.$('#modal').open,false);assert.ok(a.$('#resumePreparation'));assert.equal(a.$('#materialMountFilter').value,'1');
 a.$('#resumePreparation').click();await settle();
 assert.equal(a.$('#projectPlate').value,'2');assert.equal(a.$('input[name="spool-0"][value="blue"]').checked,true);assert.equal(a.$('input[name="grams-0"]').value,'24');assert.equal(a.$('input[name="flowCalibration"]').checked,true);
 assert.equal(a.$('[data-prepare-pane="1"]').hidden,false);assert.equal(writes(a).length,0);
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

test('preparation shows exact slice use, rounds reservations up and blocks an A1 mini file on another model',async t=>{
 const a=await app(t);a.state.slice={model:'Bambu Lab A1 mini',bedType:'Textured PEI Plate',nozzleDiameters:['0.4'],plates:[{...plate(1,'#EEEAE2',39),seconds:6929,warnings:['文件不支持传统延时摄影'],filaments:[{index:0,material:'PLA',color:'#EEEAE2',grams:39,usedGrams:38.57}]}]};
 await a.PrinterControl.options(a.data.jobs.find(j=>j.id===4));
 assert.match(a.$('#projectPreview').textContent,/38\.57/);assert.match(a.$('#projectPreview').textContent,/预留 39/);assert.match(a.$('#prepareCompatibility').textContent,/机型.*不匹配/);
 choose(a.w,a.$('input[name="spool-0"][value="white"]'));a.$('[data-prepare-step="2"]').click();
 assert.equal(a.$('#prepareSave').disabled,true);assert.equal(a.$('input[name="timelapse"]').disabled,true);
 a.$('#projectOptions').dispatchEvent(new a.w.Event('submit',{cancelable:true}));await settle();assert.equal(writes(a).length,0);assert.match(a.$('#projectError').textContent,/重新切片/);
});

test('X2D single-nozzle preparation can be saved while the unadapted dispatch remains explicit',async t=>{
 const a=await app(t);Object.assign(a.state.snapshot.printers[0],{modelId:'x2d',model:'X2D'});await a.Remote.refresh({renderPage:false});a.state.slice.model='Bambu Lab X2D';
 await a.PrinterControl.options(a.data.jobs.find(j=>j.id===4));choose(a.w,a.$('input[name="spool-0"][value="white"]'));a.$('[data-prepare-step="2"]').click();
 assert.match(a.$('#prepareChecklist').textContent,/打印启动映射尚未开放/);assert.equal(a.$('#prepareSave').disabled,false);
 a.$('[data-prepare-fix="support"]').click();assert.equal(a.$('#prepareSupport').open,true);
 a.$('#projectOptions').dispatchEvent(new a.w.Event('submit',{cancelable:true}));await settle();assert.equal(writes(a).length,1);assert.equal(writes(a)[0].url,'/api/jobs/options');
 a.state.snapshot.jobs=a.state.snapshot.jobs.filter(j=>j.id===4);await a.Remote.refresh({renderPage:false});a.PrinterControl.open(1);await settle();assert.equal(a.$('#controlSend').disabled,true);assert.match(a.$('#controlSend').textContent,/待适配/);
});

test('entering preparation review is read-only until a second explicit save',async t=>{
 const a=await app(t);await a.PrinterControl.options(a.data.jobs.find(j=>j.id===4));choose(a.w,a.$('input[name="spool-0"][value="white"]'));
 a.$('#projectOptions').requestSubmit();await settle();assert.equal(writes(a).length,0);assert.equal(a.$('[data-prepare-pane="2"]').hidden,false);
 a.$('#projectOptions').requestSubmit();await settle();assert.equal(writes(a).length,1);assert.equal(writes(a)[0].url,'/api/jobs/options');
});

test('adapted X2D single-nozzle preparation explains the physical route and keeps unverified options off',async t=>{
 const a=await app(t);Object.assign(a.state.snapshot.printers[0],{modelId:'x2d',model:'X2D',state:'idle'});a.state.snapshot.jobs=a.state.snapshot.jobs.filter(j=>j.id===4);a.state.snapshot.capabilities.x2dSingleNozzlePrint=true;a.state.snapshot.capabilities.printStart=false;await a.Remote.refresh({renderPage:false});
 a.state.slice.model='Bambu Lab X2D';a.state.slice.plates[0].nozzle={logical:0,physical:1,diameter:'0.4',volume:'Standard'};
 await a.PrinterControl.options(a.data.jobs.find(j=>j.id===4));choose(a.w,a.$('input[name="spool-0"][value="white"]'));a.$('[data-prepare-step="2"]').click();
 assert.equal(a.$('input[name="flowCalibration"]').disabled,true);assert.equal(a.$('input[name="timelapse"]').disabled,true);assert.equal(a.$('#prepareSave').disabled,false);assert.match(a.$('#prepareChecklist').textContent,/X2D 单喷嘴打印验证/);assert.doesNotMatch(a.$('#prepareChecklist').textContent,/打印启动映射尚未开放/);
 a.PrinterControl.open(1);await settle();assert.equal(a.$('#controlSend').disabled,false);assert.equal(writes(a).length,0);
});

test('a late slice read does not replace a different dialog',async t=>{
 const a=await app(t);let finish;const original=a.w.fetch;
 a.w.fetch=(url,options)=>url.startsWith('/api/jobs/slice')?new Promise(resolve=>finish=()=>resolve(reply(200,{ok:true,slice:a.state.slice}))):original(url,options);
 const pending=a.PrinterControl.options(a.data.jobs.find(j=>j.id===4));await settle();a.PrinterControl.open(1);await settle();finish();await pending;
 assert.ok(a.$('#controlRoot'));assert.equal(a.$('#projectOptions'),null);assert.equal(writes(a).length,0);
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

test('a delayed setting response cannot replace a dialog opened after leaving the editor',async t=>{
 const a=await app(t);a.PrinterControl.open(1);await settle();a.$('[data-equipment="fan:1"]').click();
 let finish,calls=0;const original=a.w.fetch;
 a.w.fetch=(url,options)=>url==='/api/printers/settings'&&options.method==='POST'?(calls++,new Promise(resolve=>finish=()=>resolve(reply(200,{ok:true})))):original(url,options);
 a.$('#equipmentForm').requestSubmit();await settle();assert.equal(calls,1);a.$('#equipmentDialog').click();await settle();
 await a.PrinterControl.options(a.data.jobs.find(j=>j.id===4));assert.ok(a.$('#projectOptions'));
 finish();await settle();assert.ok(a.$('#projectOptions'));assert.equal(a.$('#controlRoot'),null);
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
test('heartbeat refresh keeps an open editing form and entered values intact',async t=>{
 const a=await app(t);await a.PrinterControl.options(a.data.jobs.find(j=>j.id===4));
 const form=a.$('#projectOptions');change(a.w,a.$('input[name="grams-0"]'),'27');
 a.intervals.find(i=>i.ms===3000).fn();await settle();
 assert.equal(a.$('#projectOptions'),form);assert.equal(a.$('input[name="grams-0"]').value,'27');
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
test('color choices are explicit, limited by material/mount/stock; drafts are separate for each plate and supply mode',async t=>{
 const a=await app(t);await a.PrinterControl.options(a.data.jobs.find(j=>j.id===4));
 assert.equal(a.$('input[type="radio"]:checked'),null);
 assert.equal(a.$('input[type="radio"]').value,'white');assert.equal(a.$('input[value="short"]').disabled,true);
 assert.equal(a.$('input[value="petg"]'),null);assert.equal(a.$('input[value="external"]'),null);
 choose(a.w,a.$('input[value="blue"]'));change(a.w,a.$('input[name="grams-0"]'),'27');
 assert.match(a.$('[data-color-summary]').textContent,/与文件原色不同/);
 change(a.w,a.$('#projectPlate'),'2');assert.equal(a.$('input[type="radio"]:checked'),null);
 choose(a.w,a.$('input[value="white"]'));change(a.w,a.$('input[name="grams-0"]'),'14');
 change(a.w,a.$('#projectPlate'),'1');assert.equal(a.$('input[type="radio"]:checked').value,'blue');assert.equal(a.$('input[name="grams-0"]').value,'27');
 change(a.w,a.$('#projectAMS'),false);assert.equal(a.$('input[type="radio"]:checked'),null);
 assert.equal(a.$('input[type="radio"]').value,'external');choose(a.w,a.$('input[value="external"]'));
 change(a.w,a.$('#projectAMS'),true);assert.equal(a.$('input[type="radio"]:checked').value,'blue');
 change(a.w,a.$('#projectPlate'),'2');assert.equal(a.$('input[type="radio"]:checked').value,'white');assert.equal(a.$('input[name="grams-0"]').value,'14');
 change(a.w,a.$('input[name="grams-0"]'),'');change(a.w,a.$('#projectPlate'),'1');change(a.w,a.$('#projectPlate'),'2');
 assert.equal(a.$('input[name="grams-0"]').value,'');assert.equal(a.$('#projectOptions').checkValidity(),false);
 assert.equal(writes(a).length,0);
});
test('saving colors submits selected plate, spool and grams without transmitting or starting a print',async t=>{
 const a=await app(t);await a.PrinterControl.options(a.data.jobs.find(j=>j.id===4));
 change(a.w,a.$('#projectPlate'),'2');choose(a.w,a.$('input[value="blue"]'));change(a.w,a.$('input[name="grams-0"]'),'23');
 a.$('[data-prepare-step="2"]').click();a.$('#projectOptions').requestSubmit();await settle();
 assert.equal(writes(a).length,1);assert.equal(writes(a)[0].url,'/api/jobs/options');
 const body=JSON.parse(writes(a)[0].options.body);
 assert.equal(body.options.plate,2);assert.equal(body.options.useAMS,true);
 assert.deepEqual(body.options.mapping,[{filament:0,amsId:0,slotId:1,spoolId:'blue',grams:23}]);
});
test('saved color mapping is restored only for its own plate and supply mode',async t=>{
 const a=await app(t);a.data.jobs.find(j=>j.id===4).printOptions={plate:2,useAMS:true,mapping:[{filament:0,spoolId:'white',grams:24}]};
 await a.PrinterControl.options(a.data.jobs.find(j=>j.id===4));
 assert.equal(a.$('#projectPlate').value,'2');assert.equal(a.$('input[type="radio"]:checked').value,'white');
 assert.equal(a.$('input[name="grams-0"]').value,'24');
 change(a.w,a.$('#projectPlate'),'1');assert.equal(a.$('input[type="radio"]:checked'),null);
 change(a.w,a.$('#projectPlate'),'2');change(a.w,a.$('#projectAMS'),false);assert.equal(a.$('input[type="radio"]:checked'),null);
});
test('multiple colors using the same spool cannot overbook its combined stock',async t=>{
 const a=await app(t);a.state.snapshot.spools.find(s=>s.id==='blue').remaining=30;await a.Remote.refresh({renderPage:false});
 a.state.slice.plates=[{...plate(1,'#4778BC',19),filaments:[{index:0,material:'PLA',color:'#4778BC',grams:19},{index:1,material:'PLA',color:'#EEEAE2',grams:19}]}];
 await a.PrinterControl.options(a.data.jobs.find(j=>j.id===4));
 choose(a.w,a.$('input[name="spool-0"][value="blue"]'));choose(a.w,a.$('input[name="spool-1"][value="blue"]'));
 a.$('[data-prepare-step="2"]').click();a.$('#projectOptions').requestSubmit();await settle();
 assert.match(a.$('#projectError').textContent,/余量不足/);assert.equal(writes(a).length,0);
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
test('STL slice print action creates the chosen spool job and dispatches without more dialogs',async t=>{
 const a=await readySlice(t);a.$('#slicePrintNow').click();await settle();assert.deepEqual(writes(a).map(c=>c.url),['/api/slicing/promote','/api/jobs/dispatch']);assert.equal(JSON.parse(writes(a)[1].options.body).jobId,9);assert.ok(a.$('#controlRoot'));
});
test('queue only never dispatches and unknown creation never silently resubmits',async t=>{
 const a=await readySlice(t);a.$('#sliceQueueOnly').click();await settle();assert.deepEqual(writes(a).map(c=>c.url),['/api/slicing/promote']);
 const b=await readySlice(t,{unknownPromote:true});b.$('#slicePrintNow').click();await settle();assert.match(b.$('#sliceQueueError').textContent,/核对/);b.$('#slicePrintNow').click();await settle();assert.equal(writes(b).length,1);assert.equal(b.$('#sliceQueueOnly').disabled,true);
});
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


async function presetWorkbench(t,{lostSave=false,conflict=false,deferRead=false}={}){
 const a=await multiWorkbench(t),{w,state}=a,original=w.fetch;let resolveRead;
 const preset={id:'private-preset-001',name:'支架 <img src=x>',revision:3,updated:1,archived:false,settings:{material:'pla-basic',layer:.16,infill:30,support:true,brim:true,advanced:{walls:4,topThickness:1.2,bottomThickness:1.2,pattern:'gyroid',supportType:'tree',supportAngle:45,plateOnly:true}}};
 const records=[preset];
 w.fetch=async(url,opts={})=>{
  if(url==='/api/slicing/presets'){
   state.calls.push({url,options:opts});
   if(opts.method==='POST'){const b=JSON.parse(opts.body);if(conflict)return reply(409,{ok:false,error:'预设已被其他页面修改'});const p={...b,revision:b.revision+1,updated:2,archived:false},i=records.findIndex(p=>p.id===b.id);if(i>=0)records[i]=p;else records.push(p);if(lostSave)throw Error('ack lost');return reply(200,{ok:true,preset:p})}
   if(deferRead)return new Promise(r=>resolveRead=()=>r(reply(200,{ok:true,presets:structuredClone(records)})));
   return reply(200,{ok:true,presets:structuredClone(records)})
  }
  if(url==='/api/slicing/presets/archive'){state.calls.push({url,options:opts});const b=JSON.parse(opts.body),p=records.find(p=>p.id===b.id);p.archived=b.archived;p.revision++;return reply(200,{ok:true})}
  return original(url,opts)
 };
 return {...a,records,resolveRead:()=>resolveRead()};
}

test('custom recipe review shows changes, preserves model edits, applies only to the current plate and is one undoable action',async t=>{
 const a=await presetWorkbench(t),{w,$,state}=a;
 $('[data-model="5"]').click();$('#sliceColor').value='#336699';$('#sliceColor').dispatchEvent(new w.Event('input',{bubbles:true}));
 $('#sliceDuplicate').click();const oldX=$('[data-place="x"]').value,oldCopies=$('#sliceCopyCount').value;
 $('#processPresetBrowse').click();await settle();assert.equal($('#processPresetRoot img'),null);assert.match($('#processPresetRoot').textContent,/支架 <img src=x>/);
 $('[data-preset-apply]').click();assert.match($('.preset-diff').textContent,/PLA Basic/);assert.equal($('#sliceMaterial').value,'petg-basic');assert.match($('.preset-material-note').textContent,/实机验证/);
 $('#presetApply').click();assert.equal($('#sliceMaterial').value,'pla-basic');assert.equal($('#sliceInfill').value,'30');assert.equal($('#sliceColor').value,'#336699');assert.equal($('[data-place="x"]').value,oldX);assert.equal($('#sliceCopyCount').value,oldCopies);assert.equal($('#sliceSupport').checked,true);
 $('#sliceUndo').click();assert.equal($('#sliceMaterial').value,'petg-basic');assert.equal($('#sliceInfill').value,'15');assert.equal($('#sliceColor').value,'#336699');assert.equal($('#sliceCopyCount').value,oldCopies);
 $('#sliceRedo').click();assert.equal($('#sliceMaterial').value,'pla-basic');
 $('#slicePlateAdd').click();change(w,$('#sliceMaterial'),'petg-basic');$('#processPresetBrowse').click();await settle();$('[data-preset-apply]').click();$('#presetApply').click();$('[data-plate="0"]').click();await settle();assert.equal($('#sliceMaterial').value,'pla-basic');
 assert.equal(state.calls.filter(c=>c.options.method==='POST'&&c.url!=='/api/slicing/draft').length,0);
});

test('saving a custom recipe sends only supported process fields and preserves current geometry without slicing',async t=>{
 const a=await presetWorkbench(t),{w,$,state}=a;$('[data-model="5"]').click();$('#sliceDuplicate').click();numberInput(w,$('#sliceWalls'),'3');
 $('#processPresetSave').click();await settle();const form=$('#presetSaveForm');form.elements.name.value='我的 PETG 支架';submitForm(w,form);await settle();
 const call=state.calls.find(c=>c.url==='/api/slicing/presets'&&c.options.method==='POST'),b=JSON.parse(call.options.body);
 assert.equal(b.name,'我的 PETG 支架');assert.equal(b.revision,0);assert.match(b.id,/^[a-f0-9-]{36}$/);assert.equal(b.settings.advanced.walls,3);
 assert.deepEqual(Object.keys(b.settings).sort(),['advanced','brim','infill','layer','material','support']);assert.equal(state.calls.filter(c=>c.url==='/api/slicing/tasks'||c.url.startsWith('/api/jobs/')).length,0);
 assert.match($('#processPresetRoot').textContent,/我的 PETG 支架/);
});

test('preset update conflicts retain the entered name and do not replace working parameters',async t=>{
 const a=await presetWorkbench(t,{conflict:true}),{w,$,state}=a;$('#processPresetSave').click();await settle();const form=$('#presetSaveForm');change(w,form.elements.target,'private-preset-001');form.elements.name.value='修订名称';submitForm(w,form);await settle();
 assert.match($('#presetSaveError').textContent,/其他页面修改/);assert.equal(form.elements.name.value,'修订名称');assert.equal($('#sliceMaterial').value,'petg-basic');assert.equal(form.querySelector('[type=submit]').disabled,false);
 const b=JSON.parse(state.calls.find(c=>c.options.method==='POST').options.body);assert.equal(b.revision,3);assert.equal(b.id,'private-preset-001');
});

test('unknown preset save is never automatically repeated; rereading finds its original request ID',async t=>{
 const a=await presetWorkbench(t,{lostSave:true}),{w,$,state}=a;$('#processPresetSave').click();await settle();const form=$('#presetSaveForm');form.elements.name.value='响应丢失的预设';submitForm(w,form);await settle();
 assert.equal(form.querySelector('[type=submit]').disabled,true);assert.match($('#presetSaveError').textContent,/重新读取/);submitForm(w,form);await settle();assert.equal(state.calls.filter(c=>c.options.method==='POST').length,1);
 $('#presetSaveCancel').click();await settle();assert.match($('#processPresetRoot').textContent,/响应丢失的预设/);assert.equal(state.calls.filter(c=>c.options.method==='POST').length,1);
});

test('preset archive can be restored without changing the plate; late private responses cannot replace another page',async t=>{
 const a=await presetWorkbench(t),{w,$,state}=a;$('#processPresetBrowse').click();await settle();$('[data-preset-archive]').click();await settle();assert.equal($('[data-preset-apply]'),null);assert.ok($('[data-preset-restore]'));$('[data-preset-restore]').click();await settle();assert.ok($('[data-preset-apply]'));assert.equal($('#sliceMaterial').value,'petg-basic');assert.equal(state.calls.filter(c=>c.url==='/api/slicing/tasks').length,0);
 const b=await presetWorkbench(t,{deferRead:true});b.$('#processPresetBrowse').click();await settle();b.$('[data-close]').click();b.$('[data-page="team"]').click();b.$('#guideBtn').click();b.resolveRead();await settle();assert.equal(b.$('#processPresetRoot'),null);assert.equal(b.$('#modal').open,true);
});


async function compareWorkbench(t,{sameGeometry=true,deferCompare=false,rejectCompare=false,unknown=false}={}){
 const a=await multiWorkbench(t),{w,state,tasks}=a,original=w.fetch;let resolveCompare;
 for(let i=1;i<=4;i++)tasks.push({id:i,modelId:5,filename:i===1?'<img src=x>.stl':'cube.stl',state:'succeeded',created:i,updated:1,options:{x:138.25,y:128,rotation:[0,0,0],scale:1,layer:i===1?.2:.16,infill:i===1?15:30,support:false,brim:false,color:'#FFFFFF'},info:{model:'X2D',plates:[{id:1,seconds:i===1?600:900,filaments:[{index:0,usedGrams:i===1?12.5:15.25,grams:16,color:'#FFFFFF',material:'PETG',profileId:'GFG00'}]}]}});
 w.fetch=async(url,opts={})=>{
  if(url.startsWith('/api/slicing/compare?')){
   state.calls.push({url,options:opts});if(rejectCompare)return reply(404,{ok:false,error:'切片不存在或无权访问'});
   const ids=new URLSearchParams(url.split('?')[1]).get('ids').split(',').map(Number),entries=ids.map(id=>{const task=structuredClone(tasks.find(t=>t.id===id));return {task,seconds:unknown?0:task.info.plates[0].seconds,grams:unknown?16:task.info.plates[0].filaments[0].usedGrams,exactUsage:!unknown,objects:1,size:[20,20,10]}}),result={ok:true,sameGeometry,entries};
   if(deferCompare)return new Promise(r=>resolveCompare=()=>r(reply(200,result)));return reply(200,result)
  }
  return original(url,opts)
 };
 return {...a,resolveCompare:()=>resolveCompare()};
}

test('slice comparison is opt-in, keeps a maximum of three choices and uses the selection order as baseline',async t=>{
 const a=await compareWorkbench(t),{w,$,state}=a;assert.equal(state.calls.some(c=>c.url.startsWith('/api/slicing/compare')),false);
 $('.slice-compare-open').click();await settle();assert.equal($('#sliceCompareRoot img'),null);assert.match($('#sliceCompareRoot').textContent,/<img src=x>.stl/);
 assert.equal($('#compareRun').disabled,true);choose(w,$('[data-compare-pick="2"]'));choose(w,$('[data-compare-pick="1"]'));assert.equal($('#compareRun').disabled,false);choose(w,$('[data-compare-pick="3"]'));assert.equal($('[data-compare-pick="4"]').disabled,true);assert.match($('#compareSelection').textContent,/基准：切片 #2/);
 $('#compareRun').click();await settle();assert.equal(state.calls.find(c=>c.url.startsWith('/api/slicing/compare')).url,'/api/slicing/compare?ids=2,1,3');assert.match($('.compare-geometry').textContent,/几何与摆放一致/);assert.ok($('.compare-award'));assert.equal(state.calls.filter(c=>c.url==='/api/jobs/dispatch').length,0);
});

test('comparing and viewing a historical result preserve the editing plate, exact estimates and optional parameter details',async t=>{
 const a=await compareWorkbench(t),{w,$,state}=a;$('[data-model="5"]').click();numberInput(w,$('[data-place="x"]'),150);$('#sliceDuplicate').click();const count=$('#sliceCopyCount').value,comparisonOriginalX=$('[data-place="x"]').value;
 $('#studioLibrary').click();$('.slice-compare-open').click();await settle();choose(w,$('[data-compare-pick="1"]'));choose(w,$('[data-compare-pick="2"]'));$('#compareRun').click();await settle();
 assert.match($('.compare-result-cards').textContent,/12.50 g/);assert.match($('.compare-result-cards').textContent,/比基准增加 2.75 g/);assert.ok($('[data-compare-same]').hidden);change(w,$('#compareAllFields'),true);assert.equal($('[data-compare-same]').hidden,false);
 $('[data-compare-view="2"]').click();await settle();assert.equal($('#modal').open,false);assert.equal($('#studioAssets').hidden,true);assert.equal($('[data-place="x"]').value,comparisonOriginalX);assert.equal($('#sliceCopyCount').value,count);assert.equal($('#sliceInfill').value,'15');assert.match($('#sliceResult').textContent,/15.25/);
 assert.equal(state.calls.filter(c=>c.url==='/api/slicing/tasks'||c.url.startsWith('/api/jobs/')).length,0);
});

test('different meshes and missing exact estimates never get a misleading minimum badge',async t=>{
 for(const options of [{sameGeometry:false},{unknown:true}]){
  const a=await compareWorkbench(t,options),{w,$}=a;$('.slice-compare-open').click();await settle();choose(w,$('[data-compare-pick="1"]'));choose(w,$('[data-compare-pick="2"]'));$('#compareRun').click();await settle();assert.equal($('.compare-award'),null);
  if(options.sameGeometry===false)assert.match($('.compare-geometry').textContent,/包含模型变化/);else{assert.match($('.compare-result-cards').textContent,/未提供/);assert.match($('.compare-result-cards').textContent,/缺少精确估算/)}
 }
});

test('comparison errors retain choices and late responses cannot overwrite a different dialog or page',async t=>{
 const a=await compareWorkbench(t,{rejectCompare:true}),{w,$}=a;$('.slice-compare-open').click();await settle();choose(w,$('[data-compare-pick="1"]'));choose(w,$('[data-compare-pick="2"]'));$('#compareRun').click();await settle();assert.match($('#compareError').textContent,/无权访问/);assert.equal($('[data-compare-pick="1"]').checked,true);assert.equal($('#compareRun').disabled,false);
 const b=await compareWorkbench(t,{deferCompare:true});b.$('.slice-compare-open').click();await settle();choose(b.w,b.$('[data-compare-pick="1"]'));choose(b.w,b.$('[data-compare-pick="2"]'));b.$('#compareRun').click();await settle();b.$('[data-close]').click();b.$('[data-page="team"]').click();b.$('#guideBtn').click();b.resolveCompare();await settle();assert.equal(b.$('#sliceCompareRoot'),null);assert.equal(b.$('#modal').open,true);
});







