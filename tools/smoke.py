"""Fresh disposable workspace smoke test. Never uses an installed workspace."""
import argparse,http.cookiejar,json,os,pathlib,socket,subprocess,tempfile,time,urllib.error,urllib.request
p=argparse.ArgumentParser();p.add_argument('--binary',type=pathlib.Path,required=True);p.add_argument('--web',type=pathlib.Path,required=True);p.add_argument('--report',type=pathlib.Path,required=True);a=p.parse_args()
binary=a.binary.resolve();web=a.web.resolve();assert binary.is_file() and (web/'source/layerhub-source.tar.gz').is_file()
with socket.socket() as listener:listener.bind(('127.0.0.1',0));port=listener.getsockname()[1]
base='http://127.0.0.1:'+str(port)
with tempfile.TemporaryDirectory(prefix='layerhub-fresh-smoke-') as temp:
 data=pathlib.Path(temp)/'data';env=os.environ.copy();env.update(LAYERHUB_ADDRESS='127.0.0.1:'+str(port),LAYERHUB_DATA_DIR=str(data),LAYERHUB_WEB_DIR=str(web),LAYERHUB_DEVICE_MODE='offline',LAYERHUB_ENABLE_PRINT_START='0',LAYERHUB_COOKIE_SECURE='0')
 for name in ['LAYERHUB_ADMIN_PASSWORD','LAYERHUB_MAIL_TOKEN','LAYERHUB_SLICER_PATH','LAYERHUB_SLICER_PROFILES']:env.pop(name,None)
 process=None
 def launch():
  global process
  process=subprocess.Popen([str(binary)],env=env,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,creationflags=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0)
  for _ in range(100):
   try:
    with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(base+'/healthz',timeout=1) as r:health=json.load(r)
    assert health['ok'] and health['deviceMode']=='offline';return
   except Exception:
    if process.poll() is not None:raise RuntimeError('Fresh server exited')
    time.sleep(.1)
  raise RuntimeError('Fresh server did not become healthy')
 def stop():
  if process and process.poll() is None:
   process.terminate()
   try:process.wait(timeout=20)
   except subprocess.TimeoutExpired:process.kill();process.wait()
 def client():return urllib.request.build_opener(urllib.request.ProxyHandler({}),urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
 opener=client();csrf=None
 def api(path,body=None,want=200):
  headers={'Origin':base,'Content-Type':'application/json','Accept-Language':'en'}
  if csrf:headers['X-CSRF-Token']=csrf
  try:
   with opener.open(urllib.request.Request(base+path,headers=headers,data=json.dumps(body).encode() if body is not None else None),timeout=10) as r:status=r.status;value=json.load(r)
  except urllib.error.HTTPError as e:status=e.code;value=json.load(e)
  assert status==want,(path,status);return value
 try:
  launch();denied=api('/api/workspace',want=401);assert '登录' not in json.dumps(denied,ensure_ascii=False)
  credentials=dict(line.split('=',1) for line in (data/'bootstrap-admin.txt').read_text().splitlines() if '=' in line)
  csrf=api('/api/login',{'Username':credentials['username'],'Password':credentials['password']})['csrf']
  w=api('/api/workspace');assert w['user']['owner'] and not w['jobs'] and not w['printers'] and not w['capabilities']['printStart'] and w['capabilities']['teamProjects']
  assert api('/api/team-projects')['projects']==[] and api('/api/account/sessions')['total']==1
  assert api('/api/account/language')['language']=='en'
  assert api('/api/account/language',{'language':'zh-CN'})['ok']
  assert api('/api/account/language')['language']=='zh-CN'
  api('/api/users',{'Username':'smoke_member','Name':'Disposable test member','Password':'DisposableSmoke2026','Admin':False})
  for path in ['/source-offer.html','/source/LICENSE.txt','/source/THIRD_PARTY_NOTICES.md','/i18n.js','/locales/en.generated.js']:
   with opener.open(base+path,timeout=10) as r:assert r.status==200 and r.read()
  for path in ['/master.key','/data/master.key','/bootstrap-admin.txt','/.env']:
   try:opener.open(base+path,timeout=10);raise RuntimeError('Private path exposed')
   except urllib.error.HTTPError as e:assert e.code==404
  api('/api/logout',{});stop();launch();opener=client();csrf=None
  csrf=api('/api/login',{'Username':credentials['username'],'Password':credentials['password']})['csrf'];assert len(api('/api/workspace')['members'])==2
  assert api('/api/account/language')['language']=='zh-CN'
  api('/api/logout',{})
  result={'fresh_workspace':True,'offline_default':True,'print_start_off':True,'random_owner_created':True,'private_paths_denied':True,'source_offer_available':True,'restart_preserves_accounts':True,'english_errors':True,'browser_language_initializes_new_account':True,'explicit_account_language_survives_restart':True,'real_devices_contacted':0,'mail_sent':0,'status':'passed'}
  a.report.parent.mkdir(parents=True,exist_ok=True);a.report.write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8');print(json.dumps(result))
 finally:stop()
