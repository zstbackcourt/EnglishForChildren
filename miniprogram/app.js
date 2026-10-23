const {createAPI} = require('./lib/api');
const config = require('./config.js');
App({onLaunch() { this.api = createAPI(wx,config); }});
