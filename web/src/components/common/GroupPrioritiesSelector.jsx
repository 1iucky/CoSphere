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

import React, { useState, useEffect, useMemo } from 'react';
import { Button, Select, Tag, Typography } from '@douyinfe/semi-ui';
import { IconClose, IconHandle, IconPlus } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { renderGroupOption } from '../../helpers';
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import {
  restrictToVerticalAxis,
  restrictToParentElement,
} from '@dnd-kit/modifiers';

const { Text } = Typography;

const MAX_GROUP_PRIORITIES = 10;

// 分组标签颜色映射
const getGroupColor = (groupValue) => {
  const colorMap = {
    default: 'green',
    claude: 'purple',
    openai: 'blue',
    gemini: 'orange',
    gf: 'cyan',
    official: 'light-blue',
    aws: 'yellow',
  };
  const lowerValue = groupValue?.toLowerCase() || '';
  for (const [key, color] of Object.entries(colorMap)) {
    if (lowerValue.includes(key)) {
      return color;
    }
  }
  return 'grey';
};

// 可拖拽的分组项组件
const SortableGroupItem = ({ item, index, groupInfo, onRemove, disabled }) => {
  const { t } = useTranslation();
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: item.group });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
    zIndex: isDragging ? 1000 : 'auto',
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      {...attributes}
      {...listeners}
      className={`flex items-center gap-3 p-3 rounded-lg border bg-[var(--semi-color-bg-2)] select-none ${
        isDragging
          ? 'border-[var(--semi-color-primary)] shadow-lg cursor-grabbing'
          : 'border-[var(--semi-color-border)] cursor-grab'
      } ${disabled ? 'opacity-60 cursor-not-allowed' : ''}`}
    >
      {/* 拖拽手柄图标 */}
      <IconHandle size="large" className="text-[var(--semi-color-text-2)] shrink-0" />

      {/* 优先级标签 */}
      <Text className="shrink-0 text-[var(--semi-color-text-2)] w-16">
        {t('优先级')} {index + 1}
      </Text>

      {/* 分组标签 */}
      <Tag
        color={getGroupColor(item.group)}
        size="large"
        className="shrink-0 max-w-[90px]"
        style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}
      >
        {item.group?.toUpperCase()}
      </Tag>

      {/* 分组描述 */}
      <Text
        ellipsis={{ showTooltip: true }}
        className="flex-1 text-[var(--semi-color-text-0)]"
      >
        {groupInfo?.label || item.group}
      </Text>

      {/* 分组倍率 */}
      <Text className="shrink-0 text-[var(--semi-color-text-2)]">
        {t('分组倍率')}
        {groupInfo?.ratio ?? 1}
      </Text>

      {/* 删除按钮 */}
      <Button
        icon={<IconClose />}
        size="small"
        theme="borderless"
        type="tertiary"
        onClick={(e) => {
          e.stopPropagation();
          e.preventDefault();
          onRemove(index);
        }}
        onPointerDown={(e) => e.stopPropagation()}
        disabled={disabled}
        aria-label={t('删除')}
        className="shrink-0"
      />
    </div>
  );
};

const normalizePriorities = (list = [], options = {}) => {
  if (!Array.isArray(list)) return [];
  const { sortByPriority = true } = options;
  const cloned = list.slice();
  if (sortByPriority) {
    cloned.sort((a, b) => (a.priority || 0) - (b.priority || 0));
  }
  const result = [];
  const seen = new Set();
  cloned.forEach((item) => {
    if (!item || !item.group) return;
    if (seen.has(item.group)) {
      return;
    }
    seen.add(item.group);
    result.push({
      group: item.group,
      priority: result.length + 1,
    });
  });
  return result;
};

const GroupPrioritiesSelector = ({
  value = [],
  onChange,
  availableGroups = [],
  disabled = false,
}) => {
  const { t } = useTranslation();

  // 使用内部 state 管理列表
  const [items, setItems] = useState(() => normalizePriorities(value));

  // 同步外部 value 到内部 state
  useEffect(() => {
    const normalized = normalizePriorities(value);
    setItems(normalized);
  }, [value]);

  const selectedGroups = useMemo(
    () => new Set(items.map((item) => item.group)),
    [items],
  );

  const availableOptions = useMemo(
    () => availableGroups.filter((group) => !selectedGroups.has(group.value)),
    [availableGroups, selectedGroups],
  );

  // 创建分组信息映射
  const groupInfoMap = useMemo(() => {
    const map = {};
    availableGroups.forEach((group) => {
      map[group.value] = group;
    });
    return map;
  }, [availableGroups]);

  // 拖拽传感器配置
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const updateItems = (newItems) => {
    const normalized = normalizePriorities(newItems, { sortByPriority: false });
    setItems(normalized);
    onChange?.(normalized);
  };

  const handleAddGroup = (groupValue) => {
    if (!groupValue || items.length >= MAX_GROUP_PRIORITIES) return;
    const newItems = [
      ...items,
      { group: groupValue, priority: items.length + 1 },
    ];
    updateItems(newItems);
  };

  const handleRemove = (index) => {
    const newItems = items.filter((_, i) => i !== index);
    updateItems(newItems);
  };

  const handleDragEnd = (event) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;

    const oldIndex = items.findIndex((item) => item.group === active.id);
    const newIndex = items.findIndex((item) => item.group === over.id);

    if (oldIndex !== -1 && newIndex !== -1) {
      const newItems = arrayMove(items, oldIndex, newIndex);
      updateItems(newItems);
    }
  };

  const canAddMore =
    !disabled &&
    items.length < MAX_GROUP_PRIORITIES &&
    availableOptions.length > 0;

  const noMoreOptions = availableOptions.length === 0;

  return (
    <div className="space-y-2">
      {/* 可拖拽的分组列表 */}
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragEnd={handleDragEnd}
        modifiers={[restrictToVerticalAxis, restrictToParentElement]}
      >
        <SortableContext
          items={items.map((item) => item.group)}
          strategy={verticalListSortingStrategy}
        >
          <div className="space-y-2">
            {items.map((item, index) => (
              <SortableGroupItem
                key={item.group}
                item={item}
                index={index}
                groupInfo={groupInfoMap[item.group]}
                onRemove={handleRemove}
                disabled={disabled}
              />
            ))}
          </div>
        </SortableContext>
      </DndContext>

      {/* 添加分组下拉框 */}
      {noMoreOptions ? (
        <div className="flex items-center gap-2 p-3 rounded-lg border border-dashed border-[var(--semi-color-border)] text-[var(--semi-color-text-2)] cursor-not-allowed">
          <IconPlus />
          <span>{t('没有更多的分组可选择，可拖拽排序（优先级从上到下）')}</span>
        </div>
      ) : (
        <Select
          placeholder={t('选择分组添加到优先级列表')}
          optionList={availableOptions}
          onChange={handleAddGroup}
          disabled={disabled}
          renderOptionItem={renderGroupOption}
          showClear
          value={null}
          style={{ width: '100%' }}
          prefix={<IconPlus className="text-[var(--semi-color-text-2)]" />}
        />
      )}

      {/* 说明文字 */}
      <div className="text-xs text-[var(--semi-color-text-2)] space-y-1 p-2 rounded bg-[var(--semi-color-fill-0)]">
        <div className="flex items-start gap-1">
          <span className="shrink-0">ⓘ</span>
          <div>
            <div>
              {t('一、令牌优先级：请求模型时将会按您选择的分组优先级顺序查找匹配的模型。')}
            </div>
            <div>
              {t('二、自动分组：默认开启（强烈建议新手保持默认开启）比如：模型m不在你的令牌分组里，而在xx分组。那就会调用xx分组下的m模型。')}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default GroupPrioritiesSelector;
