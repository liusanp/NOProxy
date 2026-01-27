FROM python:3.11-slim

WORKDIR /app

# 安装 Python 依赖，跳过 Playwright 浏览器下载
COPY backend/requirements.txt ./backend/
ENV PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1
RUN pip install --no-cache-dir -r backend/requirements.txt

COPY backend ./backend
COPY frontend/dist ./frontend/dist

RUN mkdir -p cache/videos

# 默认使用外部 CDP
ENV BROWSER_MODE=cdp
ENV CDP_URL=http://host.docker.internal:9222
ENV PYTHONPATH=/app

EXPOSE 8000

CMD ["python", "-m", "uvicorn", "backend.main:app", "--host", "0.0.0.0", "--port", "8000"]
