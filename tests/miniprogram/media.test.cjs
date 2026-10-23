const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const os=require('node:os');
const path=require('node:path');
let createMediaCache;try{({createMediaCache}=require('../../miniprogram/lib/media'))}catch(e){if(e.code!=='MODULE_NOT_FOUND')throw e;}
const SIZE=262144;
function fixture(t, mutate, size=SIZE*2+7) {
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'english-media-test-'));t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));
 const fake={env:{USER_DATA_PATH:dir},getFileSystemManager(){const manager={};for(const name of ['mkdir','readdir','unlink','access','writeFile','appendFile','rename'])manager[name]=o=>{try{let result={};if(name==='mkdir'){if(fs.existsSync(o.dirPath))throw Error('mkdir:fail file already exists');fs.mkdirSync(o.dirPath,{recursive:o.recursive});}if(name==='readdir')result.files=fs.readdirSync(o.dirPath);if(name==='unlink')fs.unlinkSync(o.filePath);if(name==='access')fs.accessSync(o.path);if(name==='writeFile'||name==='appendFile')fs[name+'Sync'](o.filePath,Buffer.from(o.data));if(name==='rename')fs.renameSync(o.oldPath,o.newPath);queueMicrotask(()=>o.success(result));}catch(e){queueMicrotask(()=>o.fail(e));}};return manager;}};
 const calls=[];const body=Buffer.alloc(size);for(let i=0;i<body.length;i++)body[i]=i%251;
 const send=async(p,o)=>{calls.push({p,o});const [,s,e]=/^bytes=(\d+)-(\d+)$/.exec(o.header.Range);const start=+s,end=Math.min(+e,body.length-1),chunk=body.subarray(start,end+1);const r={statusCode:206,header:{'Content-Range':`bytes ${start}-${end}/${body.length}`,ETag:'"content-one"'},data:chunk.buffer.slice(chunk.byteOffset,chunk.byteOffset+chunk.byteLength)};return mutate?mutate(r,calls.length):r;};
 return {dir,fake,send,calls,body};
}
test('cloud media downloads bounded ranges once, verifies bytes and caches local file',async t=>{assert.equal(typeof createMediaCache,'function');const f=fixture(t),cache=createMediaCache(f.fake,f.send);const [a,b]=await Promise.all([cache.resolve('/media/audio/1'),cache.resolve('/media/audio/1')]);assert.equal(a,b);assert.match(a,/\.mp3$/);assert.deepEqual(fs.readFileSync(a),f.body);assert.equal(f.calls.length,3);assert.equal(f.calls[1].o.header['If-Range'],'"content-one"');assert.equal(await cache.resolve('/media/audio/1'),a);assert.equal(f.calls.length,3);fs.unlinkSync(a);await cache.resolve('/media/audio/1');assert.equal(f.calls.length,6);});
for(const [label,mutate] of [
 ['whole response',r=>({...r,statusCode:200})],
 ['short body',r=>({...r,data:new ArrayBuffer(1)})],
 ['wrong range',r=>({...r,header:{...r.header,'Content-Range':'bytes 1-262144/524295'}})],
 ['oversized asset',r=>({...r,header:{...r.header,'Content-Range':'bytes 0-262143/16777217'}})],
 ['missing etag',r=>({...r,header:{...r.header,ETag:undefined}})],
 ['changed etag',(r,n)=>({...r,header:{...r.header,ETag:n>1?'"changed"':r.header.ETag}})],
 ['changed total',(r,n)=>n>1?{...r,header:{...r.header,'Content-Range':'bytes 262144-524287/524296'}}:r]
])test('rejects '+label+' and deletes incomplete file',async t=>{assert.equal(typeof createMediaCache,'function');const f=fixture(t,mutate),cache=createMediaCache(f.fake,f.send);await assert.rejects(cache.resolve('/media/audio/1'));const files=fs.readdirSync(path.join(f.dir,'english-cloud-media-v1'));assert.equal(files.length,0);});
test('failed downloads can retry and no token is sent to public media',async t=>{assert.equal(typeof createMediaCache,'function');let fail=true;const f=fixture(t,r=>{if(fail)throw Error('offline');return r}),cache=createMediaCache(f.fake,f.send);await assert.rejects(cache.resolve('/media/audio/1'));fail=false;await cache.resolve('/media/audio/1');assert.ok(f.calls.every(c=>!c.o.header.Authorization));await assert.rejects(cache.resolve('/api/players'));await assert.rejects(cache.resolve('/assets/scenes/../private.png'));});

test('cold restart reuses only its cache directory and leaves saves alone',async t=>{const f=fixture(t);fs.writeFileSync(path.join(f.dir,'save.json'),'keep');const a=await createMediaCache(f.fake,f.send).resolve('/media/audio/1');const b=await createMediaCache(f.fake,f.send).resolve('/media/audio/1');assert.equal(a,b);assert.deepEqual(fs.readFileSync(b),f.body);assert.equal(fs.readFileSync(path.join(f.dir,'save.json'),'utf8'),'keep');});

test('failed eviction retains budget and prevents further media writes',async t=>{const f=fixture(t,null,16*1024*1024),manager=f.fake.getFileSystemManager();f.fake.getFileSystemManager=()=>manager;const cache=createMediaCache(f.fake,f.send);await cache.resolve('/media/audio/1');await cache.resolve('/media/audio/3');manager.unlink=o=>o.fail(new Error('permission denied'));await assert.rejects(cache.resolve('/media/audio/5'));const files=fs.readdirSync(path.join(f.dir,'english-cloud-media-v1'));assert.equal(files.length,2);});
test('uncleanable partial files block new writes until cleanup succeeds',async t=>{const f=fixture(t,(r,n)=>n===2?{...r,statusCode:500}:r),manager=f.fake.getFileSystemManager();f.fake.getFileSystemManager=()=>manager;const original=manager.unlink;manager.unlink=o=>o.fail(new Error('permission denied'));const cache=createMediaCache(f.fake,f.send);await assert.rejects(cache.resolve('/media/audio/1'));const calls=f.calls.length;await assert.rejects(cache.resolve('/media/audio/3'));assert.equal(f.calls.length,calls);manager.unlink=original;await cache.resolve('/media/audio/3');assert.ok(!fs.readdirSync(path.join(f.dir,'english-cloud-media-v1')).some(f=>f.endsWith('.part')));});
