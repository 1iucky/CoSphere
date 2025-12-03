# 设计规格文档: 令牌多分组优先级管理

**项目**: New API - AI网关与资产管理系统
**功能**: 令牌多分组优先级管理
**创建日期**: 2025-11-30
**文档状态**: 草稿

---

## 1. 架构设计

### 1.1 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                         前端层 (React)                        │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ EditTokenModal                                       │   │
│  │ - 多分组选择器 (带拖动排序)                          │   │
│  │ - 自动智能分组开关                                    │   │
│  │ - 功能说明提示                                        │   │
│  └─────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ TokensTable                                          │   │
│  │ - 分组优先级展示 (group1 > group2 > group3)         │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              ↓ HTTP API
┌─────────────────────────────────────────────────────────────┐
│                      后端层 (Go + Gin)                        │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ controller/token.go                                  │   │
│  │ - AddToken() / UpdateToken()                         │   │
│  │ - 验证并保存多分组配置                                │   │
│  └─────────────────────────────────────────────────────┘   │
│                              ↓                               │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ middleware/distributor.go                            │   │
│  │ - Distribute() 中间件                                 │   │
│  │ - 多分组优先级转发逻辑                                │   │
│  └─────────────────────────────────────────────────────┘   │
│                              ↓                               │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ service/channel_select.go (新增)                     │   │
│  │ - SelectChannelWithPriority()                        │   │
│  │ - 按优先级遍历分组选择渠道                            │   │
│  │ - 自动智能分组降级逻辑                                │   │
│  └─────────────────────────────────────────────────────┘   │
│                              ↓                               │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ model/token.go                                       │   │
│  │ - Token struct (新增 GroupPriorities 字段)           │   │
│  │ - GetGroupPriorities() / SetGroupPriorities()        │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                       存储层                                 │
│  ┌─────────────┐           ┌───────────────────────────┐   │
│  │   Redis     │           │   PostgreSQL/MySQL        │   │
│  │   缓存层     │  ←同步→   │   持久化层                 │   │
│  │  token:xxx  │           │   tokens 表                │   │
│  └─────────────┘           └───────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### 1.2 分层职责

#### Controller 层
- **职责**: 处理 HTTP 请求、参数验证、响应格式化
- **核心逻辑**:
  - 验证多分组配置的合法性
  - 调用 Model 层保存数据
  - 不包含业务逻辑

#### Middleware 层
- **职责**: 请求拦截、令牌验证、分组选择
- **核心逻辑**:
  - **认证阶段** (`middleware/auth.go`): 令牌校验通过后,必须将完整 Token 实例缓存到 context,避免后续重复查询数据库
    - 需在 `SetupContextForToken()` 中添加: `c.Set("token", token)` 或使用新增常量 `ContextKeyToken`
    - 已有的 Redis 缓存 (`model/token.go:128-167`) 会自动包含新增的 `GroupPriorities` 和 `AutoSmartGroup` 字段
  - **分发阶段** (`middleware/distributor.go`): 从 context 获取缓存的 token 实例,不再查询数据库
  - 调用 Service 层选择合适的渠道
  - 更新 `ContextKeyUsingGroup` 供定价、配额、日志等后续环节使用

#### Service 层
- **职责**: 实现核心业务逻辑
- **核心逻辑**:
  - 多分组优先级遍历
  - 自动智能分组降级
  - 渠道可用性判断

#### Model 层
- **职责**: 数据模型定义、数据库操作、缓存操作
- **核心逻辑**:
  - Token 结构体扩展
  - GroupPriorities JSON 序列化/反序列化
  - Redis 缓存同步

---

## 2. 数据模型设计

### 2.1 数据库表结构

#### tokens 表 (已有表，新增字段)

```sql
-- 新增字段
ALTER TABLE tokens ADD COLUMN group_priorities VARCHAR(2048) DEFAULT '' COMMENT 'JSON格式的多分组优先级配置';
ALTER TABLE tokens ADD COLUMN auto_smart_group BOOLEAN DEFAULT false COMMENT '是否启用自动智能分组';

-- 保留原有 group 字段用于向后兼容
-- group VARCHAR(255) DEFAULT '' COMMENT '单分组(向后兼容)';
```

### 2.2 Go 数据结构

#### Token 结构体扩展

```go
// model/token.go

type Token struct {
    Id                 int            `json:"id"`
    UserId             int            `json:"user_id" gorm:"index"`
    Key                string         `json:"key" gorm:"type:char(48);uniqueIndex"`
    Status             int            `json:"status" gorm:"default:1"`
    Name               string         `json:"name" gorm:"index"`
    CreatedTime        int64          `json:"created_time" gorm:"bigint"`
    AccessedTime       int64          `json:"accessed_time" gorm:"bigint"`
    ExpiredTime        int64          `json:"expired_time" gorm:"bigint;default:-1"`
    RemainQuota        int            `json:"remain_quota" gorm:"default:0"`
    UnlimitedQuota     bool           `json:"unlimited_quota"`
    ModelLimitsEnabled bool           `json:"model_limits_enabled"`
    ModelLimits        string         `json:"model_limits" gorm:"type:varchar(1024);default:''"`
    AllowIps           *string        `json:"allow_ips" gorm:"default:''"`
    Group              string         `json:"group" gorm:"default:''"` // 向后兼容
    GroupPriorities    string         `json:"group_priorities" gorm:"type:varchar(2048);default:''"` // 新增
    AutoSmartGroup     bool           `json:"auto_smart_group" gorm:"default:false"` // 新增
    DeletedAt          gorm.DeletedAt `gorm:"index"`
}

// GroupPriority 分组优先级结构
type GroupPriority struct {
    Group    string `json:"group"`
    Priority int    `json:"priority"`
}

// GetGroupPriorities 获取分组优先级列表
func (token *Token) GetGroupPriorities() ([]GroupPriority, error) {
    if token.GroupPriorities == "" {
        // 向后兼容：如果没有配置多分组，使用 Group 字段
        if token.Group != "" {
            return []GroupPriority{{Group: token.Group, Priority: 1}}, nil
        }
        return []GroupPriority{}, nil
    }

    var priorities []GroupPriority
    err := json.Unmarshal([]byte(token.GroupPriorities), &priorities)
    if err != nil {
        return nil, err
    }

    // 按优先级排序
    sort.Slice(priorities, func(i, j int) bool {
        return priorities[i].Priority < priorities[j].Priority
    })

    return priorities, nil
}

// SetGroupPriorities 设置分组优先级列表
func (token *Token) SetGroupPriorities(priorities []GroupPriority) error {
    if len(priorities) == 0 {
        token.GroupPriorities = ""
        return nil
    }

    // 验证优先级
    for i, p := range priorities {
        if p.Group == "" {
            return errors.New("分组名称不能为空")
        }
        if p.Priority < 1 {
            return errors.New("优先级必须大于0")
        }
        // 检查重复
        for j := i + 1; j < len(priorities); j++ {
            if priorities[j].Group == p.Group {
                return errors.New("分组不能重复")
            }
        }
    }

    data, err := json.Marshal(priorities)
    if err != nil {
        return err
    }

    token.GroupPriorities = string(data)

    // 同步更新 Group 字段为第一个分组（向后兼容）
    if len(priorities) > 0 {
        sort.Slice(priorities, func(i, j int) bool {
            return priorities[i].Priority < priorities[j].Priority
        })
        token.Group = priorities[0].Group
    }

    return nil
}
```

### 2.3 前端数据结构

```typescript
// web/src/types/token.ts

interface GroupPriority {
  group: string;
  priority: number;
}

interface Token {
  id: number;
  user_id: number;
  name: string;
  status: number;
  created_time: number;
  accessed_time: number;
  expired_time: number;
  remain_quota: number;
  unlimited_quota: boolean;
  model_limits_enabled: boolean;
  model_limits: string;
  allow_ips: string;
  group: string; // 单分组（向后兼容）
  group_priorities: string; // JSON 字符串
  auto_smart_group: boolean; // 自动智能分组
}

// 辅助函数
function parseGroupPriorities(token: Token): GroupPriority[] {
  if (!token.group_priorities) {
    // 向后兼容：返回单分组
    if (token.group) {
      return [{ group: token.group, priority: 1 }];
    }
    return [];
  }
  try {
    const priorities: GroupPriority[] = JSON.parse(token.group_priorities);
    return priorities.sort((a, b) => a.priority - b.priority);
  } catch (e) {
    console.error('Failed to parse group_priorities:', e);
    return [];
  }
}

function formatGroupPrioritiesDisplay(token: Token): string {
  const priorities = parseGroupPriorities(token);
  return priorities.map(p => p.group).join(' > ');
}
```

---

## 3. API 设计

### 3.1 创建令牌 API

**现有 API**: `POST /api/token/`

**Request Body 扩展**:
```json
{
  "name": "my-token",
  "expired_time": 1735660800,
  "remain_quota": 5000000,
  "unlimited_quota": false,
  "model_limits_enabled": false,
  "model_limits": "",
  "allow_ips": "",
  "group": "default", // 保留向后兼容
  "group_priorities": [ // 新增
    { "group": "gpt-4-group", "priority": 1 },
    { "group": "claude-group", "priority": 2 },
    { "group": "default", "priority": 3 }
  ],
  "auto_smart_group": true // 新增
}
```

**Response**:
```json
{
  "success": true,
  "message": "",
  "data": null
}
```

**Controller 层处理逻辑**:
```go
// controller/token.go

func AddToken(c *gin.Context) {
    var tokenInput struct {
        Name               string                  `json:"name"`
        ExpiredTime        int64                   `json:"expired_time"`
        RemainQuota        int                     `json:"remain_quota"`
        UnlimitedQuota     bool                    `json:"unlimited_quota"`
        ModelLimitsEnabled bool                    `json:"model_limits_enabled"`
        ModelLimits        string                  `json:"model_limits"`
        AllowIps           *string                 `json:"allow_ips"`
        Group              string                  `json:"group"` // 向后兼容
        GroupPriorities    []model.GroupPriority   `json:"group_priorities"` // 新增
        AutoSmartGroup     bool                    `json:"auto_smart_group"` // 新增
    }

    err := c.ShouldBindJSON(&tokenInput)
    if err != nil {
        common.ApiError(c, err)
        return
    }

    // 验证分组配置（含数量/优先级/权限）
    if err := validateGroupPriorities(tokenInput.GroupPriorities); err != nil {
        c.JSON(http.StatusOK, gin.H{
            "success": false,
            "message": err.Error(),
        })
        return
    }
    if err := validateUserGroupAccess(c.GetInt("id"), tokenInput.GroupPriorities); err != nil {
        c.JSON(http.StatusOK, gin.H{
            "success": false,
            "message": err.Error(),
        })
        return
    }

    // 生成令牌key
    key, err := common.GenerateKey()
    if err != nil {
        c.JSON(http.StatusOK, gin.H{
            "success": false,
            "message": "生成令牌失败",
        })
        return
    }

    cleanToken := model.Token{
        UserId:             c.GetInt("id"),
        Name:               tokenInput.Name,
        Key:                key,
        CreatedTime:        common.GetTimestamp(),
        AccessedTime:       common.GetTimestamp(),
        ExpiredTime:        tokenInput.ExpiredTime,
        RemainQuota:        tokenInput.RemainQuota,
        UnlimitedQuota:     tokenInput.UnlimitedQuota,
        ModelLimitsEnabled: tokenInput.ModelLimitsEnabled,
        ModelLimits:        tokenInput.ModelLimits,
        AllowIps:           tokenInput.AllowIps,
        AutoSmartGroup:     tokenInput.AutoSmartGroup,
    }

    // 设置分组优先级
    if len(tokenInput.GroupPriorities) > 0 {
        err = cleanToken.SetGroupPriorities(tokenInput.GroupPriorities)
        if err != nil {
            c.JSON(http.StatusOK, gin.H{
                "success": false,
                "message": "分组配置错误: " + err.Error(),
            })
            return
        }
    } else if tokenInput.Group != "" {
        // 向后兼容：单分组，同时进行权限校验
        if err := validateUserGroupAccess(c.GetInt("id"), []model.GroupPriority{{Group: tokenInput.Group, Priority: 1}}); err != nil {
            c.JSON(http.StatusOK, gin.H{
                "success": false,
                "message": err.Error(),
            })
            return
        }
        cleanToken.Group = tokenInput.Group
    }

    err = cleanToken.Insert()
    if err != nil {
        common.ApiError(c, err)
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "",
    })
}
```

### 3.2 更新令牌 API

**现有 API**: `PUT /api/token/`

**处理逻辑**: 与创建类似，增加对 `group_priorities` 和 `auto_smart_group` 的更新

```go
// model/token.go 的 Update 方法需要扩展

func (token *Token) Update() (err error) {
    defer func() {
        if shouldUpdateRedis(true, err) {
            gopool.Go(func() {
                err := cacheSetToken(*token)
                if err != nil {
                    common.SysLog("failed to update token cache: " + err.Error())
                }
            })
        }
    }()
    err = DB.Model(token).Select("name", "status", "expired_time", "remain_quota", "unlimited_quota",
        "model_limits_enabled", "model_limits", "allow_ips", "group", "group_priorities", "auto_smart_group").Updates(token).Error
    return err
}
```

### 3.3 获取令牌列表 API

**现有 API**: `GET /api/token/`

**Response 扩展**:
```json
{
  "success": true,
  "message": "",
  "data": {
    "items": [
      {
        "id": 1,
        "name": "my-token",
        "status": 1,
        "group": "gpt-4-group",
        "group_priorities": "[{\"group\":\"gpt-4-group\",\"priority\":1},{\"group\":\"claude-group\",\"priority\":2}]",
        "auto_smart_group": true,
        ...
      }
    ],
    "total": 10,
    "page": 1,
    "page_size": 10
  }
}
```

---

## 4. 转发逻辑设计

### 4.1 多分组优先级选择流程

```
┌─────────────────────────────────────────────────────────┐
│ 1. middleware/distributor.go: Distribute()              │
│    - 从 token 获取 group_priorities 配置                 │
│    - 调用 SelectChannelWithPriority()                   │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 2. service/channel_select.go: SelectChannelWithPriority│
│    - 获取分组优先级列表                                   │
│    - 按优先级从高到低遍历                                 │
└─────────────────────────────────────────────────────────┘
                         ↓
        ┌────────────────┴────────────────┐
        ↓                                  ↓
┌───────────────────────┐      ┌──────────────────────────┐
│ 3a. 尝试分组1          │      │ 3b. 分组1失败             │
│ - 查找可用渠道         │  →   │ - 记录失败日志            │
│ - 模型匹配             │      │ - 尝试下一个分组          │
└───────────────────────┘      └──────────────────────────┘
        ↓ 成功                          ↓ 继续
┌───────────────────────┐      ┌──────────────────────────┐
│ 4. 返回选中渠道        │      │ 4. 遍历所有分组           │
│ - 设置 context         │      │ - 分组2, 分组3...         │
│ - 记录使用日志         │      └──────────────────────────┘
└───────────────────────┘                  ↓ 全部失败
                                ┌──────────────────────────┐
                                │ 5. 自动智能分组判断       │
                                │ - 检查 auto_smart_group   │
                                └──────────────────────────┘
                 ┌──────────────┴──────────────┐
                 ↓ true                         ↓ false
    ┌────────────────────────┐      ┌─────────────────────┐
    │ 6a. 启用智能分组        │      │ 6b. 返回错误         │
    │ - 按费率排序所有分组    │      │ - 无可用渠道         │
    │ - 从低到高尝试          │      └─────────────────────┘
    └────────────────────────┘
                 ↓
    ┌────────────────────────┐
    │ 7. 记录实际使用的分组   │
    │ - 日志记录              │
    └────────────────────────┘
```

> **上下文复用** (避免重复数据库查询)：
>
> `middleware/auth.go` 中的 `SetupContextForToken` 需在校验通过后缓存完整 `token` 实例到 context:
> ```go
> // 方式1: 直接使用 c.Set (推荐)
> c.Set("token", token)
>
> // 方式2: 或添加新常量到 constant/context_key.go
> // ContextKeyToken ContextKey = "token"
> // common.SetContextKey(c, constant.ContextKeyToken, token)
> ```
>
> 这样 `Distribute()` 可以直接从 context 获取 token,完全避免数据库查询,充分利用 Redis 缓存机制 (`model/token.go:128-167`)。

### 4.2 核心代码实现

#### service/channel_select.go (新增函数)

```go
package service

import (
    "errors"
    "fmt"

    "github.com/QuantumNous/new-api/common"
    "github.com/QuantumNous/new-api/constant"
    "github.com/QuantumNous/new-api/logger"
    "github.com/QuantumNous/new-api/model"
    "github.com/QuantumNous/new-api/setting/ratio_setting"
    "github.com/gin-gonic/gin"
)

// SelectChannelWithPriority 按分组优先级选择渠道
func SelectChannelWithPriority(c *gin.Context, token *model.Token, modelName string, retry int) (*model.Channel, string, error) {
    priorities, err := token.GetGroupPriorities()
    if err != nil {
        logger.LogError(c, "Failed to parse group priorities: "+err.Error())
        fallbackGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
        if fallbackGroup == "" {
            fallbackGroup = token.Group
        }
        return CacheGetRandomSatisfiedChannel(c, fallbackGroup, modelName, retry)
    }

    baseGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
    if baseGroup == "" {
        baseGroup = token.Group
    }
    if baseGroup == "" {
        baseGroup = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
    }

    if len(priorities) == 0 {
        return CacheGetRandomSatisfiedChannel(c, baseGroup, modelName, retry)
    }

    for _, p := range priorities {
        logger.LogDebug(c, fmt.Sprintf("Trying group: %s (priority: %d)", p.Group, p.Priority))

        channel, selectGroup, err := CacheGetRandomSatisfiedChannel(c, p.Group, modelName, retry)
        if err != nil {
            logger.LogDebug(c, fmt.Sprintf("Group %s failed: %s", p.Group, err.Error()))
            continue
        }

        if channel != nil {
            logger.LogInfo(c, fmt.Sprintf("Selected channel from group: %s", selectGroup))
            common.SetContextKey(c, constant.ContextKeySelectedGroup, selectGroup)
            common.SetContextKey(c, constant.ContextKeyUsingGroup, selectGroup)
            return channel, selectGroup, nil
        }
    }

    logger.LogWarn(c, "All configured groups failed")

    if token.AutoSmartGroup {
        logger.LogInfo(c, "Auto smart group enabled, trying fallback groups by ratio")
        return selectChannelByRatio(c, modelName, retry, priorities)
    }

    return nil, "", errors.New("所有配置的分组都无可用渠道")
}

// selectChannelByRatio 按费率从低到高选择分组
func selectChannelByRatio(c *gin.Context, modelName string, retry int, excludePriorities []model.GroupPriority) (*model.Channel, string, error) {
    userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
    usableGroups := GetUserUsableGroups(userGroup)
    ratioMap := ratio_setting.GetGroupRatioCopy()

    excludeMap := make(map[string]bool)
    for _, p := range excludePriorities {
        excludeMap[p.Group] = true
    }

    type groupWithRatio struct {
        group string
        ratio float64
    }
    candidates := make([]groupWithRatio, 0)

    for group := range usableGroups {
        if excludeMap[group] {
            continue
        }
        ratio, ok := ratioMap[group]
        if !ok {
            continue
        }
        candidates = append(candidates, groupWithRatio{
            group: group,
            ratio: ratio,
        })
    }

    if len(candidates) == 0 {
        return nil, "", errors.New("没有可用的备用分组")
    }

    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].ratio < candidates[j].ratio
    })

    for _, item := range candidates {
        logger.LogDebug(c, fmt.Sprintf("Auto smart group trying: %s (ratio: %.2f)", item.group, item.ratio))

        channel, selectGroup, err := CacheGetRandomSatisfiedChannel(c, item.group, modelName, retry)
        if err != nil {
            logger.LogDebug(c, fmt.Sprintf("Auto smart group %s failed: %s", item.group, err.Error()))
            continue
        }

        if channel != nil {
            logger.LogInfo(c, fmt.Sprintf("Auto smart group selected: %s", selectGroup))
            common.SetContextKey(c, constant.ContextKeySelectedGroup, selectGroup)
            common.SetContextKey(c, constant.ContextKeyUsingGroup, selectGroup)
            common.SetContextKey(c, constant.ContextKeyAutoSmartGroupUsed, true)
            return channel, selectGroup, nil
        }
    }

    return nil, "", errors.New("自动智能分组也无法找到可用渠道")
}
```

#### middleware/distributor.go (修改)

```go
// 修改 Distribute 函数中的渠道选择逻辑

func Distribute() func(c *gin.Context) {
    return func(c *gin.Context) {
        // ... 现有代码 ...

        if shouldSelectChannel {
            // ... 模型验证代码 ...

            var selectGroup string
            var channel *model.Channel
            var err error

            // 检查是否是令牌请求,优先使用多分组优先级逻辑
            tokenId := c.GetInt("token_id")
            if tokenId > 0 {
                // 从 context 获取认证阶段缓存的 token 实例 (避免重复数据库查询)
                token, exists := c.Get("token")
                if exists && token != nil {
                    if t, ok := token.(*model.Token); ok {
                        // 使用多分组优先级选择逻辑
                        channel, selectGroup, err = service.SelectChannelWithPriority(c, t, modelRequest.Model, 0)
                    } else {
                        // token 类型断言失败,降级到单分组逻辑
                        logger.LogWarn(c, "Token type assertion failed, fallback to single group")
                        usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
                        channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(c, usingGroup, modelRequest.Model, 0)
                    }
                } else {
                    // 未找到缓存的 token (理论上不应该出现,说明认证阶段未正确缓存)
                    logger.LogError(c, "Token not found in context, please check auth middleware")
                    usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
                    channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(c, usingGroup, modelRequest.Model, 0)
                }
            } else {
                // 非令牌请求(如管理员指定渠道),使用原有逻辑
                usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
                channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(c, usingGroup, modelRequest.Model, 0)
            }

            if err != nil {
                message := fmt.Sprintf("获取模型 %s 的可用渠道失败: %s", modelRequest.Model, err.Error())
                abortWithOpenAiMessage(c, http.StatusServiceUnavailable, message, string(types.ErrorCodeModelNotFound))
                return
            }

            if channel == nil {
                abortWithOpenAiMessage(c, http.StatusServiceUnavailable, fmt.Sprintf("模型 %s 无可用渠道", modelRequest.Model), string(types.ErrorCodeModelNotFound))
                return
            }

            // SelectChannelWithPriority 内部已更新 ContextKeyUsingGroup
            // 此处无需重复设置,确保 service 层返回的 selectGroup 已生效
        }

        common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
        SetupContextForSelectedChannel(c, channel, modelRequest.Model)
        c.Next()
    }
}
```

---

## 5. 缓存策略设计

### 5.1 Redis 缓存结构

```
Key: token:<HMAC(token_key)>
Type: Hash
Fields:
  - id: token ID
  - user_id: 用户ID
  - name: 令牌名称
  - status: 状态
  - group: 单分组（向后兼容）
  - group_priorities: 多分组优先级 JSON
  - auto_smart_group: 自动智能分组开关
  - remain_quota: 剩余额度
  - ... 其他字段
TTL: 按配置设置（default: 3600秒）
```

### 5.2 缓存同步逻辑

```go
// model/token_cache.go

func cacheSetToken(token Token) error {
    key := common.GenerateHMAC(token.Key)
    token.Clean() // 清理敏感信息

    // 将整个 token 对象序列化为 JSON 存入 Redis
    err := common.RedisHSetObj(fmt.Sprintf("token:%s", key), &token,
        time.Duration(common.RedisKeyCacheSeconds())*time.Second)
    if err != nil {
        return err
    }
    return nil
}

func cacheGetTokenByKey(key string) (*Token, error) {
    hmacKey := common.GenerateHMAC(key)
    if !common.RedisEnabled {
        return nil, fmt.Errorf("redis is not enabled")
    }
    var token Token
    err := common.RedisHGetObj(fmt.Sprintf("token:%s", hmacKey), &token)
    if err != nil {
        return nil, err
    }
    token.Key = key
    return &token, nil
}
```

### 5.3 缓存失效策略

- **创建令牌**: 自动写入缓存
- **更新令牌**: 更新数据库后异步更新缓存
- **删除令牌**: 删除数据库后异步删除缓存
- **缓存过期**: 按 TTL 自动过期，查询时重新加载

---

## 6. 前端设计

### 6.1 EditTokenModal 组件改造

#### 6.1.1 多分组选择器组件

```jsx
// web/src/components/table/tokens/modals/EditTokenModal.jsx

import React from 'react';
import { Form, Select, Tag, Space, Button, Tooltip } from '@douyinfe/semi-ui';
import { IconArrowUp, IconArrowDown, IconDelete } from '@douyinfe/semi-icons';

const GroupPrioritiesSelector = ({ value = [], onChange, groups }) => {
  const emit = (list) => {
    const normalized = list.map((item, index) => ({
      ...item,
      priority: index + 1,
    }));
    onChange?.(normalized);
  };

  const handleAddGroup = (group) => {
    if (!group) return;
    if (value.some((g) => g.group === group)) {
      return;
    }
    emit([...value, { group, priority: value.length + 1 }]);
  };

  const handleRemoveGroup = (group) => {
    emit(value.filter((g) => g.group !== group));
  };

  const handleSwap = (from, to) => {
    if (to < 0 || to >= value.length) return;
    const cloned = [...value];
    [cloned[from], cloned[to]] = [cloned[to], cloned[from]];
    emit(cloned);
  };

  return (
    <div>
      <Select
        placeholder='选择分组'
        optionList={groups.filter((g) => !value.find((sg) => sg.group === g.value))}
        onChange={handleAddGroup}
        showClear
        style={{ width: '100%', marginBottom: 12 }}
      />

      <div>
        {value.map((item, index) => (
          <div
            key={item.group}
            className='flex items-center gap-2 mb-2 p-2 border rounded'
          >
            <Tag color='blue' size='large'>
              ☆{index + 1}
            </Tag>
            <span className='flex-1 font-medium'>{item.group}</span>
            <Space>
              <Button
                icon={<IconArrowUp />}
                size='small'
                disabled={index === 0}
                onClick={() => handleSwap(index, index - 1)}
              />
              <Button
                icon={<IconArrowDown />}
                size='small'
                disabled={index === value.length - 1}
                onClick={() => handleSwap(index, index + 1)}
              />
              <Button
                icon={<IconDelete />}
                size='small'
                type='danger'
                onClick={() => handleRemoveGroup(item.group)}
              />
            </Space>
          </div>
        ))}
      </div>

      {value.length > 0 && (
        <div className='mt-2 text-sm text-gray-600'>
          当前优先级顺序: {value.map((g) => g.group).join(' > ')}
        </div>
      )}
    </div>
  );
};
```

#### 6.1.2 表单字段集成

```jsx
// EditTokenModal.jsx 表单部分

<Form.Slot
  label={
    <Space>
      <span>令牌分组优先级</span>
      <Tooltip content="配置多个分组,系统将按优先级顺序依次尝试。数字越小优先级越高。">
        <IconHelpCircle />
      </Tooltip>
    </Space>
  }
>
  <GroupPrioritiesSelector
    value={formApi.getValue('group_priorities')}
    onChange={(value) => formApi.setValue('group_priorities', value)}
    groups={groups}
  />
  <div className="text-xs text-gray-500 mt-1">
    请求转发时,系统会按优先级从高到低依次尝试各分组,直到成功或全部失败。
  </div>
</Form.Slot>

<Form.Switch
  field="auto_smart_group"
  label={
    <Space>
      <span>自动智能分组</span>
      <Tooltip content="当所有配置的分组都失败时,自动从其他可用分组中按费率从低到高选择。">
        <IconHelpCircle />
      </Tooltip>
    </Space>
  }
  extraText="启用后,配置的分组全部失败时,系统会自动尝试其他可用分组,按费率从低到高选择,提高请求成功率。"
  size="large"
/>
```

### 6.2 TokensTable 列展示

```jsx
// web/src/components/table/tokens/TokensColumnDefs.jsx

{
  title: '分组',
  dataIndex: 'group_priorities',
  key: 'group_priorities',
  render: (text, record) => {
    // 解析分组优先级
    const formatGroupDisplay = (token) => {
      if (!token.group_priorities) {
        // 向后兼容：显示单分组
        return token.group || '-';
      }
      try {
        const priorities = JSON.parse(token.group_priorities);
        const sorted = priorities.sort((a, b) => a.priority - b.priority);
        return sorted.map(p => p.group).join(' > ');
      } catch (e) {
        return token.group || '-';
      }
    };

    const display = formatGroupDisplay(record);

    return (
      <div>
        <div className="font-medium">{display}</div>
        {record.auto_smart_group && (
          <Tag size="small" color="green" className="mt-1">
            智能分组
          </Tag>
        )}
      </div>
    );
  },
}
```

---

## 7. 数据迁移方案

### 7.1 数据库迁移 SQL

```sql
-- PostgreSQL
ALTER TABLE tokens ADD COLUMN IF NOT EXISTS group_priorities VARCHAR(2048) DEFAULT '';
ALTER TABLE tokens ADD COLUMN IF NOT EXISTS auto_smart_group BOOLEAN DEFAULT false;

-- MySQL
ALTER TABLE tokens ADD COLUMN group_priorities VARCHAR(2048) DEFAULT '' COMMENT 'JSON格式的多分组优先级配置';
ALTER TABLE tokens ADD COLUMN auto_smart_group TINYINT(1) DEFAULT 0 COMMENT '是否启用自动智能分组';

-- SQLite
ALTER TABLE tokens ADD COLUMN group_priorities TEXT DEFAULT '';
ALTER TABLE tokens ADD COLUMN auto_smart_group INTEGER DEFAULT 0;
```

### 7.2 数据迁移脚本

不需要迁移现有数据，因为：
1. 保留了 `group` 字段，现有令牌继续使用单分组模式
2. 新字段 `group_priorities` 默认为空，向后兼容逻辑会使用 `group` 字段
3. `auto_smart_group` 默认为 `false`，不改变现有行为

### 7.3 回滚方案

如果需要回滚：
```sql
-- 删除新增字段
ALTER TABLE tokens DROP COLUMN group_priorities;
ALTER TABLE tokens DROP COLUMN auto_smart_group;
```

---

## 8. 错误处理与日志

### 8.1 错误类型定义

```go
// types/error.go

const (
    ErrorCodeGroupConfigInvalid = "GROUP_CONFIG_INVALID"
    ErrorCodeNoAvailableGroup   = "NO_AVAILABLE_GROUP"
    ErrorCodeAllGroupsFailed    = "ALL_GROUPS_FAILED"
)
```

### 8.2 日志记录点

```go
// 关键日志记录点

// 1. 分组选择开始
logger.LogInfo(c, fmt.Sprintf("Token %d group priorities: %v", tokenId, priorities))

// 2. 每个分组尝试
logger.LogDebug(c, fmt.Sprintf("Trying group: %s (priority: %d)", group, priority))

// 3. 分组失败
logger.LogWarn(c, fmt.Sprintf("Group %s failed: %s", group, err.Error()))

// 4. 分组成功
logger.LogInfo(c, fmt.Sprintf("Selected channel from group: %s, channel_id: %d", group, channelId))

// 5. 自动智能分组触发
logger.LogInfo(c, "Auto smart group enabled, trying fallback groups by ratio")

// 6. 最终失败
logger.LogError(c, fmt.Sprintf("All groups failed for token %d, model %s", tokenId, model))
```

---

## 9. 测试策略

### 9.1 单元测试

#### model/token_test.go
```go
func TestToken_GetGroupPriorities(t *testing.T) {
    tests := []struct {
        name          string
        token         Token
        want          []GroupPriority
        wantErr       bool
    }{
        {
            name: "正常多分组",
            token: Token{
                GroupPriorities: `[{"group":"g1","priority":1},{"group":"g2","priority":2}]`,
            },
            want: []GroupPriority{
                {Group: "g1", Priority: 1},
                {Group: "g2", Priority: 2},
            },
            wantErr: false,
        },
        {
            name: "向后兼容单分组",
            token: Token{
                Group: "default",
                GroupPriorities: "",
            },
            want: []GroupPriority{
                {Group: "default", Priority: 1},
            },
            wantErr: false,
        },
        {
            name: "无效JSON",
            token: Token{
                GroupPriorities: "invalid json",
            },
            want: nil,
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := tt.token.GetGroupPriorities()
            if (err != nil) != tt.wantErr {
                t.Errorf("GetGroupPriorities() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("GetGroupPriorities() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### 9.2 集成测试

#### service/channel_select_test.go
```go
func TestSelectChannelWithPriority(t *testing.T) {
    // Mock setup
    // ...

    tests := []struct {
        name          string
        token         *Token
        modelName     string
        wantGroup     string
        wantErr       bool
    }{
        {
            name: "优先级1成功",
            token: &Token{
                GroupPriorities: `[{"group":"high","priority":1},{"group":"low","priority":2}]`,
            },
            modelName: "gpt-4",
            wantGroup: "high",
            wantErr: false,
        },
        {
            name: "优先级1失败,降级到2",
            token: &Token{
                GroupPriorities: `[{"group":"unavailable","priority":1},{"group":"available","priority":2}]`,
            },
            modelName: "gpt-4",
            wantGroup: "available",
            wantErr: false,
        },
        {
            name: "所有分组失败,自动智能分组",
            token: &Token{
                GroupPriorities: `[{"group":"g1","priority":1}]`,
                AutoSmartGroup: true,
            },
            modelName: "gpt-4",
            wantGroup: "fallback-group", // 按费率最低的分组
            wantErr: false,
        },
    }

    // 测试实现...
}
```

### 9.3 E2E 测试场景

1. **场景1**: 创建多分组令牌
   - 前端选择3个分组并拖动排序
   - 提交保存
   - 验证数据库存储正确
   - 验证 Redis 缓存同步

2. **场景2**: 多分组优先级转发
   - 使用令牌发起请求
   - 验证按优先级顺序尝试分组
   - 验证日志记录完整

3. **场景3**: 自动智能分组降级
   - 配置的分组全部不可用
   - 启用自动智能分组
   - 验证按费率选择备用分组
   - 验证成功响应

4. **场景4**: 向后兼容性
   - 使用旧的单分组令牌
   - 验证正常工作
   - 编辑后升级为多分组
   - 验证功能正常

---

## 10. 性能优化

### 10.1 优化点

1. **缓存优先**: 令牌验证优先从 Redis 读取，减少数据库查询
2. **异步更新**: 缓存更新采用异步方式，不阻塞主流程
3. **批量查询**: 渠道查询使用现有的批量查询逻辑
4. **早返回**: 分组匹配成功后立即返回，避免不必要的遍历

### 10.2 性能指标

| 指标 | 目标 | 测量方法 |
|------|------|----------|
| 分组选择延迟 | < 50ms | 中间件耗时统计 |
| Redis 缓存命中率 | ≥ 95% | Redis 监控指标 |
| 数据库查询次数 | 缓存命中: 0次<br/>未命中: ≤1次 | SQL 日志统计 |
| QPS 支持 | ≥ 1000 | 压力测试 |

---

## 11. 安全性考虑

### 11.1 输入验证

```go
// 分组配置验证
func validateGroupPriorities(priorities []GroupPriority) error {
    if len(priorities) > 10 {
        return errors.New("最多支持10个分组")
    }

    groupMap := make(map[string]bool)
    for _, p := range priorities {
        if p.Group == "" {
            return errors.New("分组名称不能为空")
        }
        if p.Priority < 1 {
            return errors.New("优先级必须大于0")
        }
        if groupMap[p.Group] {
            return errors.New("分组不能重复")
        }
        groupMap[p.Group] = true
    }

    return nil
}
```

### 11.2 权限控制

```go
// 验证用户是否有权访问配置的分组
func validateUserGroupAccess(userId int, groupPriorities []GroupPriority) error {
    user, err := model.GetUserById(userId, false)
    if err != nil {
        return err
    }

    userGroups := GetUserUsableGroups(user.Group)

    for _, p := range groupPriorities {
        if !contains(userGroups, p.Group) {
            return errors.New(fmt.Sprintf("无权访问分组: %s", p.Group))
        }
    }

    return nil
}
```

### 11.3 注入防护

- JSON 序列化使用标准库 `encoding/json`
- 数据库操作使用 GORM 参数化查询
- 前端使用 React 自动转义

---

## 12. 监控与告警

### 12.1 监控指标

```go
// metrics/token.go

var (
    // 多分组选择计数器
    multiGroupSelectCounter = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "token_multi_group_select_total",
            Help: "Total number of multi-group selections",
        },
        []string{"group", "priority", "status"},
    )

    // 自动智能分组计数器
    autoSmartGroupCounter = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "token_auto_smart_group_total",
            Help: "Total number of auto smart group fallbacks",
        },
    )

    // 分组选择耗时
    groupSelectDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "token_group_select_duration_seconds",
            Help: "Duration of group selection",
            Buckets: prometheus.DefBuckets,
        },
        []string{"status"},
    )
)
```

### 12.2 告警规则

```yaml
groups:
  - name: token_multi_group
    rules:
      - alert: HighGroupFailureRate
        expr: rate(token_multi_group_select_total{status="failed"}[5m]) > 0.1
        for: 5m
        annotations:
          summary: "分组失败率过高"
          description: "分组选择失败率超过10%"

      - alert: FrequentAutoSmartGroup
        expr: rate(token_auto_smart_group_total[5m]) > 10
        for: 5m
        annotations:
          summary: "自动智能分组频繁触发"
          description: "可能配置的分组不可用"
```

---

## 13. 部署方案

### 13.1 灰度发布流程

1. **Phase 1 (5% 流量)**:
   - 数据库迁移（新增字段）
   - 部署后端代码
   - 监控错误日志和性能指标

2. **Phase 2 (20% 流量)**:
   - 部署前端代码
   - 允许部分用户使用新功能
   - 收集用户反馈

3. **Phase 3 (50% 流量)**:
   - 扩大测试范围
   - 性能调优

4. **Phase 4 (100% 流量)**:
   - 全量发布
   - 持续监控

### 13.2 回滚方案

- 保留 `group` 字段，可以快速回滚到单分组模式
- 新功能通过特性开关控制，可以动态关闭
- 数据库字段可以保留，不影响旧版本运行

---

## 14. 文档修订历史

| 版本 | 日期 | 作者 | 修改内容 |
|------|------|------|----------|
| 1.0 | 2025-11-30 | Claude Code | 初始版本 |
| 1.1 | 2025-11-30 | Claude Code | 根据审批反馈修订:<br/>1. 明确认证阶段需缓存 token 实例到 context<br/>2. 优化 Distribute() 逻辑避免重复数据库查询<br/>3. 确保 SelectChannelWithPriority 正确更新 ContextKeyUsingGroup<br/>4. 补充上下文复用机制说明 |
