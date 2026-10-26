'use strict';
const SlicerPresets=(()=>{
 const clone=v=>JSON.parse(JSON.stringify(v));
 function settings(o){return {material:o.material||'petg-basic',layer:o.layer,infill:o.infill,support:!!o.support,brim:!!o.brim,advanced:clone(SlicerProcess.effective(o))}}
 const material=s=>SlicerMaterials.get(s.material).label;
 const summary=s=>I18n.msg`${material(s)} · ${s.layer.toFixed(2)} mm · ${s.infill}% 填充 · ${s.advanced.walls} 层墙`;
 function fields(s){const a=s.advanced;return {材料:material(s),层高:s.layer.toFixed(2)+' mm',填充密度:s.infill+'%',墙壁层数:a.walls+I18n.t(' 层'),顶面厚度:a.topThickness+' mm',底面厚度:a.bottomThickness+' mm',填充图案:({grid:I18n.t('网格'),gyroid:'Gyroid',rectilinear:I18n.t('直线')})[a.pattern],生成支撑:s.support?I18n.t('开启'):I18n.t('关闭'),支撑类型:a.supportType==='tree'?I18n.t('树状自动'):I18n.t('普通自动'),悬垂阈值:a.supportAngle+'°',仅打印板支撑:a.plateOnly?I18n.t('开启'):I18n.t('关闭'),外侧裙边:s.brim?I18n.t('开启'):I18n.t('关闭')}}
 function mount({host,get,apply,context,canApply=()=>true}){
  let alive=true,presets=[];const uid=data.user?.id;
  const valid=()=>alive&&host.isConnected&&data.user?.id===uid;
  const tools=document.createElement('div');tools.className='process-recipe-tools';tools.innerHTML=I18n.html('<button type="button" class="text-link" id="processPresetBrowse">我的工艺预设</button><button type="button" class="text-link" id="processPresetSave">保存当前工艺</button>');host.append(tools);
  async function load(root){try{const r=await Remote.request('/api/slicing/presets');if(!valid()||!root.isConnected)return false;presets=r.presets;return true}catch(e){if(valid()&&root.isConnected){root.innerHTML=I18n.html`<p class="form-error" role="alert">${esc(I18n.system(e.message))}</p><button class="button" id="presetReadAgain">重新读取</button>`;root.querySelector('button').onclick=browse}return false}}
  function shell(title){modal(title,I18n.html('<div class="process-presets" id="processPresetRoot"><p role="status">正在读取私有预设…</p></div>'));$('#modal').classList.add('process-preset-modal');return $('#processPresetRoot')}
  async function browse(){if(!valid())return;const root=shell(I18n.t('我的工艺预设'));if(await load(root))drawLibrary(root)}
  function drawLibrary(root){
   const active=presets.filter(p=>!p.archived),archived=presets.filter(p=>p.archived);
   root.innerHTML=I18n.html`<div class="preset-library-intro"><div><strong>把调好的工艺留下来</strong><p>X2D · 0.4 mm · 仅当前账号可见，跨电脑保存。</p></div><button type="button" class="button primary small" id="presetNew">保存当前工艺</button></div><div class="preset-list">${active.map(card).join('')||I18n.html('<div class="preset-empty">还没有保存的工艺。<br>调好左侧参数后，保存一份，下次直接套用。</div>')}</div>${archived.length?I18n.html`<details class="preset-archived"><summary>已收起的预设 · ${archived.length}</summary>${archived.map(p=>I18n.html`<div class="preset-archive-row"><span>${esc(p.name)}</span><button type="button" class="text-link" data-preset-restore="${esc(p.id)}">恢复</button></div>`).join('')}</details>`:''}<p class="form-help">预设包含材料、质量、强度与支撑。预设只包含整盘设置；套用时保留模型摆放、数量、颜色和对象单独工艺。最多保留 100 份（含已收起）。</p><p class="form-error" id="presetLibraryError" role="alert"></p>`;
   root.querySelector('#presetNew').onclick=save;
   root.querySelectorAll('[data-preset-apply]').forEach(b=>b.onclick=()=>review(presets.find(p=>p.id===b.dataset.presetApply)));
   root.querySelectorAll('[data-preset-archive]').forEach(b=>b.onclick=()=>archive(presets.find(p=>p.id===b.dataset.presetArchive),true,b,root));
   root.querySelectorAll('[data-preset-restore]').forEach(b=>b.onclick=()=>archive(presets.find(p=>p.id===b.dataset.presetRestore),false,b,root));
  }
  function card(p){return I18n.html`<article class="preset-card"><div><h3>${esc(p.name)}</h3><p>${esc(summary(p.settings))}</p><small>${p.settings.support?I18n.t('已开启支撑'):I18n.t('未开启支撑')} · ${p.settings.brim?I18n.t('外侧裙边'):I18n.t('无裙边')}${SlicerMaterials.get(p.settings.material).printReady?'':I18n.t(' · 在线打印待验证')}</small></div><div class="preset-card-actions"><button type="button" class="button small" data-preset-apply="${esc(p.id)}">检查并套用</button><button type="button" class="text-link" data-preset-archive="${esc(p.id)}" aria-label="收起预设 ${esc(p.name)}">收起</button></div></article>`}
  async function archive(p,value,button,root){if(!valid()||!p||button.disabled)return;button.disabled=true;try{await Remote.request('/api/slicing/presets/archive',{id:p.id,revision:p.revision,archived:value});if(!valid()||!root.isConnected)return;if(await load(root))drawLibrary(root)}catch(e){if(!root.isConnected||!valid())return;root.querySelector('#presetLibraryError').textContent=I18n.system(e.message)+(e.unconfirmed?I18n.t(' 请重新读取预设列表确认结果。'):'');if(e.unconfirmed){const b=document.createElement('button');b.className='button';b.textContent=I18n.t('重新读取预设');b.onclick=browse;root.append(b)}else button.disabled=false}}
  async function save(){
   if(!valid())return;
   const current=settings(get()),err=I18n.system(SlicerProcess.error)({...get(),advanced:current.advanced});if(err){toast(err);return}
   const root=shell(I18n.t('保存当前工艺'));if(!await load(root))return;
   const active=presets.filter(p=>!p.archived),requestID=crypto.randomUUID();
   root.innerHTML=I18n.html`<div class="preset-save-snapshot"><strong>当前盘的工艺快照</strong><p>${esc(summary(current))}</p><small>${esc(SlicerProcess.describe({...current}))}</small></div><form id="presetSaveForm"><label>保存方式<select class="input" name="target"><option value="">保存为新预设</option>${active.map(p=>I18n.html`<option value="${esc(p.id)}">更新：${esc(p.name)}</option>`).join('')}</select></label><label>预设名称<input class="input" name="name" required maxlength="40" placeholder="例如：PETG 支架 · 日常强度" value="${esc(material(current)+' · '+current.layer.toFixed(2)+' mm')}"></label><p class="form-help" id="presetSaveHint">仅保存整盘参数，不包含对象单独设置；之后修改工作台不会改动此预设。</p><p class="form-error" id="presetSaveError" role="alert"></p><div class="preset-dialog-actions"><button type="button" class="button" id="presetSaveCancel">返回预设库</button><button type="submit" class="button primary">保存预设</button></div></form>`;
   const form=root.querySelector('form');form.elements.target.onchange=()=>{const p=active.find(p=>p.id===form.elements.target.value);form.elements.name.value=p?.name||material(current)+' · '+current.layer.toFixed(2)+' mm';root.querySelector('#presetSaveHint').textContent=p?I18n.t('将用这份当前盘快照更新所选预设；已保存的工程和历史切片保持原参数。'):I18n.t('仅保存整盘参数，不包含对象单独设置；之后修改工作台不会改动此预设。');form.querySelector('[type=submit]').textContent=p?I18n.t('确认更新预设'):I18n.t('保存预设')};
   root.querySelector('#presetSaveCancel').onclick=browse;
   form.onsubmit=async e=>{e.preventDefault();if(form.dataset.submitting||!valid())return;form.dataset.submitting='true';const b=form.querySelector('[type=submit]');b.disabled=true;const target=active.find(p=>p.id===form.elements.target.value),body={id:target?.id||requestID,revision:target?.revision||0,name:form.elements.name.value,settings:current};try{await Remote.request('/api/slicing/presets',body);if(!valid()||!root.isConnected)return;toast(I18n.t('工艺预设已保存'));await browse()}catch(error){if(!valid()||!root.isConnected)return;root.querySelector('#presetSaveError').textContent=I18n.system(error.message);if(error.unconfirmed){root.querySelector('#presetSaveError').textContent+=I18n.t(' 请返回预设库重新读取，确认保存结果。')}else{delete form.dataset.submitting;b.disabled=false}}};
  }
  function review(p){
   if(!valid()||!p)return;if(!canApply()){toast(I18n.t('请先结束当前编辑或等待切片提交完成'));return}
   const initial=settings(get()),plate=context(),signature=JSON.stringify(initial),before=fields(initial),after=fields(p.settings),changes=Object.keys(before).filter(k=>before[k]!==after[k]);
   const root=shell(I18n.t('套用工艺前检查'));
   root.innerHTML=I18n.html`<div class="preset-review-heading"><span class="preset-review-icon">${icon('layers')}</span><div><strong>${esc(p.name)}</strong><p>${esc(summary(p.settings))}</p></div></div>${changes.length?I18n.html`<div class="preset-diff"><table><thead><tr><th>参数</th><th>当前盘</th><th>套用后</th></tr></thead><tbody>${changes.map(k=>`<tr><th>${esc(k)}</th><td>${esc(before[k])}</td><td>${esc(after[k])}</td></tr>`).join('')}</tbody></table></div>`:I18n.html('<p class="preset-no-diff">当前盘与这份预设的参数一致。</p>')}<p class="form-help">仅应用到当前打印板，保留模型摆放、数量与显示颜色。更改后需重新切片，可用撤销恢复。</p>${SlicerMaterials.get(p.settings.material).printReady?'':I18n.html('<p class="preset-material-note">此材料支持切片、预览和下载；在线打印仍待实机验证。</p>')}<p class="form-error" id="presetReviewError" role="alert"></p><div class="preset-dialog-actions"><button type="button" class="button" id="presetReviewBack">返回预设库</button><button type="button" class="button primary" id="presetApply" ${changes.length?'':'disabled'}>套用到当前盘</button></div>`;
   root.querySelector('#presetReviewBack').onclick=browse;
   root.querySelector('#presetApply').onclick=()=>{if(!valid()||!root.isConnected)return;if(!canApply()||context()!==plate||JSON.stringify(settings(get()))!==signature){root.querySelector('#presetReviewError').textContent=I18n.t('当前盘或工艺已发生变化，请返回预设库重新检查。');return}apply(clone(p.settings));$('#modal').close();toast(I18n.t('已套用工艺，可使用撤销恢复'))};
  }
  tools.querySelector('#processPresetBrowse').onclick=browse;tools.querySelector('#processPresetSave').onclick=save;
  return {close(){alive=false;tools.remove()}};
 }
 return {mount,settings,fields};
})();
