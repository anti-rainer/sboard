#!/bin/bash

# SBoard 面板安装入口
# 自动检测平台并下载执行对应的安装脚本
#
# 用法:
#   curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/anti-rainer/sboard/main/scripts/install.sh | bash

set -e

# GitHub 配置
# 加速域名候选，按顺序尝试，全部失败后直连 GitHub
GH_PROXIES=(
    "https://ghfast.top/"
    "https://v6.gh-proxy.org/"
    ""
)
GH_PROXY="https://ghfast.top/"

# download_script <dest>
# 依次用各加速前缀下载引导脚本
download_script() {
    local dest="$1"
    local proxy url
    for proxy in "${GH_PROXIES[@]}"; do
        if [[ -n "$proxy" ]]; then
            url="${proxy}https://raw.githubusercontent.com/anti-rainer/sboard/main/scripts/install-sboard.sh"
        else
            url="https://raw.githubusercontent.com/anti-rainer/sboard/main/scripts/install-sboard.sh"
        fi
        if curl -fsSL --connect-timeout 15 --max-time 120 "$url" -o "$dest" 2>/dev/null && [[ -s "$dest" ]]; then
            SCRIPT_URL="$url"
            return 0
        fi
    done
    return 1
}

# 检测操作系统
case "$(uname -s 2>/dev/null)" in
    Linux|linux|Darwin|darwin)
        # Linux/macOS: 下载脚本到临时文件后执行
        TMP_SCRIPT=$(mktemp)
        if ! download_script "$TMP_SCRIPT"; then
            echo -e "${RED}下载安装脚本失败（所有加速源与直连均不可用）${NC}" >&2
            rm -f "$TMP_SCRIPT"
            exit 1
        fi
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
