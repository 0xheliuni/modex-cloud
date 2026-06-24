import React, { useEffect, useRef, useState } from 'react';
import { Card, Table, Button, Modal, Form, Toast, Tag, Space, Popconfirm, Typography, Banner, ArrayField } from '@douyinfe/semi-ui';
import { IconPlus, IconDelete, IconEdit } from '@douyinfe/semi-icons';
import { adminApi } from '../lib/api.js';
import { CHANNEL_TYPES } from '../lib/constants.js';

const { Text } = Typography;

export default function AdminPlatforms() {
  const [list, setList] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState(null);
  const [submitting, setSubmitting] = useState(false);
  const formRef = useRef();

  const load = async () => {
    setLoading(true);
    try {
      setList(await adminApi.platforms());
    } catch (e) {
      Toast.error(e.message);
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => { load(); }, []);

  const openCreate = () => { setEditing(null); setModalOpen(true); };
  const openEdit = (row) => { setEditing(row); setModalOpen(true); };

  const parseJSON = (s, fallback) => { try { return JSON.parse(s); } catch { return fallback; } };

  const onSubmit = async () => {
    const v = await formRef.current.formApi.validate().catch(() => null);
    if (!v) return;
    setSubmitting(true);
    const groups = (v.groups || [])
      .filter((g) => g && g.name && g.name.trim())
      .map((g) => ({ name: g.name.trim(), show_amount: !!g.show_amount }));
    const body = {
      name: v.name,
      base_url: v.base_url,
      agt_token: v.agt_token || '', // empty on edit = keep existing
      status: v.status ? 1 : 2,
      name_prefix: v.name_prefix || '',
      groups,
      // The validation group whitelist is derived from the configured groups so
      // the system-assigned group always passes ChannelUpload validation.
      allowed_groups: groups.map((g) => g.name),
      allowed_types: v.allowed_types || [],
      allowed_models: v.allowed_models || [],
    };
    try {
      if (editing) await adminApi.updatePlatform(editing.id, body);
      else await adminApi.createPlatform(body);
      Toast.success('已保存');
      setModalOpen(false);
      load();
    } catch (e) {
      Toast.error(e.message);
    } finally {
      setSubmitting(false);
    }
  };

  const onDelete = async (id) => {
    try { await adminApi.deletePlatform(id); Toast.success('已删除'); load(); }
    catch (e) { Toast.error(e.message); }
  };

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '名称', dataIndex: 'name' },
    { title: 'Base URL', dataIndex: 'base_url', render: (v) => <Text type="tertiary">{v}</Text> },
    { title: '名称前缀', dataIndex: 'name_prefix', render: (v) => (v ? <Text code>{v}</Text> : <Text type="tertiary">无</Text>) },
    {
      title: '分组',
      dataIndex: 'groups',
      render: (s) => {
        const gs = parseJSON(s, []);
        if (!gs.length) return <Text type="tertiary">未配置</Text>;
        return (
          <Space wrap>
            {gs.map((g) => (
              <Tag key={g.name} color={g.show_amount ? 'green' : 'grey'}>
                {g.name}{g.show_amount ? ' · 显示金额' : ''}
              </Tag>
            ))}
          </Space>
        );
      },
    },
    { title: 'AGT 令牌', dataIndex: 'agt_token_last4', render: (v) => <Text code>{v || '未设置'}</Text> },
    { title: '状态', dataIndex: 'status', render: (s) => <Tag color={s === 1 ? 'green' : 'grey'}>{s === 1 ? '启用' : '禁用'}</Tag> },
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          <Button size="small" icon={<IconEdit />} onClick={() => openEdit(r)}>编辑</Button>
          <Popconfirm title="删除该平台？（有渠道引用时会被拒绝）" onConfirm={() => onDelete(r.id)}>
            <Button size="small" type="danger" icon={<IconDelete />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const initValues = editing
    ? {
        name: editing.name,
        base_url: editing.base_url,
        status: editing.status === 1,
        name_prefix: editing.name_prefix || '',
        groups: parseJSON(editing.groups, []),
        allowed_types: parseJSON(editing.allowed_types, []),
        allowed_models: parseJSON(editing.allowed_models, []),
        allowed_groups: parseJSON(editing.allowed_groups, []),
      }
    : { status: true, groups: [{ name: 'default', show_amount: false }] };

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Text strong style={{ fontSize: 16 }}>目标平台管理</Text>
        <Button type="primary" theme="solid" icon={<IconPlus />} onClick={openCreate}>新建平台</Button>
      </div>
      <Card bodyStyle={{ padding: 0 }}>
        <Table columns={columns} dataSource={list} loading={loading} rowKey="id" pagination={false} />
      </Card>

      <Modal
        title={editing ? '编辑平台' : '新建平台'}
        visible={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={onSubmit}
        confirmLoading={submitting}
        maskClosable={false}
        width={560}
      >
        <Form key={editing?.id || 'new'} getFormApi={(api) => (formRef.current = { formApi: api })} initValues={initValues} labelPosition="top">
          <Form.Input field="name" label="平台名称" rules={[{ required: true }]} placeholder="如 AGT-生产" />
          <Form.Input field="base_url" label="Base URL" rules={[{ required: true }]} placeholder="https://open.naci-tech.com" />
          {editing ? (
            <Banner type="info" closeIcon={null} description="留空则保留原有 AGT 令牌；填写则替换。" style={{ marginBottom: 12 }} />
          ) : null}
          <Form.Input
            field="agt_token"
            label="AGT 访问令牌"
            mode="password"
            rules={editing ? [] : [{ required: true, message: '请输入 AGT 令牌' }]}
            placeholder={editing ? '留空=不修改' : 'Bearer 令牌，加密存储'}
          />
          <Form.Input
            field="name_prefix"
            label="渠道名称前缀"
            placeholder="如 modex（渠道名将自动生成为 前缀-用户名-序号）"
          />
          <Form.Slot label="分组配置（每组可单独控制是否向供应商显示消耗金额）">
            <ArrayField field="groups">
              {({ add, arrayFields }) => (
                <div>
                  {arrayFields.map(({ field, key, remove }) => (
                    <Space key={key} align="end" style={{ marginBottom: 8 }}>
                      <Form.Input
                        field={`${field}[name]`}
                        noLabel
                        placeholder="分组名，如 default / vip"
                        style={{ width: 220 }}
                      />
                      <Form.Switch field={`${field}[show_amount]`} label="显示金额" labelPosition="left" />
                      <Button type="danger" theme="borderless" icon={<IconDelete />} onClick={remove} />
                    </Space>
                  ))}
                  <Button theme="light" icon={<IconPlus />} onClick={add}>添加分组</Button>
                </div>
              )}
            </ArrayField>
          </Form.Slot>
          <Form.Select field="allowed_types" label="允许的提供商类型（空=全部）" multiple style={{ width: '100%' }}>
            {CHANNEL_TYPES.map((t) => (<Form.Select.Option key={t.value} value={t.value}>{t.label}</Form.Select.Option>))}
          </Form.Select>
          <Form.Select field="allowed_models" label="允许的模型（空=不限；回车添加）" multiple filter allowCreate style={{ width: '100%' }} placeholder="如 gpt-4o" />
          <Form.Switch field="status" label="启用" />
        </Form>
      </Modal>
    </div>
  );
}
