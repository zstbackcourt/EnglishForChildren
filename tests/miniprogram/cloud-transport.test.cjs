const {test}=require('node:test');
const assert=require('node:assert/strict');
const {createAPI}=require('../../miniprogram/lib/api');
const config={transport:'cloudrun',cloudEnv:'prod-test',cloudService:'game',apiBase:'https://unused.run.tcloudbase.com',devLogin:false};
test('cloud mode logs in and fetches catalog through the named private service without HTTP',async()=>{
 let init=0,login=0;const calls=[];
 const wx={request(){assert.fail('public HTTP must not be used')},login(o){login++;o.success({code:'temporary-code'})},cloud:{init(o){init++;assert.equal(o.env,'prod-test')},async callContainer(o){calls.push(o);return {statusCode:200,data:o.path==='/api/session'?{token:'test-session',expiresAt:'2099-01-01T00:00:00Z'}:[{id:1}]}}}};
 const api=createAPI(wx,config);assert.deepEqual(await api.request('/api/lessons'),[{id:1}]);assert.equal(init,1);assert.equal(login,1);assert.equal(calls[0].config.env,'prod-test');assert.equal(calls[0].header['X-WX-SERVICE'],'game');assert.equal(calls[1].header.Authorization,'Bearer test-session');assert.ok(calls.every(c=>!c.url));
});
test('cloud retry refreshes authentication and reports safe errors',async()=>{
 let sessions=0,requests=0;const wx={login:o=>o.success({code:'code'}),cloud:{init(){},async callContainer(o){if(o.path==='/api/session'){sessions++;return {statusCode:200,data:{token:'t'+sessions,expiresAt:'2099-01-01T00:00:00Z'}}}requests++;return {statusCode:requests===1?401:200,data:[]}}}};
 const api=createAPI(wx,config);assert.deepEqual(await api.request('/api/players'),[]);assert.equal(sessions,2);wx.cloud.callContainer=async()=>{throw Error('internal credential-bearing error')};await assert.rejects(api.request('/api/players'),e=>/网络连接失败/.test(e.message)&&!e.message.includes('credential'));
});
test('credential configuration failures show an actionable safe message',async()=>{
 const wx={login:o=>o.success({code:'code'}),cloud:{init(){},async callContainer(){return {statusCode:401,data:{reason:'credential_rejected',error:'untrusted upstream detail'}}}}};
 await assert.rejects(createAPI(wx,config).request('/api/players'),e=>/联系管理员/.test(e.message)&&!e.message.includes('upstream'));
});
