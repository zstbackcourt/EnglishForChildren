// CloudRun's private channel delivers bounded binary responses; players use local files.
const CHUNK_BYTES = 256 * 1024;
const MAX_FILE_BYTES = 16 * 1024 * 1024;
const CACHE_BYTES = 32 * 1024 * 1024;
function validAsset(path) { return /^\/(assets\/(scenes|pages)\/[\w.-]+\.(png|jpe?g|webp)|media\/audio\/\d+)$/.test(path || ''); }
function createMediaCache(wx, send) {
  const cached = new Map(), pending = new Map();
  let manager, directory, initialized = false, used = 0, clock = 0, tail = Promise.resolve(), orphan = null;
  const io = (method, options) => new Promise((resolve, reject) => manager[method](Object.assign({}, options, {success: resolve, fail: reject})));
  async function initialize() {
    if (initialized) return;
    manager = wx.getFileSystemManager();
    directory = wx.env.USER_DATA_PATH + '/english-cloud-media-v1';
    let entries;
    try { entries = await io('readdir', {dirPath: directory}); }
    catch (e) {
      if (!missing(e)) throw e;
      await io('mkdir', {dirPath: directory, recursive: true});
      entries = await io('readdir', {dirPath: directory});
    }
    // This directory contains disposable public media only, never roles or progress.
    for (const name of entries.files) if (/^[\w.-]+$/.test(name)) await io('unlink', {filePath: directory + '/' + name});
    initialized = true;
  }
  function missing(error) { return error && (error.code === 'ENOENT' || /no such file|not exist|ENOENT/i.test(error.errMsg || error.message || '')); }
  async function remove(filePath) {
    try { await io('unlink', {filePath}); } catch (e) { if (!missing(e)) throw e; }
  }
  async function discard(key) {
    const entry = cached.get(key);
    if (!entry) return;
    await remove(entry.file);
    cached.delete(key); used -= entry.bytes;
  }
  async function trim(extra) {
    while (used + extra > CACHE_BYTES && cached.size) {
      const oldest = Array.from(cached.entries()).sort((a,b) => a[1].touched - b[1].touched)[0][0];
      await discard(oldest);
    }
  }
  async function download(path) {
    try {
      await initialize();
      if (orphan) { await remove(orphan); orphan = null; }
    } catch (_) { throw new Error('素材缓存暂时不可用，请重试'); }
    const hit = cached.get(path);
    if (hit) {
      try { await io('access', {path: hit.file}); hit.touched = ++clock; return hit.file; }
      catch (_) { await discard(path); }
    }
    const file = directory + '/' + path.slice(1).replace(/\//g, '_') + (path.startsWith('/media/audio/') ? '.mp3' : '');
    const partial = file + '.part';
    let start = 0, total, etag;
    try {
      while (total === undefined || start < total) {
        const end = total === undefined ? CHUNK_BYTES - 1 : Math.min(start + CHUNK_BYTES - 1, total - 1);
        const header = {Range: 'bytes=' + start + '-' + end};
        if (etag) header['If-Range'] = etag;
        const response = await send(path, {header, responseType: 'arraybuffer', dataType: 'text'});
        const headers = {};
        Object.keys(response.header || {}).forEach(k => { headers[k.toLowerCase()] = response.header[k]; });
        const range = /^bytes (\d+)-(\d+)\/(\d+)$/.exec(headers['content-range'] || '');
        const currentTag = headers.etag;
        const length = response.data && response.data.byteLength;
        if (response.statusCode !== 206 || !range || Object.prototype.toString.call(response.data) !== '[object ArrayBuffer]' ||
            typeof currentTag !== 'string' || !/^"[\x21\x23-\x7e]{1,128}"$/.test(currentTag)) throw new Error('媒体响应无效');
        const first = Number(range[1]), last = Number(range[2]), size = Number(range[3]);
        if (!Number.isSafeInteger(size) || size < 1 || size > MAX_FILE_BYTES || first !== start ||
            last !== Math.min(end, size - 1) || length !== last - first + 1 ||
            (total !== undefined && (total !== size || etag !== currentTag))) throw new Error('媒体内容已变化');
        if (total === undefined) {
          total = size; etag = currentTag;
          await trim(total);
          await io('writeFile', {filePath: partial, data: response.data});
        } else await io('appendFile', {filePath: partial, data: response.data});
        start = last + 1;
      }
      await io('rename', {oldPath: partial, newPath: file});
      cached.set(path, {file, bytes: total, touched: ++clock}); used += total;
      return file;
    } catch (_) {
      try { await remove(partial); } catch (_) { orphan = partial; }
      throw new Error('素材加载失败，请重试');
    }
  }
  function resolve(path) {
    if (!validAsset(path)) return Promise.reject(new Error('素材地址无效'));
    if (pending.has(path)) return pending.get(path);
    // Serialize downloads to bound memory, disk usage and cloud request concurrency.
    const result = tail.then(() => download(path)).finally(() => pending.delete(path));
    pending.set(path, result); tail = result.catch(() => {});
    return result;
  }
  return {resolve};
}
module.exports = {createMediaCache, validAsset};
