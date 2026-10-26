"""Exercise the actual doctor CLI using disposable file fixtures, never a live workspace."""
import argparse,json,os,pathlib,subprocess,tempfile
p=argparse.ArgumentParser();p.add_argument('--binary',type=pathlib.Path,required=True);p.add_argument('--report',type=pathlib.Path,required=True);a=p.parse_args();binary=a.binary.resolve();assert binary.is_file()
with tempfile.TemporaryDirectory(prefix='layerhub-doctor-smoke-') as temp:
 root=pathlib.Path(temp);web=root/'web';web.mkdir();(web/'locales').mkdir();(web/'source').mkdir()
 for name in ['index.html','app.js','locales/en.generated.js','source/layerhub-source.tar.gz','source/LICENSE.txt']:(web/name).write_bytes(b'disposable diagnostic file fixture')
 data=root/'missing';secret='private-doctor-canary-do-not-export'
 env=os.environ.copy();env.update(LAYERHUB_ADDRESS='127.0.0.1:5181',LAYERHUB_DATA_DIR=str(data),LAYERHUB_WEB_DIR=str(web),LAYERHUB_DEVICE_MODE='live',LAYERHUB_ENABLE_PRINT_START='0',LAYERHUB_MAIL_TOKEN=secret*2,LAYERHUB_PUBLIC_URL='https://private-doctor.invalid/',LAYERHUB_COOKIE_SECURE='1')
 for name in ['LAYERHUB_ADMIN_PASSWORD','LAYERHUB_SLICER_PATH','LAYERHUB_SLICER_PROFILES']:env.pop(name,None)
 def run(*flags,want=0):
  r=subprocess.run([str(binary),*flags],env=env,capture_output=True,timeout=15,creationflags=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0)
  assert r.returncode==want,('exit status',r.returncode,want)
  raw=r.stdout.decode('utf-8');assert secret not in raw and str(root) not in raw and 'private-doctor.invalid' not in raw
  return json.loads(raw) if '--json' in flags and want!=2 else raw
 report=run('--doctor','--json');assert not data.exists() and report['mode']=='local' and report['schema']==1
 assert next(c for c in report['checks'] if c['id']=='database')['status']=='unknown'
 run('--json',want=2);assert not data.exists()
 data.mkdir();db=data/'layerhub.sqlite';payload=b'SQLite format 3\0unchanged disposable fixture';db.write_bytes(payload)
 run('--doctor','--json','--probe-write');assert db.read_bytes()==payload and sorted(p.name for p in data.iterdir())==['layerhub.sqlite']
 chinese=run('--doctor','--json','--language=zh-CN');assert any(c['label']=='数据库' for c in chinese['checks'])
 db.write_bytes(b'invalid header');run('--doctor','--json',want=1);assert db.read_bytes()==b'invalid header'
 result={'status':'passed','binary_cli_exercised':True,'fixtures':'temporary files only','prestart_no_workspace_created':True,'explicit_temp_probe_cleaned':True,'sqlite_not_opened_or_changed':True,'invalid_flags_exit_2':True,'invalid_database_exit_1':True,'chinese_json_output':True,'private_configuration_omitted':True,'service_started':False,'real_printer_commands':0,'test_emails':0}
 a.report.parent.mkdir(parents=True,exist_ok=True);a.report.write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8');print(json.dumps(result))
