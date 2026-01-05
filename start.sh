#!/bin/bash

# NOProxy 启动脚本

echo "=== NOProxy 启动脚本 ==="

# 检查是否安装了必要的依赖
check_dependencies() {
    echo "检查依赖..."

    if ! command -v python3 &> /dev/null; then
        echo "错误: 未找到 python3"
        exit 1
    fi

    if ! command -v npm &> /dev/null; then
        echo "错误: 未找到 npm"
        exit 1
    fi

    echo "依赖检查通过"
}

# 安装后端依赖
install_backend() {
    echo "安装后端依赖..."
    pip install -r requirements.txt
    playwright install chromium
}

# 安装前端依赖
install_frontend() {
    echo "安装前端依赖..."
    cd frontend
    npm install
    cd ..
}

# 启动后端
start_backend() {
    echo "启动后端服务..."
    cd "$(dirname "$0")"
    python3 -m uvicorn backend.main:app --host 0.0.0.0 --port 8000 --reload
}

# 启动前端
start_frontend() {
    echo "启动前端开发服务器..."
    cd frontend
    npm run dev
}

# 构建前端
build_frontend() {
    echo "构建前端..."
    cd frontend
    npm run build
    cd ..
}

case "$1" in
    install)
        check_dependencies
        install_backend
        install_frontend
        echo "安装完成!"
        ;;
    backend)
        start_backend
        ;;
    frontend)
        start_frontend
        ;;
    build)
        build_frontend
        ;;
    *)
        echo "用法: $0 {install|backend|frontend|build}"
        echo ""
        echo "命令说明:"
        echo "  install  - 安装所有依赖"
        echo "  backend  - 启动后端服务"
        echo "  frontend - 启动前端开发服务器"
        echo "  build    - 构建前端生产版本"
        exit 1
        ;;
esac
