import { useState } from 'react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Card, Empty, Modal, Table, Tooltip } from 'antd';
import { ExpandAltOutlined } from '@ant-design/icons';

import type { AiUsageRank } from '@/generated/zod';
import { formatTokens, formatUsd } from './aiUsageFormat';

const TOP = 5;

interface AiRankCardProps {
  icon: ReactNode;
  title: string;
  nameTitle: string;
  rows: AiUsageRank[];
  // Projects show a folder's name with its path on hover; models show the id as is.
  displayName?: (row: AiUsageRank) => string;
}

/** The costliest few of a ranking, with the whole list one click away. */
export default function AiRankCard({ icon, title, nameTitle, rows, displayName }: AiRankCardProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const label = (row: AiUsageRank) => (displayName ? displayName(row) : row.name);

  return (
    <Card className="ov-rank" styles={{ body: { padding: 0 } }}>
      <div className="ov-wide-head">
        <div className="ov-wide-head-stack">
          <div className="ov-kicker ov-card-title ov-kicker-icon">
            {icon}
            {title}
          </div>
          <div className="ov-sub">{t('pages.aiUsage.rankingSub')}</div>
        </div>
        <Tooltip title={t('pages.index.traffic.showAll')}>
          <Button
            className="ov-rank-expand"
            type="text"
            icon={<ExpandAltOutlined />}
            aria-label={`${title}: ${t('pages.index.traffic.showAll')}`}
            disabled={rows.length === 0}
            onClick={() => setOpen(true)}
          />
        </Tooltip>
      </div>
      {rows.length === 0 ? (
        <Empty
          className="ov-rank-empty"
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={t('pages.index.traffic.rankEmpty')}
        />
      ) : (
        <ol className="ov-rank-list">
          {rows.slice(0, TOP).map((row) => (
            <li key={row.name}>
              <span className="ov-rank-name" title={row.name}>
                {label(row)}
              </span>
              <span className="ov-rank-usage">
                <span>{formatTokens(row.tokens)}</span>
                <span className="ai-rank-cost">{formatUsd(row.costUsd)}</span>
              </span>
            </li>
          ))}
        </ol>
      )}

      <Modal open={open} title={title} footer={null} width={720} onCancel={() => setOpen(false)}>
        <Table<AiUsageRank>
          size="small"
          rowKey="name"
          pagination={false}
          scroll={{ y: '60vh' }}
          dataSource={rows}
          columns={[
            { title: '#', key: 'rank', width: 48, render: (_, __, i) => i + 1 },
            {
              title: nameTitle,
              key: 'name',
              ellipsis: true,
              render: (_, row) => <span title={row.name}>{label(row)}</span>,
            },
            {
              title: t('pages.aiUsage.colRequests'),
              dataIndex: 'requests',
              key: 'requests',
              align: 'right',
            },
            {
              title: t('pages.aiUsage.colTokens'),
              dataIndex: 'tokens',
              key: 'tokens',
              align: 'right',
              render: (v: number) => formatTokens(v),
            },
            {
              title: t('pages.aiUsage.colCost'),
              dataIndex: 'costUsd',
              key: 'cost',
              align: 'right',
              render: (v: number) => formatUsd(v),
            },
          ]}
        />
      </Modal>
    </Card>
  );
}
