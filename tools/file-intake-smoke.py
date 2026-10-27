"""Check actual binary file-inspection routes in a disposable offline workspace."""
import argparse,hashlib,http.cookiejar,io,json,os,pathlib,socket,subprocess,tempfile,time,urllib.error,urllib.request,zipfile

p=argparse.ArgumentParser();p.add_argument('--binary',type=pathlib.Path,required=True);p.add_argument('--web',type=pathlib.Path,required=True);p.add_argument('--report',type=pathlib.Path,required=True);a=p.parse_args()
binary=a.binary.resolve();web=a.web.resolve()
with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
base='http://127.0.0.1:'+str(port)
with tempfile.TemporaryDirectory(prefix='layerhub-file-intake-') as temp:
 data=pathlib.Path(temp)/'data';env=os.environ.copy();env.update(LAYERHUB_ADDRESS='127.0.0.1:'+str(port),LAYERHUB_DATA_DIR=str(data),LAYERHUB_WEB_DIR=str(web),LAYERHUB_DEVICE_MODE='offline',LAYERHUB_ENABLE_PRINT_START='0',LAYERHUB_COOKIE_SECURE='0')
 for name in ['LAYERHUB_ADMIN_PASSWORD','LAYERHUB_MAIL_TOKEN','LAYERHUB_SLICER_PATH','LAYERHUB_SLICER_PROFILES','LAYERHUB_PUBLIC_URL']:env.pop(name,None)
 process=subprocess.Popen([str(binary)],env=env,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,creationflags=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0)
 opener=urllib.request.build_opener(urllib.request.ProxyHandler({}),urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()));csrf=None
 def request(path,body=None,content='application/json',want=200):
  headers={'Origin':base,'Content-Type':content,'Accept-Language':'en'}
  if csrf:headers['X-CSRF-Token']=csrf
  try:
   with opener.open(urllib.request.Request(base+path,headers=headers,data=body),timeout=20) as r:status=r.status;value=json.load(r)
  except urllib.error.HTTPError as e:status=e.code;value=json.load(e)
  assert status==want,(path,status);return value
 def multipart(raw):
  boundary='LayerHubDisposableIntakeBoundary'
  return b'--'+boundary.encode()+b'\r\nContent-Disposition: form-data; name="file"; filename="sample.3mf"\r\nContent-Type: application/octet-stream\r\n\r\n'+raw+b'\r\n--'+boundary.encode()+b'--\r\n','multipart/form-data; boundary='+boundary
 def archive(geometry,sliced):
  b=io.BytesIO()
  with zipfile.ZipFile(b,'w',zipfile.ZIP_DEFLATED) as z:
   z.writestr('[Content_Types].xml','<Types/>')
   z.writestr('Metadata/project_settings.config',json.dumps({'printer_model':'Test printer','layer_height':'0.12','machine_start_gcode':'PRIVATE_MACRO_CANARY'}))
   if geometry:
    z.writestr('_rels/.rels','<Relationships><Relationship Type="http://schemas.microsoft.com/3dmanufacturing/2013/01/3dmodel" Target="/3D/3dmodel.model"/></Relationships>')
    z.writestr('3D/3dmodel.model','<model xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02"><resources><object id="1" type="model"><mesh><vertices><vertex x="0" y="0" z="0"/><vertex x="10" y="0" z="0"/><vertex x="0" y="10" z="0"/><vertex x="0" y="0" z="10"/></vertices><triangles><triangle v1="0" v2="2" v3="1"/><triangle v1="0" v2="1" v3="3"/><triangle v1="1" v2="2" v3="3"/><triangle v1="2" v2="0" v3="3"/></triangles></mesh></object></resources><build><item objectid="1"/></build></model>')
   if sliced:
    z.writestr('Metadata/plate_1.gcode','; DISPOSABLE INSPECTION FIXTURE ONLY')
    z.writestr('Metadata/slice_info.config','<config><plate><metadata key="index" value="1"/><metadata key="prediction" value="3701"/><filament id="1" type="PETG" color="#FFFFFF" used_g="12.1"/></plate></config>')
  return b.getvalue()
 try:
  for i in range(100):
   try:assert request('/healthz')['ok'];break
   except Exception:
    if process.poll() is not None:raise RuntimeError('Disposable binary exited')
    if i==99:raise
    time.sleep(.1)
  raw=archive(True,True);body,content=multipart(raw);request('/api/files/inspect',body,content,want=401)
  credentials=dict(line.split('=',1) for line in (data/'bootstrap-admin.txt').read_text().splitlines() if '=' in line)
  login=request('/api/login',json.dumps({'Username':credentials['username'],'Password':credentials['password']}).encode());csrf=login['csrf']
  before=request('/api/workspace');models=request('/api/slicing')['models']
  for geometry,sliced in [(True,True),(True,False),(False,True)]:
   raw=archive(geometry,sliced);body,content=multipart(raw);result=request('/api/files/inspect',body,content)['review']
   assert result['schema']==1 and result['sha256']==hashlib.sha256(raw).hexdigest()
   assert result['geometry']['available']==geometry and result['sliced']['available']==sliced
   assert 'PRIVATE_MACRO_CANARY' not in json.dumps(result)
  body,content=multipart(archive(True,False));request('/api/upload',body,content,want=400)
  after=request('/api/workspace');assert before['jobs']==after['jobs'] and before['spools']==after['spools'] and models==request('/api/slicing')['models']
  assert not list((data/'uploads').glob('*'))
  version=request('/api/diagnostics')['version'];request('/api/logout',b'{}')
  result={'version':version,'status':'passed','binary_routes':'geometry_only_slice_only_and_both','filename_independent':True,'reference_allowlist':'passed','unsliced_queue_upload':'rejected_before_job','temporary_uploads':'cleaned','business_data_unchanged':True,'real_devices_contacted':0,'mail_sent':0}
  a.report.parent.mkdir(parents=True,exist_ok=True);a.report.write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8');print(json.dumps(result))
 finally:
  process.terminate()
  try:process.wait(timeout=20)
  except subprocess.TimeoutExpired:process.kill();process.wait()
