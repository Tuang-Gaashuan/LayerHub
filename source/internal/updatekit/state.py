"""Read-only upgrade guards and stopped-workspace backups; no restore action."""
import contextlib,hashlib,json,os,pathlib,shutil,sqlite3,stat,sys,tarfile
mode=sys.argv[1];data=pathlib.Path(sys.argv[2]).resolve();database=data/'layerhub.sqlite'
if not database.is_file() or database.is_symlink():raise SystemExit('Database not found or not a regular file')
def inspect():
 if pathlib.Path(str(data)+'.restore.json').exists():raise SystemExit('Backup restore is pending')
 with contextlib.closing(sqlite3.connect(database.as_uri()+'?mode=ro',uri=True)) as db:
  db.execute('PRAGMA query_only=ON')
  commands=db.execute("SELECT COUNT(*) FROM printer_commands WHERE state IN ('pending','uncertain')").fetchone()[0]
  active=db.execute("SELECT id,COALESCE(printer_id,0),state FROM jobs WHERE state IN ('uploading','starting','printing','paused','pause','stopped','checking') ORDER BY id").fetchall()
  slices=0
  if db.execute("SELECT 1 FROM sqlite_master WHERE type='table' AND name='slice_tasks'").fetchone():slices=db.execute("SELECT COUNT(*) FROM slice_tasks WHERE state IN ('queued','running')").fetchone()[0]
  if commands or slices:raise SystemExit('Unconfirmed control or active slicing exists')
  return {'active':active,'pending_commands':commands,'active_slices':slices}
if mode=='inspect':print(json.dumps(inspect()));sys.exit(0)
if mode!='backup':raise SystemExit('Unsupported action')
inspect();backup=pathlib.Path(sys.argv[3]).resolve();env=pathlib.Path(sys.argv[4]).resolve()
if backup.is_relative_to(data) or data.is_relative_to(backup) or backup.exists():raise SystemExit('Backup location is not isolated/new')
files=[];total=0
for p in data.rglob('*'):
 info=p.lstat()
 if p.is_symlink() or not (stat.S_ISDIR(info.st_mode) or stat.S_ISREG(info.st_mode)):raise SystemExit('Workspace links/special files require manual review')
 if p.is_file():total+=info.st_size;files.append(p)
backup.parent.mkdir(parents=True,exist_ok=True,mode=0o700)
if shutil.disk_usage(backup.parent).free<total+database.stat().st_size+128*1024*1024:raise SystemExit('Insufficient backup space')
backup.mkdir(mode=0o700);snapshot=backup/'layerhub.sqlite'
with contextlib.closing(sqlite3.connect(database.as_uri()+'?mode=ro',uri=True)) as source,contextlib.closing(sqlite3.connect(snapshot)) as destination:
 source.backup(destination)
 if destination.execute('PRAGMA integrity_check').fetchone()[0]!='ok':raise SystemExit('Backup integrity check failed')
snapshot.chmod(0o600)
config=backup/'environment.private';config.write_bytes(env.read_bytes());config.chmod(0o600)
meta={'environment':{'mode':stat.S_IMODE(env.stat().st_mode),'uid':env.stat().st_uid,'gid':env.stat().st_gid},'data':{'mode':stat.S_IMODE(data.stat().st_mode),'uid':data.stat().st_uid,'gid':data.stat().st_gid}}
(backup/'original-permissions.json').write_text(json.dumps(meta,indent=2)+'\n');(backup/'original-permissions.json').chmod(0o600)
archive=backup/'workspace.tar.gz'
with tarfile.open(archive,'w:gz') as t:
 for p in files:
  relative=p.relative_to(data).as_posix()
  if relative in ['layerhub.sqlite','layerhub.sqlite-wal','layerhub.sqlite-shm']:continue
  t.add(p,arcname='data/'+relative,recursive=False)
 info=tarfile.TarInfo('data/layerhub.sqlite');original=database.stat();info.size=snapshot.stat().st_size;info.mode=stat.S_IMODE(original.st_mode);info.uid=original.st_uid;info.gid=original.st_gid
 with snapshot.open('rb') as f:t.addfile(info,f)
archive.chmod(0o600)
print(json.dumps({'backup':'complete','files':len(files),'sqlite_integrity':'ok','data_and_config_preserved':True}))
