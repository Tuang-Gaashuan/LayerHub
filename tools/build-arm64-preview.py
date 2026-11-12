"""Build a local Jetson ARM64 preview without modifying published release assets."""
import argparse,hashlib,importlib.util,io,json,os,pathlib,shutil,subprocess,tarfile,tempfile
p=argparse.ArgumentParser();p.add_argument('--go',default='go');a=p.parse_args()
root=pathlib.Path(__file__).resolve().parent.parent;version=(root/'VERSION').read_text().strip();name='LayerHub-'+version+'-jetson-arm64-preview'
source=root/'source';out=root/'local-packages/jetson-arm64';out.mkdir(parents=True,exist_ok=True)
env=dict(os.environ,CGO_ENABLED='0',GOOS='linux',GOARCH='arm64',GOARM64='v8.0')
binaries=root/'binaries/linux-arm64';binaries.mkdir(parents=True,exist_ok=True)
spec=importlib.util.spec_from_file_location('platform_check',root/'deploy/nas/check-platform.py');checker=importlib.util.module_from_spec(spec);spec.loader.exec_module(checker)
for binary,package in [('layerhub','server'),('layerhub-mail-worker','mail-worker')]:
    subprocess.run([a.go,'build','-mod=vendor','-trimpath','-buildvcs=false','-o',str(binaries/binary),'./cmd/'+package],env=env,cwd=source,check=True)
    checker.check_binary(binaries/binary,'arm64')
subprocess.run([__import__('sys').executable,'tools/make-source.py'],cwd=source,check=True)
with tempfile.TemporaryDirectory(prefix='.arm64-stage-',dir=out) as tmp:
    stage=pathlib.Path(tmp)/name;stage.mkdir();shutil.copytree(binaries,stage/'bin');shutil.copytree(source/'web',stage/'web')
    for folder in ['deploy','licenses','docs']:shutil.copytree(root/folder,stage/folder)
    for file in root.glob('*.md'):shutil.copy2(file,stage/file.name)
    for file in ['VERSION','LICENSE']:shutil.copy2(root/file,stage/file)
    (stage/'PREVIEW.txt').write_text('Jetson Orin Nano 8GB / Ubuntu 22.04 ARM64 preview.\nCross-built only; real-device performance and printer acceptance pending.\nUse JETSON.zh-CN.md or JETSON.md. Existing AMD64 UpdateKit is not supported on ARM64.\n',encoding='utf-8')
    (stage/'start.sh').write_text('''#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
python3 "$root/deploy/nas/check-platform.py" "$root/bin/layerhub"
export LAYERHUB_ADDRESS="${LAYERHUB_ADDRESS:-127.0.0.1:5181}"
export LAYERHUB_DATA_DIR="${LAYERHUB_DATA_DIR:-$root/data}"
export LAYERHUB_WEB_DIR="${LAYERHUB_WEB_DIR:-$root/web}"
export LAYERHUB_DEVICE_MODE="${LAYERHUB_DEVICE_MODE:-offline}"
export LAYERHUB_ENABLE_PRINT_START="${LAYERHUB_ENABLE_PRINT_START:-0}"
exec "$root/bin/layerhub"
''',encoding='utf-8',newline='\n')
    archive=out/(name+'.tar.gz')
    with tarfile.open(archive,'w:gz') as tar:
        for file in sorted(stage.rglob('*')):
            if not file.is_file() or file.is_symlink():continue
            rel=file.relative_to(stage)
            if any(v in rel.parts for v in ['data','__pycache__']):continue
            def clean(info):
                info.uid=info.gid=0;info.uname=info.gname='';info.mode=0o755 if file.suffix=='.sh' or rel.parts[0]=='bin' else 0o644;return info
            tar.add(file,arcname=name+'/'+rel.as_posix(),recursive=False,filter=clean)
    digest=hashlib.sha256(archive.read_bytes()).hexdigest();(out/'SHA256SUMS').write_text(digest+'  '+archive.name+'\n',encoding='utf-8')
    report={'status':'cross_built_preview','architecture':'linux/arm64','target':'Jetson Orin Nano 8GB / Ubuntu 22.04','archive':archive.name,'sha256':digest,'bytes':archive.stat().st_size,'standalone_ELF_verified':True,'real_jetson_run':False,'printer_commands':0,'published_release_modified':False}
    (out/'build-report.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8');print(json.dumps(report))
