"""Create two scoped acceptance employees through the existing HRM APIs."""
import hashlib
import json
import secrets
from production_hrm_acceptance import PRIVATE, request


def checked(path,data):
    status,body=request(path,'POST',data)
    if status!=200 or body.get('code')!=0:
        (PRIVATE/'fixture-last-error.json').write_text(json.dumps(body,ensure_ascii=False,indent=2),encoding='utf-8')
        raise RuntimeError(body.get('message','HRM rejected acceptance fixture'))
    return body['data']


def main():
    state=PRIVATE/'test-accounts.json'
    if state.exists():
        print('Acceptance account fixture already exists; unchanged.')
        return
    accounts=[]
    for suffix in ['a','b']:
        username='qa_im_'+suffix
        password=secrets.token_urlsafe(24)
        display='IM acceptance '+suffix.upper()
        uid=checked('/admin/system/account/save',{'id':0,'name':display,'nickname':display,'username':username,'password':hashlib.md5(password.encode()).hexdigest(),'roleid':[],'dept_id':0,'mobile':'','email':'','tel':'','remark':'Temporary production acceptance fixture; remove after checks'})
        item={'username':username,'password':password,'uid':uid,'name':display}
        accounts.append(item)
        state.write_text(json.dumps({'lanOrigin':'https://192.168.10.11:9885','hrmOrigin':'https://192.168.10.11:9880','internetOrigin':'https://im.szputy.com:9885','accounts':accounts},indent=2)+'\n',encoding='utf-8')
        employee=checked('/plugins/biz-suite/api/admin/hrm/employees',{'user_id':uid,'employee_no':username,'name':display,'status':'active','dept_id':0,'wecom_userid':''})
        item['employeeID']=employee['id']
        state.write_text(json.dumps({'lanOrigin':'https://192.168.10.11:9885','hrmOrigin':'https://192.168.10.11:9880','internetOrigin':'https://im.szputy.com:9885','accounts':accounts},indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'acceptanceAccountsCreated':len(accounts),'credentialsFile':str(state),'realEmployeesModified':False}))


if __name__=='__main__':
    main()
