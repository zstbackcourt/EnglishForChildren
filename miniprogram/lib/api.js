/* wx is the only transport boundary. Session tokens stay in memory. */
function createAPI(wx, config) {
  const base = config.apiBase.replace(/\/$/, '');
  let session = null;
  let signingIn = null;
  function send(path, method, data, token) {
    return new Promise((resolve, reject) => wx.request({
      url: base + path, method, data, timeout: 20000,
      header: Object.assign({'Content-Type': 'application/json'}, token ? {Authorization: 'Bearer ' + token} : {}),
      success: resolve, fail: () => reject(new Error('网络连接失败，请重试'))
    }));
  }
  function valid(s) { return s && s.token && Date.parse(s.expiresAt) > Date.now() + 30000; }
  function login() {
    if (valid(session)) return Promise.resolve(session);
    if (signingIn) return signingIn;
    signingIn = (async () => {
      const code = config.devLogin ? (config.devCode || 'dev:local-family') : await new Promise((resolve, reject) => wx.login({
        success: r => r.code ? resolve(r.code) : reject(new Error('微信登录失败，请重试')),
        fail: () => reject(new Error('微信登录失败，请重试'))
      }));
      const r = await send('/api/session', 'POST', {code});
      if (r.statusCode !== 200 || !valid(r.data)) throw new Error('登录失败，请重试');
      session = r.data;
      return session;
    })().finally(() => { signingIn = null; });
    return signingIn;
  }
  async function request(path, method = 'GET', data) {
    if (!/^\/api\//.test(path) || path === '/api/session') throw new Error('接口地址无效');
    let auth = await login();
    let r = await send(path, method, data, auth.token);
    if (r.statusCode === 401) {
      // A late rejection of an old token must not discard a fresh session.
      if (session && session.token === auth.token) session = null;
      auth = await login();
      r = await send(path, method, data, auth.token);
    }
    if (r.statusCode < 200 || r.statusCode >= 300) {
      if (r.statusCode === 401) throw new Error('登录失效，请重试');
      throw new Error((r.data && r.data.error) || '服务暂时不可用，请重试');
    }
    return r.data;
  }
  function asset(path) {
    if (!/^\/(assets\/(scenes|pages)\/[\w.-]+|media\/audio\/\d+)$/.test(path || '')) return '';
    return base + path;
  }
  return {request, asset, config};
}
module.exports = {createAPI};
