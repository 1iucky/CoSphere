# ============================================
# Stage 1: 前端构建
# ============================================
# 使用阿里云镜像加速
FROM m.daocloud.io/docker.io/oven/bun:latest AS frontend-builder

WORKDIR /build

# 1. 先复制依赖文件（利用缓存）
COPY web/package.json web/bun.lock ./

# 2. 安装依赖（只有 package.json 变化才重新执行）
RUN bun install

# 3. 复制源码
COPY ./web .
COPY ./VERSION .

# 4. 构建前端
RUN DISABLE_ESLINT_PLUGIN='true' \
    VITE_REACT_APP_VERSION=$(cat VERSION) \
    VITE_REACT_APP_SERVER_URL=/cosphere \
    bun run build

# ============================================
# Stage 2: Go 依赖下载（独立阶段，最大化缓存利用）
# ============================================
FROM m.daocloud.io/docker.io/library/golang:alpine AS go-deps

WORKDIR /build

# 设置 Go 代理（加速国内下载）
ENV GOPROXY=https://goproxy.cn,direct
ENV GO111MODULE=on

# 只复制依赖文件
COPY go.mod go.sum ./

# 下载依赖（只有 go.mod/go.sum 变化才重新执行）
RUN go mod download

# ============================================
# Stage 3: Go 编译
# ============================================
FROM m.daocloud.io/docker.io/library/golang:alpine AS backend-builder

ENV GO111MODULE=on CGO_ENABLED=0
ENV GOPROXY=https://goproxy.cn,direct

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64}

WORKDIR /build

# 从依赖阶段复制已下载的模块
COPY --from=go-deps /go/pkg/mod /go/pkg/mod
COPY --from=go-deps /build/go.mod /build/go.sum ./

# 复制源码
COPY . .

# 复制前端构建产物
COPY --from=frontend-builder /build/dist ./web/dist

# 编译（使用缓存的依赖）
RUN go build -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$(cat VERSION)'" -o new-api

# ============================================
# Stage 4: 最终镜像（最小化）
# ============================================
FROM m.daocloud.io/docker.io/library/alpine:latest

# 使用阿里云 APK 源加速
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk upgrade --no-cache \
    && apk add --no-cache ca-certificates tzdata \
    && update-ca-certificates

COPY --from=backend-builder /build/new-api /

EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/new-api"]
