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

import React, { useEffect, useState, useRef } from 'react';
import { Button, Col, Form, Row, Spin } from '@douyinfe/semi-ui';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
  verifyJSON,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function RequestRateLimit(props) {
  const { t } = useTranslation();

  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    ModelRequestRateLimitEnabled: false,
    ModelRequestRateLimitCount: -1,
    ModelRequestRateLimitSuccessCount: 1000,
    ModelRequestRateLimitDurationMinutes: 1,
    ModelRequestRateLimitGroup: '',
    ModelRequestRateLimitScope: 'user',
    ModelRequestConcurrencyLimitEnabled: false,
    ModelRequestConcurrencyLimitUserGroup: '',
    ModelRequestConcurrencyLimitTokenGroup: '',
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    const requestQueue = updateArray.map((item) => {
      let value = '';
      if (typeof inputs[item.key] === 'boolean') {
        value = String(inputs[item.key]);
      } else {
        value = inputs[item.key];
      }
      return API.put('/api/option/', {
        key: item.key,
        value,
      });
    });
    setLoading(true);
    Promise.all(requestQueue)
      .then((res) => {
        if (requestQueue.length === 1) {
          if (res.includes(undefined)) return;
        } else if (requestQueue.length > 1) {
          if (res.includes(undefined))
            return showError(t('部分保存失败，请重试'));
        }

        for (let i = 0; i < res.length; i++) {
          if (!res[i].data.success) {
            return showError(res[i].data.message);
          }
        }

        showSuccess(t('保存成功'));
        props.refresh();
      })
      .catch(() => {
        showError(t('保存失败，请重试'));
      })
      .finally(() => {
        setLoading(false);
      });
  }

  useEffect(() => {
    const currentInputs = {};
    for (let key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        currentInputs[key] = props.options[key];
      }
    }
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    refForm.current.setValues(currentInputs);
  }, [props.options]);

  return (
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('模型请求速率限制')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'ModelRequestRateLimitEnabled'}
                  label={t('启用用户模型请求速率限制（可能会影响高并发性能）')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      ModelRequestRateLimitEnabled: value,
                    });
                  }}
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('限制周期')}
                  step={1}
                  min={0}
                  suffix={t('分钟')}
                  extraText={t('频率限制的周期（分钟）')}
                  field={'ModelRequestRateLimitDurationMinutes'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ModelRequestRateLimitDurationMinutes: String(value),
                    })
                  }
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={16}>
                <Form.RadioGroup
                  field='ModelRequestRateLimitScope'
                  label={t('速率限制粒度')}
                  type='button'
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ModelRequestRateLimitScope: value,
                    })
                  }
                  extraText={
                    <div>
                      <p>{t('说明：')}</p>
                      <ul>
                        <li>{t('用户粒度：同一用户所有令牌共享限流配额')}</li>
                        <li>{t('令牌粒度：每个令牌独立计算限流配额')}</li>
                        <li>{t('速率限制与并发限制相互独立，同时生效')}</li>
                      </ul>
                    </div>
                  }
                >
                  <Form.Radio value='user'>{t('用户粒度')}</Form.Radio>
                  <Form.Radio value='token'>{t('令牌粒度')}</Form.Radio>
                </Form.RadioGroup>
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('用户每周期最多请求次数')}
                  step={1}
                  min={0}
                  max={100000000}
                  suffix={t('次')}
                  extraText={t('包括失败请求的次数，0代表不限制')}
                  field={'ModelRequestRateLimitCount'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ModelRequestRateLimitCount: String(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('用户每周期最多请求完成次数')}
                  step={1}
                  min={1}
                  max={100000000}
                  suffix={t('次')}
                  extraText={t('只包括请求成功的次数')}
                  field={'ModelRequestRateLimitSuccessCount'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ModelRequestRateLimitSuccessCount: String(value),
                    })
                  }
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={16}>
                <Form.TextArea
                  label={t('分组速率限制')}
                  placeholder={t(
                    '{\n  "default": [200, 100],\n  "vip": [0, 1000]\n}',
                  )}
                  field={'ModelRequestRateLimitGroup'}
                  autosize={{ minRows: 5, maxRows: 15 }}
                  trigger='blur'
                  stopValidateWithError
                  rules={[
                    {
                      validator: (rule, value) => verifyJSON(value),
                      message: t('不是合法的 JSON 字符串'),
                    },
                  ]}
                  extraText={
                    <div>
                      <p>{t('说明：')}</p>
                      <ul>
                        <li>
                          {t(
                            '使用 JSON 对象格式，格式为：{"组名": [最多请求次数, 最多请求完成次数]}',
                          )}
                        </li>
                        <li>
                          {t(
                            '示例：{"default": [200, 100], "vip": [0, 1000]}。',
                          )}
                        </li>
                        <li>
                          {t(
                            '[最多请求次数]必须大于等于0，[最多请求完成次数]必须大于等于1。',
                          )}
                        </li>
                        <li>
                          {t(
                            '[最多请求次数]和[最多请求完成次数]的最大值为2147483647。',
                          )}
                        </li>
                        <li>{t('分组速率配置优先级高于全局速率限制。')}</li>
                        <li>{t('限制周期统一使用上方配置的“限制周期”值。')}</li>
                      </ul>
                    </div>
                  }
                  onChange={(value) => {
                    setInputs({ ...inputs, ModelRequestRateLimitGroup: value });
                  }}
                />
              </Col>
            </Row>
          </Form.Section>
          <Form.Section text={t('模型请求并发限制')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'ModelRequestConcurrencyLimitEnabled'}
                  label={t('启用用户模型请求并发限制（可能会影响高并发性能）')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      ModelRequestConcurrencyLimitEnabled: value,
                    });
                  }}
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={16}>
                <Form.TextArea
                  label={t('用户并发限制')}
                  placeholder={t(
                    '{\n  "default": 3,\n  "vip": 5\n}',
                  )}
                  field={'ModelRequestConcurrencyLimitUserGroup'}
                  autosize={{ minRows: 4, maxRows: 15 }}
                  trigger='blur'
                  stopValidateWithError
                  rules={[
                    {
                      validator: (rule, value) => verifyJSON(value),
                      message: t('不是合法的 JSON 字符串'),
                    },
                  ]}
                  extraText={
                    <div>
                      <p>{t('说明：')}</p>
                      <ul>
                        <li>
                          {t(
                            '使用 JSON 对象格式，格式为：{"组名": 并发数}',
                          )}
                        </li>
                        <li>
                          {t('示例：{"default": 3, "vip": 5}。')}
                        </li>
                        <li>{t('并发数必须大于等于0，0代表不限制。')}</li>
                        <li>{t('按用户ID维度分别统计并发数。')}</li>
                      </ul>
                    </div>
                  }
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      ModelRequestConcurrencyLimitUserGroup: value,
                    });
                  }}
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={16}>
                <Form.TextArea
                  label={t('令牌并发限制')}
                  placeholder={t('{\n  "default": 2\n}')}
                  field={'ModelRequestConcurrencyLimitTokenGroup'}
                  autosize={{ minRows: 4, maxRows: 15 }}
                  trigger='blur'
                  stopValidateWithError
                  rules={[
                    {
                      validator: (rule, value) => verifyJSON(value),
                      message: t('不是合法的 JSON 字符串'),
                    },
                  ]}
                  extraText={
                    <div>
                      <p>{t('说明：')}</p>
                      <ul>
                        <li>
                          {t(
                            '使用 JSON 对象格式，格式为：{"组名": 并发数}',
                          )}
                        </li>
                        <li>{t('示例：{"default": 2}。')}</li>
                        <li>{t('并发数必须大于等于0，0代表不限制。')}</li>
                        <li>{t('按令牌维度分别统计并发数。')}</li>
                        <li>{t('用户与令牌并发限制会同时校验，任一超限都会拒绝。')}</li>
                      </ul>
                    </div>
                  }
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      ModelRequestConcurrencyLimitTokenGroup: value,
                    });
                  }}
                />
              </Col>
            </Row>
          </Form.Section>
          <Row>
            <Button size='default' onClick={onSubmit}>
              {t('保存模型速率限制')}
            </Button>
          </Row>
        </Form>
      </Spin>
    </>
  );
}
