# 任务拆解文档: 令牌多分组优先级管理

**项目**: New API - AI网关与资产管理系统
**功能**: 令牌多分组优先级管理
**创建日期**: 2025-11-30
**文档状态**: 草稿

---

## 任务列表

### 后端任务

- [x] 1. 数据库迁移 - 新增 group_priorities 和 auto_smart_group 字段

**涉及文件**: `migrations/` 或使用 GORM AutoMigrate

**_Prompt**:
  Implement the task for spec token-multi-group-priority, first run spec-workflow-guide to get the workflow guide then implement the task:

  Role: 后端数据库工程师，负责数据库 schema 变更

  Task: 为 tokens 表添加两个新字段
  - group_priorities: VARCHAR(2048)，存储 JSON 格式的分组优先级配置
  - auto_smart_group: BOOLEAN，标识是否启用自动智能分组
  需要兼容 PostgreSQL, MySQL, SQLite 三种数据库。

  Restrictions:
  - 不能删除或修改现有的 group 字段（向后兼容）
  - 新字段必须有合理的默认值
  - 迁移脚本必须是幂等的

  _Leverage:
  - 参考现有迁移脚本格式
  - 使用 GORM AutoMigrate 或手动 SQL 脚本

  _Requirements:
  - US-1: 创建多分组令牌
  - 技术约束: 兼容 PostgreSQL, MySQL, SQLite

  Success Criteria:
  - [x] 1.1 三种数据库的迁移 SQL 已准备
  - [x] 1.2 group_priorities 字段类型为 VARCHAR(2048)，默认值为空字符串
  - [x] 1.3 auto_smart_group 字段类型为 BOOLEAN，默认值为 false
  - [x] 1.4 迁移脚本可以安全重复执行
  - [x] 1.5 现有 tokens 表数据不受影响

  Instructions:
  1. Mark this task as in-progress in tasks.md (change [ ] to [-])
  2. Create migration script or use GORM AutoMigrate
  3. Test migration on local database
  4. Verify existing data integrity
  5. Use log-implementation tool to record migration details
  6. Mark task as completed in tasks.md (change [-] to [x])
_

---

- [x] 2. Model 层 - 扩展 Token 结构体和方法

**涉及文件**: `model/token.go`

**_Prompt**:
  Implement the task for spec token-multi-group-priority, first run spec-workflow-guide to get the workflow guide then implement the task:

  Role: 后端 Go 开发工程师，负责数据模型层实现

  Task: 扩展 Token 结构体并实现分组优先级相关方法
  1. 在 Token 结构体中添加 GroupPriorities string 和 AutoSmartGroup bool
  2. 定义 GroupPriority 结构体
  3. 实现 GetGroupPriorities() 方法 - 解析 JSON 并返回排序后的分组列表
  4. 实现 SetGroupPriorities() 方法 - 验证并设置分组优先级
  5. 更新 Update() 方法以包含新字段

  Restrictions:
  - 保持向后兼容：如果 group_priorities 为空，GetGroupPriorities() 应返回基于 group 字段的单分组
  - SetGroupPriorities() 必须验证：最多10个分组、优先级>0、无重复分组
  - 遵循现有代码风格

  _Leverage:
  - 现有的 Token 结构体定义
  - encoding/json 进行 JSON 序列化
  - sort 包进行排序

  _Requirements:
  - US-1: 创建多分组令牌
  - US-2: 编辑令牌分组优先级
  - 业务约束: 最多10个分组

  Success Criteria:
  - [x] 2.1 Token 结构体新增 GroupPriorities string 和 AutoSmartGroup bool
  - [x] 2.2 GroupPriority 结构体定义（Group string, Priority int）
  - [x] 2.3 GetGroupPriorities() 正确解析 JSON 并排序
  - [x] 2.4 GetGroupPriorities() 支持向后兼容（空时返回 group 字段）
  - [x] 2.5 SetGroupPriorities() 包含完整验证逻辑
  - [x] 2.6 SetGroupPriorities() 同步更新 Group 字段
  - [x] 2.7 Update() 方法的 Select 子句包含新字段
  - [x] 2.8 代码有适当的错误处理和注释

  Instructions:
  1. Mark this task as in-progress in tasks.md
  2. Read existing model/token.go
  3. Implement struct changes and methods as specified in design.md
  4. Add unit tests for GetGroupPriorities() and SetGroupPriorities()
  5. Test backward compatibility
  6. Use log-implementation tool with detailed artifacts (functions, validation logic)
  7. Mark task as completed in tasks.md
_

---

- [x] 3. 缓存层 - 更新 Redis 缓存同步逻辑

**涉及文件**: `model/token_cache.go`

**_Prompt**:
  Implement the task for spec token-multi-group-priority, first run spec-workflow-guide to get the workflow guide then implement the task:

  Role: 后端缓存工程师，负责 Redis 缓存一致性

  Task: 确保新增的 group_priorities 和 auto_smart_group 字段被正确缓存到 Redis
  - 验证 cacheSetToken() 能够序列化新字段
  - 验证 cacheGetTokenByKey() 能够反序列化新字段
  - 确保缓存更新逻辑包含新字段

  Restrictions:
  - 不能破坏现有缓存逻辑
  - 必须保持缓存与数据库的一致性
  - 遵循现有的异步更新模式

  _Leverage:
  - 现有的 cacheSetToken() 和 cacheGetTokenByKey() 函数
  - common.RedisHSetObj() 和 common.RedisHGetObj()
  - Token 结构体的 JSON tags

  _Requirements:
  - 性能要求: Redis 缓存命中率 ≥ 95%
  - 非功能需求: 缓存与数据库一致性

  Success Criteria:
  - [x] 3.1 cacheSetToken() 正确缓存 group_priorities 和 auto_smart_group
  - [x] 3.2 cacheGetTokenByKey() 正确读取并反序列化新字段
  - [x] 3.3 缓存 TTL 设置合理
  - [x] 3.4 编写测试验证缓存读写

  Instructions:
  1. Mark this task as in-progress in tasks.md
  2. Read model/token_cache.go
  3. Verify Token struct JSON serialization includes new fields
  4. Test cache write and read
  5. Verify cache-database consistency
  6. Use log-implementation tool to record changes
  7. Mark task as completed in tasks.md
_

---

- [x] 4. Service 层 - 实现多分组优先级选择逻辑

**涉及文件**: `service/channel_select.go`

**_Prompt**:
  Implement the task for spec token-multi-group-priority, first run spec-workflow-guide to get the workflow guide then implement the task:

  Role: 后端业务逻辑工程师，负责核心转发逻辑

  Task: 实现多分组优先级选择和自动智能分组降级逻辑
  1. 新增 SelectChannelWithPriority() 函数：按优先级遍历分组选择渠道
  2. 新增 selectChannelByRatio() 函数：按费率从低到高尝试其他分组
  3. 处理边界情况和错误

  Restrictions:
  - 必须记录详细的调试日志
  - 优先级为空时应降级到现有逻辑
  - 遵循 SOLID 原则
  - 性能要求: 延迟 < 50ms

  _Leverage:
  - service.CacheGetRandomSatisfiedChannel()
  - service.GetUserUsableGroups()
  - ratio_setting.GetGroupRatioCopy()
  - logger.LogDebug/LogInfo/LogWarn/LogError

  _Requirements:
  - US-4: 多分组优先级转发
  - US-5: 自动智能分组
  - 性能要求: 延迟 < 50ms

  Success Criteria:
  - [x] 4.1 SelectChannelWithPriority() 正确实现优先级遍历
  - [x] 4.2 按优先级顺序尝试分组并复用 CacheGetRandomSatisfiedChannel（保持 auto 逻辑）
  - [x] 4.3 分组失败时记录日志并继续下一个
  - [x] 4.4 selectChannelByRatio() 仅在用户可用分组集合内按倍率排序
  - [x] 4.5 自动智能分组逻辑正确触发
  - [x] 4.6 成功后把 ContextKeyUsingGroup/ContextKeySelectedGroup 更新为实际分组
  - [x] 4.7 日志记录完整
  - [x] 4.8 性能符合要求

  Instructions:
  1. Mark this task as in-progress in tasks.md
  2. Read existing service/channel_select.go
  3. Implement SelectChannelWithPriority() as specified in design.md
  4. Implement selectChannelByRatio()
  5. Add comprehensive logging
  6. Write unit tests
  7. Performance test to ensure < 50ms latency
  8. Use log-implementation tool with detailed artifacts
  9. Mark task as completed in tasks.md
_

---

- [x] 5. Middleware 层 - 集成多分组选择逻辑

**涉及文件**: `middleware/distributor.go`, `middleware/auth.go`

**_Prompt**:
  Implement the task for spec token-multi-group-priority, first run spec-workflow-guide to get the workflow guide then implement the task:

  Role: 后端中间件工程师，负责请求分发逻辑

  Task: 修改认证和分发中间件以支持多分组优先级选择，完全避免重复数据库查询
  1. **认证阶段** (middleware/auth.go): 在 SetupContextForToken() 中缓存完整 token 实例到 context
     - 使用 c.Set("token", token) 缓存 token
     - 充分利用现有 Redis 缓存机制 (model/token.go:128-167)
  2. **分发阶段** (middleware/distributor.go): 修改 Distribute() 中间件
     - 从 context 获取认证阶段缓存的 token: token, exists := c.Get("token")
     - **禁止**调用 model.GetTokenById() 重复查询数据库
     - 如果是令牌请求，调用 service.SelectChannelWithPriority(c, token, modelName, retry)
     - SelectChannelWithPriority 内部会更新 ContextKeyUsingGroup，无需在 Distribute() 中重复设置
  3. 添加降级处理：如果 token 不存在或类型断言失败，记录错误日志并降级到原有逻辑

  Restrictions:
  - **严格禁止**在 Distribute() 中调用数据库查询 token（如 model.GetTokenById）
  - 不能破坏现有的非令牌请求逻辑
  - 必须保持向后兼容
  - 错误处理要完善

  _Leverage:
  - 现有的 Distribute() 函数和 SetupContextForToken() 函数
  - service.SelectChannelWithPriority() (已更新 ContextKeyUsingGroup)
  - service.CacheGetRandomSatisfiedChannel()
  - 现有 Redis 缓存机制 (自动包含新字段)

  _Requirements:
  - US-4: 多分组优先级转发
  - 向后兼容: 非令牌请求和单分组令牌正常工作
  - 性能要求: 避免重复数据库查询，充分利用缓存

  Success Criteria:
  - [x] 5.1 middleware/auth.go 的 SetupContextForToken() 中添加 c.Set("token", token)
  - [x] 5.2 Distribute() 从 context 获取 token: token, exists := c.Get("token")
  - [x] 5.3 **确认删除**所有 model.GetTokenById() 调用
  - [x] 5.4 令牌请求使用 SelectChannelWithPriority()
  - [x] 5.5 非令牌请求使用原有逻辑
  - [x] 5.6 token 不存在或类型断言失败时有降级处理和错误日志
  - [x] 5.7 SelectChannelWithPriority 已更新 ContextKeyUsingGroup，Distribute() 无需重复设置
  - [x] 5.8 错误处理完善
  - [x] 5.9 向后兼容测试通过
  - [x] 5.10 性能测试确认无重复数据库查询

  Instructions:
  1. Mark this task as in-progress in tasks.md
  2. Read middleware/auth.go SetupContextForToken() function
  3. Add c.Set("token", token) to cache token instance
  4. Read middleware/distributor.go Distribute() function
  5. Modify channel selection logic: use c.Get("token") instead of model.GetTokenById()
  6. Add type assertion and error handling for token
  7. Call SelectChannelWithPriority() when token exists
  8. Remove any redundant ContextKeyUsingGroup setting (already done in service layer)
  9. Test with both token and non-token requests
  10. Test backward compatibility
  11. Use log-implementation tool with integration details and performance metrics
  12. Mark task as completed in tasks.md
_

---

- [x] 6. Controller 层 - 扩展创建和更新令牌 API

**涉及文件**: `controller/token.go`

**_Prompt**:
  Implement the task for spec token-multi-group-priority, first run spec-workflow-guide to get the workflow guide then implement the task:

  Role: 后端 API 工程师，负责 HTTP API 实现

  Task: 扩展 AddToken() 和 UpdateToken() 函数以支持多分组配置
  1. AddToken(): 接收 group_priorities 数组和 auto_smart_group 布尔值
  2. 验证分组配置（最多10个、无重复、优先级有效）
  3. UpdateToken(): 支持更新这两个字段

  Restrictions:
  - 必须验证输入参数
  - 遵循现有 API 响应格式
  - 保持向后兼容（支持旧的单 group 字段）
  - 验证用户是否有权访问配置的分组

  _Leverage:
  - 现有的 AddToken() 和 UpdateToken() 函数
  - token.SetGroupPriorities() 方法
  - common.ApiError() 和 common.ApiSuccess()

  _Requirements:
  - US-1: 创建多分组令牌
  - US-2: 编辑令牌分组优先级
  - 业务约束: 最多10个分组
  - 安全性: 权限控制、参数验证

  Success Criteria:
  - [x] 6.1 AddToken() 接收 group_priorities 和 auto_smart_group
  - [x] 6.2 UpdateToken() 支持更新这两个字段
  - [x] 6.3 参数验证完善
  - [x] 6.4 向后兼容单 group 字段
  - [x] 6.5 错误消息清晰友好
  - [x] 6.6 API 响应格式一致
  - [x] 6.7 单元测试覆盖验证逻辑

  Instructions:
  1. Mark this task as in-progress in tasks.md
  2. Read controller/token.go AddToken() and UpdateToken()
  3. Modify input struct to include new fields
  4. Add validation logic
  5. Call token.SetGroupPriorities()
  6. Test API with Postman or curl
  7. Test backward compatibility
  8. Use log-implementation tool with API endpoint details
  9. Mark task as completed in tasks.md
_

---

### 前端任务

- [x] 7. 前端类型 - 定义 TypeScript 接口

**涉及文件**: `web/src/types/token.ts` 或组件内定义

**_Prompt**:
  Implement the task for spec token-multi-group-priority, first run spec-workflow-guide to get the workflow guide then implement the task:

  Role: 前端 TypeScript 开发工程师

  Task: 定义多分组优先级相关的 TypeScript 类型
  1. 定义 GroupPriority 接口
  2. 扩展 Token 接口
  3. 实现 parseGroupPriorities() 辅助函数
  4. 实现 formatGroupPrioritiesDisplay() 辅助函数

  Restrictions:
  - 向后兼容：group 字段继续保留
  - 类型定义要准确
  - 辅助函数要处理边界情况

  _Leverage:
  - 现有的 Token 接口定义
  - JSON.parse()
  - try-catch 错误处理

  _Requirements:
  - US-1, US-2, US-3

  Success Criteria:
  - [x] 7.1 GroupPriority 接口定义
  - [x] 7.2 Token 接口包含 group_priorities 和 auto_smart_group
  - [x] 7.3 parseGroupPriorities() 正确解析并排序
  - [x] 7.4 parseGroupPriorities() 处理空值和向后兼容
  - [x] 7.5 formatGroupPrioritiesDisplay() 返回正确格式
  - [x] 7.6 错误处理完善

  Instructions:
  1. Mark this task as in-progress in tasks.md
  2. Create or locate token type definitions
  3. Define interfaces and helper functions
  4. Add JSDoc comments
  5. Test with sample data
  6. Use log-implementation tool
  7. Mark task as completed in tasks.md
_

---

- [x] 8. 前端组件 - 实现 GroupPrioritiesSelector

**涉及文件**: `web/src/components/GroupPrioritiesSelector.jsx`

**_Prompt**:
  Implement the task for spec token-multi-group-priority, first run spec-workflow-guide to get the workflow guide then implement the task:

  Role: 前端 React 组件开发工程师

  Task: 创建可复用的多分组选择器组件，支持添加、删除、拖动排序
  1. 接收 value, onChange, groups 作为 props
  2. 显示分组选择下拉框（过滤已选分组）
  3. 显示已选分组列表（优先级数字、上移/下移/删除按钮）
  4. 实现上移/下移逻辑（交换位置并重新分配优先级）
  5. 实时显示优先级顺序预览

  Restrictions:
  - 使用 Semi Design 组件
  - 组件必须是受控组件
  - 优先级从 1 开始，连续递增

  _Leverage:
  - Semi Design 的 Select, Tag, Button, Space 组件
  - Semi Icons
  - React useState hook

  _Requirements:
  - US-1: 创建多分组令牌 - 支持多选和拖动排序
  - US-2: 编辑令牌分组优先级

  Success Criteria:
  - [x] 8.1 Select 组件显示可选分组
  - [x] 8.2 已选分组列表显示优先级数字
  - [x] 8.3 上移/下移按钮功能正常
  - [x] 8.4 删除按钮功能正常
  - [x] 8.5 优先级自动重新分配
  - [x] 8.6 实时预览格式正确
  - [x] 8.7 组件样式美观、响应式

  Instructions:
  1. Mark this task as in-progress in tasks.md
  2. Create new component file
  3. Implement component logic as specified in design.md
  4. Use Semi Design components
  5. Test add/remove/move operations
  6. Test edge cases
  7. Use log-implementation tool with component details
  8. Mark task as completed in tasks.md
_

---

- [x] 9. 前端表单 - 集成多分组选择器到 EditTokenModal

**涉及文件**: `web/src/components/table/tokens/modals/EditTokenModal.jsx`

**_Prompt**:
  Implement the task for spec token-multi-group-priority, first run spec-workflow-guide to get the workflow guide then implement the task:

  Role: 前端 React 表单开发工程师

  Task: 将 GroupPrioritiesSelector 组件集成到令牌编辑表单中
  1. 导入 GroupPrioritiesSelector 组件
  2. 添加"令牌分组优先级"字段（使用 Form.Slot + Tooltip + extraText）
  3. 添加"自动智能分组"开关字段（使用 Form.Switch + Tooltip + extraText）
  4. 初始化表单值时处理 group_priorities
  5. 提交时发送数据到后端

  Restrictions:
  - 保持现有表单布局和风格
  - 向后兼容单 group 字段
  - 表单验证要完善
  - 提示文本要清晰友好

  _Leverage:
  - 现有的 EditTokenModal 组件
  - GroupPrioritiesSelector 组件
  - Semi Design Form, Tooltip 组件

  _Requirements:
  - US-1: 创建多分组令牌 - 表单中显示功能说明
  - US-2: 编辑令牌分组优先级 - 表单中显示功能说明

  Success Criteria:
  - [x] 9.1 表单包含分组优先级选择器
  - [x] 9.2 表单包含自动智能分组开关
  - [x] 9.3 字段附近有清晰的 Tooltip 说明
  - [x] 9.4 extraText 提供详细说明
  - [x] 9.5 创建令牌时默认值正确
  - [x] 9.6 编辑令牌时正确加载现有配置
  - [x] 9.7 提交时数据格式正确
  - [x] 9.8 向后兼容单分组模式

  Instructions:
  1. Mark this task as in-progress in tasks.md
  2. Read existing EditTokenModal.jsx
  3. Import GroupPrioritiesSelector
  4. Add form fields as specified in design.md
  5. Handle form initialization and submission
  6. Test create and edit flows
  7. Test backward compatibility
  8. Use log-implementation tool with integration details
  9. Mark task as completed in tasks.md
_

---

- [x] 10. 前端列表 - 更新 TokensTable 分组列展示

**涉及文件**: `web/src/components/table/tokens/TokensColumnDefs.jsx`

**_Prompt**:
  Implement the task for spec token-multi-group-priority, first run spec-workflow-guide to get the workflow guide then implement the task:

  Role: 前端数据展示工程师

  Task: 修改令牌列表的分组列，支持多分组优先级展示
  1. 修改"分组"列的 render 函数
  2. 实现 formatGroupDisplay() 辅助函数
  3. 如果 auto_smart_group 为 true，显示"智能分组" Tag

  Restrictions:
  - 保持列表整体布局
  - 向后兼容单分组展示
  - 分组过多时不能破坏布局
  - 使用一致的样式

  _Leverage:
  - 现有的 TokensColumnDefs.jsx
  - Semi Design Tag 组件
  - parseGroupPriorities() 辅助函数

  _Requirements:
  - US-3: 查看令牌分组配置 - 简化文本展示

  Success Criteria:
  - [x] 10.1 多分组以 "group1 > group2 > group3" 格式展示
  - [x] 10.2 单分组向后兼容显示
  - [x] 10.3 auto_smart_group 为 true 时显示绿色 Tag
  - [x] 10.4 布局美观、不破坏表格
  - [x] 10.5 分组过多时有合理处理

  Instructions:
  1. Mark this task as in-progress in tasks.md
  2. Read existing TokensColumnDefs.jsx
  3. Modify group column render function
  4. Implement formatGroupDisplay() helper
  5. Add Tag for auto smart group
  6. Test with various scenarios
  7. Test backward compatibility
  8. Use log-implementation tool
  9. Mark task as completed in tasks.md
_

---

## 任务依赖关系

```
任务1 (数据库迁移)
  ↓
任务2 (Model 层) → 任务3 (缓存层)
  ↓
任务4 (Service 层)
  ↓
任务5 (Middleware 层)
  ↓
任务6 (Controller 层)

任务7 (前端类型)
  ↓
任务8 (前端选择器组件)
  ↓
任务9 (前端表单集成)
  ↓
任务10 (前端列表展示)
```

**建议执行顺序**: 后端: 1→2→3→4→5→6 | 前端: 7→8→9→10（后端和前端可并行）

---

## 验收检查清单

- [ ] 11. 用户能在创建令牌时选择多个分组并拖动排序
- [ ] 11.1 用户能在编辑令牌时修改分组配置
- [ ] 11.2 表单字段附近显示清晰的功能说明
- [ ] 11.3 令牌列表正确显示多分组
- [ ] 11.4 系统按优先级顺序转发请求
- [ ] 11.5 分组失败时自动降级
- [ ] 11.6 自动智能分组正常工作
- [ ] 11.7 现有单分组令牌不受影响（向后兼容）
- [ ] 11.8 多分组选择逻辑延迟 < 50ms
- [ ] 11.9 Redis 缓存命中率 ≥ 95%
- [ ] 11.10 支持 1000 QPS 并发
- [x] 11.11 单元测试通过
- [ ] 11.12 集成测试覆盖主要场景
- [x] 11.13 代码审查通过

---

**文档修订历史**:

| 版本 | 日期 | 作者 | 修改内容 |
|------|------|------|----------|
| 1.0 | 2025-11-30 | Claude Code | 初始版本 |
| 1.1 | 2025-11-30 | Claude Code | 修正格式以符合 spec-workflow 规范 |
