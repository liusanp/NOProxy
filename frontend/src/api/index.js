import axios from 'axios'

const api = axios.create({
  baseURL: '/api',
  timeout: 60000
})

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
  }
}

export default api
