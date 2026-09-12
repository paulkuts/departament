"""Integration smoke on the isolated local demo server. Never prints credentials."""
import json
import time
import urllib.request
import urllib.error
import uuid
from pathlib import Path

BASE = 'http://127.0.0.1:18182/api/v1'
credentials = json.loads((Path(__file__).resolve().parent.parent / '.local/demo-credentials.json').read_text(encoding='utf-8-sig'))
checks = 0
def call(path, method='GET', body=None, token=None, expected=200):
    global checks
    time.sleep(.11)  # Respect the same per-IP limiter used by the UI.
    headers = {'Content-Type':'application/json'}
    if token: headers['Authorization'] = 'Bearer ' + token
    req = urllib.request.Request(BASE + path, data=json.dumps(body).encode() if body is not None else None, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req) as response: code, raw = response.status, response.read()
    except urllib.error.HTTPError as error: code, raw = error.code, error.read()
    assert code == expected, f'{method} {path}: expected {expected}, got {code}'
    checks += 1
    return json.loads(raw) if raw else None

tokens = {}
profiles = {}
for account in credentials:
    tokens[account['role']] = call('/auth/signin','POST',{'email':account['email'],'password':account['password']})['access_token']
    profiles[account['role']] = call('/me',token=tokens[account['role']])
a, s = tokens['admin'], tokens['staff']
new_member = {'full_name':'Тест регистрации','email':str(uuid.uuid4())+'@demo.invalid','password':str(uuid.uuid4()),'role':'admin'}
registered = call('/auth/register','POST',new_member,expected=201)
assert registered['role'] == 'staff'
new_token = call('/auth/signin','POST',{'email':new_member['email'],'password':new_member['password']})['access_token']
assert call('/me',token=new_token)['role'] == 'staff'
call('/auth/signin','POST',{'email':credentials[0]['email'],'password':'incorrect-password'},expected=401)
for path in ['/inventory','keys','articles']:
    call('/'+path.lstrip('/'),expected=401)
note = call('/notes','POST',{'title':'План исследования','body':'Проверить методику и сверить результаты.'},s)
assert any(n['id']==note['id'] for n in call('/notes',token=s))
assert not any(n['id']==note['id'] for n in call('/notes',token=a))
call('/notes/'+str(note['id']),'PUT',{'title':'Чужая заметка','body':'Запрещено'},a,404)
call('/notes/'+str(note['id']),'PUT',{'title':'План исследования','body':'Методика проверена.'},s)
call('/reference','POST',{'category':'Работа','name':'Порядок записи','value':'Обратитесь к ответственному за прибор','notes':'Демонстрационная запись'},s,403)
ref = call('/reference','POST',{'category':'Работа','name':'Порядок записи','value':'Обратитесь к ответственному за прибор','notes':'Демонстрационная запись'},a)
assert any(r['id']==ref['id'] for r in call('/reference',token=s))
inv=call('/inventory?type=equipment&limit=20',token=a)['inventory'][0]
call('/inventory/'+str(inv['id'])+'/comments','POST',{'body':'Демонстрация: оборудование проверено перед использованием.'},s,201)
loan=call('/inventory/'+str(inv['id'])+'/loans','POST',{'borrower_id':profiles['staff']['id'],'comment':'Тестовая выдача'},a,201)
call('/inventory/'+str(inv['id'])+'/loans','POST',{'borrower_id':profiles['staff']['id'],'comment':'Повтор'},a,409)
call('/loans/'+str(loan['id'])+'/return','POST',{},s,403)
call('/loans/'+str(loan['id'])+'/return','POST',{},a)
key=next(k for k in call('/keys',token=a) if k['status']=='available')
call('/keys','POST',{'key_number':'forbidden','room_description':'test'},s,403)
public=call('/keys/'+str(key['id'])+'/public-link','POST',{},a)
call('/public/keys/'+public['public_id'])
req=call('/public/keys/'+public['public_id']+'/requests','POST',{'name':'Тестовый посетитель','affiliation':'Учебная группа','purpose':'Демонстрация выдачи'},expected=201)
call('/key-requests/'+str(req['id'])+'/approve','POST',{'user_id':profiles['staff']['id']},a)
assert call('/keys/'+str(key['id']),token=a)['status']=='issued'
call('/keys/'+str(key['id'])+'/return','POST',{'comment':'Тестовый возврат'},s)
for path in ['/articles','events','users','reference','notes']:
    call('/'+path.lstrip('/'),token=a)
print(f'PASS: {checks} live API checks; auth, roles, notes isolation, reference, comments, loans, guest key workflow.')
