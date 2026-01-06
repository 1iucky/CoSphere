#!/bin/bash

###############################################################################
# CoSphere 数据库导出脚本
# 用途: 从本地 Docker PostgreSQL 容器导出数据库，准备迁移到生产环境
# 日期: $(date +%Y-%m-%d)
###############################################################################

set -e  # 遇到错误立即退出

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 配置信息
CONTAINER_NAME="postgres"
DB_USER="root"
DB_PASSWORD="123456"
DB_NAME="new-api"
BACKUP_DIR="./database-backups"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}CoSphere 数据库导出工具${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# 1. 检查 Docker 容器是否运行
echo -e "${YELLOW}[步骤 1/6] 检查 Docker 容器状态...${NC}"
if ! docker ps | grep -q "$CONTAINER_NAME"; then
    echo -e "${RED}错误: Docker 容器 '$CONTAINER_NAME' 未运行！${NC}"
    echo "请先启动容器: docker-compose up -d"
    exit 1
fi
echo -e "${GREEN}✓ Docker 容器运行正常${NC}"
echo ""

# 2. 创建备份目录
echo -e "${YELLOW}[步骤 2/6] 创建备份目录...${NC}"
mkdir -p "$BACKUP_DIR"
echo -e "${GREEN}✓ 备份目录已创建: $BACKUP_DIR${NC}"
echo ""

# 3. 导出数据库（自定义二进制格式）
echo -e "${YELLOW}[步骤 3/6] 导出数据库（二进制格式，推荐）...${NC}"
DUMP_FILE="$BACKUP_DIR/new-api-backup-${TIMESTAMP}.dump"
docker exec "$CONTAINER_NAME" pg_dump \
    -U "$DB_USER" \
    -d "$DB_NAME" \
    -F c \
    -v \
    > "$DUMP_FILE"

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ 二进制格式导出成功: $DUMP_FILE${NC}"
    DUMP_SIZE=$(du -h "$DUMP_FILE" | cut -f1)
    echo -e "${GREEN}  文件大小: $DUMP_SIZE${NC}"
else
    echo -e "${RED}✗ 二进制格式导出失败${NC}"
    exit 1
fi
echo ""

# 4. 导出数据库（SQL 文本格式，备用）
echo -e "${YELLOW}[步骤 4/6] 导出数据库（SQL 格式，备用）...${NC}"
SQL_FILE="$BACKUP_DIR/new-api-backup-${TIMESTAMP}.sql"
docker exec "$CONTAINER_NAME" pg_dump \
    -U "$DB_USER" \
    -d "$DB_NAME" \
    -v \
    > "$SQL_FILE"

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ SQL 格式导出成功: $SQL_FILE${NC}"
    SQL_SIZE=$(du -h "$SQL_FILE" | cut -f1)
    echo -e "${GREEN}  文件大小: $SQL_SIZE${NC}"
else
    echo -e "${RED}✗ SQL 格式导出失败${NC}"
    exit 1
fi
echo ""

# 5. 压缩备份文件
echo -e "${YELLOW}[步骤 5/6] 压缩备份文件...${NC}"
gzip -k "$DUMP_FILE"  # -k 保留原文件
gzip -k "$SQL_FILE"

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ 压缩完成${NC}"
    echo -e "${GREEN}  - ${DUMP_FILE}.gz ($(du -h ${DUMP_FILE}.gz | cut -f1))${NC}"
    echo -e "${GREEN}  - ${SQL_FILE}.gz ($(du -h ${SQL_FILE}.gz | cut -f1))${NC}"
else
    echo -e "${YELLOW}⚠ 压缩失败，但原始文件已保存${NC}"
fi
echo ""

# 6. 生成数据库统计信息
echo -e "${YELLOW}[步骤 6/6] 生成数据库统计信息...${NC}"
STATS_FILE="$BACKUP_DIR/database-stats-${TIMESTAMP}.txt"

cat > "$STATS_FILE" << EOF
========================================
CoSphere 数据库导出统计信息
========================================
导出时间: $(date '+%Y-%m-%d %H:%M:%S')
数据库名: $DB_NAME
容器名称: $CONTAINER_NAME

文件信息:
----------------------------------------
二进制格式: $(basename $DUMP_FILE)
文件大小: $(du -h $DUMP_FILE | cut -f1)
压缩文件: $(basename $DUMP_FILE).gz
压缩大小: $(du -h ${DUMP_FILE}.gz | cut -f1)

SQL 格式: $(basename $SQL_FILE)
文件大小: $(du -h $SQL_FILE | cut -f1)
压缩文件: $(basename $SQL_FILE).gz
压缩大小: $(du -h ${SQL_FILE}.gz | cut -f1)

数据库统计:
----------------------------------------
EOF

# 获取表数量和记录数
docker exec "$CONTAINER_NAME" psql -U "$DB_USER" -d "$DB_NAME" -t -c "
SELECT
    'Tables: ' || COUNT(*)
FROM information_schema.tables
WHERE table_schema = 'public' AND table_type = 'BASE TABLE';
" >> "$STATS_FILE"

# 获取关键表的记录数
docker exec "$CONTAINER_NAME" psql -U "$DB_USER" -d "$DB_NAME" -t -c "
SELECT
    table_name || ': ' ||
    (xpath('/row/count/text()', xml_count))[1]::text::int AS count
FROM (
    SELECT
        table_name,
        table_schema,
        query_to_xml(
            format('SELECT COUNT(*) FROM %I.%I', table_schema, table_name),
            false, true, ''
        ) AS xml_count
    FROM information_schema.tables
    WHERE table_schema = 'public'
        AND table_type = 'BASE TABLE'
        AND table_name IN ('users', 'tokens', 'channels', 'redemptions', 'logs', 'abilities')
) t
ORDER BY table_name;
" >> "$STATS_FILE"

echo -e "${GREEN}✓ 统计信息已保存: $STATS_FILE${NC}"
echo ""

# 显示统计信息
cat "$STATS_FILE"
echo ""

# 7. 生成导入脚本（生产环境使用）
echo -e "${YELLOW}生成生产环境导入脚本...${NC}"
IMPORT_SCRIPT="$BACKUP_DIR/import-to-production-${TIMESTAMP}.sh"

cat > "$IMPORT_SCRIPT" << 'EOF'
#!/bin/bash

###############################################################################
# CoSphere 数据库导入脚本（生产环境）
# ⚠️ 警告: 此脚本会清空现有数据库并导入新数据，请谨慎操作！
###############################################################################

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}CoSphere 数据库导入工具（生产环境）${NC}"
echo -e "${YELLOW}========================================${NC}"
echo ""

# 配置信息（请根据生产环境修改）
PROD_DB_HOST="localhost"
PROD_DB_PORT="5432"
PROD_DB_USER="root"
PROD_DB_NAME="new-api"
PROD_DB_PASSWORD=""  # 请设置生产环境密码

# 检查密码是否已设置
if [ -z "$PROD_DB_PASSWORD" ]; then
    echo -e "${RED}错误: 请先在脚本中设置 PROD_DB_PASSWORD 变量${NC}"
    echo "编辑此脚本，设置生产环境数据库密码"
    exit 1
fi

# 查找最新的备份文件
DUMP_FILE=$(ls -t new-api-backup-*.dump 2>/dev/null | head -1)
if [ -z "$DUMP_FILE" ]; then
    echo -e "${RED}错误: 未找到备份文件${NC}"
    exit 1
fi

echo -e "${GREEN}找到备份文件: $DUMP_FILE${NC}"
echo ""

# 确认操作
echo -e "${RED}⚠️  警告: 此操作将清空现有数据库并导入新数据！${NC}"
echo -e "${YELLOW}目标数据库: $PROD_DB_HOST:$PROD_DB_PORT/$PROD_DB_NAME${NC}"
echo ""
read -p "确认继续？(输入 YES 继续): " CONFIRM

if [ "$CONFIRM" != "YES" ]; then
    echo -e "${YELLOW}操作已取消${NC}"
    exit 0
fi

# 备份生产数据库（如果存在数据）
echo -e "${YELLOW}[步骤 1/4] 备份生产环境现有数据...${NC}"
BACKUP_FILE="production-backup-before-import-$(date +%Y%m%d_%H%M%S).dump"
PGPASSWORD="$PROD_DB_PASSWORD" pg_dump \
    -h "$PROD_DB_HOST" \
    -p "$PROD_DB_PORT" \
    -U "$PROD_DB_USER" \
    -d "$PROD_DB_NAME" \
    -F c \
    -f "$BACKUP_FILE" 2>/dev/null || echo "跳过备份（数据库可能为空）"
echo -e "${GREEN}✓ 备份完成（如果有数据）${NC}"
echo ""

# 导入数据
echo -e "${YELLOW}[步骤 2/4] 导入数据到生产环境...${NC}"
PGPASSWORD="$PROD_DB_PASSWORD" pg_restore \
    -h "$PROD_DB_HOST" \
    -p "$PROD_DB_PORT" \
    -U "$PROD_DB_USER" \
    -d "$PROD_DB_NAME" \
    -v \
    -c \
    --if-exists \
    "$DUMP_FILE"

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ 数据导入成功${NC}"
else
    echo -e "${RED}✗ 数据导入失败${NC}"
    exit 1
fi
echo ""

# 验证导入
echo -e "${YELLOW}[步骤 3/4] 验证数据导入...${NC}"
PGPASSWORD="$PROD_DB_PASSWORD" psql \
    -h "$PROD_DB_HOST" \
    -p "$PROD_DB_PORT" \
    -U "$PROD_DB_USER" \
    -d "$PROD_DB_NAME" \
    -c "SELECT COUNT(*) as user_count FROM users;" \
    -c "SELECT COUNT(*) as token_count FROM tokens;" \
    -c "SELECT COUNT(*) as channel_count FROM channels;"

echo -e "${GREEN}✓ 验证完成${NC}"
echo ""

# 提示后续步骤
echo -e "${YELLOW}[步骤 4/4] 后续步骤${NC}"
echo "1. 检查应用配置文件中的数据库连接字符串"
echo "2. 重启应用服务: systemctl restart new-api 或 docker-compose restart"
echo "3. 访问应用前端测试功能"
echo "4. 检查日志: tail -f /var/log/new-api/error.log"
echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}数据库导入完成！${NC}"
echo -e "${GREEN}========================================${NC}"
EOF

chmod +x "$IMPORT_SCRIPT"
echo -e "${GREEN}✓ 导入脚本已生成: $IMPORT_SCRIPT${NC}"
echo ""

# 完成总结
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}导出完成！${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${YELLOW}备份文件位置:${NC}"
echo "  📁 目录: $BACKUP_DIR/"
echo "  📦 二进制备份: $(basename $DUMP_FILE)"
echo "  📦 压缩备份: $(basename $DUMP_FILE).gz (推荐用于传输)"
echo "  📝 SQL 备份: $(basename $SQL_FILE)"
echo "  📊 统计信息: $(basename $STATS_FILE)"
echo "  🚀 导入脚本: $(basename $IMPORT_SCRIPT)"
echo ""
echo -e "${YELLOW}下一步操作:${NC}"
echo "  1. 将备份文件传输到生产服务器:"
echo "     ${GREEN}scp ${DUMP_FILE}.gz user@production-server:/backup/${NC}"
echo ""
echo "  2. 或者传输整个备份目录:"
echo "     ${GREEN}scp -r $BACKUP_DIR user@production-server:/backup/${NC}"
echo ""
echo "  3. 在生产服务器上解压并导入:"
echo "     ${GREEN}gunzip $(basename $DUMP_FILE).gz${NC}"
echo "     ${GREEN}bash $(basename $IMPORT_SCRIPT)${NC}"
echo ""
echo -e "${YELLOW}⚠️  重要提示:${NC}"
echo "  - 导入前请务必在生产环境做好数据备份"
echo "  - 导入脚本中需要设置生产环境数据库密码"
echo "  - 建议在低流量时段进行迁移操作"
echo ""
