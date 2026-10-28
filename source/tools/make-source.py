"""Build the corresponding-source offer, never including runtime data or itself."""
import hashlib,pathlib,tarfile
project=pathlib.Path(__file__).resolve().parent.parent
root=project.parent if project.name=='source' and (project.parent/'deploy').is_dir() else project
out=project/'web/source';out.mkdir(exist_ok=True)
archive=out/'layerhub-source.tar.gz'
allowed=['source','deploy','tools','licenses','validation','.github'] if root!=project else ['cmd','internal','web','vendor','qa','tools','assets-source','licenses']
allowed += ['go.mod','go.sum','LICENSE','THIRD_PARTY_NOTICES.md','README.md','README.zh-CN.md','DEPLOYMENT.md','01-部署指导-云服务器-NAS-打印机.md','TESTING.md','02-外部验收与反馈.md','OPERATIONS.md','03-升级备份与排障.md','LANGUAGES.md','DIAGNOSTICS.md','IMPORTING.md','A1_SETUP.md','MULTIPLATE.md','MOTION.md','STABILITY.md','STUDIO_SHORTCUT.md','PRINT_PREPARATION.md','BETA_MIGRATION.md','RELEASE_NOTES.md','SECURITY.md','CHANGELOG.md','VERSION','.gitignore','.gitattributes']
with tarfile.open(archive,'w:gz') as tar:
 for name in allowed:
  path=root/name
  if not path.exists():continue
  files=sorted(path.rglob('*')) if path.is_dir() else [path]
  for p in files:
   rel=p.relative_to(root)
   relative=p.relative_to(project) if p.is_relative_to(project) else None
   if p.is_symlink() or not p.is_file() or p.suffix=='.log' or any(v in rel.parts for v in ['node_modules','__pycache__','.git']) or relative is not None and relative.parts[:2]==('web','source'):continue
   if p.suffix in ['.sqlite','.key','.pyc'] or p.name=='.env':raise SystemExit('Runtime secret/data unexpectedly present in source tree')
   def clean(info):
    info.uid=info.gid=0;info.uname=info.gname=''
    info.mode=0o755 if p.suffix=='.sh' else 0o644
    return info
   tar.add(p,arcname='layerhub-source/'+rel.as_posix(),recursive=False,filter=clean)
print('Corresponding source SHA256:',hashlib.sha256(archive.read_bytes()).hexdigest())
