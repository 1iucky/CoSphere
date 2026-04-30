import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Col,
  Empty,
  Input,
  Row,
  Space,
  Table,
  Tag,
  Toast,
  Typography,
} from '@douyinfe/semi-ui';
import { IconCopy, IconSearch } from '@douyinfe/semi-icons';
import { API, copy, timestamp2string } from '../../helpers';

const STORAGE_KEY = 'token-query:last-token';
const DEFAULT_PAGE_SIZE = 10;
const TOKEN_REGEXP = /^sk-[a-zA-Z0-9]{48}$/;

const maskToken = (token) => {
  if (!token || token.length < 12) {
    return token || '';
  }
  return `${token.slice(0, 6)}******${token.slice(-6)}`;
};

const normalizeToken = (token) => token.trim();

const renderStreamTag = (isStream) => (
  <Tag color={isStream ? 'blue' : 'purple'} shape='circle'>
    {isStream ? '流式' : '非流式'}
  </Tag>
);

const renderUseTime = (useTime, isStream) => (
  <Space>
    <Tag color={useTime <= 30 ? 'green' : useTime <= 120 ? 'orange' : 'red'}>
      {useTime}s
    </Tag>
    {renderStreamTag(isStream)}
  </Space>
);

const TokenQueryPage = () => {
  const [inputValue, setInputValue] = useState('');
  const [storedToken, setStoredToken] = useState('');
  const [maskedDisplay, setMaskedDisplay] = useState(false);
  const [loading, setLoading] = useState(false);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [result, setResult] = useState({
    token_name: '',
    detail: {
      page: 1,
      page_size: DEFAULT_PAGE_SIZE,
      total: 0,
      items: [],
    },
    daily_stats: [],
  });

  useEffect(() => {
    const lastToken = localStorage.getItem(STORAGE_KEY) || '';
    if (!lastToken) {
      return;
    }
    setStoredToken(lastToken);
    setInputValue(maskToken(lastToken));
    setMaskedDisplay(true);
  }, []);

  const resolvedToken = useMemo(() => {
    if (maskedDisplay && storedToken) {
      return storedToken;
    }
    return normalizeToken(inputValue);
  }, [inputValue, maskedDisplay, storedToken]);

  const dailyTotal = useMemo(() => {
    return (result.daily_stats || []).reduce(
      (sum, item) => sum + (item.call_count || 0),
      0,
    );
  }, [result.daily_stats]);

  const todayCalls = useMemo(() => {
    const stats = result.daily_stats || [];
    return stats.length > 0 ? stats[0].call_count || 0 : 0;
  }, [result.daily_stats]);

  const persistToken = useCallback((token) => {
    localStorage.setItem(STORAGE_KEY, token);
    setStoredToken(token);
    setInputValue(maskToken(token));
    setMaskedDisplay(true);
  }, []);

  const handleSearch = useCallback(
    async (page = 1, nextPageSize = pageSize) => {
      const token = normalizeToken(resolvedToken);
      if (!token) {
        Toast.warning('请先输入令牌');
        return;
      }
      if (!TOKEN_REGEXP.test(token)) {
        Toast.error('令牌格式非法，请输入完整的 sk- 令牌');
        return;
      }
      setLoading(true);
      try {
        const res = await API.get('/api/log/token/usage', {
          params: {
            key: token,
            p: page,
            page_size: nextPageSize,
          },
          skipErrorHandler: true,
        });
        const { success, message, data } = res.data;
        if (!success) {
          Toast.error(message || '查询失败');
          return;
        }
        setResult(data);
        setPageSize(nextPageSize);
        persistToken(token);
      } catch (error) {
        if (error?.response?.status === 429) {
          Toast.error('查询过于频繁，请 1 分钟后再试');
          return;
        }
        Toast.error(
          error?.response?.data?.message ||
            error?.message ||
            '查询失败，请稍后重试',
        );
      } finally {
        setLoading(false);
      }
    },
    [pageSize, persistToken, resolvedToken],
  );

  const detailColumns = useMemo(
    () => [
      {
        title: '调用时间',
        dataIndex: 'created_at',
        render: (value) => timestamp2string(value),
      },
      {
        title: '令牌名称',
        dataIndex: 'token_name',
        render: (value) => value || result.token_name || '-',
      },
      {
        title: '模型名称',
        dataIndex: 'model_name',
        render: (value) => <Tag color='white'>{value || '-'}</Tag>,
      },
      {
        title: '用时',
        dataIndex: 'use_time',
        render: (value, record) => renderUseTime(value || 0, record.is_stream),
      },
      {
        title: '提示 Token',
        dataIndex: 'prompt_tokens',
      },
      {
        title: '补全 Token',
        dataIndex: 'completion_tokens',
      },
      {
        title: '详情',
        dataIndex: 'content',
        render: (value) => (
          <Typography.Paragraph
            ellipsis={{
              rows: 2,
              showTooltip: true,
            }}
            style={{ maxWidth: 420, marginBottom: 0 }}
          >
            {value || '-'}
          </Typography.Paragraph>
        ),
      },
    ],
    [result.token_name],
  );

  const statsColumns = useMemo(
    () => [
      {
        title: '日期',
        dataIndex: 'date',
      },
      {
        title: '调用次数',
        dataIndex: 'call_count',
      },
      {
        title: '模型调用次数',
        dataIndex: 'model_call_counts',
        render: (items) => {
          if (!items || items.length === 0) {
            return <Typography.Text type='tertiary'>暂无数据</Typography.Text>;
          }
          return (
            <Space wrap>
              {items.map((item) => (
                <Tag
                  key={`${item.model_name}-${item.call_count}`}
                  color='white'
                >
                  {item.model_name}: {item.call_count}
                </Tag>
              ))}
            </Space>
          );
        },
      },
    ],
    [],
  );

  return (
    <div className='max-w-7xl mx-auto px-4 py-6'>
      <Space vertical align='stretch' spacing='large' className='w-full'>
        <Card
          className='!rounded-2xl shadow-sm border-0'
          title='按令牌查询使用记录'
        >
          <Space vertical align='stretch' spacing='medium' className='w-full'>
            <Typography.Text type='secondary'>
              查询单个令牌当天的消费明细，以及近 7
              天按日聚合的调用次数与模型调用次数。
            </Typography.Text>
            <Input
              showClear
              value={inputValue}
              placeholder='请输入完整令牌，例如 sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'
              prefix={<IconSearch />}
              onFocus={() => {
                if (maskedDisplay && storedToken) {
                  setInputValue(storedToken);
                  setMaskedDisplay(false);
                }
              }}
              onBlur={() => {
                if (
                  storedToken &&
                  normalizeToken(inputValue) === normalizeToken(storedToken)
                ) {
                  setInputValue(maskToken(storedToken));
                  setMaskedDisplay(true);
                }
              }}
              onChange={(value) => {
                setInputValue(value);
                setMaskedDisplay(false);
              }}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  handleSearch(1, pageSize);
                }
              }}
              suffix={
                <Button
                  type='primary'
                  theme='solid'
                  loading={loading}
                  onClick={() => handleSearch(1, pageSize)}
                >
                  查询
                </Button>
              }
            />
          </Space>
        </Card>

        <Row gutter={16}>
          <Col xs={24} md={8}>
            <Card className='!rounded-2xl shadow-sm border-0' title='令牌名称'>
              <Space align='center'>
                <Typography.Title heading={5} style={{ margin: 0 }}>
                  {result.token_name || '-'}
                </Typography.Title>
                {result.token_name ? (
                  <Button
                    theme='borderless'
                    icon={<IconCopy />}
                    onClick={async () => {
                      if (await copy(result.token_name)) {
                        Toast.success('令牌名称已复制');
                      }
                    }}
                  />
                ) : null}
              </Space>
            </Card>
          </Col>
          <Col xs={24} md={8}>
            <Card
              className='!rounded-2xl shadow-sm border-0'
              title='今日调用次数'
            >
              <Typography.Title heading={3} style={{ margin: 0 }}>
                {todayCalls}
              </Typography.Title>
            </Card>
          </Col>
          <Col xs={24} md={8}>
            <Card
              className='!rounded-2xl shadow-sm border-0'
              title='近 7 天调用次数'
            >
              <Typography.Title heading={3} style={{ margin: 0 }}>
                {dailyTotal}
              </Typography.Title>
            </Card>
          </Col>
        </Row>

        <Card className='!rounded-2xl shadow-sm border-0' title='当天使用明细'>
          <Table
            rowKey={(record) =>
              `${record.created_at}-${record.model_name}-${record.use_time}`
            }
            loading={loading}
            columns={detailColumns}
            dataSource={result.detail?.items || []}
            pagination={{
              currentPage: result.detail?.page || 1,
              pageSize: result.detail?.page_size || pageSize,
              pageSizeOptions: [10, 20, 50],
              total: result.detail?.total || 0,
              showSizeChanger: true,
              onPageChange: (currentPage) => {
                handleSearch(currentPage, result.detail?.page_size || pageSize);
              },
              onPageSizeChange: (nextPageSize) => {
                handleSearch(1, nextPageSize);
              },
            }}
            empty={
              <Empty description='当天暂无消费明细' style={{ padding: 24 }} />
            }
          />
        </Card>

        <Card
          className='!rounded-2xl shadow-sm border-0'
          title='近 7 天调用统计'
        >
          <Table
            rowKey='date'
            loading={loading}
            columns={statsColumns}
            dataSource={result.daily_stats || []}
            pagination={false}
            empty={
              <Empty
                description='暂无统计数据，请输入令牌后查询'
                style={{ padding: 24 }}
              />
            }
          />
        </Card>
      </Space>
    </div>
  );
};

export default TokenQueryPage;
