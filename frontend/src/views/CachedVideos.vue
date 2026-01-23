<template>
  <div class="cached-videos">
    <div class="page-header">
      <h1 class="page-title">已缓存视频</h1>
      <div class="cache-info" v-if="cacheInfo">
        <span class="cache-size">{{ cacheInfo.total_size_mb }} MB</span>
        <span class="cache-count">{{ cacheInfo.total }} 个视频</span>
        <button
          v-if="canDelete && cacheInfo.total > 0"
          class="clear-btn"
          @click="confirmClearAll"
          :disabled="clearing"
        >
          {{ clearing ? '清除中...' : '清空全部' }}
        </button>
      </div>
    </div>

    <!-- 加载状态 -->
    <div v-if="loading" class="loading">
      <div class="spinner"></div>
      <p>正在加载缓存列表...</p>
    </div>

    <!-- 错误提示 -->
    <div v-else-if="error" class="error">
      <p>{{ error }}</p>
      <button @click="fetchCachedVideos">重试</button>
    </div>

    <!-- 空状态 -->
    <div v-else-if="videos.length === 0" class="empty">
      <p>暂无缓存视频</p>
      <router-link to="/" class="back-link">去浏览视频</router-link>
    </div>

    <!-- 视频网格 -->
    <template v-else>
      <div class="video-grid">
        <div
          v-for="video in videos"
          :key="video.viewkey"
          class="video-card"
          @click="playVideo(video)"
        >
          <div class="thumbnail">
            <img
              :src="getThumbnailUrl(video.viewkey)"
              :alt="video.title"
              @error="handleImageError"
            >
            <div class="play-icon">▶</div>
            <span class="video-type">{{ video.type.toUpperCase() }}</span>
            <span class="video-size">{{ formatSize(video.size) }}</span>
          </div>
          <div class="info">
            <h3 class="title">{{ video.title || video.viewkey }}</h3>
          </div>
          <button
            v-if="canDelete"
            class="delete-btn"
            @click.stop="confirmDelete(video)"
            title="删除缓存"
          >
            ×
          </button>
        </div>
      </div>

      <!-- 分页 -->
      <div v-if="totalPages > 1" class="pagination">
        <button
          class="page-btn"
          :disabled="currentPage === 1"
          @click="goToPage(currentPage - 1)"
        >
          上一页
        </button>
        <div class="page-numbers">
          <button
            v-for="p in visiblePages"
            :key="p"
            class="page-num"
            :class="{ active: p === currentPage }"
            @click="goToPage(p)"
          >
            {{ p }}
          </button>
        </div>
        <button
          class="page-btn"
          :disabled="currentPage === totalPages"
          @click="goToPage(currentPage + 1)"
        >
          下一页
        </button>
      </div>
    </template>
  </div>
</template>

<script>
import { cacheApi, videoApi } from '../api'

export default {
  name: 'CachedVideos',
  inject: ['isAdmin'],
  data() {
    return {
      videos: [],
      cacheInfo: null,
      loading: false,
      error: null,
      clearing: false,
      currentPage: 1,
      totalPages: 1
    }
  },
  computed: {
    canDelete() {
      return this.isAdmin()
    },
    adminToken() {
      return sessionStorage.getItem('adminToken')
    },
    visiblePages() {
      const pages = []
      const total = this.totalPages
      const current = this.currentPage
      let start = Math.max(1, current - 2)
      let end = Math.min(total, current + 2)

      if (end - start < 4) {
        if (start === 1) {
          end = Math.min(total, start + 4)
        } else if (end === total) {
          start = Math.max(1, end - 4)
        }
      }

      for (let i = start; i <= end; i++) {
        pages.push(i)
      }
      return pages
    }
  },
  mounted() {
    // 从 URL 读取页码
    const page = parseInt(this.$route.query.page) || 1
    this.currentPage = page
    this.fetchCachedVideos()
  },
  watch: {
    '$route.query.page'(newPage) {
      const page = parseInt(newPage) || 1
      if (page !== this.currentPage) {
        this.currentPage = page
        this.fetchCachedVideos()
      }
    }
  },
  methods: {
    async fetchCachedVideos() {
      this.loading = true
      this.error = null

      try {
        const response = await cacheApi.getList(this.currentPage)
        this.cacheInfo = response.data
        this.totalPages = response.data.total_pages

        // 获取每个视频的详情
        const videosWithDetails = await Promise.all(
          response.data.videos.map(async (v) => {
            try {
              const detail = await videoApi.getDetail(v.viewkey)
              return {
                ...v,
                title: detail.data.title
              }
            } catch {
              return {
                ...v,
                title: null
              }
            }
          })
        )

        this.videos = videosWithDetails
      } catch (err) {
        console.error('获取缓存列表失败:', err)
        this.error = '获取缓存列表失败'
      } finally {
        this.loading = false
      }
    },

    goToPage(page) {
      if (page < 1 || page > this.totalPages || page === this.currentPage) return
      // 通过 URL 参数保持分页状态
      this.$router.push({ query: { page } })
      window.scrollTo({ top: 0, behavior: 'smooth' })
    },

    getThumbnailUrl(viewkey) {
      return videoApi.getThumbnailUrl(viewkey)
    },

    formatSize(bytes) {
      if (bytes < 1024) return bytes + ' B'
      if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
      if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
      return (bytes / (1024 * 1024 * 1024)).toFixed(2) + ' GB'
    },

    playVideo(video) {
      this.$router.push({
        name: 'VideoPlayer',
        params: { id: video.viewkey }
      })
    },

    handleImageError(e) {
      e.target.style.display = 'none'
    },

    async confirmDelete(video) {
      const title = video.title || video.viewkey
      if (!confirm(`确定删除「${title}」的缓存？`)) {
        return
      }

      try {
        await cacheApi.delete(video.viewkey, this.adminToken)
        // 重新获取当前页
        await this.fetchCachedVideos()
        // 如果当前页为空且不是第一页，跳转到上一页
        if (this.videos.length === 0 && this.currentPage > 1) {
          this.$router.replace({ query: { page: this.currentPage - 1 } })
        }
      } catch (err) {
        console.error('删除失败:', err)
        alert(err.response?.data?.detail || '删除失败')
      }
    },

    async confirmClearAll() {
      if (!confirm('确定清空所有缓存？此操作不可恢复！')) {
        return
      }

      this.clearing = true
      try {
        await cacheApi.clearAll(this.adminToken)
        this.videos = []
        this.currentPage = 1
        this.totalPages = 1
        this.cacheInfo = {
          ...this.cacheInfo,
          total_size: 0,
          total_size_mb: 0,
          total: 0
        }
      } catch (err) {
        console.error('清空失败:', err)
        alert(err.response?.data?.detail || '清空失败')
      } finally {
        this.clearing = false
      }
    }
  }
}
</script>

<style scoped>
.cached-videos {
  padding: 1rem 0;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 2rem;
  flex-wrap: wrap;
  gap: 1rem;
}

.page-title {
  font-size: 1.5rem;
}

.cache-info {
  display: flex;
  align-items: center;
  gap: 1rem;
  color: #888;
  font-size: 0.9rem;
}

.cache-size {
  background-color: #333;
  padding: 0.25rem 0.75rem;
  border-radius: 4px;
  color: #e50914;
  font-weight: 500;
}

.cache-count {
  color: #aaa;
}

.clear-btn {
  padding: 0.5rem 1rem;
  background-color: transparent;
  color: #e50914;
  border: 1px solid #e50914;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.85rem;
  transition: all 0.2s;
}

.clear-btn:hover:not(:disabled) {
  background-color: #e50914;
  color: #fff;
}

.clear-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.video-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 1.5rem;
}

.video-card {
  background-color: #1a1a1a;
  border-radius: 8px;
  overflow: hidden;
  cursor: pointer;
  transition: transform 0.2s, box-shadow 0.2s;
  position: relative;
}

.video-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 8px 25px rgba(0, 0, 0, 0.4);
}

.thumbnail {
  position: relative;
  aspect-ratio: 16 / 9;
  background-color: #333;
  overflow: hidden;
}

.thumbnail img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.play-icon {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 60px;
  height: 60px;
  background-color: rgba(229, 9, 20, 0.9);
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.5rem;
  opacity: 0;
  transition: opacity 0.2s;
}

.video-card:hover .play-icon {
  opacity: 1;
}

.video-type {
  position: absolute;
  top: 8px;
  left: 8px;
  background-color: rgba(0, 0, 0, 0.7);
  color: #fff;
  padding: 2px 6px;
  border-radius: 3px;
  font-size: 0.7rem;
  font-weight: 600;
}

.video-size {
  position: absolute;
  bottom: 8px;
  right: 8px;
  background-color: rgba(0, 0, 0, 0.7);
  color: #fff;
  padding: 2px 6px;
  border-radius: 3px;
  font-size: 0.75rem;
}

.info {
  padding: 1rem;
}

.title {
  font-size: 0.95rem;
  font-weight: 500;
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.delete-btn {
  position: absolute;
  top: 8px;
  right: 8px;
  width: 28px;
  height: 28px;
  background-color: rgba(0, 0, 0, 0.7);
  color: #fff;
  border: none;
  border-radius: 50%;
  cursor: pointer;
  font-size: 1.2rem;
  line-height: 1;
  opacity: 0;
  transition: opacity 0.2s, background-color 0.2s;
  display: flex;
  align-items: center;
  justify-content: center;
}

.video-card:hover .delete-btn {
  opacity: 1;
}

.delete-btn:hover {
  background-color: #e50914;
}

.loading, .error, .empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 300px;
  color: #888;
}

.spinner {
  width: 40px;
  height: 40px;
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

.back-link {
  margin-top: 1rem;
  color: #e50914;
  text-decoration: underline;
}

.pagination {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 0.5rem;
  margin-top: 2rem;
  padding: 1rem 0;
}

.page-btn {
  padding: 0.5rem 1rem;
  background-color: #333;
  color: #fff;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  transition: background-color 0.2s;
}

.page-btn:hover:not(:disabled) {
  background-color: #e50914;
}

.page-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.page-numbers {
  display: flex;
  gap: 0.25rem;
}

.page-num {
  width: 36px;
  height: 36px;
  background-color: #333;
  color: #fff;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  transition: background-color 0.2s;
}

.page-num:hover {
  background-color: #444;
}

.page-num.active {
  background-color: #e50914;
}
</style>
