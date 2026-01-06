# 订阅系统 API 文档

> 本文档为前端开发参考，详细描述订阅系统相关 API 接口。
>
> OpenAPI 规范文件：`docs/api/subscription-api.yaml`

## 目录

- [认证方式](#认证方式)
- [通用响应格式](#通用响应格式)
- [管理员 API](#管理员-api)
  - [套餐管理](#套餐管理)
  - [订阅管理](#订阅管理)
  - [订单管理](#订单管理)
  - [优惠券管理](#优惠券管理)
- [用户 API](#用户-api)
  - [用户订阅](#用户订阅)
  - [用户优惠券](#用户优惠券)
  - [兑换码](#兑换码)
- [公开 API](#公开-api)
- [错误码](#错误码)
- [枚举值](#枚举值)

---

## 认证方式

所有需要认证的 API 需要在请求头中携带 Token：

```
Authorization: Bearer {token}
```

- **管理员 API**：需要管理员权限（role >= 10）
- **用户 API**：需要登录用户权限
- **公开 API**：无需认证

---

## 通用响应格式

### 成功响应

```json
{
  "success": true,
  "message": "操作成功",
  "data": { ... }
}
```

### 错误响应

```json
{
  "success": false,
  "message": "错误描述"
}
```

### 分页响应

```json
{
  "success": true,
  "data": [...],
  "total": 100,
  "page": 1,
  "page_size": 20
}
```

---

## 管理员 API

### 套餐管理

#### 获取套餐列表

```
GET /api/admin/subscription-plans
```

**查询参数**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| status | string | 否 | 状态筛选：draft/active/archived |
| billing_cycle | string | 否 | 计费周期：monthly/yearly/custom |
| min_price | integer | 否 | 最低价格（分） |
| max_price | integer | 否 | 最高价格（分） |
| page | integer | 否 | 页码，默认 1 |
| page_size | integer | 否 | 每页数量，默认 20，最大 100 |

**响应示例**

```json
{
  "success": true,
  "data": [
    {
      "id": 1,
      "sku": "PLAN001",
      "name": "基础套餐",
      "description": "适合个人用户",
      "price_cents": 9900,
      "currency": "CNY",
      "billing_cycle": "monthly",
      "billing_cycle_value": 1,
      "allow_wallet_fallback": true,
      "status": "active",
      "limits": [
        {
          "id": 1,
          "plan_id": 1,
          "period": "day",
          "quota": 100000,
          "unit": "quota",
          "enabled": true,
          "window_strategy": "rolling"
        }
      ],
      "created_at": 1704067200,
      "updated_at": 1704067200
    }
  ]
}
```

#### 创建套餐

```
POST /api/admin/subscription-plans
```

**请求体**

```json
{
  "sku": "PLAN001",
  "name": "基础套餐",
  "description": "适合个人用户",
  "price_cents": 9900,
  "currency": "CNY",
  "billing_cycle": "monthly",
  "billing_cycle_value": 1,
  "allow_wallet_fallback": true,
  "status": "draft",
  "model_whitelist": "gpt-4,claude-3",
  "channel_groups": "default",
  "limits": [
    {
      "period": "day",
      "quota": 100000,
      "unit": "quota",
      "enabled": true,
      "window_strategy": "rolling"
    }
  ]
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| sku | string | 否 | SKU 编码，最大 64 字符 |
| name | string | 是 | 套餐名称，1-128 字符 |
| description | string | 否 | 套餐描述 |
| price_cents | integer | 是 | 价格（分），必须 > 0 |
| currency | string | 否 | 币种，默认 CNY |
| billing_cycle | string | 是 | 计费周期：monthly/yearly/custom |
| billing_cycle_value | integer | 否 | 自定义周期天数 |
| allow_wallet_fallback | boolean | 否 | 是否允许钱包回退 |
| status | string | 否 | 状态：draft/active/archived |
| model_whitelist | string | 否 | 模型白名单（逗号分隔） |
| channel_groups | string | 否 | 渠道组（逗号分隔） |
| limits | array | 否 | 限额配置列表 |

**limits 子对象**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| period | string | 是 | 周期：five_hours/day/week/month |
| quota | integer | 是 | 限额值，必须 > 0 |
| unit | string | 否 | 单位：quota/tokens/requests |
| enabled | boolean | 否 | 是否启用 |
| window_strategy | string | 否 | 窗口策略：rolling/fixed/natural |

#### 获取套餐详情

```
GET /api/admin/subscription-plans/:id
```

#### 更新套餐

```
PUT /api/admin/subscription-plans/:id
```

请求体格式同创建套餐。

#### 删除套餐

```
DELETE /api/admin/subscription-plans/:id
```

> 注意：只能删除草稿状态的套餐

#### 发布套餐

```
POST /api/admin/subscription-plans/:id/publish
```

将草稿状态的套餐发布为激活状态。

#### 下架套餐

```
POST /api/admin/subscription-plans/:id/unpublish
```

将激活状态的套餐下架为归档状态。

---

### 订阅管理

#### 获取订阅列表

```
GET /api/admin/subscriptions
```

**查询参数**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| user_id | integer | 否 | 用户 ID 筛选 |
| plan_id | integer | 否 | 套餐 ID 筛选 |
| status | string | 否 | 状态：pending/active/expired/cancelled |
| page | integer | 否 | 页码 |
| page_size | integer | 否 | 每页数量 |

#### 获取订阅详情

```
GET /api/admin/subscriptions/:id
```

**响应示例**

```json
{
  "success": true,
  "data": {
    "id": 1,
    "user_id": 100,
    "plan_id": 1,
    "order_id": 1,
    "status": "active",
    "start_at": 1704067200,
    "end_at": 1706745600,
    "priority": 0,
    "auto_wallet_fallback": true,
    "plan_name": "基础套餐",
    "remaining_days": 25,
    "is_expiring_soon": false,
    "usage_summary": [
      {
        "period": "day",
        "used_quota": 50000,
        "limit_quota": 100000,
        "usage_rate": 0.5,
        "is_over_limit": false
      }
    ],
    "usage_history": [
      {
        "id": 1,
        "period": "day",
        "used_quota": 80000,
        "limit_quota": 100000,
        "usage_rate": 0.8,
        "window_start": 1703980800,
        "window_end": 1704067200,
        "is_over_limit": false,
        "created_at": 1704067200
      }
    ]
  }
}
```

#### 取消订阅

```
POST /api/admin/subscriptions/:id/cancel
```

**请求体**

```json
{
  "reason": "用户申请取消"
}
```

> 注意：只能取消 active 或 pending 状态的订阅

#### 退款订阅

```
POST /api/admin/subscriptions/:id/refund
```

**请求体**

```json
{
  "amount": 5000,
  "reason": "用户申请部分退款",
  "refund_type": "manual"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| amount | integer | 是 | 退款金额（分），必须 > 0 |
| reason | string | 是 | 退款原因，最大 500 字符 |
| refund_type | string | 否 | 退款类型：manual/auto，默认 manual |

> **重要**：此 API 仅记录退款流水，不实际增加用户余额。实际退款需线下处理。

**响应示例**

```json
{
  "success": true,
  "message": "退款已记录，实际退款请走线下处理"
}
```

#### 更新订阅优先级

```
PUT /api/admin/subscriptions/:id/priority
```

**请求体**

```json
{
  "priority": 1
}
```

#### 激活订阅

```
POST /api/admin/subscriptions/:id/activate
```

将 pending 状态的订阅激活。

#### 强制过期订阅

```
POST /api/admin/subscriptions/:id/expire
```

---

### 订单管理

#### 获取订单列表

```
GET /api/admin/subscription-orders
```

**查询参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| user_id | integer | 用户 ID |
| plan_id | integer | 套餐 ID |
| status | string | 状态：pending/paid/cancelled/expired/failed |
| payment_channel | string | 支付渠道 |
| start_time | integer | 开始时间戳 |
| end_time | integer | 结束时间戳 |

#### 获取订单详情

```
GET /api/admin/subscription-orders/:id
```

#### 退款订单

```
POST /api/admin/subscription-orders/:id/refund
```

#### 取消订单

```
POST /api/admin/subscription-orders/:id/cancel
```

---

### 优惠券管理

#### 获取优惠券列表

```
GET /api/admin/subscription-coupons
```

**查询参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| status | string | 状态：active/inactive/expired |
| scope | string | 适用范围：wallet/subscription/wallet_subscription |
| type | string | 类型：discount/full_reduction/instant_reduction |
| keyword | string | 搜索关键词（code/name） |
| bind_user_id | integer | 绑定用户 ID |

#### 创建优惠券

```
POST /api/admin/subscription-coupons
```

**请求体**

```json
{
  "code": "COUPON20250101ABC123",
  "name": "新年特惠券",
  "description": "新用户专享",
  "type": "discount",
  "scope": "subscription",
  "discount_value": 20,
  "threshold_amount": 0,
  "currency": "CNY",
  "applicable_plan_ids": [1, 2],
  "total_count": 1000,
  "per_user_limit": 1,
  "valid_from": 1704067200,
  "valid_to": 1706745600,
  "status": "active"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| code | string | 是 | 领取码，格式：COUPON + YYYYMMDD + 6位随机 |
| name | string | 是 | 名称，1-128 字符 |
| description | string | 否 | 描述 |
| type | string | 是 | 类型：discount(折扣)/full_reduction(满减)/instant_reduction(立减) |
| scope | string | 是 | 适用范围：wallet/subscription/wallet_subscription |
| discount_value | integer | 是 | 折扣值（百分比 1-100 或金额分） |
| threshold_amount | integer | 否 | 满减阈值（分） |
| currency | string | 是 | 币种：CNY |
| applicable_plan_ids | array | 否 | 适用套餐 ID 列表，空表示全部 |
| total_count | integer | 是 | 总发行量 |
| per_user_limit | integer | 是 | 每用户限领数量 |
| valid_from | integer | 是 | 有效期开始时间戳 |
| valid_to | integer | 是 | 有效期结束时间戳 |
| status | string | 否 | 状态：active/inactive/expired |
| bind_user_id | integer | 否 | 专属用户 ID |

#### 更新优惠券

```
PUT /api/admin/subscription-coupons/:id
```

> 注意：code 不可修改，total_count 只能增加不能减少

#### 删除优惠券

```
DELETE /api/admin/subscription-coupons/:id
```

> 注意：只能删除未使用的优惠券

#### 绑定优惠券到兑换码

```
POST /api/admin/subscription-coupons/:id/bind-redemption
```

**请求体**

```json
{
  "redemption_id": 1,
  "bound_user_id": 100
}
```

#### 解绑优惠券

```
DELETE /api/admin/subscription-coupons/:id/unbind
```

#### 获取优惠券绑定列表

```
GET /api/admin/subscription-coupons/:id/bindings
```

---

## 用户 API

### 用户订阅

> **注意**：旧路径 `/api/subscription/self/*` 已废弃，请使用新路径 `/api/user/subscriptions/*`

#### 获取我的订阅列表

```
GET /api/user/subscriptions
```

#### 获取我的活跃订阅

```
GET /api/user/subscriptions/active
```

#### 获取订阅详情

```
GET /api/user/subscriptions/:id
```

#### 获取订阅使用量

```
GET /api/user/subscriptions/:id/usage
```

**响应示例**

```json
{
  "success": true,
  "data": {
    "subscription_id": 1,
    "periods": [
      {
        "period": "day",
        "used_quota": 50000,
        "limit_quota": 100000,
        "usage_rate": 0.5,
        "is_over_limit": false
      },
      {
        "period": "month",
        "used_quota": 500000,
        "limit_quota": 2000000,
        "usage_rate": 0.25,
        "is_over_limit": false
      }
    ]
  }
}
```

#### 获取订阅历史记录

```
GET /api/user/subscriptions/:id/history
```

#### 更新订阅设置

```
PUT /api/user/subscriptions/:id/settings
```

**请求体**

```json
{
  "auto_wallet_fallback": true,
  "bind_channel_group": "default"
}
```

#### 更新订阅自动钱包兜底

```
PUT /api/user/subscriptions/:id/auto-wallet
```

**请求体**

```json
{
  "enabled": true
}
```

**响应示例**

```json
{
  "success": true,
  "data": {
    "auto_wallet_fallback": true
  }
}
```

#### 取消订阅

```
POST /api/user/subscriptions/:id/cancel
```

**请求体**

```json
{
  "reason": "不再需要"
}
```

#### 批量更新订阅优先级

```
PUT /api/user/subscriptions/priorities
```

**请求体**

```json
{
  "subscription_ids": [3, 1, 2]
}
```

按优先级顺序排列的订阅 ID 列表（第一个优先级最高）。

#### 重新排序订阅优先级

```
POST /api/user/subscriptions/reorder
```

**请求体**

```json
{
  "subscription_ids": [3, 1, 2]
}
```

---

### 用户设置

#### 获取自动钱包兜底设置

```
GET /api/user/settings/auto-wallet-fallback
```

**响应示例**

```json
{
  "success": true,
  "data": {
    "user_setting": true,
    "system_default": false,
    "effective": true,
    "is_explicit": true,
    "using_system_default": false
  }
}
```

| 字段 | 说明 |
|------|------|
| user_setting | 用户设置的值（null 表示未设置） |
| system_default | 系统默认值 |
| effective | 当前生效的值 |
| is_explicit | 是否为用户显式设置 |
| using_system_default | 是否正在使用系统默认 |

#### 更新自动钱包兜底设置

```
PUT /api/user/settings/auto-wallet-fallback
```

**请求体**

```json
{
  "enabled": true
}
```

或重置为系统默认：

```json
{
  "reset": true
}
```

> **注意**：`enabled` 和 `reset` 不能同时提供

**响应示例**

```json
{
  "success": true,
  "data": {
    "auto_wallet_fallback": true,
    "is_explicit": true,
    "using_system_default": false
  }
}
```

---

### 用户优惠券

#### 获取我的优惠券列表

```
GET /api/user/coupons
```

**查询参数**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| status | string | 否 | 状态筛选：available/used/expired |
| page | integer | 否 | 页码，默认 1 |
| page_size | integer | 否 | 每页数量，默认 20 |

#### 领取优惠券

```
POST /api/user/coupons/claim
```

**请求体**

```json
{
  "claim_code": "COUPON20250101ABC123"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| claim_code | string | 是 | 优惠券领取码 |

**响应示例**

```json
{
  "success": true,
  "data": {
    "id": 1,
    "coupon_id": 100,
    "code": "COUPON20250101ABC123",
    "status": "available",
    "coupon_name": "新用户优惠券",
    "coupon_type": "discount",
    "discount_value": 10,
    "valid_from": 1704067200,
    "valid_to": 1706745600,
    "remaining_days": 30,
    "can_use": true
  }
}
```

#### 获取可领取的优惠券模板列表

```
GET /api/user/coupons/available
```

返回用户可以领取的优惠券模板列表（尚未领取的、有效的券）。

**响应示例**

```json
{
  "success": true,
  "data": [
    {
      "id": 1,
      "code": "NEWUSER2025",
      "name": "新用户优惠券",
      "description": "新用户专属优惠",
      "type": "discount",
      "scope": "subscription",
      "discount_value": 10,
      "threshold_amount": 0,
      "currency": "CNY",
      "valid_from": 1704067200,
      "valid_to": 1706745600,
      "remaining_count": 100,
      "remaining_days": 30
    }
  ]
}
```

#### 获取优惠券详情

```
GET /api/user/coupons/:id
```

#### 优惠券使用预览

```
POST /api/user/payment/coupon/preview
```

**请求体**

```json
{
  "user_coupon_id": 1,
  "order_amount": 10000,
  "scene": "subscription",
  "plan_id": 1
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| user_coupon_id | integer | 是 | 用户优惠券 ID |
| order_amount | integer | 是 | 订单金额（分） |
| scene | string | 是 | 使用场景：wallet/subscription |
| plan_id | integer | 否 | 套餐 ID（subscription 场景时可选） |

**响应示例**

```json
{
  "success": true,
  "data": {
    "valid": true,
    "original_amount": 10000,
    "discount_amount": 1000,
    "final_amount": 9000,
    "coupon_type": "discount",
    "threshold_met": true
  }
}
```

---

### 兑换码

#### 使用兑换码

```
POST /api/user/redemptions/use
```

**请求体**

```json
{
  "redemption_code": "REDEEM-XXXX-XXXX-XXXX",
  "redeem_option": "stack"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| redemption_code | string | 是 | 兑换码 |
| redeem_option | string | 否 | 兑换选项（存在订阅冲突时需要指定） |

**redeem_option 可选值**

| 值 | 说明 |
|------|------|
| stack | 叠加到现有订阅（同套餐） |
| coexist | 共存（保留旧订阅，创建新订阅） |
| convert | 转换（旧订阅折算到新订阅） |
| replace | 替换（取消旧订阅，创建新订阅） |
| extend | 延长（同套餐延长有效期） |

**响应示例（成功）**

```json
{
  "success": true,
  "subscription_id": 1,
  "redeem_option": "stack",
  "extended_days": 30,
  "new_end_at": 1706745600,
  "coupon_used": false,
  "discount_amount": 0,
  "idempotent": false,
  "message": "兑换成功，订阅已延长"
}
```

**响应示例（存在冲突，需要用户选择）**

```json
{
  "success": true,
  "message": "兑换预览成功",
  "conflict": {
    "has_conflict": true,
    "conflict_type": "different_plan",
    "existing_plan_id": 1,
    "existing_plan_name": "基础套餐",
    "new_plan_id": 2,
    "new_plan_name": "高级套餐",
    "available_options": ["coexist", "convert", "replace"],
    "recommended_option": "convert"
  }
}
```

> **注意**：响应为顶层字段，不包裹在 `data` 中

#### 预览兑换

```
POST /api/user/redemptions/preview
```

**请求体**

```json
{
  "redemption_code": "REDEEM-XXXX-XXXX-XXXX"
}
```

预览兑换结果，检测是否存在订阅冲突（不实际执行）。

---

## 公开 API

#### 获取可用套餐列表

```
GET /api/subscription-plans
```

返回状态为 active 的公开套餐列表，无需认证。

---

## 错误码

| HTTP 状态码 | 说明 |
|-------------|------|
| 200 | 成功（通过 success 字段区分业务成功/失败） |
| 400 | 请求参数错误 |
| 401 | 未授权（Token 无效或过期） |
| 403 | 权限不足 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |

---

## 枚举值

### 套餐状态 (subscription_plan.status)

| 值 | 说明 |
|------|------|
| draft | 草稿 |
| active | 激活 |
| archived | 归档 |

### 订阅状态 (subscription.status)

| 值 | 说明 |
|------|------|
| pending | 待激活 |
| active | 活跃 |
| expired | 已过期 |
| cancelled | 已取消 |

### 订单状态 (subscription_order.status)

| 值 | 说明 |
|------|------|
| pending | 待支付 |
| paid | 已支付 |
| cancelled | 已取消 |
| expired | 已过期 |
| failed | 失败 |

### 优惠券类型 (coupon.type)

| 值 | 说明 |
|------|------|
| discount | 折扣券（百分比） |
| full_reduction | 满减券 |
| instant_reduction | 立减券 |

### 优惠券适用范围 (coupon.scope)

| 值 | 说明 |
|------|------|
| wallet | 余额充值 |
| subscription | 订阅 |
| wallet_subscription | 充值+订阅均可 |

### 计费周期 (subscription_plan.billing_cycle)

| 值 | 说明 |
|------|------|
| monthly | 月付 |
| yearly | 年付 |
| custom | 自定义 |

### 限额周期 (subscription_plan_limit.period)

| 值 | 说明 |
|------|------|
| five_hours | 5小时 |
| day | 天 |
| week | 周 |
| month | 月 |

### 窗口策略 (subscription_plan_limit.window_strategy)

| 值 | 说明 |
|------|------|
| rolling | 滚动窗口 |
| fixed | 固定窗口 |
| natural | 自然周期 |

### 兑换选项 (redeem_option)

| 值 | 说明 |
|------|------|
| stack | 叠加到现有订阅 |
| coexist | 共存 |
| convert | 转换 |

### 退款类型 (refund_type)

| 值 | 说明 |
|------|------|
| manual | 人工退款 |
| auto | 自动退款 |

---

## 使用 Swagger UI 查看

可以使用以下方式查看交互式 API 文档：

### 方式一：在线 Swagger Editor

1. 访问 https://editor.swagger.io/
2. 导入 `docs/api/subscription-api.yaml` 文件

### 方式二：本地 Docker

```bash
docker run -p 8080:8080 -e SWAGGER_JSON=/api/subscription-api.yaml -v $(pwd)/docs/api:/api swaggerapi/swagger-ui
```

然后访问 http://localhost:8080

### 方式三：使用 redoc

```bash
npx @redocly/cli preview-docs docs/api/subscription-api.yaml
```
