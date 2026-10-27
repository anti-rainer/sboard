#!/bin/bash

# SBoard 面板安装入口
# 自动检测平台并下载执行对应的安装脚本
#
# 用法:
#   curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/anti-rainer/sboard/main/scripts/install.sh | bash

set -e

# GitHub 配置
GH_PROXY="https://ghfast.top/"
SCRIPT_URL="${GH_PROXY}https://raw.githubusercontent.com/anti-rainer/sboard/main/scripts/install-sboard.sh"

# 检测操作系统
case "$(uname -s 2>/dev/null)" in
    Linux|linux|Darwin|darwin)
        # Linux/macOS: 下载脚本到临时文件后执行
        TMP_SCRIPT=$(mktemp)
        curl -fsSL "$SCRIPT_URL" -o "$TMP_SCRIPT"
        chmod +x "$TMP_SCRIPT"
        # 透传全部参数给子脚本；有终端时把 stdin 接回终端以保留交互能力，
        # 无终端（CI/管道）时直接执行，避免 /dev/tty 打不开而失败。
        if { exec 3<>/dev/tty; } 2>/dev/null; then
            exec 3>&-
            bash "$TMP_SCRIPT" "$@" </dev/tty
        else
            bash "$TMP_SCRIPT" "$@"
        fi
        rm -f "$TMP_SCRIPT"
        ;;
    MINGW*|MSYS*|CYGWIN*)
        echo "检测到 Windows 环境，请使用 PowerShell 安装:"
        echo ""
        echo "  irm ${GH_PROXY}https://raw.githubusercontent.com/anti-rainer/sboard/main/scripts/install-sboard.ps1 | iex"
        echo ""
        ;;
    *)
        echo "不支持的操作系统: $(uname -s)"
        exit 1
        ;;
esac
