import { Button, Modal, Tag, Tooltip, Typography } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { banMinutesLeft, type ClientIpBan, type ClientIpInfo } from '@/lib/clients/ip-log';

const MONO = 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace';

interface ClientIpLogModalProps {
  open: boolean;
  email?: string;
  zIndex?: number;
  ips: ClientIpInfo[];
  bans: ClientIpBan[];
  // When the lists were fetched (ms); the minutes left on each ban count from it.
  nowMs: number;
  loading: boolean;
  clearing: boolean;
  unbanning: string | null;
  onRefresh: () => void;
  onClear: () => void;
  onUnban: (network: string) => void;
  onClose: () => void;
}

// The single place the IP log is rendered, so the edit form and the info card
// label exempt and banned addresses the same way.
export default function ClientIpLogModal({
  open,
  email,
  zIndex,
  ips,
  bans,
  nowMs,
  loading,
  clearing,
  unbanning,
  onRefresh,
  onClear,
  onUnban,
  onClose,
}: ClientIpLogModalProps) {
  const { t } = useTranslation();

  function exemptLabel(entry: ClientIpInfo): string {
    switch (entry.exempt) {
      case 'host':
        return entry.exemptHost
          ? t('pages.clients.ipExemptHost', { name: entry.exemptHost })
          : t('pages.clients.ipExemptThisPanel');
      case 'allowlist':
        return t('pages.clients.ipExemptAllowlist');
      case 'private':
        return t('pages.clients.ipExemptPrivate');
      default:
        return '';
    }
  }

  return (
    <Modal
      open={open}
      title={`${t('pages.clients.ipLog')}${email ? ` — ${email}` : ''}`}
      width={480}
      zIndex={zIndex}
      onCancel={onClose}
      footer={[
        <Button key="refresh" icon={<ReloadOutlined />} loading={loading} onClick={onRefresh}>
          {t('refresh')}
        </Button>,
        <Button key="clear" danger loading={clearing} disabled={ips.length === 0} onClick={onClear}>
          {t('pages.clients.clearAll')}
        </Button>,
        <Button key="close" type="primary" onClick={onClose}>
          {t('close')}
        </Button>,
      ]}
    >
      {ips.length > 0 ? (
        <div>
          {ips.map((entry, idx) => {
            const exempt = exemptLabel(entry);
            return (
              <div
                key={idx}
                style={{
                  display: 'flex',
                  flexWrap: 'wrap',
                  alignItems: 'center',
                  gap: 6,
                  marginBottom: 6,
                }}
              >
                <Tag color="blue" style={{ margin: 0, maxWidth: '100%', fontFamily: MONO }}>
                  {entry.ip}
                  {entry.time ? ` (${entry.time})` : ''}
                  {entry.node ? (
                    <span style={{ marginInlineStart: 6, fontWeight: 600 }}>@ {entry.node}</span>
                  ) : null}
                </Tag>
                {exempt ? (
                  <Tooltip title={t('pages.clients.ipExemptHint')}>
                    <Tag style={{ margin: 0 }}>{exempt}</Tag>
                  </Tooltip>
                ) : null}
                {entry.bannedUntil * 1000 > nowMs ? (
                  <Tag color="red" style={{ margin: 0 }}>
                    {t('pages.clients.ipBannedFor', {
                      minutes: banMinutesLeft(entry.bannedUntil, nowMs),
                    })}
                  </Tag>
                ) : null}
              </div>
            );
          })}
        </div>
      ) : (
        <Tag>{t('tgbot.noIpRecord')}</Tag>
      )}

      {bans.length > 0 ? (
        <div style={{ marginTop: 16 }}>
          <Typography.Text strong>{t('pages.clients.ipBans')}</Typography.Text>
          {bans.map((ban) => (
            <div
              key={ban.network}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                padding: '6px 0',
                borderBottom: '1px solid var(--ant-color-border-secondary)',
              }}
            >
              <span style={{ flex: 1, minWidth: 0, fontFamily: MONO, overflowWrap: 'anywhere' }}>
                {ban.network}
              </span>
              <Tag color="red" style={{ margin: 0 }}>
                {t('pages.clients.ipBannedFor', {
                  minutes: banMinutesLeft(ban.expiresAt, nowMs),
                })}
              </Tag>
              <Button
                size="small"
                loading={unbanning === ban.network}
                onClick={() => onUnban(ban.network)}
              >
                {t('pages.clients.ipUnban')}
              </Button>
            </div>
          ))}
        </div>
      ) : null}
    </Modal>
  );
}
