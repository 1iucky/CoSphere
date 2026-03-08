#!/bin/bash
# 订阅系统灰度发布控制脚本
# 用法: ./subscription-grayscale.sh <command> [options]
#
# 命令:
#   enable <percentage>  - 开启灰度到指定百分比 (1-100)
#   rollout              - 按阶段逐步放量 (1% -> 10% -> 50% -> 100%)
#   disable              - 关闭灰度并禁用订阅系统
#   full-enable          - 完全启用订阅系统（跳过灰度）
#   status               - 查看当前灰度状态
#
# 环境变量:
#   DATABASE_URL         - PostgreSQL 数据库连接字符串
#   REDIS_URL            - Redis 连接字符串（可选，用于刷新缓存）

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 检查必需的环境变量
check_env() {
    if [ -z "$DATABASE_URL" ]; then
        echo -e "${RED}错误: 请设置 DATABASE_URL 环境变量${NC}"
        echo "示例: export DATABASE_URL='postgres://user:pass@localhost:5432/dbname'"
        exit 1
    fi
}

# 执行数据库更新
update_option() {
    local key=$1
    local value=$2

    echo -e "${BLUE}更新配置: $key = $value${NC}"

    psql "$DATABASE_URL" -c "
        INSERT INTO options (key, value) VALUES ('$key', '$value')
        ON CONFLICT (key) DO UPDATE SET value = '$value';
    " > /dev/null 2>&1

    if [ $? -eq 0 ]; then
        echo -e "${GREEN}配置更新成功${NC}"
    else
        echo -e "${RED}配置更新失败${NC}"
        exit 1
    fi
}

# 获取当前配置值
get_option() {
    local key=$1
    psql "$DATABASE_URL" -t -c "SELECT value FROM options WHERE key = '$key';" 2>/dev/null | xargs
}

# 刷新缓存（如果 Redis 可用）
refresh_cache() {
    if [ -n "$REDIS_URL" ]; then
        echo -e "${BLUE}刷新 Redis 缓存...${NC}"
        # 删除订阅相关缓存键
        redis-cli -u "$REDIS_URL" KEYS "subscription:*" | xargs -r redis-cli -u "$REDIS_URL" DEL > /dev/null 2>&1 || true
        echo -e "${GREEN}缓存刷新完成${NC}"
    else
        echo -e "${YELLOW}提示: 未设置 REDIS_URL，请手动重启应用以刷新配置缓存${NC}"
    fi
}

# 显示当前状态
show_status() {
    echo -e "${BLUE}=== 订阅系统灰度状态 ===${NC}"

    local v2_enabled=$(get_option "SUBSCRIPTION_V2_ENABLED")
    local grayscale_mode=$(get_option "SUBSCRIPTION_GRAYSCALE_MODE")
    local grayscale_threshold=$(get_option "SUBSCRIPTION_GRAYSCALE_THRESHOLD")

    echo "全局开关 (V2_ENABLED): ${v2_enabled:-false}"
    echo "灰度模式 (GRAYSCALE_MODE): ${grayscale_mode:-off}"
    echo "灰度阈值 (GRAYSCALE_THRESHOLD): ${grayscale_threshold:-0}%"

    echo ""
    if [ "$v2_enabled" = "true" ]; then
        echo -e "${GREEN}状态: 订阅系统已全局启用${NC}"
    elif [ "$grayscale_mode" = "user_id" ] && [ "$grayscale_threshold" -gt 0 ] 2>/dev/null; then
        echo -e "${YELLOW}状态: 灰度发布中 (${grayscale_threshold}% 用户)${NC}"
    else
        echo -e "${RED}状态: 订阅系统已禁用${NC}"
    fi
}

# 开启灰度
enable_grayscale() {
    local percentage=$1

    if [ -z "$percentage" ] || [ "$percentage" -lt 1 ] || [ "$percentage" -gt 100 ] 2>/dev/null; then
        echo -e "${RED}错误: 请指定有效的百分比 (1-100)${NC}"
        exit 1
    fi

    echo -e "${BLUE}开启订阅系统灰度发布: ${percentage}%${NC}"

    # 确保全局开关关闭（灰度模式下不需要）
    update_option "SUBSCRIPTION_V2_ENABLED" "false"

    # 设置灰度模式为按用户ID
    update_option "SUBSCRIPTION_GRAYSCALE_MODE" "user_id"

    # 设置灰度阈值
    update_option "SUBSCRIPTION_GRAYSCALE_THRESHOLD" "$percentage"

    refresh_cache

    echo ""
    echo -e "${GREEN}灰度发布已开启！${NC}"
    echo "预计影响用户: ${percentage}% (user_id % 100 < ${percentage})"

    show_status
}

# 分阶段放量
rollout() {
    local stages=(1 10 50 100)
    local current_threshold=$(get_option "SUBSCRIPTION_GRAYSCALE_THRESHOLD")
    current_threshold=${current_threshold:-0}

    echo -e "${BLUE}=== 订阅系统分阶段放量 ===${NC}"
    echo "放量阶段: 1% -> 10% -> 50% -> 100%"
    echo "当前阈值: ${current_threshold}%"
    echo ""

    # 找到下一个阶段
    local next_stage=""
    for stage in "${stages[@]}"; do
        if [ "$stage" -gt "$current_threshold" ]; then
            next_stage=$stage
            break
        fi
    done

    if [ -z "$next_stage" ]; then
        echo -e "${YELLOW}已达到最大放量 (100%)${NC}"
        echo "如需完全启用，请执行: $0 full-enable"
        exit 0
    fi

    echo -e "${YELLOW}即将放量到 ${next_stage}%${NC}"
    read -p "确认继续? (y/N): " confirm

    if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
        echo "操作已取消"
        exit 0
    fi

    enable_grayscale "$next_stage"
}

# 关闭灰度并禁用订阅系统
disable_grayscale() {
    echo -e "${YELLOW}警告: 即将关闭订阅系统灰度并禁用订阅功能${NC}"
    read -p "确认继续? (y/N): " confirm

    if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
        echo "操作已取消"
        exit 0
    fi

    echo -e "${BLUE}关闭订阅系统...${NC}"

    # 关闭全局开关
    update_option "SUBSCRIPTION_V2_ENABLED" "false"

    # 关闭灰度模式
    update_option "SUBSCRIPTION_GRAYSCALE_MODE" "off"

    # 重置灰度阈值
    update_option "SUBSCRIPTION_GRAYSCALE_THRESHOLD" "0"

    refresh_cache

    echo ""
    echo -e "${GREEN}订阅系统已禁用${NC}"

    show_status
}

# 完全启用订阅系统
full_enable() {
    echo -e "${YELLOW}警告: 即将完全启用订阅系统（对所有用户生效）${NC}"
    read -p "确认继续? (y/N): " confirm

    if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
        echo "操作已取消"
        exit 0
    fi

    echo -e "${BLUE}完全启用订阅系统...${NC}"

    # 开启全局开关
    update_option "SUBSCRIPTION_V2_ENABLED" "true"

    # 关闭灰度模式（不再需要）
    update_option "SUBSCRIPTION_GRAYSCALE_MODE" "off"

    # 重置灰度阈值
    update_option "SUBSCRIPTION_GRAYSCALE_THRESHOLD" "0"

    refresh_cache

    echo ""
    echo -e "${GREEN}订阅系统已完全启用！${NC}"

    show_status
}

# 显示帮助信息
show_help() {
    echo "订阅系统灰度发布控制脚本"
    echo ""
    echo "用法: $0 <command> [options]"
    echo ""
    echo "命令:"
    echo "  enable <percentage>  - 开启灰度到指定百分比 (1-100)"
    echo "  rollout              - 按阶段逐步放量 (1% -> 10% -> 50% -> 100%)"
    echo "  disable              - 关闭灰度并禁用订阅系统"
    echo "  full-enable          - 完全启用订阅系统（跳过灰度）"
    echo "  status               - 查看当前灰度状态"
    echo "  help                 - 显示此帮助信息"
    echo ""
    echo "环境变量:"
    echo "  DATABASE_URL         - PostgreSQL 数据库连接字符串 (必需)"
    echo "  REDIS_URL            - Redis 连接字符串 (可选，用于刷新缓存)"
    echo ""
    echo "示例:"
    echo "  $0 status                    # 查看当前状态"
    echo "  $0 enable 10                 # 开启 10% 灰度"
    echo "  $0 rollout                   # 按阶段放量"
    echo "  $0 full-enable               # 完全启用"
    echo "  $0 disable                   # 禁用订阅系统"
}

# 主函数
main() {
    local command=$1
    shift || true

    case $command in
        enable)
            check_env
            enable_grayscale "$1"
            ;;
        rollout)
            check_env
            rollout
            ;;
        disable)
            check_env
            disable_grayscale
            ;;
        full-enable)
            check_env
            full_enable
            ;;
        status)
            check_env
            show_status
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            echo -e "${RED}错误: 未知命令 '$command'${NC}"
            echo ""
            show_help
            exit 1
            ;;
    esac
}

# 执行主函数
main "$@"
