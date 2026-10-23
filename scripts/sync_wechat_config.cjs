'use strict';
const fs=require('node:fs');
const path=require('node:path');
const {runtimeConfigSource}=require('./upload_wechat.cjs');

function syncRuntimeConfig(root=path.join(__dirname,'..')) {
 const mini=path.join(path.resolve(root),'miniprogram');
 const config=JSON.parse(fs.readFileSync(path.join(mini,'config.json'),'utf8'));
 fs.writeFileSync(path.join(mini,'config.js'),runtimeConfigSource(config));
}
module.exports={syncRuntimeConfig};
if(require.main===module)syncRuntimeConfig();
