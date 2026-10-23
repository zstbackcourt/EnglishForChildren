import test from 'node:test';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {checkBackend} from '../scripts/check_wechat_backend.mjs';

test('backend gate rejects the CloudRun starter HTML even when HTTP status is 200',async()=>{
 const result=await checkBackend('https://learning.example.net',async()=>new Response('<h1>欢迎使用微信云托管</h1>',{headers:{'content-type':'text/html'}}));
 assert.equal(result.ok,false);assert.match(result.error,/healthz/);
 const large=await checkBackend('https://learning.example.net',async()=>new Response('<html>'+('sample'.repeat(2000)),{headers:{'content-type':'text/html'}}));
 assert.equal(large.ok,false);assert.match(large.error,/就绪 JSON/);
});

function backend({players=401,audio=206,range='bytes 0-3/100',releaseId}={}) {
 return async(url,options)=>{
  const p=new URL(url).pathname;
  if(p==='/healthz')return Response.json({status:'ok',releaseId});
  if(p==='/api/players')return Response.json({}, {status:players});
  if(p==='/media/audio/1'){
   assert.equal(options.headers.Range,'bytes=0-3');
   return new Response(new Uint8Array([73,68,51,4]),{status:audio,headers:{'content-type':'audio/mpeg','content-range':range}});
  }
  throw new Error('unexpected route');
 };
}
test('backend gate requires login protection and real partial audio',async()=>{
 assert.equal((await checkBackend('https://learning.example.net',backend())).ok,true);
 assert.equal((await checkBackend('https://learning.example.net',backend({players:200}))).ok,false);
 assert.equal((await checkBackend('https://learning.example.net',backend({audio:200}))).ok,false);
 assert.equal((await checkBackend('https://learning.example.net',backend({range:'bytes 4-7/100'}))).ok,false);
});
test('backend gate waits for this release marker rather than accepting an older healthy server',async()=>{
 const expected='7c4721d86a017f20a6a61bba5f0cc999';
 assert.equal((await checkBackend('https://learning.example.net',backend({releaseId:'previous-release'}),expected)).ok,false);
 assert.equal((await checkBackend('https://learning.example.net',backend({releaseId:expected}),expected)).ok,true);
});
test('backend probe has a timeout, rejects redirects, and returns no network secrets',async()=>{
 const result=await checkBackend('https://learning.example.net',async(url,options)=>{
  assert.equal(options.redirect,'error');assert.ok(options.signal);
  throw new Error('private network value');
 });
 assert.equal(result.ok,false);assert.doesNotMatch(result.error,/private network/);
});

test('container preflight lists every missing variable without printing provided secrets',()=>{
 const result=spawnSync('/bin/sh',['deploy/wechat/cloudrun-entrypoint.sh','--check-config'],{encoding:'utf8',env:{PATH:process.env.PATH,WECHAT_APP_SECRET:'do-not-log-this'}});
 assert.notEqual(result.status,0);
 for(const key of ['WECHAT_APP_ID','WECHAT_SESSION_KEY','MYSQL_ADDRESS','MYSQL_USERNAME','MYSQL_PASSWORD','MYSQL_DATABASE'])assert.ok(result.stderr.includes(key),key);
 assert.doesNotMatch(result.stderr,/do-not-log-this/);
});
test('container configuration can be validated without launching a server or contacting MySQL',()=>{
 const env={PATH:process.env.PATH,WECHAT_APP_ID:'wx1f23b102bcb34ed7',WECHAT_APP_SECRET:'test-only',WECHAT_SESSION_KEY:'a'.repeat(32),MYSQL_ADDRESS:'localhost:3306',MYSQL_USERNAME:'test',MYSQL_PASSWORD:'private-test-value',MYSQL_DATABASE:'test'};
 const result=spawnSync('/bin/sh',['deploy/wechat/cloudrun-entrypoint.sh','--check-config'],{encoding:'utf8',env});
 assert.equal(result.status,0,result.stderr);assert.doesNotMatch(result.stdout+result.stderr,/private-test-value/);
});
