# approving 镜像(构建上下文 = 仓库根,因为要把 web/ 前端一起打进来)。
#
# 硬性约定(deployment docs):**配置不打进镜像**。这里只构建并
# 承载业务二进制 + 前端静态产物;运行时配置由环境变量 + ConfigMap 挂载的
# CONFIG_PATH 提供,镜像内不 COPY 任何配置 / 密钥。
#
# 需要 CGO(go-sqlite3)。运行阶段用 node:20-bookworm-slim:既有 glibc/SQLite
# 运行时依赖,也提供 node 供 set_plan 做 Mermaid 11.x 语法校验。

# --- Web (Vite) 构建 ---
FROM node:20-bookworm-slim AS web-builder
WORKDIR /web
# .npmrc 把 registry 指向 registry.npmmirror.com(见 web/.npmrc):国内网络直连
# registry.npmjs.org 拉大包(monaco-editor)易 ECONNRESET,换镜像源规避。
COPY web/package*.json web/.npmrc ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# --- Go 构建 ---
FROM golang:1.25-bookworm AS build
WORKDIR /src
# 国内直连 proxy.golang.org / sum.golang.org 易超时(与 Dockerfile.dev 一致),
# 走七牛 goproxy.cn + Google 中国区校验和镜像。
ENV GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=sum.golang.google.cn
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ .
# 前端产物随二进制由 Go 进程从文件系统服务(非 go:embed)。
COPY --from=web-builder /web/dist ./web/dist
# Optional revision stamp for the overview-page service commit badge.
# Production images have no .git (root .dockerignore); pass GIT_COMMIT or
# GITHUB_SHA as build-arg. Empty args still produce a working binary.
ARG GIT_COMMIT=""
ARG GITHUB_SHA=""
RUN COMMIT_SHA="${GIT_COMMIT:-$GITHUB_SHA}" && \
    CGO_ENABLED=1 go build -trimpath \
      -ldflags="-s -w -X github.com/cocofhu/approving/internal/version.commit=${COMMIT_SHA}" \
      -o /out/approving-server ./cmd/server
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/approving ./cmd/doctor

# --- 运行 ---
# 使用 node bookworm-slim 作为运行底包,使 set_plan 的 Mermaid 11.x 语法校验
# (node + 内嵌 bundle.mjs)在镜像内可用,并与 web-builder 同 glibc。
FROM node:20-bookworm-slim
# 后端不再直连 Docker:沙箱由 sandbox-gateway 经 REST 创建、数据面走 SSH,
# 因此运行镜像无需 docker CLI / dockerd,只需 CA 证书(HTTPS 连网关 / git)。
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
WORKDIR /app
COPY --from=build /out/approving-server /app/approving-server
COPY --from=build /out/approving /app/approving
COPY --from=build /src/web/dist /app/web/dist

# 数据落 SQLite,挂到持久卷;端口与默认 DB 路径可被部署的 env 覆盖。
ENV APPROVING_PORT=8080 \
    APPROVING_DB=/data/approving.db
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/app/approving-server"]
