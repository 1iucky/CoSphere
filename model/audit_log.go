package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// AuditLog 审计日志模型
type AuditLog struct {
	Id         int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserId     *int64 `gorm:"column:user_id;index:idx_audit_logs_user_created_at" json:"user_id"` // 操作涉及的用户
	OperatorId *int64 `gorm:"column:operator_id" json:"operator_id"`                               // 操作执行者
	ObjectType string `gorm:"column:object_type;type:varchar(64);not null;index:idx_audit_logs_object" json:"object_type"`
	ObjectId   *int64 `gorm:"column:object_id;index:idx_audit_logs_object" json:"object_id"`
	Action     string `gorm:"column:action;type:varchar(64);not null" json:"action"`
	IpAddress  string `gorm:"column:ip_address;type:varchar(64)" json:"ip_address"`
	UserAgent  string `gorm:"column:user_agent;type:text" json:"user_agent"`
	Metadata   string `gorm:"column:metadata;type:text" json:"metadata"` // JSON 格式的额外元数据
	CreatedAt  int64  `gorm:"column:created_at;not null;index:idx_audit_logs_created_at;index:idx_audit_logs_user_created_at" json:"created_at"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}

// CreateAuditLog 创建审计日志
func CreateAuditLog(log *AuditLog) error {
	if log == nil {
		return errors.New("审计日志不能为空")
	}

	log.CreatedAt = time.Now().Unix()

	return DB.Create(log).Error
}

// CreateAuditLogWithTx 在事务中创建审计日志
func CreateAuditLogWithTx(tx *gorm.DB, log *AuditLog) error {
	if log == nil {
		return errors.New("审计日志不能为空")
	}

	log.CreatedAt = time.Now().Unix()

	return tx.Create(log).Error
}

// GetAuditLogsByUser 获取指定用户的审计日志
func GetAuditLogsByUser(userId int64, offset int, limit int) ([]*AuditLog, error) {
	var logs []*AuditLog

	err := DB.Where("user_id = ?", userId).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&logs).Error

	return logs, err
}

// GetAuditLogsByObject 获取指定对象的审计日志
func GetAuditLogsByObject(objectType string, objectId int64, offset int, limit int) ([]*AuditLog, error) {
	var logs []*AuditLog

	err := DB.Where("object_type = ? AND object_id = ?", objectType, objectId).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&logs).Error

	return logs, err
}

// CountAuditLogsByUser 统计指定用户的审计日志数量
func CountAuditLogsByUser(userId int64) (int64, error) {
	var count int64

	err := DB.Model(&AuditLog{}).
		Where("user_id = ?", userId).
		Count(&count).Error

	return count, err
}

// CountAuditLogsByObject 统计指定对象的审计日志数量
func CountAuditLogsByObject(objectType string, objectId int64) (int64, error) {
	var count int64

	err := DB.Model(&AuditLog{}).
		Where("object_type = ? AND object_id = ?", objectType, objectId).
		Count(&count).Error

	return count, err
}

// DeleteOldAuditLogs 删除指定时间之前的审计日志（用于日志清理）
func DeleteOldAuditLogs(before int64) error {
	return DB.Where("created_at < ?", before).Delete(&AuditLog{}).Error
}

// GetRedemptionAuditLogByRedemptionId 根据兑换码 ID 和用户 ID 查询兑换审计日志
// 用于 stack/extend 场景的幂等性检查
// 返回最新的一条成功兑换记录（action 以 redeem_ 开头）
func GetRedemptionAuditLogByRedemptionId(userId int64, redemptionId int64) (*AuditLog, error) {
	if userId == 0 || redemptionId == 0 {
		return nil, nil
	}

	var log AuditLog
	// 修复 2.16：使用严格边界匹配，避免 redemption_id=12 匹配到 123
	// JSON 中数字后面要么是逗号 `,`，要么是右大括号 `}`
	// 使用 OR 条件匹配两种情况：
	// 1. "redemption_id":12, (后面还有其他字段)
	// 2. "redemption_id":12} (最后一个字段)
	idStr := formatInt64(redemptionId)
	patternWithComma := "%\"redemption_id\":" + idStr + ",%"
	patternWithBrace := "%\"redemption_id\":" + idStr + "}%"

	err := DB.Where("user_id = ? AND object_type = ? AND action LIKE ?", userId, "subscription_redemption", "redeem_%").
		Where("(metadata LIKE ? OR metadata LIKE ?)", patternWithComma, patternWithBrace).
		Order("created_at DESC").
		First(&log).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &log, nil
}

// formatInt64 将 int64 转换为字符串（避免导入 strconv）
func formatInt64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
