'use strict';
const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('./frontend-deps/node_modules/jsdom');
const web=path.resolve(__dirname,'../web');
function runtime(t,{stored,browser=['en-US']}={}){
 const dom=new JSDOM('<!doctype html><html><head><title></title></head><body><div class="topbar-right"></div><div id="modalContent"></div></body></html>',{url:'http://localhost/',runScripts:'outside-only'}),w=dom.window;t.after(()=>w.close());
 Object.defineProperty(w.navigator,'languages',{value:browser,configurable:true});
 if(stored!==undefined)w.localStorage.setItem('layerhub.language',stored);
 w.eval(fs.readFileSync(path.join(web,'locales/en.generated.js'),'utf8')+'\n'+fs.readFileSync(path.join(web,'i18n.js'),'utf8'));
 return w;
}
test('language detection follows saved choices, then supported browser languages, with a safe fallback',t=>{
 assert.equal(runtime(t,{browser:['zh-TW','en-US']}).I18n.language(),'zh-CN');
 assert.equal(runtime(t,{stored:'en',browser:['zh-CN']}).I18n.language(),'en');
 assert.equal(runtime(t,{stored:'unsupported',browser:['fr-FR']}).I18n.language(),'en');
 assert.equal(runtime(t,{stored:'zh-CN',browser:['en-US']}).I18n.language(),'zh-CN');
});
test('compiled HTML changes only application text while preserving attributes, user strings and literal slots',t=>{
 const w=runtime(t),I=w.I18n;
 const value='&lt;img src=x onerror=alert(1)&gt;__lh_expr_9__';
 const html=I.html`<button class="button primary" data-new-task>${value} 新建打印任务</button>`;
 const box=w.document.createElement('div');box.innerHTML=html;
 assert.equal(box.querySelector('button').className,'button primary');assert.equal(box.querySelector('button').getAttribute('data-new-task'),'');assert.equal(box.querySelector('img'),null);
 assert.match(box.textContent,/__lh_expr_9__/);assert.match(box.textContent,/New print job/);
 assert.equal(I.msg`创建 ${'__lh_expr_0__'} 块打印板`,'Create __lh_expr_0__ plates');
});
test('known system patterns retain raw diagnostic arguments and numerical values',t=>{
 const I=runtime(t).I18n;
 assert.equal(I.system('目标 75 %'),'Target 75 %');
 assert.equal(I.system('等待物理喷嘴 1 的新规格；需与切片 0.4 mm / Standard 一致'),'Waiting for fresh nozzle 1 specifications; they must match slice 0.4 mm / Standard');
 assert.equal(I.system(false),false);assert.equal(I.system(null),null);assert.equal(I.system(0),0);
});
test('unavailable browser storage prevents reload instead of losing the current page',async t=>{
 const w=runtime(t,{stored:'en'});let reloads=0;
 w.Storage.prototype.setItem=()=>{throw Error('Storage blocked')};
 await assert.rejects(w.I18n.change('zh-CN',{reload:()=>reloads++}),/Storage blocked/);
 assert.equal(reloads,0);assert.equal(w.I18n.language(),'en');
});
test('historical notification copy is localized without translating filenames or printer names',t=>{
 const I=runtime(t).I18n;
 assert.equal(I.notificationTitle('LayerHub · 打印已开始 · 关闭 · 模型工作台.gcode'),'LayerHub · Print started · 关闭 · 模型工作台.gcode');
 const body=I.notificationBody('打印已开始\n设备：关闭\n任务：模型工作台.gcode\n任务编号：6\n\n打印机已上报正在执行此任务。');
 assert.match(body,/Print started/);assert.match(body,/Printer: 关闭/);assert.match(body,/Job: 模型工作台.gcode/);assert.match(body,/The printer reported that this job is running/);
 assert.equal(I.system('目标 75 % · 已由 关闭 核对设备实际状态'),'Target 75 % · Reviewed against the physical printer by 关闭');
});
