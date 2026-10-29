const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),{spawnSync}=require('node:child_process');
test('locale output is stable across LF and CRLF source checkouts',()=>{
 const temp=fs.mkdtempSync(path.join(os.tmpdir(),'layerhub-locale-lines-'));
 try{
 for(const dir of ['qa/frontend-deps','web/locales','internal/app/locales','internal/device'])fs.mkdirSync(path.join(temp,dir),{recursive:true});
 fs.copyFileSync(path.join(__dirname,'build-locales.cjs'),path.join(temp,'qa/build-locales.cjs'));
 fs.symlinkSync(path.join(__dirname,'frontend-deps/node_modules'),path.join(temp,'qa/frontend-deps/node_modules'),process.platform==='win32'?'junction':'dir');
 fs.writeFileSync(path.join(temp,'web/locales/en.json'),JSON.stringify({'标题':'Title','内容':'Content'}));
 const html='<main>\n<h1>标题</h1>\n<p>内容</p>\n</main>\n',js='I18n.html`<h1>标题</h1>\n<p>内容</p>`;\n';
 const write=crlf=>{fs.writeFileSync(path.join(temp,'web/index.html'),crlf?html.replaceAll('\n','\r\n'):html);fs.writeFileSync(path.join(temp,'web/app.js'),crlf?js.replaceAll('\n','\r\n'):js)};
 const run=args=>{const r=spawnSync(process.execPath,[path.join(temp,'qa/build-locales.cjs'),...args],{encoding:'utf8'});assert.equal(r.status,0,r.stderr)};
 write(false);run([]);const before=fs.readFileSync(path.join(temp,'web/locales/en.generated.js'));write(true);run(['--check']);assert.deepEqual(fs.readFileSync(path.join(temp,'web/locales/en.generated.js')),before);
 }finally{
 assert.ok(path.resolve(temp).startsWith(path.resolve(os.tmpdir())+path.sep+'layerhub-locale-lines-'));
 fs.rmSync(temp,{recursive:true,force:true});
 }
});
