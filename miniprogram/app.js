const {createAPI} = require('./lib/api');
const config = require('./config.json');
App({onLaunch() { this.api = createAPI(wx,config); }});
