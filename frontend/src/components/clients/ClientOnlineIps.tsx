import { Button, Modal, Tag, Typography } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import type { ClientOnlineIps } from '@/generated/zod';
import { banMinutesLeft, formatIpSlots } from '@/lib/clients/online-ips';

const MONO = 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace';
const ROW = {
  display: 'flex',
  flexWrap: 'wrap',
  alignItems: 'center',
  gap: 6,
  padding: '6px 0',
  borderBottom: '1px solid var(--ant-color-border-secondary)',
} as const;

interface OnlineIpsListProps {
  data: ClientOnlineIps | null;
  // When the data was fetched (ms); the minutes left on each ban count from it.
  nowMs: number;
  unbanning?: string | null;
  // Admin views pass it to offer Unban; the user portal leaves it out.
  onUnban?: (network: string) => void;
  bannedHint?: string;
}

// The one rendering of a client's online IPs and bans, so the admin views and
// the user portal always agree on what counts and what is banned.
export function OnlineIpsList({ data, nowMs, unbanning, onUnban, bannedHint }: OnlineIpsListProps) {
  const { t } = useTranslation();
  const online = data?.online ?? [];
  const bans = data?.bans ?? [];
  return (
    <div>
      {online.length === 0 ? (
        <Typography.Text type="secondary">{t('pages.clients.noOnlineIps')}</Typography.Text>
      ) : (
        online.map((entry) => (
          <div key={entry.network} style={ROW}>
            <span style={{ fontFamily: MONO, overflowWrap: 'anywhere' }}>
              {entry.addresses.join(', ')}
            </span>
            {entry.servers.map((server) => (
              <Tag key={server} style={{ margin: 0 }}>
                {server}
              </Tag>
            ))}
            {!entry.counted ? (
              <Tag style={{ margin: 0 }}>
                {t('pages.clients.ipExemptAllowlist')} · {t('pages.clients.ipNotCounted')}
              </Tag>
            ) : null}
          </div>
        ))
      )}
      {bans.length > 0 ? (
        <div style={{ marginTop: 16 }}>
          <Typography.Text strong>{t('pages.clients.ipBans')}</Typography.Text>
          {bannedHint ? (
            <div>
              <Typography.Text type="secondary">{bannedHint}</Typography.Text>
            </div>
          ) : null}
          {bans.map((ban) => (
            <div key={ban.network} style={ROW}>
              <span style={{ flex: 1, minWidth: 0, fontFamily: MONO, overflowWrap: 'anywhere' }}>
                {ban.network}
              </span>
              <Tag color="red" style={{ margin: 0 }}>
                {t('pages.clients.ipBannedFor', { minutes: banMinutesLeft(ban.expiresAt, nowMs) })}
              </Tag>
              {onUnban ? (
                <Button
                  size="small"
                  loading={unbanning === ban.network}
                  onClick={() => onUnban(ban.network)}
                >
                  {t('pages.clients.ipUnban')}
                </Button>
              ) : null}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

interface ClientOnlineIpsModalProps {
  open: boolean;
  email?: string;
  zIndex?: number;
  data: ClientOnlineIps | null;
  nowMs: number;
  loading: boolean;
  unbanning: string | null;
  onRefresh: () => void;
  onUnban: (network: string) => void;
  onClose: () => void;
}

export default function ClientOnlineIpsModal({
  open,
  email,
  zIndex,
  data,
  nowMs,
  loading,
  unbanning,
  onRefresh,
  onUnban,
  onClose,
}: ClientOnlineIpsModalProps) {
  const { t } = useTranslation();
  return (
    <Modal
      open={open}
      title={`${t('pages.clients.onlineIps')}${email ? ` — ${email}` : ''}`}
      width={480}
      zIndex={zIndex}
      onCancel={onClose}
      footer={[
        <Button key="refresh" icon={<ReloadOutlined />} loading={loading} onClick={onRefresh}>
          {t('refresh')}
        </Button>,
        <Button key="close" type="primary" onClick={onClose}>
          {t('close')}
        </Button>,
      ]}
    >
      {data ? (
        <Typography.Title level={3} style={{ marginTop: 0 }}>
          {formatIpSlots(data.count, data.limit)}
        </Typography.Title>
      ) : null}
      <OnlineIpsList data={data} nowMs={nowMs} unbanning={unbanning} onUnban={onUnban} />
    </Modal>
  );
}
