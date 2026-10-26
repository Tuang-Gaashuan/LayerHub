'use strict';
// Approved V3 illustration catalog. AMS is a representative illustrated configuration.
const PRINTER_MODELS = [
  {
    "id": "a1-mini",
    "name": "A1 mini",
    "series": "A",
    "ams": "AMS lite",
    "subtitle": I18n.t("单悬臂 · 紧凑开放式"),
    "status": "current"
  },
  {
    "id": "a1",
    "name": "A1",
    "series": "A",
    "ams": "AMS lite",
    "subtitle": I18n.t("双立柱 · 开放式"),
    "status": "current"
  },
  {
    "id": "a2l",
    "name": "A2L",
    "series": "A",
    "ams": "AMS lite",
    "subtitle": I18n.t("大幅面 · 开放式"),
    "status": "current"
  },
  {
    "id": "p1p",
    "name": "P1P",
    "series": "P",
    "ams": "AMS",
    "subtitle": I18n.t("开放式 CoreXY 框架"),
    "status": "legacy"
  },
  {
    "id": "p1s",
    "name": "P1S",
    "series": "P",
    "ams": "AMS",
    "subtitle": I18n.t("深色封闭机箱 · 小型按键屏"),
    "status": "current"
  },
  {
    "id": "p2s",
    "name": "P2S",
    "series": "P",
    "ams": "AMS 2 Pro",
    "subtitle": I18n.t("封闭机箱 · 大触控屏"),
    "status": "current"
  },
  {
    "id": "x1",
    "name": "X1",
    "series": "X",
    "ams": "AMS",
    "subtitle": I18n.t("浅色机身 · 触控屏"),
    "status": "legacy"
  },
  {
    "id": "x1-carbon",
    "name": "X1 Carbon",
    "series": "X",
    "ams": "AMS",
    "subtitle": I18n.t("银灰机身 · 深色顶盖"),
    "status": "legacy"
  },
  {
    "id": "x1e",
    "name": "X1E",
    "series": "X",
    "ams": "AMS",
    "subtitle": I18n.t("企业机型 · 银灰外壳"),
    "status": "legacy"
  },
  {
    "id": "x2d",
    "name": "X2D",
    "series": "X",
    "ams": "AMS 2 Pro",
    "subtitle": I18n.t("新一代 X 系列 · 双喷嘴"),
    "status": "current"
  },
  {
    "id": "h2s",
    "name": "H2S",
    "series": "H",
    "ams": "AMS 2 Pro",
    "subtitle": I18n.t("宽体封闭机箱 · 单喷嘴"),
    "status": "current"
  },
  {
    "id": "h2d",
    "name": "H2D",
    "series": "H",
    "ams": "AMS 2 Pro",
    "subtitle": I18n.t("宽体封闭机箱 · 双喷嘴"),
    "status": "current"
  },
  {
    "id": "h2d-pro",
    "name": "H2D Pro",
    "series": "H",
    "ams": "AMS 2 Pro + AMS HT",
    "subtitle": I18n.t("企业机型 · 双喷嘴"),
    "status": "current"
  },
  {
    "id": "h2c",
    "name": "H2C",
    "series": "H",
    "ams": "AMS 2 Pro",
    "subtitle": I18n.t("Vortek 换热端系统"),
    "status": "current"
  }
];
function printerModel(printer) {
  if (printer.modelId) return PRINTER_MODELS.find(m=>m.id===printer.modelId);
  const name=String(printer.model||'').split(' · ')[0].replace(/^Bambu Lab /,'').trim().toLowerCase();
  return PRINTER_MODELS.find(m=>m.name.toLowerCase()===name);
}
function normalizePrinter(printer) {
  const model=printerModel(printer);
  if (!model) return printer;
  const configuration=printer.configuration==='bare'||printer.configuration==='ams'
    ? printer.configuration : /AMS/i.test(printer.model||'')?'ams':'bare';
  printer.modelId=model.id;
  printer.configuration=configuration;
  printer.model=model.name+' · '+(configuration==='ams'?model.ams:I18n.t('裸机'));
  return printer;
}
function printerSupply(printer) {
  const model=printerModel(printer);
  return model ? (printer.configuration==='ams'?model.ams:I18n.t('外置单卷')) : I18n.t('配置待确认');
}
function printerAsset(printer) {
  const model=printerModel(printer);
  return model ? './assets/printers/'+model.id+'-'+(printer.configuration==='ams'?'ams':'bare')+'.png' : '';
}
