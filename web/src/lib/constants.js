// Channel-type catalog for the six supported providers. Numbers match the
// backend constant/channel.go and AGT/new-api wire format.
export const CHANNEL_TYPES = [
  { value: 1, label: 'OpenAI', hint: 'sk-...' },
  { value: 3, label: 'Azure OpenAI', hint: 'api-key' },
  { value: 14, label: 'Anthropic Claude', hint: 'sk-ant-...' },
  { value: 24, label: 'Google Gemini', hint: 'AIza...' },
  { value: 33, label: 'AWS Claude (Bedrock)', hint: 'AKIA... / 多行凭证' },
  { value: 41, label: 'Google Vertex AI', hint: 'service-account JSON' },
];

export const TYPE_LABEL = Object.fromEntries(CHANNEL_TYPES.map((t) => [t.value, t.label]));

export const ROLE_LABEL = { 1: '供应商', 10: '管理员', 100: '超级管理员' };

export const KEY_STATE = {
  pending: { text: '待同步', color: 'amber' },
  synced: { text: '已同步·密钥已销毁', color: 'green' },
  failed: { text: '同步失败', color: 'red' },
};

export const SYNC_STATUS = {
  0: { text: '待同步', color: 'grey' },
  1: { text: '成功', color: 'green' },
  2: { text: '失败', color: 'red' },
};

// AGT / new-api quota unit: 500000 = $1. Formats consumed quota as a USD string.
export const QUOTA_PER_UNIT = 500000;
export function formatQuota(usedQuota) {
  const n = Number(usedQuota) || 0;
  return `$${(n / QUOTA_PER_UNIT).toFixed(2)}`;
}
