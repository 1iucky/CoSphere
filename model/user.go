package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// User if you add sensitive fields, don't forget to clean them in setupLogin function.
// Otherwise, the sensitive information will be saved on local storage in plain text!
type User struct {
	Id                 int            `json:"id"`
	Username           string         `json:"username" gorm:"unique;index" validate:"max=20"`
	Password           string         `json:"password" gorm:"not null;" validate:"min=8,max=20"`
	OriginalPassword   string         `json:"original_password" gorm:"-:all"` // this field is only for Password change verification, don't save it to database!
	DisplayName        string         `json:"display_name" gorm:"index" validate:"max=20"`
	Role               int            `json:"role" gorm:"type:int;default:1"`   // admin, common
	Status             int            `json:"status" gorm:"type:int;default:1"` // enabled, disabled
	Email              string         `json:"email" gorm:"index" validate:"max=50"`
	GitHubId           string         `json:"github_id" gorm:"column:github_id;index"`
	DiscordId          string         `json:"discord_id" gorm:"column:discord_id;index"`
	GoogleId           string         `json:"google_id" gorm:"column:google_id;index"`
	OidcId             string         `json:"oidc_id" gorm:"column:oidc_id;index"`
	WeChatId           string         `json:"wechat_id" gorm:"column:wechat_id;index"`
	TelegramId         string         `json:"telegram_id" gorm:"column:telegram_id;index"`
	VerificationCode   string         `json:"verification_code" gorm:"-:all"`                                    // this field is only for Email verification, don't save it to database!
	AccessToken        *string        `json:"access_token" gorm:"type:char(32);column:access_token;uniqueIndex"` // this token is for system management
	Quota              int            `json:"quota" gorm:"type:int;default:0"`
	UsedQuota          int            `json:"used_quota" gorm:"type:int;default:0;column:used_quota"` // used quota
	RequestCount       int            `json:"request_count" gorm:"type:int;default:0;"`               // request number
	Group              string         `json:"group" gorm:"type:varchar(64);default:'default'"`
	AffCode            string         `json:"aff_code" gorm:"type:varchar(32);column:aff_code;uniqueIndex"`
	AffCount           int            `json:"aff_count" gorm:"type:int;default:0;column:aff_count"`
	AffQuota           int            `json:"aff_quota" gorm:"type:int;default:0;column:aff_quota"`           // 邀请剩余额度
	AffHistoryQuota    int            `json:"aff_history_quota" gorm:"type:int;default:0;column:aff_history"` // 邀请历史额度
	InviterId          int            `json:"inviter_id" gorm:"type:int;column:inviter_id;index"`
	DeletedAt          gorm.DeletedAt `gorm:"index"`
	LinuxDOId          string         `json:"linux_do_id" gorm:"column:linux_do_id;index"`
	Setting            string         `json:"setting" gorm:"type:text;column:setting"`
	Remark             string         `json:"remark,omitempty" gorm:"type:varchar(255)" validate:"max=255"`
	StripeCustomer     string         `json:"stripe_customer" gorm:"type:varchar(64);column:stripe_customer;index"`
	AutoWalletFallback bool           `json:"auto_wallet_fallback" gorm:"default:false;column:auto_wallet_fallback"` // 订阅额度耗尽时是否自动使用钱包余额
}

func (user *User) ToBaseUser() *UserBase {
	cache := &UserBase{
		Id:       user.Id,
		Group:    user.Group,
		Quota:    user.Quota,
		Status:   user.Status,
		Username: user.Username,
		Setting:  user.Setting,
		Email:    user.Email,
	}
	return cache
}

func (user *User) GetAccessToken() string {
	if user.AccessToken == nil {
		return ""
	}
	return *user.AccessToken
}

func (user *User) SetAccessToken(token string) {
	user.AccessToken = &token
}

func (user *User) GetSetting() dto.UserSetting {
	setting := dto.UserSetting{}
	if user.Setting != "" {
		err := json.Unmarshal([]byte(user.Setting), &setting)
		if err != nil {
			common.SysLog("failed to unmarshal setting: " + err.Error())
		}
	}
	return setting
}

func (user *User) SetSetting(setting dto.UserSetting) {
	settingBytes, err := json.Marshal(setting)
	if err != nil {
		common.SysLog("failed to marshal setting: " + err.Error())
		return
	}
	user.Setting = string(settingBytes)
}

// 根据用户角色生成默认的边栏配置
func generateDefaultSidebarConfigForRole(userRole int) string {
	defaultConfig := map[string]interface{}{}

	// 聊天区域 - 所有用户都可以访问
	defaultConfig["chat"] = map[string]interface{}{
		"enabled":    true,
		"playground": true,
		"chat":       true,
	}

	// 控制台区域 - 所有用户都可以访问
	defaultConfig["console"] = map[string]interface{}{
		"enabled":    true,
		"detail":     true,
		"token":      true,
		"log":        true,
		"midjourney": true,
		"task":       true,
	}

	// 个人中心区域 - 所有用户都可以访问
	defaultConfig["personal"] = map[string]interface{}{
		"enabled":  true,
		"topup":    true,
		"personal": true,
	}

	// 管理员区域 - 根据角色决定
	if userRole == common.RoleAdminUser {
		// 管理员可以访问管理员区域，但不能访问系统设置
		defaultConfig["admin"] = map[string]interface{}{
			"enabled":    true,
			"channel":    true,
			"models":     true,
			"redemption": true,
			"user":       true,
			"setting":    false, // 管理员不能访问系统设置
		}
	} else if userRole == common.RoleRootUser {
		// 超级管理员可以访问所有功能
		defaultConfig["admin"] = map[string]interface{}{
			"enabled":    true,
			"channel":    true,
			"models":     true,
			"redemption": true,
			"user":       true,
			"setting":    true,
		}
	}
	// 普通用户不包含admin区域

	// 转换为JSON字符串
	configBytes, err := json.Marshal(defaultConfig)
	if err != nil {
		common.SysLog("生成默认边栏配置失败: " + err.Error())
		return ""
	}

	return string(configBytes)
}

// CheckUserExistOrDeleted check if user exist or deleted, if not exist, return false, nil, if deleted or exist, return true, nil
func CheckUserExistOrDeleted(username string, email string) (bool, error) {
	var user User

	// err := DB.Unscoped().First(&user, "username = ? or email = ?", username, email).Error
	// check email if empty
	var err error
	if email == "" {
		err = DB.Unscoped().First(&user, "username = ?", username).Error
	} else {
		err = DB.Unscoped().First(&user, "username = ? or email = ?", username, email).Error
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// not exist, return false, nil
			return false, nil
		}
		// other error, return false, err
		return false, err
	}
	// exist, return true, nil
	return true, nil
}

func GetMaxUserId() int {
	var user User
	DB.Unscoped().Last(&user)
	return user.Id
}

func GetAllUsers(pageInfo *common.PageInfo) (users []*User, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Get total count within transaction
	err = tx.Unscoped().Model(&User{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated users within same transaction
	err = tx.Unscoped().Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Omit("password").Find(&users).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func SearchUsers(keyword string, group string, startIdx int, num int) ([]*User, int64, error) {
	var users []*User
	var total int64
	var err error

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

	// 构建基础查询
	query := tx.Unscoped().Model(&User{})

	// 构建搜索条件
	likeCondition := "username LIKE ? OR email LIKE ? OR display_name LIKE ?"

	// 尝试将关键字转换为整数ID
	keywordInt, err := strconv.Atoi(keyword)
	if err == nil {
		// 如果是数字，同时搜索ID和其他字段
		likeCondition = "id = ? OR " + likeCondition
		if group != "" {
			query = query.Where("("+likeCondition+") AND "+commonGroupCol+" = ?",
				keywordInt, "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%", group)
		} else {
			query = query.Where(likeCondition,
				keywordInt, "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
		}
	} else {
		// 非数字关键字，只搜索字符串字段
		if group != "" {
			query = query.Where("("+likeCondition+") AND "+commonGroupCol+" = ?",
				"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%", group)
		} else {
			query = query.Where(likeCondition,
				"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
		}
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	err = query.Omit("password").Order("id desc").Limit(num).Offset(startIdx).Find(&users).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func GetUserById(id int, selectAll bool) (*User, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	user := User{Id: id}
	var err error = nil
	if selectAll {
		err = DB.First(&user, "id = ?", id).Error
	} else {
		err = DB.Omit("password").First(&user, "id = ?", id).Error
	}
	return &user, err
}

func GetUserByIdWithTx(tx *gorm.DB, id int, selectAll bool) (*User, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	if tx == nil {
		tx = DB
	}

	user := User{Id: id}
	var err error
	if selectAll {
		err = tx.First(&user, "id = ?", id).Error
	} else {
		err = tx.Omit("password").First(&user, "id = ?", id).Error
	}

	return &user, err
}

func GetUserIdByAffCode(affCode string) (int, error) {
	if affCode == "" {
		return 0, errors.New("affCode 为空！")
	}
	var user User
	err := DB.Select("id").First(&user, "aff_code = ?", affCode).Error
	return user.Id, err
}

func DeleteUserById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	user := User{Id: id}
	return user.Delete()
}

func HardDeleteUserById(id int) error {
	if id == 0 {
		return errors.New("id 为空！")
	}
	err := DB.Unscoped().Delete(&User{}, "id = ?", id).Error
	return err
}

func inviteUser(inviterId int) (err error) {
	user, err := GetUserById(inviterId, true)
	if err != nil {
		return err
	}
	user.AffCount++
	user.AffQuota += common.QuotaForInviter
	user.AffHistoryQuota += common.QuotaForInviter
	return DB.Save(user).Error
}

func (user *User) TransferAffQuotaToQuota(quota int) error {
	// 检查quota是否小于最小额度
	if float64(quota) < common.QuotaPerUnit {
		return fmt.Errorf("转移额度最小为%s！", logger.LogQuota(int(common.QuotaPerUnit)))
	}

	// 开始数据库事务
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback() // 确保在函数退出时事务能回滚

	// 加锁查询用户以确保数据一致性
	err := tx.Set("gorm:query_option", "FOR UPDATE").First(&user, user.Id).Error
	if err != nil {
		return err
	}

	// 再次检查用户的AffQuota是否足够
	if user.AffQuota < quota {
		return errors.New("邀请额度不足！")
	}

	// 更新用户额度
	user.AffQuota -= quota
	user.Quota += quota

	// 保存用户状态
	if err := tx.Save(user).Error; err != nil {
		return err
	}

	// 提交事务
	return tx.Commit().Error
}

func (user *User) Insert(inviterId int) error {
	var err error
	if user.Password != "" {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}
	user.Quota = common.QuotaForNewUser
	//user.SetAccessToken(common.GetUUID())
	user.AffCode = common.GetRandomString(4)

	// 初始化用户设置，包括默认的边栏配置
	if user.Setting == "" {
		defaultSetting := dto.UserSetting{}
		// 这里暂时不设置SidebarModules，因为需要在用户创建后根据角色设置
		user.SetSetting(defaultSetting)
	}

	result := DB.Create(user)
	if result.Error != nil {
		return result.Error
	}

	// 用户创建成功后，初始化自动兜底配置（任务 2.2.7 要求）
	// 此时 user.Id 已经由数据库生成，可以调用初始化方法
	systemDefault := common.GetAutoWalletFallbackDefault()
	err = InitializeAutoWalletFallbackForUser(user.Id, systemDefault)
	if err != nil {
		// 初始化失败不应阻止用户创建，仅记录日志
		common.SysLog(fmt.Sprintf("初始化用户 %d 自动兜底配置失败: %s", user.Id, err.Error()))
	}

	// 用户创建成功后，根据角色初始化边栏配置
	// 需要重新获取用户以确保有正确的ID和Role
	var createdUser User
	if err := DB.Where("username = ?", user.Username).First(&createdUser).Error; err == nil {
		// 生成基于角色的默认边栏配置
		defaultSidebarConfig := generateDefaultSidebarConfigForRole(createdUser.Role)
		if defaultSidebarConfig != "" {
			currentSetting := createdUser.GetSetting()
			currentSetting.SidebarModules = defaultSidebarConfig
			createdUser.SetSetting(currentSetting)
			createdUser.Update(false)
			common.SysLog(fmt.Sprintf("为新用户 %s (角色: %d) 初始化边栏配置", createdUser.Username, createdUser.Role))
		}
	}

	if common.QuotaForNewUser > 0 {
		RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("新用户注册赠送 %s", logger.LogQuota(common.QuotaForNewUser)))
	}
	if inviterId != 0 {
		if common.QuotaForInvitee > 0 {
			_ = IncreaseUserQuota(user.Id, common.QuotaForInvitee, true)
			RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("使用邀请码赠送 %s", logger.LogQuota(common.QuotaForInvitee)))
		}
		if common.QuotaForInviter > 0 {
			//_ = IncreaseUserQuota(inviterId, common.QuotaForInviter)
			RecordLog(inviterId, LogTypeSystem, fmt.Sprintf("邀请用户赠送 %s", logger.LogQuota(common.QuotaForInviter)))
			_ = inviteUser(inviterId)
		}
	}
	return nil
}

func (user *User) Update(updatePassword bool) error {
	var err error
	if updatePassword {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}
	newUser := *user
	DB.First(&user, user.Id)
	if err = DB.Model(user).Updates(newUser).Error; err != nil {
		return err
	}

	// Update cache
	return updateUserCache(*user)
}

func (user *User) Edit(updatePassword bool) error {
	var err error
	if updatePassword {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}

	newUser := *user
	updates := map[string]interface{}{
		"username":     newUser.Username,
		"display_name": newUser.DisplayName,
		"group":        newUser.Group,
		"quota":        newUser.Quota,
		"remark":       newUser.Remark,
	}
	if updatePassword {
		updates["password"] = newUser.Password
	}

	DB.First(&user, user.Id)
	if err = DB.Model(user).Updates(updates).Error; err != nil {
		return err
	}

	// Update cache
	return updateUserCache(*user)
}

func (user *User) Delete() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	if err := DB.Delete(user).Error; err != nil {
		return err
	}

	// 清除缓存
	return invalidateUserCache(user.Id)
}

func (user *User) HardDelete() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	err := DB.Unscoped().Delete(user).Error
	return err
}

// ValidateAndFill check password & user status
func (user *User) ValidateAndFill() (err error) {
	// When querying with struct, GORM will only query with non-zero fields,
	// that means if your field's value is 0, '', false or other zero values,
	// it won't be used to build query conditions
	password := user.Password
	username := strings.TrimSpace(user.Username)
	if username == "" || password == "" {
		return errors.New("用户名或密码为空")
	}
	// find buy username or email
	DB.Where("username = ? OR email = ?", username, username).First(user)
	okay := common.ValidatePasswordAndHash(password, user.Password)
	if !okay || user.Status != common.UserStatusEnabled {
		return errors.New("用户名或密码错误，或用户已被封禁")
	}
	return nil
}

func (user *User) FillUserById() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	DB.Where(User{Id: user.Id}).First(user)
	return nil
}

func (user *User) FillUserByEmail() error {
	if user.Email == "" {
		return errors.New("email 为空！")
	}
	DB.Where(User{Email: user.Email}).First(user)
	return nil
}

func (user *User) FillUserByGitHubId() error {
	if user.GitHubId == "" {
		return errors.New("GitHub id 为空！")
	}
	DB.Where(User{GitHubId: user.GitHubId}).First(user)
	return nil
}

func (user *User) FillUserByDiscordId() error {
	if user.DiscordId == "" {
		return errors.New("discord id 为空！")
	}
	DB.Where(User{DiscordId: user.DiscordId}).First(user)
	return nil
}

func (user *User) FillUserByGoogleId() error {
	if user.GoogleId == "" {
		return errors.New("google id 为空！")
	}
	DB.Where(User{GoogleId: user.GoogleId}).First(user)
	return nil
}

func (user *User) FillUserByOidcId() error {
	if user.OidcId == "" {
		return errors.New("oidc id 为空！")
	}
	DB.Where(User{OidcId: user.OidcId}).First(user)
	return nil
}

func (user *User) FillUserByWeChatId() error {
	if user.WeChatId == "" {
		return errors.New("WeChat id 为空！")
	}
	DB.Where(User{WeChatId: user.WeChatId}).First(user)
	return nil
}

func (user *User) FillUserByTelegramId() error {
	if user.TelegramId == "" {
		return errors.New("Telegram id 为空！")
	}
	err := DB.Where(User{TelegramId: user.TelegramId}).First(user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("该 Telegram 账户未绑定")
	}
	return nil
}

func IsEmailAlreadyTaken(email string) bool {
	return DB.Unscoped().Where("email = ?", email).Find(&User{}).RowsAffected == 1
}

func IsWeChatIdAlreadyTaken(wechatId string) bool {
	return DB.Unscoped().Where("wechat_id = ?", wechatId).Find(&User{}).RowsAffected == 1
}

func IsGitHubIdAlreadyTaken(githubId string) bool {
	return DB.Unscoped().Where("github_id = ?", githubId).Find(&User{}).RowsAffected == 1
}

func IsDiscordIdAlreadyTaken(discordId string) bool {
	return DB.Unscoped().Where("discord_id = ?", discordId).Find(&User{}).RowsAffected == 1
}

func IsGoogleIdAlreadyTaken(googleId string) bool {
	return DB.Unscoped().Where("google_id = ?", googleId).Find(&User{}).RowsAffected == 1
}

func IsOidcIdAlreadyTaken(oidcId string) bool {
	return DB.Where("oidc_id = ?", oidcId).Find(&User{}).RowsAffected == 1
}

func IsTelegramIdAlreadyTaken(telegramId string) bool {
	return DB.Unscoped().Where("telegram_id = ?", telegramId).Find(&User{}).RowsAffected == 1
}

func ResetUserPasswordByEmail(email string, password string) error {
	if email == "" || password == "" {
		return errors.New("邮箱地址或密码为空！")
	}
	hashedPassword, err := common.Password2Hash(password)
	if err != nil {
		return err
	}
	err = DB.Model(&User{}).Where("email = ?", email).Update("password", hashedPassword).Error
	return err
}

func IsAdmin(userId int) bool {
	if userId == 0 {
		return false
	}
	var user User
	err := DB.Where("id = ?", userId).Select("role").Find(&user).Error
	if err != nil {
		common.SysLog("no such user " + err.Error())
		return false
	}
	return user.Role >= common.RoleAdminUser
}

//// IsUserEnabled checks user status from Redis first, falls back to DB if needed
//func IsUserEnabled(id int, fromDB bool) (status bool, err error) {
//	defer func() {
//		// Update Redis cache asynchronously on successful DB read
//		if shouldUpdateRedis(fromDB, err) {
//			gopool.Go(func() {
//				if err := updateUserStatusCache(id, status); err != nil {
//					common.SysError("failed to update user status cache: " + err.Error())
//				}
//			})
//		}
//	}()
//	if !fromDB && common.RedisEnabled {
//		// Try Redis first
//		status, err := getUserStatusCache(id)
//		if err == nil {
//			return status == common.UserStatusEnabled, nil
//		}
//		// Don't return error - fall through to DB
//	}
//	fromDB = true
//	var user User
//	err = DB.Where("id = ?", id).Select("status").Find(&user).Error
//	if err != nil {
//		return false, err
//	}
//
//	return user.Status == common.UserStatusEnabled, nil
//}

func ValidateAccessToken(token string) (user *User) {
	if token == "" {
		return nil
	}
	token = strings.Replace(token, "Bearer ", "", 1)
	user = &User{}
	if DB.Where("access_token = ?", token).First(user).RowsAffected == 1 {
		return user
	}
	return nil
}

// GetUserQuota gets quota from Redis first, falls back to DB if needed
func GetUserQuota(id int, fromDB bool) (quota int, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserQuotaCache(id, quota); err != nil {
					common.SysLog("failed to update user quota cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		quota, err := getUserQuotaCache(id)
		if err == nil {
			return quota, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select("quota").Find(&quota).Error
	if err != nil {
		return 0, err
	}

	return quota, nil
}

func GetUserUsedQuota(id int) (quota int, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("used_quota").Find(&quota).Error
	return quota, err
}

func GetUserEmail(id int) (email string, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("email").Find(&email).Error
	return email, err
}

// GetUserGroup gets group from Redis first, falls back to DB if needed
func GetUserGroup(id int, fromDB bool) (group string, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserGroupCache(id, group); err != nil {
					common.SysLog("failed to update user group cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		group, err := getUserGroupCache(id)
		if err == nil {
			return group, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select(commonGroupCol).Find(&group).Error
	if err != nil {
		return "", err
	}

	return group, nil
}

// GetUserSetting gets setting from Redis first, falls back to DB if needed
func GetUserSetting(id int, fromDB bool) (settingMap dto.UserSetting, err error) {
	var setting string
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserSettingCache(id, setting); err != nil {
					common.SysLog("failed to update user setting cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		setting, err := getUserSettingCache(id)
		if err == nil {
			return setting, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select("setting").Find(&setting).Error
	if err != nil {
		return settingMap, err
	}
	userBase := &UserBase{
		Setting: setting,
	}
	return userBase.GetSetting(), nil
}

func IncreaseUserQuota(id int, quota int, db bool) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	gopool.Go(func() {
		err := cacheIncrUserQuota(id, int64(quota))
		if err != nil {
			common.SysLog("failed to increase user quota: " + err.Error())
		}
	})
	if !db && common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUserQuota, id, quota)
		return nil
	}
	return increaseUserQuota(id, quota)
}

func increaseUserQuota(id int, quota int) (err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota + ?", quota)).Error
	if err != nil {
		return err
	}
	return err
}

func DecreaseUserQuota(id int, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	gopool.Go(func() {
		err := cacheDecrUserQuota(id, int64(quota))
		if err != nil {
			common.SysLog("failed to decrease user quota: " + err.Error())
		}
	})
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUserQuota, id, -quota)
		return nil
	}
	return decreaseUserQuota(id, quota)
}

func decreaseUserQuota(id int, quota int) (err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota - ?", quota)).Error
	if err != nil {
		return err
	}
	return err
}

func DeltaUpdateUserQuota(id int, delta int) (err error) {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		return IncreaseUserQuota(id, delta, false)
	} else {
		return DecreaseUserQuota(id, -delta)
	}
}

//func GetRootUserEmail() (email string) {
//	DB.Model(&User{}).Where("role = ?", common.RoleRootUser).Select("email").Find(&email)
//	return email
//}

func GetRootUser() (user *User) {
	DB.Where("role = ?", common.RoleRootUser).First(&user)
	return user
}

func UpdateUserUsedQuotaAndRequestCount(id int, quota int) {
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUsedQuota, id, quota)
		addNewRecord(BatchUpdateTypeRequestCount, id, 1)
		return
	}
	updateUserUsedQuotaAndRequestCount(id, quota, 1)
}

func updateUserUsedQuotaAndRequestCount(id int, quota int, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"request_count": gorm.Expr("request_count + ?", count),
		},
	).Error
	if err != nil {
		common.SysLog("failed to update user used quota and request count: " + err.Error())
		return
	}

	//// 更新缓存
	//if err := invalidateUserCache(id); err != nil {
	//	common.SysError("failed to invalidate user cache: " + err.Error())
	//}
}

func updateUserUsedQuota(id int, quota int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"used_quota": gorm.Expr("used_quota + ?", quota),
		},
	).Error
	if err != nil {
		common.SysLog("failed to update user used quota: " + err.Error())
	}
}

func updateUserRequestCount(id int, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Update("request_count", gorm.Expr("request_count + ?", count)).Error
	if err != nil {
		common.SysLog("failed to update user request count: " + err.Error())
	}
}

// GetUsernameById gets username from Redis first, falls back to DB if needed
func GetUsernameById(id int, fromDB bool) (username string, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserNameCache(id, username); err != nil {
					common.SysLog("failed to update user name cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		username, err := getUserNameCache(id)
		if err == nil {
			return username, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select("username").Find(&username).Error
	if err != nil {
		return "", err
	}

	return username, nil
}

func IsLinuxDOIdAlreadyTaken(linuxDOId string) bool {
	var user User
	err := DB.Unscoped().Where("linux_do_id = ?", linuxDOId).First(&user).Error
	return !errors.Is(err, gorm.ErrRecordNotFound)
}

func (user *User) FillUserByLinuxDOId() error {
	if user.LinuxDOId == "" {
		return errors.New("linux do id is empty")
	}
	err := DB.Where("linux_do_id = ?", user.LinuxDOId).First(user).Error
	return err
}

func RootUserExists() bool {
	var user User
	err := DB.Where("role = ?", common.RoleRootUser).First(&user).Error
	if err != nil {
		return false
	}
	return true
}

// ===================== 订阅相关辅助方法 =====================

// GetEffectiveAutoWalletFallback 获取用户有效的钱包自动回退设置
// 实现"用户设置 > 系统默认"的继承逻辑
// 从 Setting JSON 中读取 auto_wallet_fallback_explicit 标记判断用户是否显式设置
// 返回值：(是否自动回退, 是否使用的系统默认值)
func (user *User) GetEffectiveAutoWalletFallback(systemDefault bool) (bool, bool) {
	// 解析 Setting JSON
	var settings dto.UserSetting
	if user.Setting != "" {
		err := json.Unmarshal([]byte(user.Setting), &settings)
		if err == nil && settings.AutoWalletFallbackExplicit != nil {
			// 用户已显式设置过，使用用户设置
			return user.AutoWalletFallback, false
		}
	}

	// 用户未显式设置，使用系统默认值
	return systemDefault, true
}

// UpdateAutoWalletFallback 更新用户的钱包自动回退设置
// 同时在 Setting JSON 中标记用户已显式设置
func UpdateAutoWalletFallback(userId int, autoFallback bool) error {
	// 获取用户
	var user User
	err := DB.First(&user, "id = ?", userId).Error
	if err != nil {
		return err
	}

	// 解析现有设置
	var settings dto.UserSetting
	if user.Setting != "" {
		json.Unmarshal([]byte(user.Setting), &settings)
	}

	// 标记为已显式设置
	explicit := true
	settings.AutoWalletFallbackExplicit = &explicit
	settings.AutoWalletFallback = autoFallback

	// 序列化回 JSON
	settingData, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	settingStr := string(settingData)

	// 更新数据库
	err = DB.Model(&User{}).Where("id = ?", userId).
		Updates(map[string]interface{}{
			"auto_wallet_fallback": autoFallback,
			"setting":              settingStr,
		}).Error
	if err != nil {
		return err
	}

	// 刷新用户设置缓存
	_ = updateUserSettingCache(userId, settingStr)

	return nil
}

// UpdateAutoWalletFallbackWithTx 在事务中更新用户的钱包自动回退设置
// 同时在 Setting JSON 中标记用户已显式设置
//
// 返回值:
//   - onCommit: 事务提交后需要调用的回调函数，用于刷新 Redis 缓存。
//     调用方必须在事务成功提交后调用此函数，忽略其返回的错误（缓存刷新失败不影响业务）。
//     示例:
//     onCommit, err := UpdateAutoWalletFallbackWithTx(tx, userId, true)
//     if err != nil { return err }
//     if err := tx.Commit().Error; err != nil { return err }
//     onCommit() // 事务提交后刷新缓存
//   - err: 数据库操作错误
func UpdateAutoWalletFallbackWithTx(tx *gorm.DB, userId int, autoFallback bool) (onCommit func(), err error) {
	// 获取用户
	var user User
	err = tx.First(&user, "id = ?", userId).Error
	if err != nil {
		return nil, err
	}

	// 解析现有设置
	var settings dto.UserSetting
	if user.Setting != "" {
		json.Unmarshal([]byte(user.Setting), &settings)
	}

	// 标记为已显式设置
	explicit := true
	settings.AutoWalletFallbackExplicit = &explicit
	settings.AutoWalletFallback = autoFallback

	// 序列化回 JSON
	settingData, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	settingStr := string(settingData)

	// 更新数据库
	err = tx.Model(&User{}).Where("id = ?", userId).
		Updates(map[string]interface{}{
			"auto_wallet_fallback": autoFallback,
			"setting":              settingStr,
		}).Error
	if err != nil {
		return nil, err
	}

	// 返回事务提交后的缓存刷新回调
	onCommit = func() {
		_ = updateUserSettingCache(userId, settingStr)
	}
	return onCommit, nil
}

// ResetAutoWalletFallback 重置用户的钱包自动回退设置为继承系统默认
// 清除 Setting JSON 中的 AutoWalletFallbackExplicit 标记，使用户恢复为继承系统默认值
func ResetAutoWalletFallback(userId int) error {
	// 获取用户
	var user User
	err := DB.First(&user, "id = ?", userId).Error
	if err != nil {
		return err
	}

	// 获取系统默认值
	systemDefault := common.GetAutoWalletFallbackDefault()

	// 解析现有设置
	var settings dto.UserSetting
	if user.Setting != "" {
		json.Unmarshal([]byte(user.Setting), &settings)
	}

	// 清除 explicit 标记（设置为 nil），并将 AutoWalletFallback 设置为系统默认值
	// 这样即使客户端直接读取 setting JSON，也能获得正确的值
	settings.AutoWalletFallbackExplicit = nil
	settings.AutoWalletFallback = systemDefault

	// 序列化回 JSON
	settingData, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	settingStr := string(settingData)

	// 更新数据库
	err = DB.Model(&User{}).Where("id = ?", userId).
		Updates(map[string]interface{}{
			"auto_wallet_fallback": systemDefault,
			"setting":              settingStr,
		}).Error
	if err != nil {
		return err
	}

	// 刷新用户设置缓存
	_ = updateUserSettingCache(userId, settingStr)

	return nil
}

// InitializeAutoWalletFallbackForUser 为单个用户初始化 AutoWalletFallback 默认值
// 用于新用户创建时或存量用户迁移时的初始化
// 如果用户已有显式设置（AutoWalletFallbackExplicit != nil），则不做任何修改
func InitializeAutoWalletFallbackForUser(userId int, systemDefault bool) error {
	// 获取用户
	user, err := GetUserById(userId, false)
	if err != nil {
		return err
	}

	// 解析现有 Setting JSON
	var settings dto.UserSetting
	if user.Setting != "" {
		err = json.Unmarshal([]byte(user.Setting), &settings)
		if err != nil {
			return err
		}
	}

	// 如果用户已有显式设置，不做修改
	if settings.AutoWalletFallbackExplicit != nil {
		return nil
	}

	// 使用系统默认值初始化（不设置 AutoWalletFallbackExplicit，保持 nil 表示未显式设置）
	return DB.Model(&User{}).
		Where("id = ?", userId).
		Update("auto_wallet_fallback", systemDefault).Error
}

// InitializeAutoWalletFallbackForAllUsers 批量初始化所有存量用户的 AutoWalletFallback 默认值
// 用于系统迁移，为所有未显式设置的用户设置系统默认值
// 返回初始化的用户数量
func InitializeAutoWalletFallbackForAllUsers(systemDefault bool) (int64, error) {
	// 查询所有用户
	var users []*User
	if err := DB.Find(&users).Error; err != nil {
		return 0, err
	}

	var initializedCount int64 = 0

	// 遍历用户，逐个初始化
	for _, user := range users {
		// 解析 Setting JSON
		var settings dto.UserSetting
		if user.Setting != "" {
			err := json.Unmarshal([]byte(user.Setting), &settings)
			if err != nil {
				// 跳过解析失败的用户
				continue
			}
		}

		// 如果用户已有显式设置，跳过
		if settings.AutoWalletFallbackExplicit != nil {
			continue
		}

		// 使用系统默认值初始化
		err := DB.Model(&User{}).
			Where("id = ?", user.Id).
			Update("auto_wallet_fallback", systemDefault).Error

		if err == nil {
			initializedCount++
		}
	}

	return initializedCount, nil
}

// UpdateUserAutoWalletFallback 更新用户的自动兜底配置
func UpdateUserAutoWalletFallback(userId int, enabled bool) error {
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	err := DB.Model(&User{}).
		Where("id = ?", userId).
		Update("auto_wallet_fallback", enabled).Error

	return err
}

// LockUserRowForUpdateWithTx 在事务中锁定用户行
// 用于并发控制场景，确保对用户相关资源的操作串行化
// 例如：创建订阅时防止并发超限
func LockUserRowForUpdateWithTx(tx *gorm.DB, userId int64) error {
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	var user User
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&user, "id = ?", userId).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("用户不存在")
		}
		return err
	}

	return nil
}
