'use strict';
const Appearance=(()=>{
 const defaults={size:'comfortable',contrast:'clear',weight:'normal',font:'system'};
 const fonts={
  system:{label:'系统无衬线',group:'日常阅读',note:'跟随设备的系统字体，清晰、轻快。'},
  noto:{label:'思源黑体',group:'日常阅读',web:'noto-sans-sc',note:'笔画规整，适合长时间阅读参数和任务。'},
  notoserif:{label:'思源宋体',group:'日常阅读',web:'noto-serif-sc',note:'具有衬线与笔画粗细变化，偏书刊阅读风格。'},
  wenkai:{label:'霞鹜文楷',group:'个性字体',web:'lxgw-wenkai',note:'自然的手写楷意，风格舒展。采用屏显字重；完整字库首次加载约 7 MB。'},
  xiaowei:{label:'站酷小薇',group:'个性字体',web:'zcool-xiaowei',note:'带有装饰感的宋体风格。此字体只有常规字重。'},
  kuaile:{label:'站酷快乐体',group:'个性字体',web:'zcool-kuaile',note:'圆润活泼，适合轻松的界面风格。此字体只有常规字重。'},
  serif:{label:'宋体 · 本机',group:'本机字体',note:'使用电脑已安装的宋体；其他设备可能显示为衬线后备字体。'},
  heiti:{label:'黑体 · 本机',group:'本机字体',note:'优先使用电脑已安装的黑体，未安装时使用无衬线后备字体。'},
  kaiti:{label:'楷体 · 本机',group:'本机字体',note:'优先使用电脑已安装的楷体，未安装时使用衬线后备字体。'},
  fangsong:{label:'仿宋 · 本机',group:'本机字体',note:'优先使用电脑已安装的仿宋，未安装时使用衬线后备字体。'}
 };
 const choices={size:{compact:'标准',comfortable:'舒适 · 默认',large:'大字'},contrast:{clear:'清晰 · 默认',strong:'高对比度'},weight:{normal:'常规',medium:'加粗'},font:Object.fromEntries(Object.entries(fonts).map(([id,f])=>[id,f.label]))};
 function loadFont(id){const font=fonts[id];if(!font?.web||document.querySelector(`link[data-reading-font="${id}"]`))return;const link=document.createElement('link');link.rel='stylesheet';link.href='./assets/fonts/'+font.web+'/font.css';link.dataset.readingFont=id;document.head.append(link)}
 const key=()=>data.user?.id?'layerhub.ui.appearance.user.'+data.user.id:null;
 const clean=p=>Object.fromEntries(Object.keys(defaults).map(k=>[k,Object.hasOwn(choices[k],p?.[k])?p[k]:defaults[k]]));
 function read(){try{return clean(JSON.parse(localStorage.getItem(key())||'{}'))}catch{return {...defaults}}}
 function apply(p){const el=document.documentElement;p=clean(p);loadFont(p.font);for(const k of Object.keys(defaults))el.dataset['ui'+k[0].toUpperCase()+k.slice(1)]=p[k]}
 function sync(){apply(key()?read():defaults)}
 function open(){
  const saved=read(),options=entries=>entries.map(([v,label])=>`<option value="${v}" ${saved.font===v?'selected':''}>${label}</option>`).join(''),select=(k,label)=>`<label><span class="field-label">${label}</span><select class="input" name="${k}" ${k==='font'?'aria-describedby="appearanceFontHint"':''}>${k==='font'?['日常阅读','个性字体','本机字体'].map(group=>`<optgroup label="${group}">${options(Object.entries(fonts).filter(([,f])=>f.group===group).map(([id,f])=>[id,f.label]))}</optgroup>`).join(''):Object.entries(choices[k]).map(([v,label])=>`<option value="${v}" ${saved[k]===v?'selected':''}>${label}</option>`).join('')}</select></label>`;
  modal('个性化显示',`<form id="appearanceForm"><p class="form-help">调整文字的清晰度与阅读大小。偏好保存在此浏览器内，按当前账户分别记忆。</p><div class="appearance-fields">${select('size','文字大小')}${select('contrast','文字对比度')}${select('weight','字重')}${select('font','字体 · 10 种')}</div><p class="appearance-font-hint" id="appearanceFontHint" aria-live="polite"></p><div id="appearancePreview" class="appearance-preview"><span class="appearance-font-name" id="appearanceFontName"></span><strong>舒适阅读，专心创造</strong><p>层层叠印，把灵感变成作品。</p><p>模型工作台 · PETG Basic · AMS 4</p><small>0123456789 · 0.20 mm · 215 °C · 80%</small></div><p id="appearanceError" class="form-error" role="alert"></p></form>`,`<button class="button" id="appearanceReset" type="button">恢复默认</button><button class="button" data-cancel>取消</button><button class="button primary" form="appearanceForm" type="submit">保存设置</button>`);
  const form=$('#appearanceForm'),preview=$('#appearancePreview'),draft=()=>clean(Object.fromEntries(new FormData(form)));
  const draw=()=>{const p=draft(),font=fonts[p.font];loadFont(p.font);for(const [k,v] of Object.entries(p))preview.dataset['ui'+k[0].toUpperCase()+k.slice(1)]=v;$('#appearanceFontName').textContent=font.label;$('#appearanceFontHint').textContent=font.note+(font.web?' 网页字体从本服务器按需加载，加载时先显示后备字体。':'')};form.onchange=draw;draw();
  $('#appearanceReset').onclick=()=>{for(const [k,v] of Object.entries(defaults))form.elements[k].value=v;draw()};
  form.onsubmit=e=>{e.preventDefault();try{const p=draft(),storageKey=key();if(!storageKey)throw Error('请登录后保存显示偏好');localStorage.setItem(storageKey,JSON.stringify(p));apply(p);$('#modal').close();toast('显示偏好已保存到此浏览器')}catch(e){$('#appearanceError').textContent=e.message||'浏览器未允许保存设置'}};
 }
 return {sync,open};
})();
