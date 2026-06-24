import React, { useEffect, useRef, useState } from 'react';
import { Card, Table, Button, Modal, Form, Toast, Space, Popconfirm, Typography, Banner } from '@douyinfe/semi-ui';
import { IconPlus, IconDelete } from '@douyinfe/semi-icons';
import { adminApi } from '../lib/api.js';
import { CHANNEL_TYPES, TYPE_LABEL } from '../lib/constants.js';

const { Text } = Typography;

export default function AdminGrants() {
  const [grants, setGrants] = useState([]);
  const [users, setUsers] = useState([]);
  const [platforms, setPlatforms] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const formRef = useRef();

  const load = async () => {
    setLoading(true);
    try {
      const [g, u, p] = await Promise.all([adminApi.grants(), adminApi.users(1), adminApi.platforms()]);
      setGrants(g.items || []);
      setUsers(u.items || []);
      setPlatforms(p || []);
    } catch (e) {
      Toast.error(e.message);
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => { load(); }, []);

  const userName = (id) => {
    const u = users.find((x) => x.id === id);
    return u ? `${u.username}${u.supplier_name ? ` (${u.supplier_name})` : ''}` : `#${id}`;
  };
  const platformName = (id) => platforms.find((x) => x.id === id)?.name || `#${id}`;

  const onSubmit = async () => {
    const v = await formRef.current.formApi.validate().catch(() => null);
    if (!v) return;
    setSubmitting(true);
    try {
      await adminApi.upsertGrant({
        user_id: v.user_id,
        platform_id: v.platform_id,
        allowed_types: v.allowed_types || [],
        allowed_models: v.allowed_models || [],
        allowed_groups: v.allowed_groups || [],
        max_channels: v.max_channels || 0,
      });
      Toast.success('授权已保存');
      setModalOpen(false);
      load();
    } catch (e) {
      Toast.error(e.message);
    } finally {
      setSubmitting(false);
    }
  };

  const onDelete = async (id) => {
    try { await adminApi.deleteGrant(id); Toast.success('已撤销'); load(); }
    catch (e) { Toast.error(e.message); }
  };

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '供应商', dataIndex: 'user_id', render: userName },
    { title: '平台', dataIndex: 'platform_id', render: platformName },
    {
      title: '类型收窄',
      dataIndex: 'allowed_types',
      render: (s) => {
        try { const a = JSON.parse(s); return a?.length ? a.map((t) => TYPE_LABEL[t] || t).join(', ') : '继承平台'; }
        catch { return '继承平台'; }
      },
    },
    { title: '渠道上限', dataIndex: 'max_channels', render: (n) => (n > 0 ? n : '不限') },
    {
      title: '操作',
      render: (_, r) => (
        <Popconfirm title="撤销该授权？" onConfirm={() => onDelete(r.id)}>
          <Button size="small" type="danger" icon={<IconDelete />}>撤销</Button>
        </Popconfirm>
      ),
    },
  ];

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Text strong style={{ fontSize: 16 }}>授权管理（供应商 ↔ 平台）</Text>
        <Button type="primary" theme="solid" icon={<IconPlus />} onClick={() => setModalOpen(true)}>新增授权</Button>
      </div>
      <Card bodyStyle={{ padding: 0 }}>
        <Table columns={columns} dataSource={grants} loading={loading} rowKey="id" pagination={false} />
      </Card>

      <Modal title="新增 / 更新授权" visible={modalOpen} onCancel={() => setModalOpen(false)} onOk={onSubmit} confirmLoading={submitting} maskClosable={false} width={560}>
        <Banner type="info" closeIcon={null} description="留空的白名单字段表示继承平台白名单；填写则在平台基础上进一步收窄（取交集）。" style={{ marginBottom: 16 }} />
        <Form getFormApi={(api) => (formRef.current = { formApi: api })} labelPosition="top">
          <Form.Select field="user_id" label="供应商" rules={[{ required: true }]} style={{ width: '100%' }} filter>
            {users.map((u) => (<Form.Select.Option key={u.id} value={u.id}>{userName(u.id)}</Form.Select.Option>))}
          </Form.Select>
          <Form.Select field="platform_id" label="平台" rules={[{ required: true }]} style={{ width: '100%' }}>
            {platforms.map((p) => (<Form.Select.Option key={p.id} value={p.id}>{p.name}</Form.Select.Option>))}
          </Form.Select>
          <Form.Select field="allowed_types" label="类型收窄（空=继承）" multiple style={{ width: '100%' }}>
            {CHANNEL_TYPES.map((t) => (<Form.Select.Option key={t.value} value={t.value}>{t.label}</Form.Select.Option>))}
          </Form.Select>
          <Form.Select field="allowed_models" label="模型收窄（空=继承）" multiple filter allowCreate style={{ width: '100%' }} />
          <Form.InputNumber field="max_channels" label="渠道上限（0=不限）" min={0} style={{ width: '100%' }} />
        </Form>
      </Modal>
    </div>
  );
}
