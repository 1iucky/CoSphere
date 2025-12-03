/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

/**
 * 分组优先级接口
 * @interface GroupPriority
 */
export interface GroupPriority {
  /** 分组名称 */
  group: string;
  /** 优先级（数值越小优先级越高，从1开始） */
  priority: number;
}

/**
 * Token 令牌接口
 * @interface Token
 */
export interface Token {
  /** 令牌ID */
  id: number;
  /** 用户ID */
  user_id: number;
  /** 令牌密钥 */
  key: string;
  /** 状态：1-启用, 2-禁用 */
  status: number;
  /** 令牌名称 */
  name: string;
  /** 创建时间（Unix时间戳） */
  created_time: number;
  /** 最后访问时间（Unix时间戳） */
  accessed_time: number;
  /** 过期时间（Unix时间戳，-1表示永不过期） */
  expired_time: number;
  /** 剩余配额 */
  remain_quota: number;
  /** 是否无限配额 */
  unlimited_quota: boolean;
  /** 是否启用模型限制 */
  model_limits_enabled: boolean;
  /** 模型限制列表（逗号分隔） */
  model_limits: string;
  /** 允许的IP地址（逗号分隔） */
  allow_ips: string | null;
  /** 已使用配额 */
  used_quota: number;
  /** 单分组（向后兼容） */
  group: string;
  /** 多分组优先级配置（JSON字符串） */
  group_priorities: string;
  /** 是否启用自动智能分组 */
  auto_smart_group: boolean;
}

/**
 * 解析分组优先级配置
 * 支持向后兼容：如果 group_priorities 为空，则返回基于 group 字段的单分组
 *
 * @param token - Token对象
 * @returns 解析后的分组优先级数组（已按优先级从低到高排序）
 *
 * @example
 * ```typescript
 * const token = {
 *   group: 'default',
 *   group_priorities: '[{"group":"vip","priority":1},{"group":"standard","priority":2}]',
 *   auto_smart_group: false
 * };
 *
 * const priorities = parseGroupPriorities(token);
 * // 返回: [{ group: 'vip', priority: 1 }, { group: 'standard', priority: 2 }]
 * ```
 */
export function parseGroupPriorities(token: Partial<Token>): GroupPriority[] {
  try {
    // 如果 group_priorities 存在且不为空，解析 JSON
    if (token.group_priorities && token.group_priorities.trim() !== '') {
      const priorities = JSON.parse(token.group_priorities) as GroupPriority[];

      // 验证解析结果
      if (!Array.isArray(priorities)) {
        console.warn('group_priorities is not an array, falling back to group field');
        return fallbackToGroupField(token.group);
      }

      // 验证每个元素的格式
      const validPriorities = priorities.filter(p => {
        if (!p || typeof p !== 'object') return false;
        if (typeof p.group !== 'string' || p.group.trim() === '') return false;
        if (typeof p.priority !== 'number' || p.priority < 1) return false;
        return true;
      });

      if (validPriorities.length === 0) {
        console.warn('No valid group priorities found, falling back to group field');
        return fallbackToGroupField(token.group);
      }

      // 按优先级从低到高排序（数值越小优先级越高）
      return validPriorities.sort((a, b) => a.priority - b.priority);
    }

    // 向后兼容：使用 group 字段
    return fallbackToGroupField(token.group);

  } catch (error) {
    console.error('Failed to parse group_priorities:', error);
    // 解析失败时降级到 group 字段
    return fallbackToGroupField(token.group);
  }
}

/**
 * 降级到单分组模式（向后兼容）
 * @param group - 分组名称
 * @returns 单分组优先级数组
 */
function fallbackToGroupField(group?: string): GroupPriority[] {
  if (group && group.trim() !== '') {
    return [{ group: group.trim(), priority: 1 }];
  }
  return [];
}

/**
 * 格式化分组优先级显示
 * 将分组优先级数组格式化为用户友好的显示文本
 *
 * @param token - Token对象
 * @param options - 格式化选项
 * @returns 格式化后的显示字符串
 *
 * @example
 * ```typescript
 * const token = {
 *   group: 'default',
 *   group_priorities: '[{"group":"vip","priority":1},{"group":"standard","priority":2}]',
 *   auto_smart_group: true
 * };
 *
 * // 默认格式（箭头分隔）
 * formatGroupPrioritiesDisplay(token);
 * // 返回: "vip > standard"
 *
 * // 详细格式（包含优先级数字）
 * formatGroupPrioritiesDisplay(token, { showPriority: true });
 * // 返回: "vip(1) > standard(2)"
 *
 * // 简洁格式（逗号分隔）
 * formatGroupPrioritiesDisplay(token, { separator: ', ' });
 * // 返回: "vip, standard"
 * ```
 */
export function formatGroupPrioritiesDisplay(
  token: Partial<Token>,
  options: {
    /** 是否显示优先级数字 */
    showPriority?: boolean;
    /** 分组之间的分隔符 */
    separator?: string;
    /** 最大显示分组数（超出部分显示为 "..."） */
    maxGroups?: number;
  } = {}
): string {
  const {
    showPriority = false,
    separator = ' > ',
    maxGroups = 10
  } = options;

  try {
    const priorities = parseGroupPriorities(token);

    if (priorities.length === 0) {
      return '-';
    }

    // 限制显示数量
    const displayPriorities = priorities.slice(0, maxGroups);
    const hasMore = priorities.length > maxGroups;

    // 格式化每个分组
    const formatted = displayPriorities.map(p => {
      if (showPriority) {
        return `${p.group}(${p.priority})`;
      }
      return p.group;
    });

    // 添加省略号
    if (hasMore) {
      formatted.push('...');
    }

    return formatted.join(separator);

  } catch (error) {
    console.error('Failed to format group priorities display:', error);
    return '-';
  }
}

/**
 * 验证分组优先级配置
 *
 * @param priorities - 分组优先级数组
 * @returns 验证结果
 *
 * @example
 * ```typescript
 * const priorities = [
 *   { group: 'vip', priority: 1 },
 *   { group: 'standard', priority: 2 }
 * ];
 *
 * const result = validateGroupPriorities(priorities);
 * if (!result.valid) {
 *   console.error(result.error);
 * }
 * ```
 */
export function validateGroupPriorities(
  priorities: GroupPriority[]
): { valid: boolean; error?: string } {
  // 检查是否为数组
  if (!Array.isArray(priorities)) {
    return { valid: false, error: '分组优先级必须是数组' };
  }

  // 检查数量限制
  if (priorities.length > 10) {
    return { valid: false, error: '最多支持10个分组' };
  }

  // 检查每个元素
  const groups = new Set<string>();
  for (const p of priorities) {
    // 检查必填字段
    if (!p.group || p.group.trim() === '') {
      return { valid: false, error: '分组名称不能为空' };
    }

    // 检查优先级有效性
    if (typeof p.priority !== 'number' || p.priority < 1) {
      return { valid: false, error: '优先级必须是大于0的整数' };
    }

    // 检查重复分组
    if (groups.has(p.group)) {
      return { valid: false, error: `分组 "${p.group}" 重复` };
    }
    groups.add(p.group);
  }

  return { valid: true };
}

/**
 * 序列化分组优先级为 JSON 字符串
 *
 * @param priorities - 分组优先级数组
 * @returns JSON 字符串
 *
 * @example
 * ```typescript
 * const priorities = [
 *   { group: 'vip', priority: 1 },
 *   { group: 'standard', priority: 2 }
 * ];
 *
 * const json = serializeGroupPriorities(priorities);
 * // 返回: '[{"group":"vip","priority":1},{"group":"standard","priority":2}]'
 * ```
 */
export function serializeGroupPriorities(priorities: GroupPriority[]): string {
  if (!priorities || priorities.length === 0) {
    return '';
  }

  try {
    // 验证数据
    const validation = validateGroupPriorities(priorities);
    if (!validation.valid) {
      throw new Error(validation.error);
    }

    // 排序后序列化
    const sorted = [...priorities].sort((a, b) => a.priority - b.priority);
    return JSON.stringify(sorted);

  } catch (error) {
    console.error('Failed to serialize group priorities:', error);
    throw error;
  }
}
