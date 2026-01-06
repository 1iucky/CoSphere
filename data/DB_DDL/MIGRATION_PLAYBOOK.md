# 订阅系统数据库迁移执行手册

**版本**: v1.0
**日期**: 2025-12-04
**适用环境**: 生产环境 / 测试环境
**预计耗时**: 5-15 分钟（取决于数据量）
**风险等级**: ⚠️ 中等（涉及表结构变更和字段扩展）

---

## 📋 目录

1. [准备工作](#准备工作)
2. [迁移前检查清单](#迁移前检查清单)
3. [迁移执行步骤](#迁移执行步骤)
4. [验证步骤](#验证步骤)
5. [应急回滚方案](#应急回滚方案)
6. [常见问题处理](#常见问题处理)
7. [联系人信息](#联系人信息)

---

## 准备工作

### 1.1 时间窗口选择

**建议迁移时间**：
- ✅ 低流量时段（凌晨 2:00-4:00）
- ✅ 非工作日（周六/周日）
- ✅ 重大节假日前避免

**所需时间**：
- 备份数据库：1-3 分钟
- 执行迁移：2-5 分钟
- 验证测试：2-5 分钟
- 总计：5-15 分钟

### 1.2 人员准备

**必须在线人员**：
- DBA（数据库管理员）- 负责执行迁移
- 后端开发负责人 - 负责服务重启和监控
- 运维工程师 - 负责监控系统状态

**可选在线人员**：
- 前端开发负责人 - 处理可能的前端问题
- 产品经理 - 协调沟通

### 1.3 工具准备

**必备工具**：
```bash
# PostgreSQL 环境
psql --version        # 确认 psql 客户端已安装
pg_dump --version     # 确认备份工具已安装

# MySQL 环境
mysql --version       # 确认 mysql 客户端已安装
mysqldump --version   # 确认备份工具已安装

# Redis 工具
redis-cli --version   # 确认 Redis 客户端已安装

# 监控工具
htop                  # 系统资源监控
tail -f /var/log/new-api/error.log  # 日志监控
```

### 1.4 文件准备

**迁移脚本**：
- `subscription_migration_postgresql.sql` (PostgreSQL)
- `subscription_migration_mysql.sql` (MySQL)
- `subscription_migration_verification.sql` (PostgreSQL 验证)
- `subscription_migration_verification_mysql.sql` (MySQL 验证)
> ⚠️ 当前版本未提供 SQLite 迁移脚本，SQLite 环境不支持本次迁移。

**文档**：
- 本执行手册
- README.md（迁移指南）

---

## 迁移前检查清单

### 2.1 环境检查

| 检查项 | 命令 | 期望结果 | 状态 |
|-------|------|---------|------|
| 数据库服务运行 | `systemctl status postgresql`<br>`systemctl status mysql` | active (running) | [ ] |
| Redis 服务运行 | `systemctl status redis` | active (running) | [ ] |
| 应用服务运行 | `systemctl status new-api` | active (running) | [ ] |
| 磁盘空间充足 | `df -h` | 可用空间 > 10GB | [ ] |
| 数据库连接正常 | `psql -U root -d new-api -c "SELECT 1;"`<br>`mysql -u root -p -e "SELECT 1;"` | 返回 1 | [ ] |

### 2.2 数据库备份检查

| 检查项 | 命令 | 期望结果 | 状态 |
|-------|------|---------|------|
| 备份目录存在 | `ls -ld /backup` | 目录存在且可写 | [ ] |
| 备份脚本测试 | `pg_dump --help`<br>`mysqldump --help` | 显示帮助信息 | [ ] |
| 历史备份可用 | `ls -lh /backup/*.sql` | 至少有 3 个历史备份 | [ ] |

### 2.3 用户通知

| 任务 | 负责人 | 完成时间 | 状态 |
|------|-------|---------|------|
| 发送维护通知邮件 | 产品经理 | 迁移前 24 小时 | [ ] |
| 更新系统公告 | 运维工程师 | 迁移前 2 小时 | [ ] |
| 发送即将开始通知 | 产品经理 | 迁移前 15 分钟 | [ ] |

---

## 迁移执行步骤

### 3.1 PostgreSQL 迁移流程

#### Step 1: 停止应用服务

```bash
# 停止 new-api 服务
sudo systemctl stop new-api

# 确认服务已停止
sudo systemctl status new-api
# 期望输出: inactive (dead)
```

**检查点**: ✅ 服务已停止

---

#### Step 2: 备份数据库

```bash
# 创建备份目录
sudo mkdir -p /backup/new-api
cd /backup/new-api

# 执行完整备份
PGPASSWORD=your-password pg_dump \
    -h localhost \
    -U root \
    -d new-api \
    -F c \
    -f new-api-backup-$(date +%Y%m%d_%H%M%S).dump

# 或者使用 SQL 格式备份
PGPASSWORD=your-password pg_dump \
    -h localhost \
    -U root \
    -d new-api \
    > new-api-backup-$(date +%Y%m%d_%H%M%S).sql

# 验证备份文件
ls -lh new-api-backup-*.dump
# 期望输出: 文件大小 > 0，且符合预期
```

**检查点**: ✅ 备份文件已创建且大小正常

---

#### Step 3: 执行迁移脚本

```bash
# 进入迁移脚本目录
cd /path/to/CoSphere/data/DB_DDL

# 记录开始时间
echo "迁移开始时间: $(date)" >> /tmp/migration.log

# 执行迁移脚本
PGPASSWORD=your-password psql \
    -h localhost \
    -U root \
    -d new-api \
    -f subscription_migration_postgresql.sql \
    2>&1 | tee -a /tmp/migration.log

# 记录结束时间
echo "迁移结束时间: $(date)" >> /tmp/migration.log

# 检查执行结果
echo "退出码: $?" >> /tmp/migration.log
```

**检查点**: ✅ 迁移脚本执行成功，无错误输出

---

#### Step 4: 执行验证脚本

```bash
# 执行验证脚本
PGPASSWORD=your-password psql \
    -h localhost \
    -U root \
    -d new-api \
    -f subscription_migration_verification.sql \
    2>&1 | tee /tmp/verification.log

# 检查验证结果
grep "✗" /tmp/verification.log
# 期望输出: 无内容（表示所有检查通过）
```

**检查点**: ✅ 所有验证检查通过

---

#### Step 5: 清理 Redis 缓存

```bash
# 清理 Redis 缓存（因为 tokens 字段扩展）
redis-cli FLUSHDB

# 确认清理成功
redis-cli DBSIZE
# 期望输出: (integer) 0
```

**检查点**: ✅ Redis 缓存已清空

---

#### Step 6: 重启应用服务

```bash
# 启动 new-api 服务
sudo systemctl start new-api

# 等待服务启动
sleep 5

# 检查服务状态
sudo systemctl status new-api
# 期望输出: active (running)

# 检查日志
tail -n 50 /var/log/new-api/error.log
# 期望输出: 无严重错误
```

**检查点**: ✅ 服务正常启动，无错误日志

---

#### Step 7: 冒烟测试

```bash
# 测试健康检查接口
curl http://localhost:3000/api/status
# 期望输出: {"success":true,...}

# 测试登录接口
curl -X POST http://localhost:3000/api/user/login \
    -H "Content-Type: application/json" \
    -d '{"username":"test","password":"test123"}'
# 期望输出: 返回 token 或明确的错误信息

# 测试 tokens 列表接口
curl http://localhost:3000/api/token \
    -H "Authorization: Bearer <token>"
# 期望输出: 返回 token 列表（包含 subscription_preferred 与 auto_smart_group 字段）
```

**检查点**: ✅ 所有接口响应正常

---

### 3.2 MySQL 迁移流程

#### Step 1: 停止应用服务

```bash
sudo systemctl stop new-api
sudo systemctl status new-api
```

---

#### Step 2: 备份数据库

```bash
sudo mkdir -p /backup/new-api
cd /backup/new-api

mysqldump \
    -h localhost \
    -u root \
    -p \
    --single-transaction \
    --routines \
    --triggers \
    new-api \
    > new-api-backup-$(date +%Y%m%d_%H%M%S).sql

ls -lh new-api-backup-*.sql
```

---

#### Step 3: 执行迁移脚本

```bash
cd /path/to/CoSphere/data/DB_DDL

echo "迁移开始时间: $(date)" >> /tmp/migration.log

mysql \
    -h localhost \
    -u root \
    -p \
    new-api \
    < subscription_migration_mysql.sql \
    2>&1 | tee -a /tmp/migration.log

echo "迁移结束时间: $(date)" >> /tmp/migration.log
echo "退出码: $?" >> /tmp/migration.log
```

---

#### Step 4: 执行验证脚本

```bash
mysql \
    -h localhost \
    -u root \
    -p \
    new-api \
    < subscription_migration_verification_mysql.sql \
    2>&1 | tee /tmp/verification.log

grep "✗" /tmp/verification.log
```

---

#### Step 5-7: 同 PostgreSQL 流程

清理 Redis → 重启服务 → 冒烟测试

---

## 验证步骤

### 4.1 数据库层验证

**PostgreSQL**:
```sql
-- 验证新表
SELECT table_name FROM information_schema.tables
WHERE table_schema = 'public'
  AND table_name LIKE 'subscription%';
-- 期望输出: 6 张表

-- 验证字段扩展
SELECT column_name FROM information_schema.columns
WHERE table_name = 'tokens'
  AND column_name IN ('subscription_preferred', 'auto_smart_group');
-- 期望输出: 两个字段均存在

-- 验证配置项
SELECT key, value FROM options WHERE key LIKE 'SUBSCRIPTION_%';
-- 期望输出: 5 个配置项
```

**MySQL**:
```sql
-- 验证新表
SELECT TABLE_NAME FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME LIKE 'subscription%';

-- 验证字段扩展
SELECT COLUMN_NAME FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'tokens'
  AND COLUMN_NAME IN ('subscription_preferred', 'auto_smart_group');

-- 验证配置项
SELECT `key`, value FROM options WHERE `key` LIKE 'SUBSCRIPTION_%';
```

### 4.2 应用层验证

**API 测试**:
```bash
# 1. 健康检查
curl http://localhost:3000/api/status
# 期望: {"success":true}

# 2. 登录测试
curl -X POST http://localhost:3000/api/user/login \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin123"}'
# 期望: 返回 token

# 3. Token 列表测试（验证字段扩展）
curl http://localhost:3000/api/token \
    -H "Authorization: Bearer <token>"
# 期望: 返回列表，包含 subscription_preferred 与 auto_smart_group 字段

# 4. 用户设置测试
curl http://localhost:3000/api/user/setting \
    -H "Authorization: Bearer <token>"
# 期望: 返回包含 auto_wallet_fallback 的设置
```

### 4.3 监控指标检查

**系统资源**:
```bash
# CPU 使用率
top -n 1 | grep "Cpu(s)"
# 期望: < 80%

# 内存使用率
free -h
# 期望: available > 1GB

# 磁盘 I/O
iostat -x 1 3
# 期望: %util < 80%
```

**数据库连接**:
```sql
-- PostgreSQL
SELECT count(*) FROM pg_stat_activity;
-- 期望: < max_connections * 0.8

-- MySQL
SHOW PROCESSLIST;
-- 期望: 连接数正常
```

---

## 应急回滚方案

### 5.1 完全回滚（推荐）

**适用场景**: 迁移失败、数据异常、服务无法启动

**PostgreSQL 回滚步骤**:

```bash
# 1. 停止服务
sudo systemctl stop new-api

# 2. 恢复数据库
cd /backup/new-api

# 方法 A: 从 dump 文件恢复
PGPASSWORD=your-password pg_restore \
    -h localhost \
    -U root \
    -d new-api \
    -c \
    --if-exists \
    new-api-backup-YYYYMMDD_HHMMSS.dump

# 方法 B: 从 SQL 文件恢复
PGPASSWORD=your-password psql \
    -h localhost \
    -U root \
    -d new-api \
    < new-api-backup-YYYYMMDD_HHMMSS.sql

# 3. 清理 Redis
redis-cli FLUSHDB

# 4. 重启服务
sudo systemctl start new-api

# 5. 验证
curl http://localhost:3000/api/status
```

**MySQL 回滚步骤**:

```bash
# 1. 停止服务
sudo systemctl stop new-api

# 2. 恢复数据库
cd /backup/new-api

mysql \
    -h localhost \
    -u root \
    -p \
    new-api \
    < new-api-backup-YYYYMMDD_HHMMSS.sql

# 3. 清理 Redis
redis-cli FLUSHDB

# 4. 重启服务
sudo systemctl start new-api

# 5. 验证
curl http://localhost:3000/api/status
```

**检查点**: ✅ 服务恢复正常，功能可用

---

### 5.2 部分回滚（Phase 3-4）

**适用场景**: Phase 1-2（建表）成功，但 Phase 3-4（字段扩展/数据迁移）失败

**PostgreSQL 部分回滚**:

```sql
-- 回滚 tokens 字段扩展
ALTER TABLE tokens DROP COLUMN IF EXISTS subscription_preferred;

-- 回滚 redemptions 字段
ALTER TABLE redemptions DROP COLUMN IF EXISTS type;
ALTER TABLE redemptions DROP COLUMN IF EXISTS payload;

-- 回滚 options 配置
DELETE FROM options WHERE key LIKE 'SUBSCRIPTION_%';

-- 回滚 users.setting（移除新增键）
UPDATE users
SET setting = (
    SELECT jsonb_object_agg(key, value)
    FROM jsonb_each(setting::jsonb)
    WHERE key NOT IN ('auto_wallet_fallback', 'notification_preferences', 'language', 'timezone')
)::text
WHERE setting::jsonb ?| ARRAY['auto_wallet_fallback', 'notification_preferences', 'language', 'timezone'];
```

**MySQL 部分回滚**:

```sql
-- 回滚 tokens 字段扩展
ALTER TABLE tokens DROP COLUMN subscription_preferred;

-- 回滚 redemptions 字段
ALTER TABLE redemptions DROP COLUMN IF EXISTS type;
ALTER TABLE redemptions DROP COLUMN IF EXISTS payload;

-- 回滚 options 配置
DELETE FROM options WHERE `key` LIKE 'SUBSCRIPTION_%';

-- 回滚 users.setting 较复杂，建议从备份恢复
```

---

## 常见问题处理

### 6.1 迁移过程中断

**症状**: 脚本执行中途退出

**原因**: 网络中断、数据库连接超时、磁盘空间不足

**处理**:
1. 检查错误日志: `tail -100 /tmp/migration.log`
2. 检查数据库状态: `psql -c "\d"`
3. 执行完全回滚
4. 排查根本原因后重新执行

---

### 6.2 验证脚本报错

**症状**: 验证脚本显示 ✗ 失败

**处理**:
1. 查看具体失败项: `grep "✗" /tmp/verification.log`
2. 手动检查数据库结构: `\d tablename`
3. 如果是关键失败（表缺失、字段缺失），执行完全回滚
4. 如果是非关键失败（索引缺失），可手动修复

---

### 6.3 服务无法启动

**症状**: `systemctl start new-api` 失败

**原因**: 数据库连接失败、字段名不匹配、配置错误

**处理**:
1. 查看服务日志: `journalctl -u new-api -n 100`
2. 查看应用日志: `tail -100 /var/log/new-api/error.log`
3. 检查数据库连接: `psql -c "SELECT 1;"`
4. 如果是字段名问题，执行部分回滚
5. 如果无法快速修复，执行完全回滚

---

### 6.4 前端报错 404/500

**症状**: 前端界面显示错误

**原因**: API 字段扩展（新增 `subscription_preferred`，保留 `auto_smart_group`）

**处理**:
1. 检查前端代码是否使用旧字段名
2. 清理浏览器缓存
3. 检查 Redis 是否已清空
4. 如果前端代码未更新，执行部分回滚恢复字段名

---

### 6.5 Redis 缓存未清理

**症状**: Token 数据显示异常

**原因**: 忘记执行 `redis-cli FLUSHDB`

**处理**:
```bash
redis-cli FLUSHDB
sudo systemctl restart new-api
```

---

## 联系人信息

| 角色 | 姓名 | 手机 | 邮箱 | 备注 |
|------|------|------|------|------|
| DBA | 待填写 | - | - | 数据库操作主责 |
| 后端负责人 | 待填写 | - | - | 服务重启和监控 |
| 运维工程师 | 待填写 | - | - | 系统监控 |
| 产品经理 | 待填写 | - | - | 用户沟通 |

---

## 附录

### A. 迁移时间估算

| 数据量 | 备份时间 | 迁移时间 | 验证时间 | 总计 |
|-------|---------|---------|---------|------|
| < 10万用户 | 1 分钟 | 2 分钟 | 2 分钟 | ~5 分钟 |
| 10-50万用户 | 2 分钟 | 3 分钟 | 3 分钟 | ~8 分钟 |
| 50-100万用户 | 3 分钟 | 5 分钟 | 5 分钟 | ~13 分钟 |
| > 100万用户 | 5 分钟 | 10 分钟 | 5 分钟 | ~20 分钟 |

### B. 关键 SQL 命令速查

**检查表是否存在**:
```sql
-- PostgreSQL
\dt subscription*

-- MySQL
SHOW TABLES LIKE 'subscription%';
```

**检查字段是否存在**:
```sql
-- PostgreSQL
\d tokens

-- MySQL
DESC tokens;
```

**检查索引**:
```sql
-- PostgreSQL
\di

-- MySQL
SHOW INDEX FROM subscription_plans;
```

---

**文档版本历史**:
- v1.0 (2025-12-04): 初始版本

**审核状态**: ⏳ 待审核

---
