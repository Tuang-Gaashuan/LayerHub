'use strict';
const Appearance=(()=>{
 const defaults={size:'comfortable',contrast:'clear',weight:'normal',font:'system'};
 const fonts={
  system:{label:I18n.t('系统无衬线'),group:I18n.t('日常阅读'),note:I18n.t('跟随设备的系统字体，清晰、轻快。')},
  noto:{label:I18n.t('思源黑体'),group:I18n.t('日常阅读'),web:'noto-sans-sc',note:I18n.t('笔画规整，适合长时间阅读参数和任务。')},
  notoserif:{label:I18n.t('思源宋体'),group:I18n.t('日常阅读'),web:'noto-serif-sc',note:I18n.t('具有衬线与笔画粗细变化，偏书刊阅读风格。')},
  wenkai:{label:I18n.t('霞鹜文楷'),group:I18n.t('个性字体'),web:'lxgw-wenkai',note:I18n.t('自然的手写楷意，风格舒展。采用屏显字重；完整字库首次加载约 7 MB。')},
  xiaowei:{label:I18n.t('站酷小薇'),group:I18n.t('个性字体'),web:'zcool-xiaowei',note:I18n.t('带有装饰感的宋体风格。此字体只有常规字重。')},
  kuaile:{label:I18n.t('站酷快乐体'),group:I18n.t('个性字体'),web:'zcool-kuaile',note:I18n.t('圆润活泼，适合轻松的界面风格。此字体只有常规字重。')},
  serif:{label:I18n.t('宋体 · 本机'),group:I18n.t('本机字体'),note:I18n.t('使用电脑已安装的宋体；其他设备可能显示为衬线后备字体。')},
  heiti:{label:I18n.t('黑体 · 本机'),group:I18n.t('本机字体'),note:I18n.t('优先使用电脑已安装的黑体，未安装时使用无衬线后备字体。')},
  kaiti:{label:I18n.t('楷体 · 本机'),group:I18n.t('本机字体'),note:I18n.t('优先使用电脑已安装的楷体，未安装时使用衬线后备字体。')},
  fangsong:{label:I18n.t('仿宋 · 本机'),group:I18n.t('本机字体'),note:I18n.t('优先使用电脑已安装的仿宋，未安装时使用衬线后备字体。')}
 };
 const choices={size:{compact:I18n.t('标准'),comfortable:I18n.t('舒适 · 默认'),large:I18n.t('大字')},contrast:{clear:I18n.t('清晰 · 默认'),strong:I18n.t('高对比度')},weight:{normal:I18n.t('常规'),medium:I18n.t('加粗')},font:Object.fromEntries(Object.entries(fonts).map(([id,f])=>[id,f.label]))};
 function loadFont(id){const font=fonts[id];if(!font?.web||document.querySelector(`link[data-reading-font="${id}"]`))return;const link=document.createElement('link');link.rel='stylesheet';link.href='./assets/fonts/'+font.web+'/font.css';link.dataset.readingFont=id;document.head.append(link)}
 const key=()=>data.user?.id?'layerhub.ui.appearance.user.'+data.user.id:null;
 const clean=p=>Object.fromEntries(Object.keys(defaults).map(k=>[k,Object.hasOwn(choices[k],p?.[k])?p[k]:defaults[k]]));
 function read(){try{return clean(JSON.parse(localStorage.getItem(key())||'{}'))}catch{return {...defaults}}}
 function apply(p){const el=document.documentElement;p=clean(p);loadFont(p.font);for(const k of Object.keys(defaults))el.dataset['ui'+k[0].toUpperCase()+k.slice(1)]=p[k]}
 function sync(){apply(key()?read():defaults)}
 function open(){
  const saved=read(),options=entries=>entries.map(([v,label])=>`<option value="${v}" ${saved.font===v?'selected':''}>${label}</option>`).join(''),select=(k,label)=>`<label><span class="field-label">${label}</span><select class="input" name="${k}" ${k==='font'?'aria-describedby="appearanceFontHint"':''}>${k==='font'?[I18n.t('日常阅读'),I18n.t('个性字体'),I18n.t('本机字体')].map(group=>`<optgroup label="${group}">${options(Object.entries(fonts).filter(([,f])=>f.group===group).map(([id,f])=>[id,f.label]))}</optgroup>`).join(''):Object.entries(choices[k]).map(([v,label])=>`<option value="${v}" ${saved[k]===v?'selected':''}>${label}</option>`).join('')}</select></label>`;
  modal(I18n.t('个性化显示'),I18n.html`<form id="appearanceForm"><p class="form-help">调整文字的清晰度与阅读大小。偏好保存在此浏览器内，按当前账户分别记忆。</p><div class="appearance-fields">${select('size',I18n.t('文字大小'))}${select('contrast',I18n.t('文字对比度'))}${select('weight',I18n.t('字重'))}${select('font',I18n.t('字体 · 10 种'))}</div><p class="appearance-font-hint" id="appearanceFontHint" aria-live="polite"></p><div id="appearancePreview" class="appearance-preview"><span class="appearance-font-name" id="appearanceFontName"></span><strong>舒适阅读，专心创造</strong><p>层层叠印，把灵感变成作品。</p><p>模型工作台 · PETG Basic · AMS 4</p><small>0123456789 · 0.20 mm · 215 °C · 80%</small></div><p id="appearanceError" class="form-error" role="alert"></p></form>`,I18n.html`<button class="button" id="appearanceReset" type="button">恢复默认</button><button class="button" data-cancel>取消</button><button class="button primary" form="appearanceForm" type="submit">保存设置</button>`);
  const form=$('#appearanceForm'),preview=$('#appearancePreview'),draft=()=>clean(Object.fromEntries(new FormData(form)));
  const draw=()=>{const p=draft(),font=fonts[p.font];loadFont(p.font);for(const [k,v] of Object.entries(p))preview.dataset['ui'+k[0].toUpperCase()+k.slice(1)]=v;$('#appearanceFontName').textContent=font.label;$('#appearanceFontHint').textContent=font.note+(font.web?I18n.t(' 网页字体从本服务器按需加载，加载时先显示后备字体。'):'')};form.onchange=draw;draw();
  $('#appearanceReset').onclick=()=>{for(const [k,v] of Object.entries(defaults))form.elements[k].value=v;draw()};
  form.onsubmit=e=>{e.preventDefault();try{const p=draft(),storageKey=key();if(!storageKey)throw Error(I18n.t('请登录后保存显示偏好'));localStorage.setItem(storageKey,JSON.stringify(p));apply(p);$('#modal').close();toast(I18n.t('显示偏好已保存到此浏览器'))}catch(e){$('#appearanceError').textContent=I18n.system(e.message)||I18n.t('浏览器未允许保存设置')}};
 }
 return {sync,open};
})();
