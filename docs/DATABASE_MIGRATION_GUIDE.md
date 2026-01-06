# 数据库迁移快速指南

## 📋 概述

本指南帮助你将本地 Docker PostgreSQL 数据库迁移到生产环境。

## 🚀 快速开始

### 步骤 1: 导出本地数据库

在项目根目录执行：

```bash
# 运行导出脚本
./scripts/export-database.sh
```

脚本会自动完成以下操作：
- ✅ 检查 Docker 容器状态
- ✅ 导出数据库（二进制格式 + SQL 格式）
- ✅ 压缩备份文件
- ✅ 生成统计信息
- ✅ 生成生产环境导入脚本

### 步骤 2: 传输备份文件到生产服务器

```bash
# 方式 A: 只传输压缩的备份文件（推荐，文件小）
scp database-backups/new-api-backup-*.dump.gz user@production-server:/backup/

# 方式 B: 传输整个备份目录（包含所有文件和脚本）
scp -r database-backups user@production-server:/backup/

# 方式 C: 通过跳板机传输
scp -J jumpserver@jump.example.com database-backups/new-api-backup-*.dump.gz user@production-server:/backup/
```

### 步骤 3: 在生产服务器上导入数据

```bash
# 1. 登录到生产服务器
ssh user@production-server

# 2. 进入备份目录
cd /backup/database-backups

# 3. 解压备份文件
gunzip new-api-backup-*.dump.gz

# 4. 编辑导入脚本，设置生产环境数据库密码
vim import-to-production-*.sh
# 找到 PROD_DB_PASSWORD="" 这一行，设置你的生产环境密码
# PROD_DB_PASSWORD="your_production_password"

# 5. 执行导入脚本
bash import-to-production-*.sh
# 按照提示输入 YES 确认导入

# 6. 验证数据导入成功后，重启应用
systemctl restart new-api
# 或 Docker 环境
docker-compose restart new-api
```

## 📁 生成的文件说明

导出脚本会在 `database-backups/` 目录生成以下文件：

| 文件 | 说明 | 用途 |
|------|------|------|
| `new-api-backup-YYYYMMDD_HHMMSS.dump` | 二进制格式备份 | 推荐用于生产环境导入 |
| `new-api-backup-YYYYMMDD_HHMMSS.dump.gz` | 压缩的二进制备份 | 推荐用于传输（文件更小） |
| `new-api-backup-YYYYMMDD_HHMMSS.sql` | SQL 文本格式备份 | 备用方案，便于查看 |
| `new-api-backup-YYYYMMDD_HHMMSS.sql.gz` | 压缩的 SQL 备份 | 备用 |
| `database-stats-YYYYMMDD_HHMMSS.txt` | 数据库统计信息 | 用于验证数据完整性 |
| `import-to-production-YYYYMMDD_HHMMSS.sh` | 生产环境导入脚本 | 在生产环境执行 |

## 🔧 手动导入方式（高级）

如果导入脚本无法使用，可以手动导入：

### 方式 A: 使用 pg_restore（推荐）

```bash
# 解压备份文件
gunzip new-api-backup-*.dump.gz

# 导入到生产数据库
PGPASSWORD=your_password pg_restore \
    -h localhost \
    -U root \
    -d new-api \
    -v \
    -c \
    --if-exists \
    new-api-backup-*.dump
```

### 方式 B: 使用 psql（SQL 格式）

```bash
# 解压 SQL 备份
gunzip new-api-backup-*.sql.gz

# 导入到生产数据库
PGPASSWORD=your_password psql \
    -h localhost \
    -U root \
    -d new-api \
    -f new-api-backup-*.sql
```

### 方式 C: Docker 环境导入

```bash
# 如果生产环境也使用 Docker

# 1. 复制备份文件到容器
docker cp new-api-backup-*.dump postgres:/tmp/

# 2. 在容器内导入
docker exec postgres pg_restore \
    -U root \
    -d new-api \
    -v \
    -c \
    --if-exists \
    /tmp/new-api-backup-*.dump

# 3. 重启应用容器
docker-compose restart new-api
```

## ✅ 验证导入结果

在生产服务器上执行以下命令验证：

```bash
# 1. 测试数据库连接
PGPASSWORD=your_password psql -U root -d new-api -c "SELECT version();"

# 2. 检查关键表数据
PGPASSWORD=your_password psql -U root -d new-api << EOF
SELECT 'Users: ' || COUNT(*) FROM users;
SELECT 'Tokens: ' || COUNT(*) FROM tokens;
SELECT 'Channels: ' || COUNT(*) FROM channels;
SELECT 'Redemptions: ' || COUNT(*) FROM redemptions;
EOF

# 3. 检查管理员账号
PGPASSWORD=your_password psql -U root -d new-api -c \
    "SELECT username, role, status FROM users WHERE role = 100;"

# 4. 测试应用 API
curl http://localhost:3000/api/status

# 5. 查看应用日志
tail -f /var/log/new-api/error.log
# 或 Docker
docker logs -f new-api
```

## 🔐 生产环境配置

### 1. 更新环境变量

编辑 `.env` 文件或 `docker-compose.yml`：

```bash
# 生产环境数据库连接字符串
SQL_DSN=postgresql://root:your_strong_password@localhost:5432/new-api

# 如果是 Docker 环境
SQL_DSN=postgresql://root:your_strong_password@postgres:5432/new-api
```

### 2. 修改默认密码（重要！）

```bash
# 登录 PostgreSQL
psql -U postgres

# 修改密码
ALTER USER root WITH PASSWORD 'your_new_strong_password';

# 退出
\q
```

### 3. 配置数据库备份策略

```bash
# 添加定时备份任务
crontab -e

# 每天凌晨 2 点备份
0 2 * * * PGPASSWORD=your_password pg_dump -U root -d new-api -F c -f /backup/new-api-$(date +\%Y\%m\%d).dump

# 保留最近 7 天的备份
0 3 * * * find /backup -name "new-api-*.dump" -mtime +7 -delete
```

## ⚠️ 注意事项

1. **数据安全**
   - ✅ 迁移前务必备份生产环境现有数据
   - ✅ 验证备份文件完整性
   - ✅ 在低流量时段进行迁移

2. **密码安全**
   - ⚠️ 务必修改默认密码 `123456`
   - ⚠️ 使用强密码（建议 16 位以上，包含大小写字母、数字、特殊字符）
   - ⚠️ 不要在脚本中硬编码密码（使用环境变量）

3. **网络安全**
   - ⚠️ 不要将数据库端口暴露到公网
   - ⚠️ 使用防火墙限制访问
   - ⚠️ 启用 SSL 连接（生产环境推荐）

4. **应用配置**
   - ✅ 确保应用的数据库连接字符串正确
   - ✅ 迁移后重启应用服务
   - ✅ 检查应用日志确认无错误

## 🆘 故障排查

### 问题 1: Docker 容器未运行

```bash
# 启动容器
docker-compose up -d

# 检查状态
docker ps | grep postgres
```

### 问题 2: 权限错误

```bash
# 确保脚本有执行权限
chmod +x scripts/export-database.sh

# 确保备份目录可写
chmod 755 database-backups
```

### 问题 3: 导入时提示数据库不存在

```bash
# 创建数据库
psql -U postgres -c "CREATE DATABASE \"new-api\";"

# 创建用户并授权
psql -U postgres << EOF
CREATE USER root WITH PASSWORD 'your_password';
GRANT ALL PRIVILEGES ON DATABASE "new-api" TO root;
EOF
```

### 问题 4: 导入时出现冲突

```bash
# 清空目标数据库后再导入
psql -U root -d new-api -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"

# 然后重新导入
pg_restore -U root -d new-api new-api-backup-*.dump
```

## 📞 获取帮助

如果遇到问题：
1. 查看 `database-stats-*.txt` 文件对比数据
2. 检查应用日志 `/var/log/new-api/error.log`
3. 查看 Docker 日志 `docker logs postgres`
4. 参考项目文档 `data/DB_DDL/MIGRATION_PLAYBOOK.md`

## 🔗 相关链接

- [PostgreSQL 官方文档](https://www.postgresql.org/docs/)
- [Docker 官方文档](https://docs.docker.com/)
- [项目 GitHub](https://github.com/Calcium-Ion/new-api)
