import React, { useEffect, useState } from 'react';
import {
  Card, Table, Button, Modal, Form, Toast, Tag, Select, Space, Typography,
  Empty, Banner, Popconfirm, Tooltip,
} from '@douyinfe/semi-ui';
import { IconPlus, IconRefresh, IconDelete, IconKey } from '@douyinfe/semi-icons';
import { supplierApi } from '../lib/api.js';
import { CHANNEL_TYPES, TYPE_LABEL, KEY_STATE, SYNC_STATUS } from '../lib/constants.js';

const { Text, Title } = Typography;

export default function SupplierChannels() {
  const [platforms, setPlatforms] = useState([]);
  const [activePlatform, setActivePlatform] = useState(null);
  const [channels, setChannels] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const formRef = React.useRef();

  const loadPlatforms = async () => {
    try {
      const data = await supplierApi.platforms();
      const items = data.items || [];
      setPlatforms(items);
      if (items.length && !activePlatform) setActivePlatform(items[0].platform_id);
    } catch (e) {
      Toast.error(e.message);
    }
  };

  const loadChannels = async (pid) => {
    if (!pid) return;
    setLoading(true);
    try {
      const data = await supplierApi.channels(pid);
      setChannels(data.items || []);
    } catch (e) {
      Toast.error(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { loadPlatforms(); }, []);
  useEffect(() => { loadChannels(activePlatform); }, [activePlatform]);

  const current = platforms.find((p) => p.platform_id === activePlatform);
  const allowedTypeOptions = CHANNEL_TYPES.filter(
    (t) => !current?.allowed_types?.length || current.allowed_types.includes(t.value),
  );

  const onCreate = async () => {
    const values = await formRef.current.formApi.validate().catch(() => null);
    if (!values) return;
    setSubmitting(true);
    try {
      await supplierApi.createChannel({
        platform_id: activePlatform,
        name: values.name,
        type: values.type,
        key: values.key,
        base_url: values.base_url || '',
        models: Array.isArray(values.models) ? values.models.join(',') : values.models || '',
        group: values.group || '',
      });
      Toast.success('密钥已上传并加密，正在同步到 AGT');
      setModalOpen(false);
      formRef.current.formApi.reset();
      loadChannels(activePlatform);
    } catch (e) {
      Toast.error(e.message);
    } finally {
      setSubmitting(false);
    }
  };

  const onResync = async (id) => {
    try {
      await supplierApi.resync(id);
      Toast.success('已触发重新同步');
      loadChannels(activePlatform);
    } catch (e) {
      Toast.error(e.message);
    }
  };

  const onDelete = async (id) => {
    try {
      await supplierApi.deleteChannel(id);
      Toast.success('已删除');
      loadChannels(activePlatform);
    } catch (e) {
      Toast.error(e.message);
    }
  };

  const columns = [
    { title: '名称', dataIndex: 'name' },
    { title: '类型', dataIndex: 'type', render: (t) => TYPE_LABEL[t] || t },
    {
      title: '密钥',
      dataIndex: 'key_last4',
      render: (v) => <Text code>{v || '••••'}</Text>,
    },
    {
      title: '密钥状态',
      dataIndex: 'key_state',
      render: (s) => {
        const m = KEY_STATE[s] || { text: s, color: 'grey' };
        return <Tag color={m.color}>{m.text}</Tag>;
      },
    },
    {
      title: '同步状态',
      dataIndex: 'sync_status',
      render: (s, r) => {
        const m = SYNC_STATUS[s] || { text: s, color: 'grey' };
        return (
          <Space>
            <Tag color={m.color}>{m.text}</Tag>
            {s === 2 && r.sync_error ? (
              <Tooltip content={r.sync_error}><Text type="danger" size="small">详情</Text></Tooltip>
            ) : null}
          </Space>
        );
      },
    },
    { title: '模型', dataIndex: 'models', render: (m) => <Text type="tertiary" size="small">{m}</Text> },
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          {r.sync_status !== 1 && (
            <Button size="small" icon={<IconRefresh />} onClick={() => onResync(r.id)}>重试</Button>
          )}
          <Popconfirm title="确认删除该渠道？" onConfirm={() => onDelete(r.id)}>
            <Button size="small" type="danger" icon={<IconDelete />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  if (!platforms.length) {
    return (
      <Empty
        image={<IconKey style={{ fontSize: 48 }} />}
        title="暂无可用平台"
        description="您还未被授权任何目标平台，请联系管理员开通。"
      />
    );
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <Space>
          <Title heading={5} style={{ margin: 0 }}>目标平台</Title>
          <Select value={activePlatform} onChange={setActivePlatform} style={{ width: 240 }}>
            {platforms.map((p) => (
              <Select.Option key={p.platform_id} value={p.platform_id}>{p.name}</Select.Option>
            ))}
          </Select>
        </Space>
        <Button type="primary" theme="solid" icon={<IconPlus />} onClick={() => setModalOpen(true)}>
          上传密钥
        </Button>
      </div>

      <Banner
        type="warning"
        closeIcon={null}
        description="密钥一旦上传即被加密；同步到 AGT 成功后本地明文与密文将被销毁，无法再次查看。如需更换请重新上传。"
        style={{ marginBottom: 16 }}
      />

      <Card bodyStyle={{ padding: 0 }}>
        <Table columns={columns} dataSource={channels} loading={loading} rowKey="id" pagination={false} />
      </Card>

      <Modal
        title="上传密钥"
        visible={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={onCreate}
        okText="加密并上传"
        confirmLoading={submitting}
        maskClosable={false}
        width={560}
      >
        <Banner
          type="info"
          closeIcon={null}
          description={`平台「${current?.name || ''}」允许的类型/模型已根据您的授权限制。`}
          style={{ marginBottom: 16 }}
        />
        <Form getFormApi={(api) => (formRef.current = { formApi: api })} labelPosition="top">
          <Form.Input field="name" label="渠道名称" rules={[{ required: true, message: '请输入名称' }]} placeholder="如 modex-openai-01" />
          <Form.Select field="type" label="提供商类型" rules={[{ required: true, message: '请选择类型' }]} style={{ width: '100%' }} placeholder="选择提供商">
            {allowedTypeOptions.map((t) => (
              <Form.Select.Option key={t.value} value={t.value}>{t.label}</Form.Select.Option>
            ))}
          </Form.Select>
          <Form.TextArea
            field="key"
            label="API 密钥 / 令牌"
            rules={[{ required: true, message: '请输入密钥' }]}
            placeholder="粘贴官方密钥（支持多行凭证，如 AWS / Vertex）。上传后将立即加密。"
            autosize={{ minRows: 2, maxRows: 6 }}
          />
          <Form.Select
            field="models"
            label="可用模型（逗号分隔或多选）"
            multiple
            filter
            allowCreate
            style={{ width: '100%' }}
            placeholder={current?.allowed_models?.length ? '从授权模型中选择' : '输入模型名后回车'}
            rules={[{ required: true, message: '请至少选择一个模型' }]}
          >
            {(current?.allowed_models || []).map((m) => (
              <Form.Select.Option key={m} value={m}>{m}</Form.Select.Option>
            ))}
          </Form.Select>
          <Form.Input field="group" label="分组（可选）" placeholder="如 default" />
          <Form.Input field="base_url" label="Base URL（可选，必须 https）" placeholder="https://..." />
        </Form>
      </Modal>
    </div>
  );
}
