<template>
  <div class="video-card" @click="$emit('click')">
    <div class="thumbnail">
      <img
        v-if="video.thumbnail"
        :src="video.thumbnail"
        :alt="video.title"
        @error="handleImageError"
      >
      <div v-else class="placeholder">
        <span>{{ video.title.charAt(0) }}</span>
      </div>
      <div class="play-icon">▶</div>
    </div>
    <div class="info">
      <h3 class="title">{{ video.title }}</h3>
      <p v-if="video.duration" class="duration">{{ video.duration }}</p>
    </div>
  </div>
</template>

<script>
export default {
  name: 'VideoCard',
  props: {
    video: {
      type: Object,
      required: true
    }
  },
  emits: ['click'],
  methods: {
    handleImageError(e) {
      e.target.style.display = 'none'
    }
  }
}
</script>

<style scoped>
.video-card {
  background-color: #1a1a1a;
  border-radius: 8px;
  overflow: hidden;
  cursor: pointer;
  transition: transform 0.2s, box-shadow 0.2s;
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

.placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #333, #555);
}

.placeholder span {
  font-size: 3rem;
  font-weight: bold;
  color: #666;
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
  margin-bottom: 0.5rem;
}

.duration {
  font-size: 0.8rem;
  color: #888;
}
</style>
