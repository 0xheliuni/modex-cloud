// Thin fetch wrapper over the AGT-envelope API ({success, message, data}).
// Session cookie auth — credentials:'include' sends the agt_session cookie.

async function request(method, path, body) {
  const opts = {
    method,
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
  };
  if (body !== undefined) opts.body = JSON.stringify(body);

  const res = await fetch(path, opts);
  let payload = {};
  try {
    payload = await res.json();
  } catch {
    throw new ApiError('服务器返回了无效响应', res.status);
  }
  if (!res.ok || !payload.success) {
    throw new ApiError(payload.message || `请求失败 (HTTP ${res.status})`, res.status);
  }
  return payload.data;
}

export class ApiError extends Error {
  constructor(message, status) {
    super(message);
    this.status = status;
  }
}

export const api = {
  get: (p) => request('GET', p),
  post: (p, b) => request('POST', p, b),
  put: (p, b) => request('PUT', p, b),
  del: (p) => request('DELETE', p),
};

// --- typed endpoint helpers ---
export const authApi = {
  login: (username, password) => api.post('/api/auth/login', { username, password }),
  logout: () => api.post('/api/auth/logout'),
  self: () => api.get('/api/auth/self'),
  changePassword: (oldPassword, newPassword) =>
    api.post('/api/auth/change-password', { old_password: oldPassword, new_password: newPassword }),
};

export const supplierApi = {
  platforms: () => api.get('/api/supplier/platforms'),
  channels: (platformId) =>
    api.get(`/api/supplier/channels${platformId ? `?platform_id=${platformId}` : ''}`),
  createChannel: (body) => api.post('/api/supplier/channels', body),
  updateChannel: (id, body) => api.put(`/api/supplier/channels/${id}`, body),
  deleteChannel: (id) => api.del(`/api/supplier/channels/${id}`),
  resync: (id) => api.post(`/api/supplier/channels/${id}/resync`),
};

export const adminApi = {
  platforms: () => api.get('/api/admin/platforms'),
  createPlatform: (b) => api.post('/api/admin/platforms', b),
  updatePlatform: (id, b) => api.put(`/api/admin/platforms/${id}`, b),
  deletePlatform: (id) => api.del(`/api/admin/platforms/${id}`),
  users: (role) => api.get(`/api/admin/users${role ? `?role=${role}` : ''}`),
  createUser: (b) => api.post('/api/admin/users', b),
  updateUser: (id, b) => api.put(`/api/admin/users/${id}`, b),
  resetPassword: (id, newPassword) =>
    api.post(`/api/admin/users/${id}/reset-password`, { new_password: newPassword }),
  deleteUser: (id) => api.del(`/api/admin/users/${id}`),
  grants: () => api.get('/api/admin/grants'),
  upsertGrant: (b) => api.post('/api/admin/grants', b),
  deleteGrant: (id) => api.del(`/api/admin/grants/${id}`),
  auditLogs: (params = '') => api.get(`/api/admin/audit-logs${params}`),
};
