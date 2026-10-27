"""Create portable archives from built, clean files. Never includes data/."""
import hashlib,pathlib,re,shutil,tarfile,zipfile
root=pathlib.Path(__file__).resolve().parent.parent
version=(root/'VERSION').read_text().strip();assert re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.]+)?',version)
out=root/'packages';out.mkdir(exist_ok=True)
source=root/'source/web/source/layerhub-source.tar.gz';assert source.is_file(),'Generate corresponding source first'
docs=['README.md','README.zh-CN.md','LICENSE','THIRD_PARTY_NOTICES.md','DEPLOYMENT.md','01-部署指导-云服务器-NAS-打印机.md','TESTING.md','02-外部验收与反馈.md','OPERATIONS.md','03-升级备份与排障.md','LANGUAGES.md','DIAGNOSTICS.md','IMPORTING.md','BETA_MIGRATION.md','RELEASE_NOTES.md','SECURITY.md','CHANGELOG.md','VERSION']
start_cmd=r'''@echo off
setlocal
pushd "%~dp0" || exit /b 1
if not defined LAYERHUB_ADDRESS set "LAYERHUB_ADDRESS=127.0.0.1:5181"
if not defined LAYERHUB_DATA_DIR set "LAYERHUB_DATA_DIR=%~dp0data"
if not defined LAYERHUB_WEB_DIR set "LAYERHUB_WEB_DIR=%~dp0web"
if not defined LAYERHUB_DEVICE_MODE set "LAYERHUB_DEVICE_MODE=offline"
if not defined LAYERHUB_ENABLE_PRINT_START set "LAYERHUB_ENABLE_PRINT_START=0"
echo LayerHub: http://127.0.0.1:5181/
echo First-run credentials: %LAYERHUB_DATA_DIR%\bootstrap-admin.txt
echo Keep this window open. Ctrl+C stops the service.
bin\layerhub.exe
set "APP_EXIT=%ERRORLEVEL%"
popd
exit /b %APP_EXIT%
'''
start_sh='''#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$root"
export LAYERHUB_ADDRESS="${LAYERHUB_ADDRESS:-127.0.0.1:5181}"
export LAYERHUB_DATA_DIR="${LAYERHUB_DATA_DIR:-$root/data}"
export LAYERHUB_WEB_DIR="${LAYERHUB_WEB_DIR:-$root/web}"
export LAYERHUB_DEVICE_MODE="${LAYERHUB_DEVICE_MODE:-offline}"
export LAYERHUB_ENABLE_PRINT_START="${LAYERHUB_ENABLE_PRINT_START:-0}"
printf '%s\\n' 'Open http://127.0.0.1:5181/' 'Initial credentials: data/bootstrap-admin.txt; Ctrl+C stops the service.'
exec "$root/bin/layerhub"
'''
import io
def entries(platform):
 for name in docs:yield root/name,name
 for p in sorted((root/'source/web').rglob('*')):
  if p.is_file():yield p,'web/'+p.relative_to(root/'source/web').as_posix()
 for folder in ['deploy','licenses','validation']:
  for p in sorted((root/folder).rglob('*')):
   if p.is_file() and p.name!='.env' and (folder!='validation' or p.suffix in ['.json','.md','.png']):yield p,p.relative_to(root).as_posix()
 if platform=='windows':yield root/'binaries/windows-amd64/layerhub.exe','bin/layerhub.exe'
 else:
  for name in ['layerhub','layerhub-mail-worker']:yield root/'binaries/linux-amd64'/name,'bin/'+name
def clean(info):
 info.uid=info.gid=0;info.uname=info.gname='';info.mode=0o755 if info.name.endswith('.sh') or '/bin/' in info.name else 0o644;return info
prefix='LayerHub-'+version
with zipfile.ZipFile(out/(prefix+'-windows-amd64.zip'),'w',zipfile.ZIP_DEFLATED,compresslevel=6) as z:
 for p,name in entries('windows'):z.write(p,prefix+'/'+name)
 z.writestr(prefix+'/start.cmd',start_cmd.replace('\n','\r\n'))
with tarfile.open(out/(prefix+'-linux-amd64.tar.gz'),'w:gz') as t:
 for p,name in entries('linux'):t.add(p,arcname=prefix+'/'+name,recursive=False,filter=clean)
 raw=start_sh.encode();info=tarfile.TarInfo(prefix+'/start.sh');info.size=len(raw);info.mode=0o755;t.addfile(info,io.BytesIO(raw))
shutil.copy2(source,out/(prefix+'-source.tar.gz'))
checks=[]
for p in sorted(out.glob(prefix+'*')):checks.append(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name)
(out/'SHA256SUMS').write_text('\n'.join(checks)+'\n',encoding='utf-8')
print('\n'.join(checks))
