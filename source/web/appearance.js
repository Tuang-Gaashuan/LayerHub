'use strict';
const Appearance=(()=>{
 const defaults={size:'comfortable',contrast:'clear',weight:'normal',font:'system',mode:'system',accent:'blue',customColor:'#3159ed',density:'comfortable'};
 const fonts={
  system:{label:I18n.t('系统无衬线'),group:I18n.t('日常阅读'),note:I18n.t('跟随设备的系统字体，清晰、轻快。')},
  theme:{label:I18n.t('主题字体（Bahnschrift + 微软雅黑）'),group:I18n.t('日常阅读'),note:I18n.t('英文和数字优先使用 Bahnschrift，中文使用微软雅黑；未安装时使用系统无衬线字体。')},
  monospace:{label:I18n.t('等宽（数据感）'),group:I18n.t('日常阅读'),note:I18n.t('英文和数字使用本机等宽字体，便于对齐数据；中文使用微软雅黑或系统后备字体。')},
  noto:{label:I18n.t('思源黑体'),group:I18n.t('日常阅读'),web:'noto-sans-sc',note:I18n.t('笔画规整，适合长时间阅读参数和任务。')},
  notoserif:{label:I18n.t('思源宋体'),group:I18n.t('日常阅读'),web:'noto-serif-sc',note:I18n.t('具有衬线与笔画粗细变化，偏书刊阅读风格。')},
  wenkai:{label:I18n.t('霞鹜文楷'),group:I18n.t('个性字体'),web:'lxgw-wenkai',note:I18n.t('自然的手写楷意，风格舒展。采用屏显字重；完整字库首次加载约 7 MB。')},
  xiaowei:{label:I18n.t('站酷小薇'),group:I18n.t('个性字体'),web:'zcool-xiaowei',note:I18n.t('带有装饰感的宋体风格。此字体只有常规字重。')},
  kuaile:{label:I18n.t('站酷快乐体'),group:I18n.t('个性字体'),web:'zcool-kuaile',note:I18n.t('圆润活泼，适合轻松的界面风格。此字体只有常规字重。')},
  serif:{label:I18n.t('宋体 · 本机'),group:I18n.t('本机字体'),note:I18n.t('使用电脑已安装的宋体；其他设备可能显示为衬线后备字体。')},
  heiti:{label:I18n.t('黑体 · 本机'),group:I18n.t('本机字体'),note:I18n.t('优先使用电脑已安装的黑体，未安装时使用无衬线后备字体。')},
  kaiti:{label:I18n.t('楷体 · 本机'),group:I18n.t('本机字体'),note:I18n.t('优先使用电脑已安装的楷体，未安装时使用衬线后备字体。')},
  fangsong:{label:I18n.t('仿宋 · 本机'),group:I18n.t('本机字体'),note:I18n.t('优先使用电脑已安装的仿宋，未安装时使用衬线后备字体。')},
  stzhongsong:{label:I18n.t('华文中宋（推荐）'),group:I18n.t('本机字体'),note:I18n.t('使用本机已安装的华文中宋，适合正文阅读；未安装时使用宋体或系统衬线字体。')},
  stsong:{label:I18n.t('华文宋体'),group:I18n.t('本机字体'),note:I18n.t('使用本机已安装的华文宋体；未安装时使用宋体或系统衬线字体。')},
  stxihei:{label:I18n.t('华文细黑'),group:I18n.t('本机字体'),note:I18n.t('使用本机已安装的华文细黑；未安装时使用微软雅黑或系统无衬线字体。')},
  stkaiti:{label:I18n.t('华文楷体'),group:I18n.t('本机字体'),note:I18n.t('使用本机已安装的华文楷体；未安装时使用楷体或系统衬线字体。')},
  youyuan:{label:I18n.t('幼圆'),group:I18n.t('本机字体'),note:I18n.t('使用本机已安装的幼圆，字形圆润；未安装时使用微软雅黑或系统无衬线字体。')},
  lishu:{label:I18n.t('隶书'),group:I18n.t('本机字体'),note:I18n.t('使用本机已安装的隶书，装饰性较强；未安装时使用宋体或系统衬线字体。')},
  sthupo:{label:I18n.t('华文琥珀'),group:I18n.t('本机字体'),note:I18n.t('使用本机已安装的华文琥珀，适合装饰性展示；未安装时使用微软雅黑或系统无衬线字体。')},
  bahnschrift:{label:'Bahnschrift',group:I18n.t('本机字体'),note:I18n.t('使用本机已安装的 Bahnschrift 显示英文和数字，中文使用微软雅黑；未安装时使用系统无衬线字体。')}
 };
 const palette={blue:'#3159ed',green:'#087f65',cyan:'#087f96',purple:'#7952cc',orange:'#b85b11',rose:'#b83b70'};
 const accentNames={blue:I18n.t('蓝色'),green:I18n.t('绿色'),cyan:I18n.t('青色'),purple:I18n.t('紫色'),orange:I18n.t('橙色'),rose:I18n.t('玫红'),custom:I18n.t('自定义')};
 const choices={size:{compact:I18n.t('标准'),comfortable:I18n.t('舒适 · 默认'),large:I18n.t('大字')},contrast:{clear:I18n.t('清晰 · 默认'),strong:I18n.t('高对比度')},weight:{normal:I18n.t('常规'),medium:I18n.t('加粗')},font:Object.fromEntries(Object.entries(fonts).map(([id,f])=>[id,f.label])),mode:{light:I18n.t('浅色'),dark:I18n.t('深色'),system:I18n.t('跟随系统')},accent:accentNames,density:{roomy:I18n.t('宽松'),comfortable:I18n.t('舒适 · 默认'),compact:I18n.t('紧凑')}};
 let editing=null;
 const media=typeof matchMedia==='function'?matchMedia('(prefers-color-scheme: dark)'):null;
 function loadFont(id){const font=fonts[id];if(!font?.web||document.querySelector(`link[data-reading-font="${id}"]`))return;const link=document.createElement('link');link.rel='stylesheet';link.href='./assets/fonts/'+font.web+'/font.css';link.dataset.readingFont=id;document.head.append(link)}
 const key=()=>data.user?.id?'layerhub.ui.appearance.user.'+data.user.id:null;
 function clean(p){const out={};for(const k of Object.keys(defaults))out[k]=k==='customColor'?(/^#[0-9a-f]{6}$/i.test(p?.[k]||'')?p[k].toLowerCase():defaults[k]):Object.hasOwn(choices[k],p?.[k])?p[k]:defaults[k];return out}
 function read(){try{return clean(JSON.parse(localStorage.getItem(key())||'{}'))}catch{return {...defaults}}}
 const rgb=hex=>[1,3,5].map(i=>parseInt(hex.slice(i,i+2),16));
 const hex=values=>'#'+values.map(v=>Math.round(v).toString(16).padStart(2,'0')).join('');
 const luminance=values=>values.map(v=>{v/=255;return v<=.04045?v/12.92:((v+.055)/1.055)**2.4}).reduce((n,v,i)=>n+v*[.2126,.7152,.0722][i],0);
 function buttonColor(raw){let values=rgb(raw);for(let i=0;i<60&&1.05/(luminance(values)+.05)<4.5;i++)values=values.map(v=>v*.94);return hex(values)}
 function apply(p){
  const el=document.documentElement;p=clean(p);loadFont(p.font);for(const k of Object.keys(defaults))el.dataset['ui'+k[0].toUpperCase()+k.slice(1)]=p[k];
  const dark=p.mode==='dark'||p.mode==='system'&&!!media?.matches,raw=palette[p.accent]||p.customColor,button=buttonColor(raw);
  el.dataset.uiColorScheme=dark?'dark':'light';el.style.colorScheme=dark?'dark':'light';el.style.setProperty('--accent-button',button);el.style.setProperty('--accent-hover',hex(rgb(button).map(v=>v*.86)));
  el.style.setProperty('--blue',dark?hex(rgb(button).map(v=>v+(255-v)*.52)):button);
  el.style.setProperty('--accent-soft',hex(rgb(raw).map((v,i)=>dark?v*.16+[19,27,40][i]*.84:v*.09+255*.91)));
  el.style.setProperty('--accent-ring',hex(rgb(raw).map((v,i)=>dark?v*.5+[19,27,40][i]*.5:v*.35+255*.65)));
  const meta=document.querySelector('meta[name="theme-color"]');if(meta)meta.content=dark?'#101722':button;
 }
 function remember(p){try{localStorage.setItem('layerhub.ui.startupMode',clean(p).mode)}catch{}}
 function sync(){if(editing){if(editing.key===key()&&$('#appearanceForm')?.isConnected&&$('#modal').open)return;editing=null;if($('#appearanceForm')&&$('#modal').open)$('#modal').close()}const p=key()?read():defaults;apply(p);remember(p)}
 media?.addEventListener?.('change',()=>{if(editing?.key===key()&&$('#appearanceForm'))apply(editing.draft());else sync()});
 function open(){
  if(editing){apply(read());editing=null}
  const saved=read(),owner=key();let committed=false,observer,session;
  const options=(k,entries)=>entries.map(([v,label])=>`<option value="${v}" ${saved[k]===v?'selected':''}>${label}</option>`).join('');
  const select=(k,label)=>`<label><span class="field-label">${label}</span><select class="input" name="${k}">${k==='font'?[I18n.t('日常阅读'),I18n.t('个性字体'),I18n.t('本机字体')].map(group=>`<optgroup label="${group}">${options(k,Object.entries(fonts).filter(([,f])=>f.group===group).map(([id,f])=>[id,f.label]))}</optgroup>`).join(''):options(k,Object.entries(choices[k]))}</select></label>`;
  const restore=()=>{observer?.disconnect();$('#modal')?.removeEventListener('close',restore);if(editing!==session)return;editing=null;if(!committed)apply(key()?read():defaults)};
  const close=()=>{restore();$('#modal').close()};
  modal(I18n.t('个性化显示'),I18n.html`<form id="appearanceForm"><p class="form-help">实时预览整个界面；保存后按当前账户记忆在此浏览器中。</p><div class="appearance-layout"><div class="appearance-controls"><fieldset><legend>外观与颜色</legend><div class="appearance-fields">${select('mode',I18n.t('界面模式'))}${select('accent',I18n.t('主题色'))}</div><div class="appearance-swatches" role="group" aria-label="预设主题色">${Object.entries(palette).map(([id,color])=>`<button type="button" class="appearance-swatch" data-accent="${id}" style="--swatch:${color}" aria-label="${accentNames[id]}" aria-pressed="false"><span></span>${accentNames[id]}</button>`).join('')}</div><label class="appearance-custom"><span>自定义颜色</span><input type="color" id="appearanceColorPicker" value="${saved.customColor}" aria-label="选择自定义颜色"><input class="input" name="customColor" value="${saved.customColor}" pattern="#[0-9a-fA-F]{6}" maxlength="7" aria-label="十六进制颜色"></label><p class="form-help">主题色用于按钮与导航；设备状态和耗材颜色保持独立。过浅的颜色会自动加深按钮，保证文字清晰。</p></fieldset><fieldset><legend>阅读与布局</legend><div class="appearance-fields">${select('font',I18n.msg`字体 · ${Object.keys(fonts).length} 种`)}${select('size',I18n.t('文字大小'))}${select('weight',I18n.t('字重'))}${select('contrast',I18n.t('文字对比度'))}${select('density',I18n.t('页面密度'))}</div><p class="appearance-font-hint" id="appearanceFontHint" aria-live="polite"></p><p class="form-help">字号与间距分别设置，大字也可以保留宽松布局。</p></fieldset></div><aside><div id="appearancePreview" class="appearance-preview"><span class="appearance-font-name" id="appearanceFontName"></span><strong>舒适阅读，专心创造</strong><p>团队设备 · 打印任务 · 耗材管理</p><div class="appearance-sample-card"><span class="appearance-sample-state">● 设备空闲</span><h3>X2D · PETG Basic</h3><p>预计 45 分钟 · 24.16 g</p><div class="appearance-sample-progress"><i></i></div><span class="appearance-sample-button">确认准备</span></div><small>0123456789 · 0.20 mm · 215 °C · 80%</small></div><p class="form-help">背景界面同步预览，取消即可还原。</p></aside></div><p id="appearanceError" class="form-error" role="alert"></p></form>`,I18n.html`<button class="button" id="appearanceReset" type="button">恢复默认</button><button class="button" data-cancel>取消</button><button class="button primary" form="appearanceForm" type="submit">保存设置</button>`,{onDismiss:close});
  const form=$('#appearanceForm'),preview=$('#appearancePreview'),draft=()=>clean(Object.fromEntries(new FormData(form)));
  session={key:owner,draft};editing=session;
  const draw=()=>{if(owner!==key()){close();return}const p=draft(),font=fonts[p.font],valid=/^#[0-9a-f]{6}$/i.test(form.elements.customColor.value);apply(p);for(const [k,v] of Object.entries(p))preview.dataset['ui'+k[0].toUpperCase()+k.slice(1)]=v;$('#appearanceFontName').textContent=font.label;$('#appearanceFontHint').textContent=font.note+(font.web?I18n.t(' 网页字体从本服务器按需加载，加载时先显示后备字体。'):'');$('#appearanceColorPicker').value=p.customColor;$('#appearanceError').textContent=valid?'':I18n.t('请输入完整颜色，例如 #3159ed');$('[type="submit"][form="appearanceForm"]').disabled=!valid;$$('[data-accent]',form).forEach(b=>b.setAttribute('aria-pressed',String(b.dataset.accent===p.accent)))};
  form.onchange=draw;
  form.elements.customColor.oninput=()=>{form.elements.accent.value='custom';draw()};
  $('#appearanceColorPicker').oninput=e=>{form.elements.accent.value='custom';form.elements.customColor.value=e.target.value;draw()};
  $$('[data-accent]',form).forEach(b=>b.onclick=()=>{form.elements.accent.value=b.dataset.accent;draw()});
  $('#appearanceReset').onclick=()=>{for(const [k,v] of Object.entries(defaults))form.elements[k].value=v;draw()};
  form.onsubmit=e=>{e.preventDefault();if(owner!==key()||!form.reportValidity())return;try{const p=draft();if(!owner)throw Error(I18n.t('请登录后保存显示偏好'));localStorage.setItem(owner,JSON.stringify(p));committed=true;apply(p);remember(p);close();toast(I18n.t('显示偏好已保存到此浏览器'))}catch(e){$('#appearanceError').textContent=I18n.system(e.message)||I18n.t('浏览器未允许保存设置')}};
  $('#modal').addEventListener('close',restore,{once:true});observer=new MutationObserver(()=>{if(!form.isConnected||!$('#modal').open)restore()});observer.observe($('#modal'),{childList:true,subtree:true,attributes:true,attributeFilter:['open']});draw();
 }
 return {sync,open};
})();
