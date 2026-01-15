import axios from 'axios'

const api = axios.create({
  baseURL: '/api',
  timeout: 60000
})

export const authApi = {
  // 验证密码
  verify(password) {
    return api.post('/auth/verify', { password })
  }
}

export const videoApi = {
  // 获取视频列表
  getList(page = 1) {
    return api.get('/videos', { params: { page } })
  },

  // 获取视频详情
  getDetail(videoId) {
    return api.get(`/videos/${videoId}`)
  },

  // 获取视频流地址
  getStreamUrl(videoId) {
    return `/api/stream/${videoId}`
  },

  // 直接获取m3u8流
  getDirectStreamUrl(m3u8Url) {
    return `/api/stream/direct?url=${encodeURIComponent(m3u8Url)}`
  },

  // 获取封面图代理地址
  getThumbnailUrl(videoId) {
    return `/api/stream/image/${videoId}`
  }
}

export const cacheApi = {
  // 获取缓存视频列表（分页）
  getList(page = 1, pageSize = null) {
    const params = { page }
    if (pageSize) params.page_size = pageSize
    return api.get('/cache', { params })
  },

  // 获取指定视频缓存状态
  getStatus(viewkey) {
    return api.get(`/cache/${viewkey}`)
  },

  // 删除指定视频缓存（需要管理员权限）
  delete(viewkey, adminToken) {
    return api.delete(`/cache/${viewkey}`, {
      headers: { 'X-Admin-Token': adminToken }
    })
  },

  // 清空所有缓存（需要管理员权限）
  clearAll(adminToken) {
    return api.delete('/cache', {
      headers: { 'X-Admin-Token': adminToken }
    })
  }
}

export default api
