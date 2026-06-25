import React from 'react';
import { Routes, Route, Navigate, useNavigate, useLocation } from 'react-router-dom';
import { Layout, Nav, Button, Avatar, Spin, Typography } from '@douyinfe/semi-ui';
import { IconKey, IconServer, IconUserGroup, IconShield, IconHistory, IconExit } from '@douyinfe/semi-icons';
import { useAuth, isAdmin } from './lib/auth.jsx';
import Login from './pages/Login.jsx';
import SupplierChannels from './pages/SupplierChannels.jsx';
import AdminPlatforms from './pages/AdminPlatforms.jsx';
import AdminUsers from './pages/AdminUsers.jsx';
import AdminGrants from './pages/AdminGrants.jsx';
import AdminAudit from './pages/AdminAudit.jsx';

const { Header, Sider, Content } = Layout;
const { Text } = Typography;

function Shell() {
  const { user, logout } = useAuth();
  const nav = useNavigate();
  const loc = useLocation();
  const admin = isAdmin(user);

  const items = admin
    ? [
        { itemKey: '/admin/platforms', text: '目标平台', icon: <IconServer /> },
        { itemKey: '/admin/users', text: '用户管理', icon: <IconUserGroup /> },
        { itemKey: '/admin/grants', text: '授权管理', icon: <IconShield /> },
        { itemKey: '/admin/audit', text: '审计日志', icon: <IconHistory /> },
      ]
    : [{ itemKey: '/channels', text: '我的密钥', icon: <IconKey /> }];

  return (
    <Layout style={{ height: '100vh' }}>
      <Sider>
        <Nav
          style={{ height: '100%' }}
          selectedKeys={[loc.pathname]}
          onSelect={({ itemKey }) => nav(itemKey)}
          items={items}
          header={{ logo: <IconKey style={{ fontSize: 28, color: '#fff' }} />, text: 'Modex Cloud' }}
          footer={{ collapseButton: true }}
        />
      </Sider>
      <Layout>
        <Header style={{ background: 'var(--semi-color-bg-1)', padding: '12px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Text strong>{admin ? '管理控制台' : `供应商 · ${user.supplier_name || user.username}`}</Text>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <Avatar size="small" color="light-blue">{(user.username || '?')[0].toUpperCase()}</Avatar>
            <Text>{user.username}</Text>
            <Button icon={<IconExit />} theme="borderless" onClick={async () => { await logout(); nav('/login'); }}>退出</Button>
          </div>
        </Header>
        <Content style={{ padding: 24, overflow: 'auto' }}>
          <Routes>
            {admin ? (
              <>
                <Route path="/admin/platforms" element={<AdminPlatforms />} />
                <Route path="/admin/users" element={<AdminUsers />} />
                <Route path="/admin/grants" element={<AdminGrants />} />
                <Route path="/admin/audit" element={<AdminAudit />} />
                <Route path="*" element={<Navigate to="/admin/platforms" replace />} />
              </>
            ) : (
              <>
                <Route path="/channels" element={<SupplierChannels />} />
                <Route path="*" element={<Navigate to="/channels" replace />} />
              </>
            )}
          </Routes>
        </Content>
      </Layout>
    </Layout>
  );
}

export default function App() {
  const { user, loading } = useAuth();

  if (loading) {
    return <div style={{ height: '100vh', display: 'grid', placeItems: 'center' }}><Spin size="large" /></div>;
  }

  return (
    <Routes>
      <Route path="/login" element={user ? <Navigate to="/" replace /> : <Login />} />
      <Route path="*" element={user ? <Shell /> : <Navigate to="/login" replace />} />
    </Routes>
  );
}
