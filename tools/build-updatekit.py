"""Build a standalone updater kit; release mode creates a new versioned artifact."""
import argparse,hashlib,json,os,pathlib,re,shutil,subprocess,tempfile,time,zipfile
p=argparse.ArgumentParser();p.add_argument('--go',default='go');p.add_argument('--release',action='store_true');a=p.parse_args()
root=pathlib.Path(__file__).resolve().parent.parent;source=root/'source'
version=(root/'VERSION').read_text().strip();assert re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+-(alpha|beta)',version)
out=root/('packages' if a.release else 'local-packages');out.mkdir(exist_ok=True)
name='LayerHub-Download-UpdateKit-'+(version if a.release else 'preview-1')
stage=pathlib.Path(tempfile.mkdtemp(prefix='.updatekit-',dir=out));kit=stage/name;kit.mkdir()
def run(*args,env=None,cwd=source):return subprocess.check_output(args,cwd=cwd,env=env,text=True).strip()
env=dict(os.environ,CGO_ENABLED='0',GOARCH='amd64')
(kit/'bin').mkdir()
for target,suffix in [('windows','windows-amd64.exe'),('linux','linux-amd64')]:
 env['GOOS']=target
 run(a.go,'build','-mod=vendor','-trimpath','-buildvcs=false','-ldflags=-s -w','-o',str(kit/'bin'/('layerhub-update-'+suffix)),'./cmd/updater',env=env)
for file in (root/'deploy/update-kit').iterdir():
 if not file.is_file():continue
 if file.suffix=='.cmd':(kit/file.name).write_bytes(file.read_text(encoding='utf-8').replace('\r\n','\n').replace('\n','\r\n').encode())
 else:shutil.copyfile(file,kit/file.name)
shutil.copyfile(root/'LICENSE',kit/'LICENSE')
# Ship only the updater's actual code and dependency, not credentials, the
# application's data directory, private ops helpers or build caches.
src=kit/'source';(src/'cmd').mkdir(parents=True);(src/'internal').mkdir()
shutil.copytree(source/'cmd/updater',src/'cmd/updater',ignore=shutil.ignore_patterns('__pycache__'))
shutil.copytree(source/'internal/updatekit',src/'internal/updatekit',ignore=shutil.ignore_patterns('__pycache__'))
shutil.copytree(source/'vendor/golang.org/x/sys',src/'vendor/golang.org/x/sys')
modules=(source/'vendor/modules.txt').read_text();section=re.search(r'(?m)^# golang.org/x/sys v[^\n]+\n.*?(?=^# |\Z)',modules,re.S).group()
(src/'vendor/modules.txt').write_text(section,encoding='utf-8')
(src/'go.mod').write_text('module layerhub.local/server\n\ngo 1.27.0\n\nrequire golang.org/x/sys v0.48.0\n',encoding='utf-8')
(src/'go.sum').write_text('\n'.join(x for x in (source/'go.sum').read_text().splitlines() if x.startswith('golang.org/x/sys v0.48.0'))+'\n',encoding='utf-8')
goroot=pathlib.Path(run(a.go,'env','GOROOT'));(kit/'licenses').mkdir()
shutil.copyfile(goroot/'LICENSE',kit/'licenses/Go-LICENSE.txt');shutil.copyfile(source/'vendor/golang.org/x/sys/LICENSE',kit/'licenses/x-sys-LICENSE.txt')
(kit/'THIRD_PARTY_NOTICES.md').write_text('# UpdateKit notices\n\nOriginal updater code: AGPL-3.0-only. Go runtime: BSD-3-Clause; golang.org/x/sys v0.48.0: BSD-3-Clause. See licenses/. Matching updater and x/sys sources are in source/. Go 1.27+ builds offline with -mod=vendor. Linux Python 3 and the operating system retain their own licenses and are not bundled.\n',encoding='utf-8')
# Check the exact shipped source can build without network/module resolution.
source_matches={}
for target,suffix in [('windows','windows-amd64.exe'),('linux','linux-amd64')]:
 verify=stage/('source-check-'+suffix);env['GOOS']=target
 run(a.go,'build','-mod=vendor','-trimpath','-buildvcs=false','-ldflags=-s -w','-o',str(verify),'./cmd/updater',env=env,cwd=src)
 source_matches[target]=hashlib.sha256(verify.read_bytes()).digest()==hashlib.sha256((kit/'bin'/('layerhub-update-'+suffix)).read_bytes()).digest()
 verify.unlink()
checks=[]
for file in sorted((kit/'bin').iterdir()):checks.append(hashlib.sha256(file.read_bytes()).hexdigest()+'  bin/'+file.name)
(kit/'SHA256SUMS').write_text('\n'.join(checks)+'\n',encoding='utf-8')
final=out/name
if final.exists():
 assert final.resolve().parent==out.resolve() and not final.is_symlink()
 previous=out/(name+'.previous-'+str(time.time_ns()));assert previous.resolve().parent==out.resolve();final.rename(previous)
kit.rename(final)
archive=out/(name+'.zip')
with zipfile.ZipFile(archive,'w',zipfile.ZIP_DEFLATED,compresslevel=6) as z:
 for file in sorted(final.rglob('*')):
  if not file.is_file() or file.is_symlink():continue
  relative=file.relative_to(final);info=zipfile.ZipInfo(name+'/'+relative.as_posix());info.compress_type=zipfile.ZIP_DEFLATED
  info.external_attr=((0o100755 if file.suffix=='.sh' or relative.parts[0]=='bin' else 0o100644)<<16)
  z.writestr(info,file.read_bytes())
digest=hashlib.sha256(archive.read_bytes()).hexdigest();(out/(name+'.zip.sha256')).write_text(digest+'  '+archive.name+'\n',encoding='utf-8')
report={'toolkit':version if a.release else 'preview-1','archive':str(archive),'directory':str(final),'sha256':digest,'bytes':archive.stat().st_size,'source_build':'Windows and Linux offline vendor verified','source_binary_hashes_match':source_matches,'published':False,'application_version_changed':False,'production_services_changed':False}
(out/'updatekit-build.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8');print(json.dumps(report))
