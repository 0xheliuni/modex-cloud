import React, { useEffect, useState } from 'react';
import { Card, Table, Toast, Tag, Typography, Input, Space, Button } from '@douyinfe/semi-ui';
import { IconSearch, IconRefresh } from '@douyinfe/semi-icons';
import { adminApi } from '../lib/api.js';

const { Text } = Typography;

const RESULT_COLOR = { success: 'green', failed: 'red' };

export default function AdminAudit() {
  const [logs, setLogs] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [action, setAction] = useState('');
  const [loading, setLoading] = useState(false);
  const pageSize = 20;

  const load = async () => {
    setLoading(true);
    try {
      const params = `?page=${page}&page_size=${pageSize}${action ? `&action=${encodeURIComponent(action)}` : ''}`;
      const data = await adminApi.auditLogs(params);
      setLogs(data.items || []);
      setTotal(data.total || 0);
    } catch (e) {
      Toast.error(e.message);
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => { load(); }, [page]);

  const fmt = (ts) => (ts ? new Date(ts * 1000).toLocaleString('zh-CN') : '-');

  const columns = [
    { title: '时间', dataIndex: 'created_time', render: fmt, width: 180 },
    { title: '操作', dataIndex: 'action', render: (a) => <Tag>{a}</Tag> },
    { title: '用户', dataIndex: 'username', render: (u, r) => `${u || '-'} (#${r.user_id})` },
    { title: '资源', render: (_, r) => <Text type="tertiary">{r.resource_type} #{r.resource_id}</Text> },
    { title: '详情', dataIndex: 'detail', render: (d) => <Text type="tertiary" size="small">{d || '-'}</Text> },
    { title: 'IP', dataIndex: 'ip' },
    { title: '结果', dataIndex: 'result', render: (r) => <Tag color={RESULT_COLOR[r] || 'grey'}>{r}</Tag> },
  ];

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Text strong style={{ fontSize: 16 }}>审计日志</Text>
        <Space>
          <Input
            prefix={<IconSearch />}
            placeholder="按操作过滤，如 CREATE_CHANNEL"
            value={action}
            onChange={setAction}
            onEnterPress={() => (page === 1 ? load() : setPage(1))}
            style={{ width: 260 }}
          />
          <Button icon={<IconRefresh />} onClick={() => (page === 1 ? load() : setPage(1))}>查询</Button>
        </Space>
      </div>
      <Card bodyStyle={{ padding: 0 }}>
        <Table
          columns={columns}
          dataSource={logs}
          loading={loading}
          rowKey="id"
          pagination={{ currentPage: page, pageSize, total, onPageChange: setPage }}
        />
      </Card>
    </div>
  );
}
