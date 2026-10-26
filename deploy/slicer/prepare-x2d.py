"""Prepare profiles and a bounded wrapper from a separately obtained runtime.
Does not download software, access a workspace or contact a printer.
"""
import argparse,hashlib,json,pathlib,shlex
p=argparse.ArgumentParser();p.add_argument('--runtime',type=pathlib.Path,required=True);p.add_argument('--profiles',type=pathlib.Path,required=True);p.add_argument('--wrapper',type=pathlib.Path,required=True);a=p.parse_args()
runtime=a.runtime.resolve();resources=runtime/'resources/profiles/BBL';profiles=a.profiles.resolve();wrapper=a.wrapper.resolve()
assert (runtime/'AppRun').is_file() and resources.is_dir(),'Expected an extracted official Linux runtime'
assert not profiles.exists() and not wrapper.exists(),'Output already exists; never overwrite an installed runtime configuration'
sources={}
def flatten(kind,name,stack=()):
 assert '/' not in name and '\\' not in name and (kind,name) not in stack
 path=resources/kind/(name+'.json');raw=path.read_bytes();data=json.loads(raw);sources[str(path.relative_to(runtime))]=hashlib.sha256(raw).hexdigest();result={}
 if data.get('inherits'):result.update(flatten(kind,data['inherits'],stack+((kind,name),)))
 for item in data.get('include',[]):result.update(flatten(kind,item,stack+((kind,name),)))
 result.update(data);result.pop('inherits',None);result.pop('include',None);result['from']='system';return result
configs={kind:(name,flatten(kind,name)) for kind,name in [('machine','Bambu Lab X2D 0.4 nozzle'),('process','0.20mm Standard @BBL X2D'),('filament','Bambu PETG Basic @BBL X2D 0.4 nozzle')]}
profiles.mkdir(parents=True)
for kind,(name,data) in configs.items():
 full=resources/(kind+'_full');full.mkdir(exist_ok=True)
 for target in [profiles/(kind+'.json'),full/(name+'.json')]:
  if target.exists():raise SystemExit('Refusing to overwrite '+str(target))
  target.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n',encoding='utf-8');target.chmod(0o644)
base=resources/'machine_full/Bambu Lab X2D.json';assert not base.exists();base.write_bytes((resources/'machine/Bambu Lab X2D.json').read_bytes());base.chmod(0o644)
(profiles/'sources.json').write_text(json.dumps(sources,indent=2)+'\n',encoding='utf-8')
wrapper.parent.mkdir(parents=True,exist_ok=True)
wrapper.write_text('#!/bin/sh\nexport LIBGL_ALWAYS_SOFTWARE=1\nexport LP_NUM_THREADS=2\nexec /usr/bin/prlimit --data=1073741824 --cpu=175 -- /usr/bin/nice -n 10 /usr/bin/xvfb-run -a -s "-screen 0 1024x768x24 -nolisten tcp" '+shlex.quote(str(runtime/'AppRun'))+' "$@"\n',encoding='utf-8');wrapper.chmod(0o755)
print('Profiles and bounded CLI wrapper prepared; no slice or print started.')
