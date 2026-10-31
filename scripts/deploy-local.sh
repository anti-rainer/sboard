#!/usr/bin/env bash
#
# deploy-local.sh — 本机构建面板二进制，原子替换部署到生产机
#
# 相比走 GitHub Actions + sboard update 的链路，这条路省掉构建排队和下载，
# 改一行代码到上线只要十几秒。
#
# 原子性来自同一个文件系统里的 rename：先把新二进制传到 /opt/sboard/sboard.new，
# 校验通过后 mv 覆盖 sboard。rename 是原子的，不存在「拷贝到一半被启动」的
# 情况；正在运行的进程持有旧 inode，直到 systemctl restart 才切换。
#
# 用法：
#   ./scripts/deploy-local.sh              # 构建并部署
#   ./scripts/deploy-local.sh --no-build   # 复用已有二进制，只部署
#   ./scripts/deploy-local.sh --dry-run    # 构建但不上传
#
set -euo pipefail

# ---------- 可覆盖的配置 ----------
SSH_HOST="${SBOARD_SSH_HOST:-root@192.129.241.179}"
SSH_PORT="${SBOARD_SSH_PORT:-14625}"
SSH_KEY="${SBOARD_SSH_KEY:-/root/.ssh/id_rsa}"
REMOTE_DIR="${SBOARD_REMOTE_DIR:-/opt/sboard}"
REMOTE_BIN="${REMOTE_DIR}/sboard"
SERVICE="${SBOARD_SERVICE:-sboard}"
LOCAL_PORT="${SBOARD_LOCAL_PORT:-17403}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_OUT="${REPO_ROOT}/.deploy/sboard"

DO_BUILD=1
DRY_RUN=0
for arg in "$@"; do
  case "$arg" in
    --no-build) DO_BUILD=0 ;;
    --dry-run)  DRY_RUN=1 ;;
    -h|--help)  sed -n '2,20p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "未知参数: $arg" >&2; exit 2 ;;
  esac
done

step() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }
ok()   { printf '    \033[32m✓\033[0m %s\n' "$*"; }
die()  { printf '\n\033[1;31m[失败]\033[0m %s\n' "$*" >&2; exit 1; }

remote() {
  ssh -n -o BatchMode=yes -o ConnectTimeout=15 -p "$SSH_PORT" -i "$SSH_KEY" "$SSH_HOST" "$@"
}

# ---------- 1. 构建 ----------
if [ "$DO_BUILD" = "1" ]; then
  step "构建前端"
  cd "${REPO_ROOT}/web"
  [ -d node_modules ] || npm ci --no-audit --no-fund
  npm run build
  # vite 输出目录就是 cmd/sboard/web，被 go:embed all:web 内嵌进二进制
  [ -f "${REPO_ROOT}/cmd/sboard/web/index.html" ] \
    || die "前端产物缺失: cmd/sboard/web/index.html"
  ok "前端产物 $(find "${REPO_ROOT}/cmd/sboard/web" -type f | wc -l) 个文件"

  step "构建二进制"
  cd "$REPO_ROOT"
  VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
  COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
  mkdir -p "$(dirname "$BUILD_OUT")"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath \
      -ldflags "-s -w -X main.Version=${VERSION} -X main.CommitHash=${COMMIT}" \
      -o "$BUILD_OUT" ./cmd/sboard
  ok "$(basename "$BUILD_OUT") $(du -h "$BUILD_OUT" | cut -f1)  版本 ${VERSION} (${COMMIT})"
else
  [ -f "$BUILD_OUT" ] || die "--no-build 但找不到 $BUILD_OUT"
  ok "复用已有二进制"
fi

# ---------- 2. 本机冒烟 ----------
step "本机冒烟测试"
VER_OUT="$("$BUILD_OUT" -v 2>&1 | head -3 || true)"
[ -n "$VER_OUT" ] || die "二进制无法运行"
printf '%s\n' "$VER_OUT" | sed 's/^/    /'
[[ "$VER_OUT" == *"SBoard"* || "$VER_OUT" == *"sboard"* ]] || die "-v 输出不含版本信息"

LOCAL_SHA="$(sha256sum "$BUILD_OUT" | cut -d' ' -f1)"
ok "sha256 ${LOCAL_SHA:0:16}…"

if [ "$DRY_RUN" = "1" ]; then
  printf '\n\033[33m--dry-run：构建完成，未上传。\033[0m\n'
  exit 0
fi

# ---------- 3. 上传 ----------
step "上传到 ${SSH_HOST}:${REMOTE_DIR}"
remote "mkdir -p '${REMOTE_DIR}'"
scp -q -o BatchMode=yes -o ConnectTimeout=15 -P "$SSH_PORT" -i "$SSH_KEY" \
  "$BUILD_OUT" "${SSH_HOST}:${REMOTE_BIN}.new"
ok "已上传 sboard.new"

REMOTE_SHA="$(remote "sha256sum '${REMOTE_BIN}.new' | cut -d' ' -f1")"
[ "$LOCAL_SHA" = "$REMOTE_SHA" ] || die "校验不一致：本地 ${LOCAL_SHA:0:16}… 远端 ${REMOTE_SHA:0:16}…"
ok "sha256 校验一致"

# ---------- 4. 原子替换 ----------
step "原子替换并重启"
# 备份当前二进制用于回滚；mv 覆盖是同一文件系统内的原子 rename
remote "set -e
  systemctl is-active '${SERVICE}' >/dev/null 2>&1 || echo '    注意：服务原本未运行'
  cp -f '${REMOTE_BIN}' '${REMOTE_BIN}.prev'
  chmod +x '${REMOTE_BIN}.new'
  mv -f '${REMOTE_BIN}.new' '${REMOTE_BIN}'
  systemctl restart '${SERVICE}'
"
ok "已替换并重启 ${SERVICE}"

# ---------- 5. 健康检查 ----------
step "健康检查"
HEALTH_OK=0
for i in $(seq 1 20); do
  sleep 2
  state="$(remote "systemctl is-active '${SERVICE}'" || true)"
  code="$(remote "curl -sS --max-time 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:${LOCAL_PORT}/ 2>/dev/null" || echo 000)"
  if [ "$state" = "active" ] && [ "$code" = "200" ]; then
    ok "服务 active，面板 HTTP 200（第 ${i} 次探测）"
    HEALTH_OK=1
    break
  fi
  printf '    等待中… 服务=%s 面板=%s\n' "$state" "$code"
done

if [ "$HEALTH_OK" != "1" ]; then
  step "健康检查失败，回滚"
  remote "set -e
    journalctl -u '${SERVICE}' -n 25 --no-pager 2>/dev/null | tail -25
    mv -f '${REMOTE_BIN}.prev' '${REMOTE_BIN}'
    systemctl restart '${SERVICE}'
  "
  die "已回滚到上一版本，请检查上面的日志"
fi

# ---------- 6. 收尾 ----------
step "部署完成"
remote "
  echo '    版本: '\$('${REMOTE_BIN}' -v 2>&1 | head -1)
  echo '    二进制: '\$(ls -lh '${REMOTE_BIN}' | awk '{print \$5, \$6, \$7, \$8}')
  echo '    agent: '\$(systemctl is-active sboard-agent 2>/dev/null || echo '-')
"
ok "提交 $(git -C "$REPO_ROOT" log --oneline -1 2>/dev/null || echo '未知')"
