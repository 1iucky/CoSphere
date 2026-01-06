# CoSphere 生产环境部署指南

## ⚠️ 问题说明

你遇到的错误：
```
The requested image's platform (linux/arm64/v8) does not match the detected host platform (linux/amd64/v4)
```

**原因**: Docker 镜像架构不匹配
- 本地开发环境: ARM64 (Apple Silicon / M1/M2)
- 生产服务器: AMD64/x86_64 (Intel/AMD)

## ✅ 解决方案

我已经为你创建了生产环境专用的配置文件，强制使用 AMD64 架构镜像。

---

## 📦 部署步骤

### 步骤 1: 传输文件到生产服务器

在**本地**执行：

```bash
# 传输生产环境配置文件
scp docker-compose.production.yml user@your-server:/path/to/cosphere/
scp .env.production.example user@your-server:/path/to/cosphere/

# 传输数据库备份（如果需要导入数据）
scp -r database-backups user@your-server:/path/to/cosphere/
```

### 步骤 2: 在生产服务器上配置

登录到**生产服务器**：

```bash
# 进入项目目录
cd /path/to/cosphere

# 复制并编辑生产环境配置
cp .env.production.example .env.production
vim .env.production
```

**必须修改以下配置**:

```bash
# 1. PostgreSQL 密码（必须修改！）
POSTGRES_PASSWORD=your_strong_password_here

# 2. 会话密钥（生成强随机字符串）
# 生成方法:
openssl rand -base64 32
# 将输出粘贴到下面
SESSION_SECRET=生成的随机字符串

# 3. 服务器地址
SERVER_ADDRESS=https://your-domain.com

# 4. Redis 密码（推荐设置）
REDIS_PASSWORD=your_redis_password
```

保存并退出（`:wq`）

### 步骤 3: 启动服务

```bash
# 使用生产环境配置启动
docker-compose -f docker-compose.production.yml --env-file .env.production up -d

# 查看服务状态
docker-compose -f docker-compose.production.yml ps

# 查看日志
docker-compose -f docker-compose.production.yml logs -f
```

### 步骤 4: 导入数据库（如果有备份）

```bash
# 进入备份目录
cd database-backups

# 解压备份文件
gunzip new-api-backup-*.dump.gz

# 使用 Docker 导入脚本
bash import-to-production-docker.sh
# 输入 YES 确认
```

### 步骤 5: 验证部署

```bash
# 1. 检查服务健康状态
docker-compose -f docker-compose.production.yml ps

# 应该看到所有服务状态为 "Up (healthy)"

# 2. 测试 API
curl http://localhost:3000/api/status

# 3. 检查数据库连接
docker exec cosphere-postgres psql -U root -d new-api -c "SELECT COUNT(*) FROM users;"

# 4. 访问前端（通过浏览器）
# http://your-server-ip:3000
```

---

## 🔧 方案二: 直接修改现有 docker-compose.yml

如果你想继续使用原来的 `docker-compose.yml`，只需添加 `platform` 配置：

```yaml
services:
  new-api:
    image: calciumion/new-api:latest
    # ⬇️ 添加这一行，强制使用 AMD64 架构
    platform: linux/amd64
    container_name: new-api
    # ... 其他配置保持不变
```

然后正常启动：
```bash
docker-compose up -d
```

---

## 🔐 生产环境安全配置

### 1. 修改默认密码

```bash
# 生成强密码
openssl rand -base64 32

# 修改 PostgreSQL 密码
docker exec -it cosphere-postgres psql -U postgres
ALTER USER root WITH PASSWORD 'your_new_strong_password';
\q

# 更新 .env.production 文件中的密码
vim .env.production
# 修改 POSTGRES_PASSWORD=your_new_strong_password

# 重启服务使配置生效
docker-compose -f docker-compose.production.yml restart new-api
```

### 2. 防火墙配置

```bash
# 只允许必要的端口
sudo ufw allow 22/tcp      # SSH
sudo ufw allow 80/tcp      # HTTP
sudo ufw allow 443/tcp     # HTTPS
sudo ufw enable

# 注意：数据库端口（5432）和 Redis（6379）不应该对外开放
# docker-compose.production.yml 已配置为只在本地监听（127.0.0.1）
```

### 3. 配置 HTTPS（推荐使用 Nginx 反向代理）

```bash
# 安装 Nginx
sudo apt update
sudo apt install -y nginx certbot python3-certbot-nginx

# 配置 Nginx 反向代理
sudo vim /etc/nginx/sites-available/cosphere
```

Nginx 配置示例：
```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

```bash
# 启用配置
sudo ln -s /etc/nginx/sites-available/cosphere /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl restart nginx

# 获取 SSL 证书
sudo certbot --nginx -d your-domain.com
```

---

## 📊 监控和维护

### 查看日志

```bash
# 查看所有服务日志
docker-compose -f docker-compose.production.yml logs -f

# 查看特定服务日志
docker logs -f cosphere
docker logs -f cosphere-postgres
docker logs -f cosphere-redis

# 查看最近 100 行日志
docker logs --tail 100 cosphere
```

### 备份数据库

```bash
# 手动备份
docker exec cosphere-postgres pg_dump \
    -U root \
    -d new-api \
    -F c \
    > backup-$(date +%Y%m%d_%H%M%S).dump

# 设置自动备份（crontab）
crontab -e
# 添加：每天凌晨 2 点备份
0 2 * * * docker exec cosphere-postgres pg_dump -U root -d new-api -F c > /backup/cosphere-$(date +\%Y\%m\%d).dump
```

### 更新服务

```bash
# 拉取最新镜像
docker-compose -f docker-compose.production.yml pull

# 重启服务
docker-compose -f docker-compose.production.yml up -d

# 清理旧镜像
docker image prune -a
```

---

## 🆘 故障排查

### 问题 1: 容器无法启动

```bash
# 查看详细错误
docker-compose -f docker-compose.production.yml logs cosphere

# 检查端口占用
sudo netstat -tlnp | grep -E '3000|5432|6379'

# 检查磁盘空间
df -h
```

### 问题 2: 数据库连接失败

```bash
# 进入容器检查
docker exec -it cosphere sh

# 测试数据库连接
wget -O- http://localhost:3000/api/status

# 检查环境变量
env | grep SQL_DSN
```

### 问题 3: 架构仍然不匹配

```bash
# 清除所有镜像和容器
docker-compose -f docker-compose.production.yml down -v
docker system prune -a

# 强制拉取 AMD64 镜像
docker pull --platform linux/amd64 calciumion/new-api:latest
docker pull --platform linux/amd64 postgres:15-alpine
docker pull --platform linux/amd64 redis:7-alpine

# 重新启动
docker-compose -f docker-compose.production.yml up -d
```

---

## 📝 快速命令参考

| 操作 | 命令 |
|------|------|
| 启动服务 | `docker-compose -f docker-compose.production.yml up -d` |
| 停止服务 | `docker-compose -f docker-compose.production.yml down` |
| 重启服务 | `docker-compose -f docker-compose.production.yml restart` |
| 查看日志 | `docker-compose -f docker-compose.production.yml logs -f` |
| 查看状态 | `docker-compose -f docker-compose.production.yml ps` |
| 进入容器 | `docker exec -it cosphere sh` |
| 备份数据库 | `docker exec cosphere-postgres pg_dump -U root -d new-api -F c > backup.dump` |

---

## ✅ 部署检查清单

完成以下检查确保部署成功：

- [ ] 已传输 `docker-compose.production.yml` 到服务器
- [ ] 已创建并配置 `.env.production` 文件
- [ ] 已修改所有默认密码
- [ ] 已设置强随机的 SESSION_SECRET
- [ ] 服务已成功启动（`docker-compose ps` 显示 healthy）
- [ ] API 状态检查通过（`curl http://localhost:3000/api/status`）
- [ ] 已导入数据库备份（如果需要）
- [ ] 可以通过浏览器访问前端
- [ ] 管理员账号可以正常登录
- [ ] 已配置防火墙规则
- [ ] 已配置 HTTPS（生产环境推荐）
- [ ] 已设置数据库自动备份

---

## 🔗 相关文件

- 生产环境配置: `docker-compose.production.yml`
- 环境变量示例: `.env.production.example`
- 数据库迁移指南: `docs/DATABASE_MIGRATION_GUIDE.md`
- 原始配置: `docker-compose.yml`

部署成功后，记得保护好 `.env.production` 文件，不要提交到 Git 仓库！
