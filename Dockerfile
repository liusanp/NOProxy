FROM python:3.11-slim

# 合并安装所有依赖，使用 chromium 替代 google-chrome，使用轻量中文字体
RUN apt-get update && apt-get install -y --no-install-recommends \
    supervisor \
    xvfb \
    chromium \
    chromium-sandbox \
    fonts-wqy-zenhei \
    libxcomposite1 \
    libxdamage1 \
    libxrandr2 \
    libgbm1 \
    libasound2 \
    libatk1.0-0 \
    libatk-bridge2.0-0 \
    libcups2 \
    libdrm2 \
    libxkbcommon0 \
    libpango-1.0-0 \
    libcairo2 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

WORKDIR /app

# 安装 Python 依赖，跳过 Playwright 浏览器下载
COPY backend/requirements.txt ./backend/
ENV PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1
RUN pip install --no-cache-dir -r backend/requirements.txt

COPY backend ./backend
COPY frontend/dist ./frontend/dist

# Supervisor 配置
RUN mkdir -p /var/log/supervisor /tmp/chrome-data
COPY docker/supervisord.conf /etc/supervisor/conf.d/supervisord.conf

# 环境变量
ENV BROWSER_MODE=cdp
ENV CDP_URL=http://127.0.0.1:9222
ENV HEADLESS=false
ENV DISPLAY=:99
ENV PYTHONPATH=/app

EXPOSE 8000

CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]
