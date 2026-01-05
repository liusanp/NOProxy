<template>
  <div class="video-list">
    <h1 class="page-title">视频列表</h1>

    <!-- 加载状态 -->
    <div v-if="loading" class="loading">
      <div class="spinner"></div>
      <p>正在加载视频列表...</p>
    </div>

    <!-- 错误提示 -->
    <div v-else-if="error" class="error">
      <p>{{ error }}</p>
      <button @click="fetchVideos">重试</button>
    </div>

    <!-- 视频网格 -->
    <div v-else class="video-grid">
      <VideoCard
        v-for="video in videos"
        :key="video.id"
        :video="video"
        @click="playVideo(video)"
      />
    </div>

    <!-- 空状态 -->
    <div v-if="!loading && !error && videos.length === 0" class="empty">
      <p>暂无视频</p>
    </div>

    <!-- 分页 -->
    <div v-if="videos.length > 0" class="pagination">
      <button
        :disabled="page === 1"
        @click="changePage(1)"
        class="page-btn"
      >
        首页
      </button>
      <button
        :disabled="page === 1"
        @click="changePage(page - 1)"
        class="page-btn"
      >
        上一页
      </button>

      <div class="page-info">
        <input
          type="number"
          v-model.number="inputPage"
          @keyup.enter="goToPage"
          :min="1"
          :max="totalPages"
          class="page-input"
        />
        <span>/ {{ totalPages }} 页</span>
      </div>

      <button
        :disabled="page >= totalPages"
        @click="changePage(page + 1)"
        class="page-btn"
      >
        下一页
      </button>
      <button
        :disabled="page >= totalPages"
        @click="changePage(totalPages)"
        class="page-btn"
      >
        末页
      </button>
    </div>
  </div>
</template>

<script>
import { videoApi } from '../api'
import VideoCard from '../components/VideoCard.vue'

export default {
  name: 'VideoList',
  components: {
    VideoCard
  },
  data() {
    return {
      videos: [],
      loading: false,
      error: null,
      page: 1,
      totalPages: 1,
      inputPage: 1
    }
  },
  mounted() {
    this.fetchVideos()
  },
  methods: {
    async fetchVideos() {
      this.loading = true
      this.error = null

      try {
        const response = await videoApi.getList(this.page)
        this.videos = response.data.videos
        this.totalPages = response.data.total_pages || 1
        this.inputPage = this.page
      } catch (err) {
        console.error('获取视频列表失败:', err)
        this.error = '获取视频列表失败，请检查后端服务是否运行'
      } finally {
        this.loading = false
      }
    },
    playVideo(video) {
      this.$router.push({
        name: 'VideoPlayer',
        params: { id: video.id }
      })
    },
    changePage(newPage) {
      if (newPage < 1 || newPage > this.totalPages) return
      this.page = newPage
      this.inputPage = newPage
      this.fetchVideos()
      window.scrollTo({ top: 0, behavior: 'smooth' })
    },
    goToPage() {
      const targetPage = parseInt(this.inputPage)
      if (targetPage >= 1 && targetPage <= this.totalPages) {
        this.changePage(targetPage)
      } else {
        this.inputPage = this.page
      }
    }
  }
}
</script>

<style scoped>
.video-list {
  padding: 1rem 0;
}

.page-title {
  font-size: 1.5rem;
  margin-bottom: 2rem;
}

.video-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 1.5rem;
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
  font-size: 1rem;
}

.error button:hover {
  background-color: #f40612;
}

.pagination {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 0.5rem;
  margin-top: 2rem;
  padding: 1rem 0;
  flex-wrap: wrap;
}

.page-btn {
  padding: 0.5rem 1rem;
  background-color: #333;
  color: white;
  border: 1px solid #555;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.9rem;
  transition: all 0.2s;
}

.page-btn:hover:not(:disabled) {
  background-color: #e50914;
  border-color: #e50914;
}

.page-btn:disabled {
  background-color: #222;
  color: #555;
  cursor: not-allowed;
}

.page-info {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: #888;
  margin: 0 0.5rem;
}

.page-input {
  width: 60px;
  padding: 0.5rem;
  background-color: #222;
  color: white;
  border: 1px solid #555;
  border-radius: 4px;
  text-align: center;
  font-size: 0.9rem;
}

.page-input:focus {
  outline: none;
  border-color: #e50914;
}

.page-input::-webkit-inner-spin-button,
.page-input::-webkit-outer-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
</style>
