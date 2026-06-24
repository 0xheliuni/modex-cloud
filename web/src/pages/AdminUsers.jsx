import React, { useEffect, useRef, useState } from 'react';
import { Card, Table, Button, Modal, Form, Toast, Tag, Space, Popconfirm, Typography } from '@douyinfe/semi-ui';
import { IconPlus, IconDelete, IconEdit, IconKey } from '@douyinfe/semi-icons';
import { adminApi } from '../lib/api.js';
import { ROLE_LABEL } from '../lib/constants.js';

const { Text } = Typography;

export default function AdminUsers() {
  const [list, setList] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [pwModal, setPwModal] = useState(null); // user being reset
  const [editing, setEditing] = useState(null);
  const [submitting, setSubmitting] = useState(false);
  const formRef = useRef();
  const pwRef = useRef();

  const load = async () => {
    setLoading(true);
    try {
      const data = await adminApi.users();
      setList(data.items || []);
    } catch (e) {
      Toast.error(e.message);
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => { load(); }, []);

  const onSubmit = async () => {
    const v = await formRef.current.formApi.validate().catch(() => null);
    if (!v) return;
    setSubmitting(true);
    try {
      if (editing) {
        await adminApi.updateUser(editing.id, {
          role: v.role, status: v.status ? 1 : 2,
          supplier_code: v.supplier_code || '', supplier_name: v.supplier_name || '',
        });
      } else {
        await adminApi.createUser({
          username: v.username, password: v.password, role: v.role || 1,
          supplier_code: v.supplier_code || '', supplier_name: v.supplier_name || '',
        });
      }
      Toast.success('已保存');
      setModalOpen(false);
      load();
    } catch (e) {
      Toast.error(e.message);
    } finally {
      setSubmitting(false);
    }
  };

  const onResetPw = async () => {
    const v = await pwRef.current.formApi.validate().catch(() => null);
    if (!v) return;
    try {
      await adminApi.resetPassword(pwModal.id, v.new_password);
      Toast.success('密码已重置');
      setPwModal(null);
    } catch (e) {
      Toast.error(e.message);
    }
  };

  const onDelete = async (id) => {
    try { await adminApi.deleteUser(id); Toast.success('已删除'); load(); }
    catch (e) { Toast.error(e.message); }
  };

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '用户名', dataIndex: 'username' },
    { title: '角色', dataIndex: 'role', render: (r) => <Tag color={r >= 10 ? 'violet' : 'blue'}>{ROLE_LABEL[r] || r}</Tag> },
    { title: '供应商编号', dataIndex: 'supplier_code', render: (v) => v || '-' },
    { title: '供应商名称', dataIndex: 'supplier_name', render: (v) => v || '-' },
    { title: '状态', dataIndex: 'status', render: (s) => <Tag color={s === 1 ? 'green' : 'grey'}>{s === 1 ? '启用' : '禁用'}</Tag> },
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          <Button size="small" icon={<IconEdit />} onClick={() => { setEditing(r); setModalOpen(true); }}>编辑</Button>
          <Button size="small" icon={<IconKey />} onClick={() => setPwModal(r)}>重置密码</Button>
          <Popconfirm title="删除该用户？将级联删除其授权与渠道" onConfirm={() => onDelete(r.id)}>
            <Button size="small" type="danger" icon={<IconDelete />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const initValues = editing
    ? { role: editing.role, status: editing.status === 1, supplier_code: editing.supplier_code, supplier_name: editing.supplier_name }
    : { role: 1 };

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Text strong style={{ fontSize: 16 }}>用户管理</Text>
        <Button type="primary" theme="solid" icon={<IconPlus />} onClick={() => { setEditing(null); setModalOpen(true); }}>新建用户</Button>
      </div>
      <Card bodyStyle={{ padding: 0 }}>
        <Table columns={columns} dataSource={list} loading={loading} rowKey="id" pagination={false} />
      </Card>

      <Modal title={editing ? '编辑用户' : '新建用户'} visible={modalOpen} onCancel={() => setModalOpen(false)} onOk={onSubmit} confirmLoading={submitting} maskClosable={false}>
        <Form key={editing?.id || 'new'} getFormApi={(api) => (formRef.current = { formApi: api })} initValues={initValues} labelPosition="top">
          {!editing && <Form.Input field="username" label="用户名" rules={[{ required: true }]} />}
          {!editing && <Form.Input field="password" label="初始密码（≥8 位）" mode="password" rules={[{ required: true, min: 8, message: '至少 8 位' }]} />}
          <Form.Select field="role" label="角色" style={{ width: '100%' }}>
            <Form.Select.Option value={1}>供应商</Form.Select.Option>
            <Form.Select.Option value={10}>管理员</Form.Select.Option>
          </Form.Select>
          <Form.Input field="supplier_code" label="供应商编号（供应商角色填写）" placeholder="如 11" />
          <Form.Input field="supplier_name" label="供应商名称" placeholder="如 Modex" />
          {editing && <Form.Switch field="status" label="启用" />}
        </Form>
      </Modal>

      <Modal title={`重置密码 - ${pwModal?.username || ''}`} visible={!!pwModal} onCancel={() => setPwModal(null)} onOk={onResetPw} maskClosable={false}>
        <Form getFormApi={(api) => (pwRef.current = { formApi: api })} labelPosition="top">
          <Form.Input field="new_password" label="新密码（≥8 位）" mode="password" rules={[{ required: true, min: 8, message: '至少 8 位' }]} />
        </Form>
      </Modal>
    </div>
  );
}
