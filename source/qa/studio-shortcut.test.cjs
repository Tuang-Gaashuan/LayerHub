const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const context=vm.createContext({I18n:{t:s=>s}});let src=fs.readFileSync(require('node:path').join(__dirname,'../web/studio-shortcut.js'),'utf8').replace('return {bind,open};','return {normalize,registration,removal,utf16};');vm.runInContext(src+';globalThis.api=StudioShortcut;',context);const a=context.api;
test('Windows setup quotes one local executable without URL arguments or a shell',()=>{
 const path='F:\\设计 文件夹\\Bambu Studio\\bambu-studio.exe',reg=a.registration(path);
 assert.match(reg,/HKEY_CURRENT_USER\\Software\\Classes\\layerhub-studio/);assert.doesNotMatch(reg,/%1|powershell|cmd\.exe|HKEY_LOCAL_MACHINE/);
 const command=JSON.parse(reg.split('\r\n').find(s=>s.startsWith('@="\\"')).slice(2));assert.equal(command,'"'+path+'"');
 const raw=Buffer.from(a.utf16(reg));assert.equal(raw[0],255);assert.equal(raw[1],254);assert.equal(raw.subarray(2).toString('utf16le'),reg);
 const remove=a.removal();assert.equal((remove.match(/\[-/g)||[]).length,1);assert.match(remove,/\[-HKEY_CURRENT_USER\\Software\\Classes\\layerhub-studio\]/);
});
