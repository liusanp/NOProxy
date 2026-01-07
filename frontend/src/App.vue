<template>
  <div id="app">
    <!-- 密码验证弹窗 -->
    <div v-if="!isAuthenticated" class="auth-overlay">
      <div class="auth-modal">
        <h2>请输入访问密码</h2>
        <form @submit.prevent="checkPassword">
          <input
            type="password"
            v-model="password"
            placeholder="密码"
            class="password-input"
            autofocus
          />
          <button type="submit" class="submit-btn" :disabled="loading">
            {{ loading ? '验证中...' : '进入' }}
          </button>
        </form>
        <p v-if="error" class="error-msg">{{ error }}</p>
      </div>
    </div>

    <!-- 主内容 -->
    <template v-else>
      <header class="header">
        <router-link to="/" class="logo">NOProxy</router-link>
        <nav class="nav">
          <router-link to="/" class="nav-link">视频列表</router-link>
          <router-link to="/cached" class="nav-link">已缓存</router-link>
        </nav>
      </header>
      <main class="main">
        <router-view />
      </main>
    </template>
  </div>
</template>

<script>
import { authApi } from './api'

export default {
  name: 'App',
  data() {
    return {
      isAuthenticated: false,
      isAdmin: false,
      password: '',
      error: '',
      loading: false
    }
  },
  provide() {
    return {
      isAdmin: () => this.isAdmin
    }
  },
  created() {
    // 检查是否已经验证过（存储在 sessionStorage）
    const authenticated = sessionStorage.getItem('authenticated')
    if (authenticated === 'true') {
      this.isAuthenticated = true
      this.isAdmin = sessionStorage.getItem('isAdmin') === 'true'
    }
  },
  methods: {
    async checkPassword() {
      if (this.loading) return
      this.loading = true
      this.error = ''

      try {
        const res = await authApi.verify(this.password)
        if (res.data.success) {
          this.isAuthenticated = true
          this.isAdmin = res.data.isAdmin || false
          sessionStorage.setItem('authenticated', 'true')
          sessionStorage.setItem('isAdmin', this.isAdmin ? 'true' : 'false')
          if (this.isAdmin) {
            sessionStorage.setItem('adminToken', this.password)
          }
        } else {
          this.error = res.data.message || '密码错误'
          this.password = ''
        }
      } catch (e) {
        this.error = '验证失败，请重试'
        this.password = ''
      } finally {
        this.loading = false
      }
    }
  }
}
</script>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
  background-color: #0f0f0f;
  color: #fff;
  min-height: 100vh;
}

#app {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}

/* 密码验证弹窗样式 */
.auth-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: #0f0f0f;
  display: flex;
  justify-content: center;
  align-items: center;
  z-index: 1000;
}

.auth-modal {
  background-color: #1a1a1a;
  padding: 2rem;
  border-radius: 8px;
  text-align: center;
  min-width: 300px;
}

.auth-modal h2 {
  margin-bottom: 1.5rem;
  color: #e50914;
}

.password-input {
  width: 100%;
  padding: 0.75rem 1rem;
  font-size: 1rem;
  border: 1px solid #333;
  border-radius: 4px;
  background-color: #0f0f0f;
  color: #fff;
  margin-bottom: 1rem;
}

.password-input:focus {
  outline: none;
  border-color: #e50914;
}

.submit-btn {
  width: 100%;
  padding: 0.75rem 1rem;
  font-size: 1rem;
  background-color: #e50914;
  color: #fff;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  transition: background-color 0.2s;
}

.submit-btn:hover {
  background-color: #b8070f;
}

.error-msg {
  color: #e50914;
  margin-top: 1rem;
  font-size: 0.9rem;
}

.header {
  background-color: #1a1a1a;
  padding: 1rem 2rem;
  border-bottom: 1px solid #333;
  display: flex;
  justify-content: space-between;
  align-items: center;
  position: sticky;
  top: 0;
  z-index: 100;
}

.logo {
  font-size: 1.5rem;
  font-weight: bold;
  color: #e50914;
  text-decoration: none;
}

.nav {
  display: flex;
  gap: 1.5rem;
}

.nav-link {
  color: #888;
  text-decoration: none;
  font-size: 0.95rem;
  transition: color 0.2s;
}

.nav-link:hover {
  color: #fff;
}

.nav-link.router-link-exact-active {
  color: #e50914;
}

.main {
  flex: 1;
  padding: 2rem;
  max-width: 1400px;
  margin: 0 auto;
  width: 100%;
}

a {
  color: inherit;
  text-decoration: none;
}
</style>
