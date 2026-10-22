export async function api(path, options = {}) {
  const controller = new AbortController(); const timer = setTimeout(() => controller.abort(), 12000);
  try {
    const response = await fetch('/api' + path, { ...options, signal: controller.signal, headers: { 'Content-Type': 'application/json', ...options.headers } });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || '请求失败，请重试');
    return data;
  } catch (error) {
    if (error.name === 'AbortError' || error instanceof TypeError) throw new Error('暂时连接不到存档服务，请检查后重试。');
    throw error;
  } finally { clearTimeout(timer); }
}
export const send = (path, data, method = 'POST') => api(path, { method, body: JSON.stringify(data) });
