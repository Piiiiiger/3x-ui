import { useTranslation } from 'react-i18next';
import { Card, Empty, Table } from 'antd';
import { MessageOutlined } from '@ant-design/icons';

import type { AiUsageSessionView } from '@/generated/zod';
import { formatTokens, formatUsd, projectName } from './aiUsageFormat';

interface AiSessionsCardProps {
  sessions: AiUsageSessionView[];
  relativeTime: (unixSeconds?: number) => string;
  showDevice: boolean;
}

/** One tool's costliest sessions of the period, named by Claude Code's own title when it gave one. */
export default function AiSessionsCard({
  sessions,
  relativeTime,
  showDevice,
}: AiSessionsCardProps) {
  const { t } = useTranslation();
  return (
    <Card styles={{ body: { padding: 0 } }}>
      <div className="ov-wide-head">
        <div className="ov-wide-head-stack">
          <div className="ov-kicker ov-card-title ov-kicker-icon">
            <MessageOutlined />
            {t('pages.aiUsage.sessionsTitle')}
          </div>
          <div className="ov-sub">{t('pages.aiUsage.sessionsSub')}</div>
        </div>
      </div>
      {sessions.length === 0 ? (
        <Empty
          className="ov-rank-empty"
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={t('pages.index.traffic.rankEmpty')}
        />
      ) : (
        <div className="ov-hosts-table">
          <Table<AiUsageSessionView>
            size="small"
            rowKey={(s) => `${s.app}:${s.sessionId}`}
            pagination={{ pageSize: 10, hideOnSinglePage: true, size: 'small' }}
            scroll={{ x: 'max-content' }}
            dataSource={sessions}
            columns={[
              {
                title: t('pages.aiUsage.colSession'),
                key: 'title',
                ellipsis: true,
                render: (_, s) => (
                  <span className="ai-session-title" title={s.sessionId}>
                    {s.title || t('pages.aiUsage.untitled')}
                  </span>
                ),
              },
              {
                title: t('pages.aiUsage.colProject'),
                key: 'project',
                ellipsis: true,
                render: (_, s) => <span title={s.project}>{projectName(s.project) || '—'}</span>,
              },
              {
                title: t('pages.aiUsage.colModel'),
                dataIndex: 'model',
                key: 'model',
                ellipsis: true,
              },
              ...(showDevice
                ? [{ title: t('pages.aiUsage.colDevice'), dataIndex: 'deviceName', key: 'device' }]
                : []),
              {
                title: t('pages.aiUsage.colTokens'),
                dataIndex: 'tokens',
                key: 'tokens',
                align: 'right' as const,
                render: (v: number) => formatTokens(v),
              },
              {
                title: t('pages.aiUsage.colCost'),
                dataIndex: 'costUsd',
                key: 'cost',
                align: 'right' as const,
                render: (v: number) => formatUsd(v),
              },
              {
                title: t('pages.aiUsage.colLastActive'),
                dataIndex: 'lastAt',
                key: 'lastAt',
                align: 'right' as const,
                render: (v: number) => relativeTime(v),
              },
            ]}
          />
        </div>
      )}
    </Card>
  );
}
