"""Exercise batch intake in a disposable offline workspace; never contact a printer."""
import argparse, contextlib, http.cookiejar, io, json, os, pathlib, socket, sqlite3, subprocess, tempfile, time, urllib.error, urllib.request, zipfile

p = argparse.ArgumentParser()
p.add_argument('--binary', type=pathlib.Path, required=True)
p.add_argument('--web', type=pathlib.Path, required=True)
p.add_argument('--report', type=pathlib.Path, required=True)
a = p.parse_args()
with socket.socket() as listener:
    listener.bind(('127.0.0.1', 0)); port = listener.getsockname()[1]
base = 'http://127.0.0.1:' + str(port)
with tempfile.TemporaryDirectory(prefix='layerhub-multiplate-smoke-') as temp:
    data = pathlib.Path(temp) / 'data'
    env = os.environ.copy()
    env.update(LAYERHUB_ADDRESS='127.0.0.1:' + str(port), LAYERHUB_DATA_DIR=str(data), LAYERHUB_WEB_DIR=str(a.web.resolve()), LAYERHUB_DEVICE_MODE='offline', LAYERHUB_ENABLE_PRINT_START='0', LAYERHUB_COOKIE_SECURE='0')
    for name in ['LAYERHUB_ADMIN_PASSWORD', 'LAYERHUB_MAIL_TOKEN', 'LAYERHUB_PUBLIC_URL']:
        env.pop(name, None)
    process = subprocess.Popen([str(a.binary.resolve())], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    client = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    csrf = ''

    def request(path, payload=None, content='application/json', want=200):
        if isinstance(payload, dict): payload = json.dumps(payload).encode()
        req = urllib.request.Request(base + path, data=payload, headers={'Origin': base, 'Content-Type': content, 'X-CSRF-Token': csrf})
        try:
            with client.open(req, timeout=15) as response: status, result = response.status, json.load(response)
        except urllib.error.HTTPError as response:
            status, result = response.code, json.load(response)
        assert status == want, (path, status, result)
        return result

    def upload(raw, fields):
        boundary = 'LayerHubDisposableBatchBoundary'
        parts = [b'--' + boundary.encode() + b'\r\nContent-Disposition: form-data; name="file"; filename="two-plates.3mf"\r\nContent-Type: application/octet-stream\r\n\r\n' + raw]
        for key, value in fields.items():
            parts.append(b'--' + boundary.encode() + b'\r\nContent-Disposition: form-data; name="' + key.encode() + b'"\r\n\r\n' + json.dumps(value).encode())
        return b'\r\n'.join(parts) + b'\r\n--' + boundary.encode() + b'--\r\n', 'multipart/form-data; boundary=' + boundary

    try:
        for attempt in range(100):
            try:
                assert request('/healthz')['deviceMode'] == 'offline'; break
            except Exception:
                if process.poll() is not None or attempt == 99: raise
                time.sleep(.1)
        credentials = dict(line.split('=', 1) for line in (data / 'bootstrap-admin.txt').read_text().splitlines() if '=' in line)
        csrf = request('/api/login', {'Username': credentials['username'], 'Password': credentials['password']})['csrf']
        request('/api/spools', {'label': 'Disposable PETG fixture', 'brand': 'Test', 'material': 'PETG', 'colorName': 'White', 'color': '#FFFFFF', 'capacity': 100, 'remaining': 100, 'owner': 'team', 'state': 'available', 'location': 'Test fixture'})
        spool = request('/api/workspace')['spools'][0]
        raw = io.BytesIO()
        with zipfile.ZipFile(raw, 'w', zipfile.ZIP_DEFLATED) as archive:
            archive.writestr('Metadata/project_settings.config', '{"printer_model":"Bambu Lab A1"}')
            for plate in [1, 7]: archive.writestr(f'Metadata/plate_{plate}.gcode', '; DISPOSABLE FIXTURE ONLY')
            archive.writestr('Metadata/slice_info.config', '<config><plate><metadata key="index" value="1"/><metadata key="prediction" value="61"/><filament id="1" type="PETG" color="#FFFFFF" used_g="9.2"/></plate><plate><metadata key="index" value="7"/><metadata key="prediction" value="181"/><filament id="1" type="PETG" color="#FFFFFF" used_g="11.3"/></plate></config>')
        batch = {'requestId': 'offline-smoke-00000001', 'items': [{'plate': 1, 'printer': 0, 'estimate': 2, 'grams': 10, 'spoolId': spool['id']}, {'plate': 7, 'printer': 0, 'estimate': 4, 'grams': 12, 'spoolId': spool['id']}]}
        body, content = upload(raw.getvalue(), {'batch': batch})
        first = request('/api/upload/batch', body, content, want=201)['receipt']
        second = request('/api/upload/batch', body, content)['receipt']
        assert first['jobIds'] == second['jobIds'] and second['replayed']
        workspace = request('/api/workspace')
        assert [job['selectedPlate'] for job in workspace['jobs']] == [1, 7]
        assert all(job['state'] == 'queued' for job in workspace['jobs']) and workspace['spools'][0]['remaining'] == 100
        assert len(list((data / 'uploads').iterdir())) == 1
        request('/api/jobs/delete', {'jobId': first['jobIds'][0]})
        assert len(list((data / 'uploads').iterdir())) == 1
        assert len(request('/api/jobs/slice?jobId=' + str(first['jobIds'][1]))['slice']['plates']) == 2
        request('/api/jobs/delete', {'jobId': first['jobIds'][1]})
        assert not list((data / 'uploads').iterdir())
        assert request('/api/jobs/import-receipt?requestId=' + batch['requestId'])['receipt']['jobIds'] == first['jobIds']
        request('/api/upload/batch', body, content)
        assert not request('/api/workspace')['jobs'] and not list((data / 'uploads').iterdir())
        with contextlib.closing(sqlite3.connect((data / 'layerhub.sqlite').as_uri() + '?mode=ro', uri=True)) as db:
            assert db.execute('PRAGMA user_version').fetchone()[0] == 5
            for table in ['printer_commands', 'job_consumption', 'mail_outbox']:
                assert db.execute('SELECT COUNT(*) FROM ' + table).fetchone()[0] == 0
        result = {'status': 'passed', 'actual_binary': True, 'offline_disposable_workspace': True, 'plates': [1, 7], 'duplicate_batches': 0, 'shared_file_lifetime': 'passed', 'receipt_survives_deletion': True, 'schema': 5, 'real_devices_contacted': 0, 'physical_commands': 0, 'mail_sent': 0}
        a.report.parent.mkdir(parents=True, exist_ok=True)
        a.report.write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
        print(json.dumps(result))
    finally:
        process.terminate()
        try: process.wait(timeout=15)
        except subprocess.TimeoutExpired: process.kill(); process.wait()
