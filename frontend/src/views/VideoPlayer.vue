<template>
  <div class="video-player-page">
    <!-- 返回按钮 -->
    <button class="back-btn" @click="goBack">
      ← 返回列表
    </button>

    <!-- 加载状态 -->
    <div v-if="loading" class="loading">
      <div class="spinner"></div>
      <p>正在加载视频...</p>
    </div>

    <!-- 错误提示 -->
    <div v-else-if="error" class="error">
      <p>{{ error }}</p>
      <button @click="loadVideo">重试</button>
    </div>

    <!-- 视频播放器 -->
    <div v-else class="player-container">
      <div class="video-wrapper">
        <video
          ref="videoPlayer"
          controls
          autoplay
          class="video-element"
          :src="streamUrl"
        ></video>
      </div>

      <div class="video-info">
        <h1 class="video-title">{{ videoDetail?.title || '加载中...' }}</h1>
        <p class="video-url">
          代理地址: {{ streamUrl }}
        </p>
        <p class="video-url" v-if="videoDetail?.m3u8_url">
          原始地址: {{ videoDetail.m3u8_url }}
        </p>
        <p class="video-url" v-else style="color: #f44;">
          未找到视频链接
        </p>
      </div>
    </div>
  </div>
</template>

<script>
import Hls from 'hls.js'
import { videoApi } from '../api'

export default {
  name: 'VideoPlayer',
  props: {
    id: {
      type: String,
      required: true
    }
  },
  data() {
    return {
      videoDetail: null,
      loading: false,
      error: null,
      hls: null
    }
  },
  computed: {
    streamUrl() {
      return videoApi.getStreamUrl(this.id)
    }
  },
  mounted() {
    this.loadVideo()
  },
  beforeUnmount() {
    this.destroyPlayer()
  },
  methods: {
    async loadVideo() {
      this.loading = true
      this.error = null

      try {
        // 获取视频详情
        const response = await videoApi.getDetail(this.id)
        this.videoDetail = response.data
        console.log('获取到视频详情:', this.videoDetail)
      } catch (err) {
        console.error('加载视频失败:', err)
        this.error = '加载视频失败，请稍后重试'
      } finally {
        this.loading = false
        // 等待 DOM 更新后再初始化播放器
        await this.$nextTick()
        if (!this.error) {
          this.initPlayer()
        }
      }
    },

    initPlayer() {
      const video = this.$refs.videoPlayer
      if (!video) {
        console.error('video元素不存在')
        return
      }

      console.log('初始化播放器')
      console.log('video.src =', video.src)
      console.log('streamUrl =', this.streamUrl)
      console.log('原始地址 =', this.videoDetail?.m3u8_url)

      // 如果是 m3u8 格式，使用 hls.js
      const originalUrl = this.videoDetail?.m3u8_url || ''
      if (Hls.isSupported() && (originalUrl.includes('m3u8') || !originalUrl.includes('.mp4'))) {
        console.log('使用 HLS.js')
        this.hls = new Hls()
        this.hls.loadSource(this.streamUrl)
        this.hls.attachMedia(video)

        this.hls.on(Hls.Events.MANIFEST_PARSED, () => {
          console.log('HLS manifest 解析成功')
          video.play().catch(e => console.log('自动播放被阻止:', e))
        })

        this.hls.on(Hls.Events.ERROR, (event, data) => {
          console.error('HLS 错误:', data)
        })
      } else {
        // 直接播放 (mp4 或 Safari)
        console.log('直接播放')
        video.play().catch(e => console.log('自动播放被阻止:', e))
      }
    },

    destroyPlayer() {
      if (this.hls) {
        this.hls.destroy()
        this.hls = null
      }
    },

    goBack() {
      this.$router.push({ name: 'VideoList' })
    }
  }
}
</script>

<style scoped>
.video-player-page {
  max-width: 1200px;
  margin: 0 auto;
}

.back-btn {
  display: inline-flex;
  align-items: center;
  padding: 0.5rem 1rem;
  background-color: transparent;
  color: #fff;
  border: 1px solid #555;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.9rem;
  margin-bottom: 1.5rem;
  transition: all 0.2s;
}

.back-btn:hover {
  background-color: #333;
  border-color: #888;
}

.loading, .error {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 400px;
  color: #888;
}

.spinner {
  width: 50px;
  height: 50px;
  border: 4px solid #333;
  border-top-color: #e50914;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin-bottom: 1rem;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.error button {
  margin-top: 1rem;
  padding: 0.75rem 1.5rem;
  background-color: #e50914;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
}

.player-container {
  background-color: #000;
  border-radius: 8px;
  overflow: hidden;
}

.video-wrapper {
  position: relative;
  width: 100%;
  aspect-ratio: 16 / 9;
  background-color: #000;
}

.video-element {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.video-info {
  padding: 1.5rem;
  background-color: #1a1a1a;
}

.video-title {
  font-size: 1.25rem;
  font-weight: 500;
  line-height: 1.4;
}

.video-url {
  font-size: 0.8rem;
  color: #888;
  margin-top: 0.5rem;
  word-break: break-all;
}
</style>
