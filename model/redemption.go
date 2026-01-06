package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"gorm.io/gorm"
)

type Redemption struct {
	Id           int            `json:"id"`
	UserId       int            `json:"user_id"`
	Key          string         `json:"key" gorm:"type:char(32);uniqueIndex"`
	Status       int            `json:"status" gorm:"default:1"`
	Name         string         `json:"name" gorm:"index"`
	Quota        int            `json:"quota" gorm:"default:100"`
	CreatedTime  int64          `json:"created_time" gorm:"bigint"`
	RedeemedTime int64          `json:"redeemed_time" gorm:"bigint"`
	Count        int            `json:"count" gorm:"-:all"` // only for api request
	UsedUserId   int            `json:"used_user_id"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
	ExpiredTime  int64          `json:"expired_time" gorm:"bigint"` // 过期时间，0 表示不过期
	Type         string         `json:"type" gorm:"type:varchar(32);default:'quota'"`     // 兑换类型：quota=额度, subscription=订阅
	Payload      *string        `json:"payload" gorm:"type:text"`                         // 订阅兑换时存储套餐信息等 JSON 数据
	BoundUserId  *int64         `json:"bound_user_id" gorm:"bigint;index"`                // 专属用户ID（可选，NULL 表示不限用户）
}

func GetAllRedemptions(startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 获取总数
	err = tx.Model(&Redemption{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func SearchRedemptions(keyword string, startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Build query based on keyword type
	query := tx.Model(&Redemption{})

	// Only try to convert to ID if the string represents a valid integer
	if id, err := strconv.Atoi(keyword); err == nil {
		query = query.Where("id = ? OR name LIKE ?", id, keyword+"%")
	} else {
		query = query.Where("name LIKE ?", keyword+"%")
	}

	// Get total count
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated data
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func GetRedemptionById(id int) (*Redemption, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	var err error = nil
	err = DB.First(&redemption, "id = ?", id).Error
	return &redemption, err
}

func Redeem(key string, userId int) (quota int, err error) {
	if key == "" {
		return 0, errors.New("未提供兑换码")
	}
	if userId == 0 {
		return 0, errors.New("无效的 user id")
	}
	redemption := &Redemption{}

	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	common.RandomSleep()
	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(keyCol+" = ?", key).First(redemption).Error
		if err != nil {
			return errors.New("无效的兑换码")
		}
		// 检查兑换码类型：此函数仅处理额度兑换
		if redemption.IsSubscriptionRedemption() {
			return errors.New(common.MsgRedemptionInvalidType + "：请使用订阅兑换接口")
		}
		if redemption.Status != common.RedemptionCodeStatusEnabled {
			return errors.New("该兑换码已被使用")
		}
		if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
			return errors.New("该兑换码已过期")
		}
		err = tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota + ?", redemption.Quota)).Error
		if err != nil {
			return err
		}
		redemption.RedeemedTime = common.GetTimestamp()
		redemption.Status = common.RedemptionCodeStatusUsed
		redemption.UsedUserId = userId
		err = tx.Save(redemption).Error
		return err
	})
	if err != nil {
		return 0, errors.New("兑换失败，" + err.Error())
	}
	RecordLog(userId, LogTypeTopup, fmt.Sprintf("通过兑换码充值 %s，兑换码ID %d", logger.LogQuota(redemption.Quota), redemption.Id))
	return redemption.Quota, nil
}

func (redemption *Redemption) Insert() error {
	var err error
	err = DB.Create(redemption).Error
	return err
}

func (redemption *Redemption) SelectUpdate() error {
	// This can update zero values
	return DB.Model(redemption).Select("redeemed_time", "status").Updates(redemption).Error
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (redemption *Redemption) Update() error {
	var err error
	err = DB.Model(redemption).Select("name", "status", "quota", "redeemed_time", "expired_time").Updates(redemption).Error
	return err
}

func (redemption *Redemption) Delete() error {
	var err error
	err = DB.Delete(redemption).Error
	return err
}

func DeleteRedemptionById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	err = DB.Where(redemption).First(&redemption).Error
	if err != nil {
		return err
	}
	return redemption.Delete()
}

func DeleteInvalidRedemptions() (int64, error) {
	now := common.GetTimestamp()
	result := DB.Where("status IN ? OR (status = ? AND expired_time != 0 AND expired_time < ?)", []int{common.RedemptionCodeStatusUsed, common.RedemptionCodeStatusDisabled}, common.RedemptionCodeStatusEnabled, now).Delete(&Redemption{})
	return result.RowsAffected, result.Error
}

// ===================== 订阅兑换相关方法 =====================

// RedemptionSubscriptionPayload 订阅兑换码的 Payload 结构
type RedemptionSubscriptionPayload struct {
	PlanId       int64  `json:"plan_id"`                  // 套餐 ID
	Duration     int64  `json:"duration"`                 // 时长（秒）
	DurationDays int    `json:"duration_days,omitempty"`  // 时长（天），与 Duration 二选一
	PricePaid    int64  `json:"price_paid,omitempty"`     // 已支付价格（分），用于退款计算
	RedeemOption string `json:"redeem_option,omitempty"`  // 兑换选项：stack/coexist/convert/replace/extend
	CouponId     *int64 `json:"coupon_id,omitempty"`      // 关联的优惠券 ID（可选）
	Extra        string `json:"extra,omitempty"`          // 额外信息
}

// GetDurationSeconds 获取时长（秒）
// 优先使用 Duration，如果为 0 则使用 DurationDays 转换
func (p *RedemptionSubscriptionPayload) GetDurationSeconds() int64 {
	if p.Duration > 0 {
		return p.Duration
	}
	if p.DurationDays > 0 {
		return int64(p.DurationDays) * 24 * 60 * 60
	}
	return 0
}

// ToJSON 将 Payload 序列化为 JSON 字符串指针
func (p *RedemptionSubscriptionPayload) ToJSON() (*string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	str := string(data)
	return &str, nil
}

// IsCoexistMode 检查是否为共存模式
func (p *RedemptionSubscriptionPayload) IsCoexistMode() bool {
	return p.RedeemOption == common.RedeemOptionCoexist
}

// IsConvertMode 检查是否为转换模式
func (p *RedemptionSubscriptionPayload) IsConvertMode() bool {
	return p.RedeemOption == common.RedeemOptionConvert
}

// GetRedemptionByKey 根据兑换码获取兑换记录
func GetRedemptionByKey(key string) (*Redemption, error) {
	if key == "" {
		return nil, errors.New("兑换码不能为空")
	}

	var redemption Redemption
	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}

	err := DB.Where(keyCol+" = ?", key).First(&redemption).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgRedemptionNotFound)
		}
		return nil, err
	}

	return &redemption, nil
}

// GetRedemptionByKeyWithTx 在事务中根据兑换码获取兑换记录（带行锁）
func GetRedemptionByKeyWithTx(tx *gorm.DB, key string, forUpdate bool) (*Redemption, error) {
	if key == "" {
		return nil, errors.New("兑换码不能为空")
	}

	var redemption Redemption
	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}

	query := tx
	if forUpdate {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}

	err := query.Where(keyCol+" = ?", key).First(&redemption).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgRedemptionNotFound)
		}
		return nil, err
	}

	return &redemption, nil
}

// GetRedemptionsByType 根据类型获取兑换码列表
func GetRedemptionsByType(redeemType string, startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	query := DB.Model(&Redemption{})

	if redeemType != "" {
		query = query.Where("type = ?", redeemType)
	}

	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

// IsSubscriptionRedemption 检查是否为订阅类型兑换码
func (r *Redemption) IsSubscriptionRedemption() bool {
	return r.Type == common.RedemptionTypeSubscription
}

// IsQuotaRedemption 检查是否为额度类型兑换码
func (r *Redemption) IsQuotaRedemption() bool {
	return r.Type == "" || r.Type == common.RedemptionTypeQuota
}

// GetSubscriptionPayload 获取订阅兑换的 Payload
func (r *Redemption) GetSubscriptionPayload() (*RedemptionSubscriptionPayload, error) {
	if !r.IsSubscriptionRedemption() {
		return nil, errors.New(common.MsgRedemptionInvalidType)
	}

	if r.Payload == nil || *r.Payload == "" {
		return nil, errors.New("兑换码 Payload 为空")
	}

	var payload RedemptionSubscriptionPayload
	err := json.Unmarshal([]byte(*r.Payload), &payload)
	if err != nil {
		return nil, err
	}

	return &payload, nil
}

// SetSubscriptionPayload 设置订阅兑换的 Payload
func (r *Redemption) SetSubscriptionPayload(payload *RedemptionSubscriptionPayload) error {
	if payload == nil {
		r.Payload = nil
		return nil
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	str := string(data)
	r.Payload = &str
	return nil
}

// ValidateForRedemption 验证兑换码是否可用于兑换
func (r *Redemption) ValidateForRedemption() error {
	if r.Status != common.RedemptionCodeStatusEnabled {
		return errors.New(common.MsgRedemptionUsed)
	}

	if r.ExpiredTime != 0 && r.ExpiredTime < common.GetTimestamp() {
		return errors.New(common.MsgRedemptionExpired)
	}

	return nil
}

// RedeemSubscription 兑换订阅（不含订阅创建逻辑，仅标记兑换状态）
func RedeemSubscription(key string, userId int) (*Redemption, error) {
	if key == "" {
		return nil, errors.New("未提供兑换码")
	}
	if userId == 0 {
		return nil, errors.New("无效的 user id")
	}

	var redemption *Redemption

	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}

	common.RandomSleep()
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		redemption = &Redemption{}
		err = tx.Set("gorm:query_option", "FOR UPDATE").Where(keyCol+" = ?", key).First(redemption).Error
		if err != nil {
			return errors.New(common.MsgRedemptionInvalid)
		}

		// 验证兑换码
		if err = redemption.ValidateForRedemption(); err != nil {
			return err
		}

		// 验证是否为订阅类型
		if !redemption.IsSubscriptionRedemption() {
			return errors.New(common.MsgRedemptionInvalidType)
		}

		// 标记为已使用
		redemption.RedeemedTime = common.GetTimestamp()
		redemption.Status = common.RedemptionCodeStatusUsed
		redemption.UsedUserId = userId

		err = tx.Save(redemption).Error
		return err
	})

	if err != nil {
		return nil, err
	}

	return redemption, nil
}

// RedeemSubscriptionWithTx 在事务中兑换订阅
func RedeemSubscriptionWithTx(tx *gorm.DB, redemption *Redemption, userId int) error {
	if redemption == nil {
		return errors.New("兑换码不能为空")
	}
	if userId == 0 {
		return errors.New("无效的 user id")
	}

	// 验证兑换码
	if err := redemption.ValidateForRedemption(); err != nil {
		return err
	}

	// 验证是否为订阅类型
	if !redemption.IsSubscriptionRedemption() {
		return errors.New(common.MsgRedemptionInvalidType)
	}

	// 标记为已使用
	redemption.RedeemedTime = common.GetTimestamp()
	redemption.Status = common.RedemptionCodeStatusUsed
	redemption.UsedUserId = userId

	return tx.Save(redemption).Error
}

// CreateSubscriptionRedemption 创建订阅类型的兑换码
func CreateSubscriptionRedemption(name string, planId int64, duration int64, durationDays int, pricePaid int64, redeemOption string, couponId *int64, expiredTime int64, creatorId int) (*Redemption, error) {
	// 参数校验
	if planId == 0 {
		return nil, errors.New("套餐 ID 不能为空")
	}

	// 验证兑换选项合法性
	validOptions := map[string]bool{
		common.RedeemOptionStack:   true,
		common.RedeemOptionCoexist: true,
		common.RedeemOptionConvert: true,
		common.RedeemOptionReplace: true,
		common.RedeemOptionExtend:  true,
	}
	if !validOptions[redeemOption] {
		return nil, errors.New("无效的兑换选项：必须是 stack/coexist/convert/replace/extend 之一")
	}

	// 验证时长：duration 或 durationDays 至少有一个 > 0
	if duration <= 0 && durationDays <= 0 {
		return nil, errors.New("时长无效：duration 或 durationDays 至少有一个必须 > 0")
	}

	// 验证价格
	if pricePaid < 0 {
		return nil, errors.New("价格不能为负数")
	}

	payload := RedemptionSubscriptionPayload{
		PlanId:       planId,
		Duration:     duration,
		DurationDays: durationDays,
		PricePaid:    pricePaid,
		RedeemOption: redeemOption,
		CouponId:     couponId,
	}

	payloadData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	payloadStr := string(payloadData)

	redemption := &Redemption{
		UserId:      creatorId,
		Key:         common.GetUUID(),
		Status:      common.RedemptionCodeStatusEnabled,
		Name:        name,
		Quota:       0, // 订阅类型不使用额度
		CreatedTime: common.GetTimestamp(),
		ExpiredTime: expiredTime,
		Type:        common.RedemptionTypeSubscription,
		Payload:     &payloadStr,
	}

	err = redemption.Insert()
	if err != nil {
		return nil, err
	}

	return redemption, nil
}

// BatchCreateSubscriptionRedemptions 批量创建订阅类型兑换码
func BatchCreateSubscriptionRedemptions(name string, planId int64, duration int64, durationDays int, pricePaid int64, redeemOption string, couponId *int64, expiredTime int64, creatorId int, count int) ([]*Redemption, error) {
	if count <= 0 {
		return nil, errors.New("数量必须大于 0")
	}

	if count > 1000 {
		return nil, errors.New("单次最多创建 1000 个兑换码")
	}

	// 参数校验
	if planId == 0 {
		return nil, errors.New("套餐 ID 不能为空")
	}

	// 验证兑换选项合法性
	validOptions := map[string]bool{
		common.RedeemOptionStack:   true,
		common.RedeemOptionCoexist: true,
		common.RedeemOptionConvert: true,
		common.RedeemOptionReplace: true,
		common.RedeemOptionExtend:  true,
	}
	if !validOptions[redeemOption] {
		return nil, errors.New("无效的兑换选项：必须是 stack/coexist/convert/replace/extend 之一")
	}

	// 验证时长：duration 或 durationDays 至少有一个 > 0
	if duration <= 0 && durationDays <= 0 {
		return nil, errors.New("时长无效：duration 或 durationDays 至少有一个必须 > 0")
	}

	// 验证价格
	if pricePaid < 0 {
		return nil, errors.New("价格不能为负数")
	}

	payload := RedemptionSubscriptionPayload{
		PlanId:       planId,
		Duration:     duration,
		DurationDays: durationDays,
		PricePaid:    pricePaid,
		RedeemOption: redeemOption,
		CouponId:     couponId,
	}

	payloadData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	payloadStr := string(payloadData)

	now := common.GetTimestamp()
	redemptions := make([]*Redemption, count)

	for i := 0; i < count; i++ {
		redemptions[i] = &Redemption{
			UserId:      creatorId,
			Key:         common.GetUUID(),
			Status:      common.RedemptionCodeStatusEnabled,
			Name:        name,
			Quota:       0,
			CreatedTime: now,
			ExpiredTime: expiredTime,
			Type:        common.RedemptionTypeSubscription,
			Payload:     &payloadStr,
		}
	}

	err = DB.CreateInBatches(redemptions, 100).Error
	if err != nil {
		return nil, err
	}

	return redemptions, nil
}

// GetRedemptionStats 获取兑换码统计
func GetRedemptionStats() (map[string]int64, error) {
	stats := make(map[string]int64)

	// 总数
	var total int64
	if err := DB.Model(&Redemption{}).Count(&total).Error; err != nil {
		return nil, err
	}
	stats["total"] = total

	// 已使用
	var used int64
	if err := DB.Model(&Redemption{}).Where("status = ?", common.RedemptionCodeStatusUsed).Count(&used).Error; err != nil {
		return nil, err
	}
	stats["used"] = used

	// 未使用
	var unused int64
	if err := DB.Model(&Redemption{}).Where("status = ?", common.RedemptionCodeStatusEnabled).Count(&unused).Error; err != nil {
		return nil, err
	}
	stats["unused"] = unused

	// 额度类型
	var quotaType int64
	if err := DB.Model(&Redemption{}).Where("type = ? OR type IS NULL OR type = ''", common.RedemptionTypeQuota).Count(&quotaType).Error; err != nil {
		return nil, err
	}
	stats["quota_type"] = quotaType

	// 订阅类型
	var subscriptionType int64
	if err := DB.Model(&Redemption{}).Where("type = ?", common.RedemptionTypeSubscription).Count(&subscriptionType).Error; err != nil {
		return nil, err
	}
	stats["subscription_type"] = subscriptionType

	return stats, nil
}

// ===================== 专属用户校验方法 =====================

// ValidateBoundUser 校验专属用户
// 若 bound_user_id 不为空，校验是否匹配当前用户
func (r *Redemption) ValidateBoundUser(userId int) error {
	if r.BoundUserId == nil {
		// 无专属用户限制，所有用户都可使用
		return nil
	}

	// 校验是否匹配当前用户
	if int64(userId) != *r.BoundUserId {
		return errors.New(common.MsgRedemptionUserMismatch)
	}

	return nil
}

// IsBoundToUser 检查兑换码是否绑定到特定用户
func (r *Redemption) IsBoundToUser() bool {
	return r.BoundUserId != nil
}

// GetBoundUserId 获取绑定的用户ID
func (r *Redemption) GetBoundUserId() *int64 {
	return r.BoundUserId
}

