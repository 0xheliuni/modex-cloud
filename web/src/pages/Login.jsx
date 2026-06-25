import React, { useState } from 'react';
import { Card, Form, Button, Toast, Typography, Banner } from '@douyinfe/semi-ui';
import { IconKey } from '@douyinfe/semi-icons';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../lib/auth.jsx';

const { Title, Text } = Typography;

export default function Login() {
  const { login } = useAuth();
  const nav = useNavigate();
  const [loading, setLoading] = useState(false);

  const onSubmit = async (values) => {
    setLoading(true);
    try {
      await login(values.username, values.password);
      Toast.success('登录成功');
      nav('/');
    } catch (e) {
      Toast.error(e.message || '登录失败');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ height: '100vh', display: 'grid', placeItems: 'center', background: 'var(--semi-color-fill-0)' }}>
      <Card style={{ width: 400 }} bodyStyle={{ padding: 32 }}>
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <IconKey style={{ fontSize: 40, color: 'var(--semi-color-primary)' }} />
          <Title heading={3} style={{ marginTop: 12 }}>Modex Cloud 密钥托管平台</Title>
          <Text type="tertiary">供应商密钥安全上传 · 一次写入永不回读</Text>
        </div>
        <Banner
          type="info"
          description="密钥上传后将被加密并同步到 Modex Cloud，同步成功后本地副本立即销毁，任何人无法再次查看明文。"
          style={{ marginBottom: 20 }}
          closeIcon={null}
        />
        <Form onSubmit={onSubmit}>
          <Form.Input
            field="username"
            label="用户名"
            placeholder="请输入用户名"
            rules={[{ required: true, message: '请输入用户名' }]}
          />
          <Form.Input
            field="password"
            label="密码"
            mode="password"
            placeholder="请输入密码"
            rules={[{ required: true, message: '请输入密码' }]}
          />
          <Button htmlType="submit" type="primary" theme="solid" block loading={loading} style={{ marginTop: 12 }}>
            登录
          </Button>
        </Form>
      </Card>
    </div>
  );
}
