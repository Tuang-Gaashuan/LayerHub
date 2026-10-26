'use strict';
// Restructure the workbench once; moving controls preserves their values and handlers.
const SlicerStudio=(()=>{
 function mount(root,{onPrepare,onPreview}){
  const find=s=>root.querySelector(s), make=(tag,cls,html='')=>{const n=document.createElement(tag);n.className=cls;n.innerHTML=html;return n};
  let mode='prepare';
  document.body.classList.add('slicer-workspace');root.classList.add('slice-studio');
  const page=document.querySelector('#pageContent'),storage=document.querySelector('#sliceStorage'),heading=page.querySelector('.page-heading');
  const header=make('header','studio-header',I18n.html`<div class="studio-brand"><span>${icon('box')}</span><div><strong>模型工作台</strong><small>MODEL STUDIO</small></div></div><div class="studio-modes" role="group" aria-label="工作阶段"><button id="studioPrepare" class="active" aria-pressed="true">${icon('box')} 准备</button><button id="studioPreview" aria-pressed="false">${icon('layers')} 预览</button></div><div class="studio-main-actions"></div>`);
  header.querySelector('.studio-main-actions').append(document.querySelector('#sliceImport'),find('#sliceCompute'));heading.remove();root.prepend(header);header.after(storage);
  const settings=find('.slice-settings'),scene=find('.slice-scene-panel'),assets=find('.slice-assets');
  const body=make('div','studio-body');storage.after(body);body.append(settings,scene,assets);
  const settingsTitle=find('.slice-settings>.slice-panel-title');settingsTitle.innerHTML=I18n.html('<span>打印配置</span><span class="slice-version">单色 · 单喷嘴</span>');
  const sections=[...settings.querySelectorAll(':scope>.slice-section')],machine=find('.slice-machine');
  const tabs=make('div','studio-settings-tabs',I18n.html`<button id="studioProcess" class="active" aria-pressed="true">工艺参数</button><button id="studioObjects" aria-pressed="false">对象与摆放</button>`);
  const process=make('div','studio-settings-scroll studio-process'),objects=make('div','studio-settings-scroll studio-objects');objects.hidden=true;
  settingsTitle.after(machine,tabs,process,objects);
  process.append(sections[0],sections[1]);objects.append(sections[3],sections[4]);
  const foot=find('.slice-settings-foot');foot.className='studio-service';foot.append(find('#sliceService'));settings.append(foot);
  const tools=make('div','studio-tools',I18n.html`<div class="studio-edit-tools" role="group" aria-label="模型编辑工具"></div><div class="studio-tool-end"></div>`),edit=tools.querySelector('.studio-edit-tools');
  for(const [id,label,title]of [['sliceUndo','↶',I18n.t('撤销 · Ctrl+Z')],['sliceRedo','↷',I18n.t('重做 · Ctrl+Y')],['sliceDuplicate',I18n.t('复制'),I18n.t('复制 · Ctrl+D')],['sliceRemoveCopy',I18n.t('删除'),I18n.t('删除选中对象')],['sliceArrange',I18n.t('自动排版'),I18n.t('自动排版整盘模型')],['sliceCenter',I18n.t('居中'),I18n.t('选中对象居中并贴底')]]){const b=find('#'+id);b.textContent=label;b.title=title;b.setAttribute('aria-label',title);b.className='studio-tool';edit.append(b)}
  const transforms=make('div','studio-transform-tools',I18n.html`<button class="studio-tool active" data-transform="select" aria-label="选择模型 · V" title="选择模型 · V" aria-pressed="true">选择</button><button class="studio-tool" data-transform="box" aria-label="框选模型 · B" title="框选模型 · B" aria-pressed="false">框选</button><button class="studio-tool" data-transform="translate" aria-label="移动模型 · M" title="移动模型 · M" aria-pressed="false">移动</button><button class="studio-tool" data-transform="rotate" aria-label="旋转模型 · R" title="旋转模型 · R" aria-pressed="false">旋转</button>`);transforms.setAttribute('role','group');transforms.setAttribute('aria-label',I18n.t('画布操作'));edit.querySelector('#sliceRedo').after(transforms);
  const quarter=make('details','studio-quarter',I18n.html('<summary class="studio-tool" aria-label="90度快捷旋转">90°翻面</summary><div class="studio-quarter-menu"><strong id="sliceRotateScope">旋转选中模型</strong><div>')+['X','Y','Z'].map((axis,i)=>I18n.html`<button type="button" class="studio-tool" data-quarter="${i}" aria-label="${axis}轴角度增加90度">${axis} +90°</button>`).join('')+I18n.html('</div><small>X / Y 翻面 · Z 平面转向<br>按摆放角度增加 90°，自动贴底</small></div>'));transforms.after(quarter);
  const quarterDismiss=e=>{if(!quarter.contains(e.target))quarter.open=false};document.addEventListener('click',quarterDismiss);
  const more=make('details','studio-more',I18n.html('<summary class="studio-tool" aria-label="更多摆放操作">更多 ···</summary><div class="studio-more-menu"></div>')),menu=more.querySelector('div');edit.append(more);
  const place=make('button','studio-tool',I18n.msg`${icon('sort')} 摆放参数`);place.id='studioPlacement';place.setAttribute('aria-pressed','false');menu.append(place,edit.querySelector('#sliceCenter'));
  const ground=make('button','studio-tool',I18n.t('重新贴底'));ground.id='sliceGround';ground.title=I18n.t('保持 X/Y、角度与缩放，将最低点放到 Z=0');menu.append(ground);
  const resize=make('button','studio-tool',I18n.t('调整模型尺寸…'));resize.id='sliceResize';menu.append(resize);
  const batchCopies=make('button','studio-tool',I18n.t('批量复制与自动分盘…'));batchCopies.id='sliceBatch';menu.append(batchCopies);
  const reset=make('button','studio-tool',I18n.t('重置旋转'));reset.id='sliceReset';menu.append(reset);
  menu.addEventListener('click',e=>{if(e.target.closest('button'))more.open=false});
  const dismiss=e=>{if(!more.contains(e.target))more.open=false},escape=e=>{if(e.key==='Escape'){more.open=false;quarter.open=false}};document.addEventListener('click',dismiss);document.addEventListener('keydown',escape);
  const views=find('.slice-view-tools');tools.querySelector('.studio-tool-end').append(views);
  const library=make('button','studio-tool studio-library-toggle',I18n.msg`${icon('files')} 模型与历史`);library.id='studioLibrary';library.setAttribute('aria-expanded','false');library.setAttribute('aria-controls','studioAssets');tools.querySelector('.studio-tool-end').append(library);
  scene.prepend(tools);sections[2].remove();find('.slice-copies .slice-copy-actions').remove();
  const copiesHelp=find('.slice-copies .form-help');copiesHelp.textContent=I18n.t('整盘最多 64 个对象，可混排不同 STL；同层、单色打印，保留至少 5 mm 间距。');
  const problems=make('div','studio-object-issues');problems.id='sliceIssues';problems.hidden=true;find('#sliceCopies').after(problems);
  const scope=make('div','studio-selection-scope',I18n.html('<strong id="sliceSelectionScope"></strong><div class="studio-selection-actions"><button type="button" id="sliceMultiToggle" aria-pressed="false">多选</button><button type="button" id="sliceSelectAll">全选本盘</button><button type="button" id="sliceSelectOnly">仅选当前</button></div><small>Ctrl / ⌘ 点选 · Shift 连选 · Ctrl+A 全选本盘</small>'));scope.id='sliceSelectionInfo';find('#sliceCopies').before(scope);
  sections[4].append(make('p','form-help',I18n.t('自动贴底已开启：旋转、缩放后，模型最低点保持 Z = 0。')));
  find('#sliceDrag').remove();
  const footHint=find('.slice-scene-footer>span:last-child');footHint.id='sliceToolHint';
  const batch=make('details','studio-batch',I18n.html('<summary>批量复制与排版说明</summary>'));batch.append(find('.slice-copies>label'),copiesHelp);sections[3].append(batch);
  const inspector=make('aside','studio-preview-inspector');inspector.hidden=true;inspector.setAttribute('aria-label',I18n.t('切片结果与逐层预览'));inspector.append(find('#sliceResult'),find('#sliceToolpath'));body.append(inspector);
  const sceneBar=find('.slice-scene-bar');sceneBar.querySelector('div:first-child').insertAdjacentHTML('beforeend',I18n.html('<span class="studio-plate-tag">打印板 01</span>'));
  const message=find('#sliceMessage');scene.append(message);
  assets.id='studioAssets';assets.hidden=true;const closeAssets=make('button','icon-button',icon('close'));closeAssets.setAttribute('aria-label',I18n.t('关闭模型与历史'));assets.querySelector('.slice-panel-title').append(closeAssets);
  function libraryOpen(open){assets.hidden=!open;root.classList.toggle('library-open',open);library.setAttribute('aria-expanded',String(open));if(!open&&assets.contains(document.activeElement))library.focus()}
  library.onclick=()=>libraryOpen(assets.hidden);closeAssets.onclick=()=>libraryOpen(false);
  assets.addEventListener('click',e=>{if(e.target.closest('[data-model],[data-slice-task]'))libraryOpen(false)});
  function settingsTab(object){process.hidden=object;objects.hidden=!object;for(const [id,active]of [['studioProcess',!object],['studioObjects',object],['studioPlacement',object]]){const b=find('#'+id);b.classList.toggle('active',active);b.setAttribute('aria-pressed',String(active))}}
  find('#studioProcess').onclick=()=>settingsTab(false);find('#studioObjects').onclick=place.onclick=()=>{onPrepare();settingsTab(true)};
  find('#studioPrepare').onclick=onPrepare;find('#studioPreview').onclick=onPreview;
  const status=make('footer','studio-bottom-bar',I18n.html`<span>X2D · 256 × 256 mm</span><span>准备 → 切片预览 → 确认设备与料卷</span><span>确认一次，自动传输并启动 · 也可仅排队</span>`);root.append(status);
  function setMode(next){mode=next;root.dataset.mode=next;const preview=next==='preview';settings.hidden=preview;inspector.hidden=!preview;for(const [id,active]of [['studioPrepare',!preview],['studioPreview',preview]]){const b=find('#'+id);b.classList.toggle('active',active);b.setAttribute('aria-pressed',String(active))}edit.hidden=preview;message.hidden=preview}
  setMode('prepare');
  return {setMode,closeLibrary(){libraryOpen(false)},get mode(){return mode},close(){document.body.classList.remove('slicer-workspace');document.removeEventListener('click',dismiss);document.removeEventListener('click',quarterDismiss);document.removeEventListener('keydown',escape)}};
 }
 return {mount};
})();
