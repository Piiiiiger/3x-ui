import { useState } from 'react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Card, Empty, Modal, Table, Tooltip } from 'antd';
import { ArrowDownOutlined, ArrowUpOutlined, ExpandAltOutlined } from '@ant-design/icons';

import { SizeFormatter } from '@/utils';

export interface RankRow {
  key: string;
  name: string;
  up: number;
  down: number;
}

const TOP = 5;

interface RankingCardProps {
  icon: ReactNode;
  title: string;
  nameTitle: string;
  rows: RankRow[];
  footer?: string;
}

function Usage({ up, down }: { up: number; down: number }) {
  return (
    <span className="ov-rank-usage">
      <span>
        <ArrowUpOutlined /> {SizeFormatter.sizeFormat(up)}
      </span>
      <span>
        <ArrowDownOutlined /> {SizeFormatter.sizeFormat(down)}
      </span>
    </span>
  );
}

/** The busiest few of a ranking, with the whole list one click away. */
export default function RankingCard({ icon, title, nameTitle, rows, footer }: RankingCardProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  return (
    <Card className="ov-rank" styles={{ body: { padding: 0 } }}>
      <div className="ov-wide-head">
        <div className="ov-wide-head-stack">
          <div className="ov-kicker ov-card-title ov-kicker-icon">
            {icon}
            {title}
          </div>
          <div className="ov-sub">{t('pages.index.traffic.rankingSub')}</div>
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
            <li key={row.key}>
              <span className="ov-rank-name">{row.name}</span>
              <Usage up={row.up} down={row.down} />
            </li>
          ))}
        </ol>
      )}
      {footer && <div className="ov-rank-foot">{footer}</div>}

      <Modal open={open} title={title} footer={null} width={640} onCancel={() => setOpen(false)}>
        <Table<RankRow>
          size="small"
          rowKey="key"
          pagination={false}
          scroll={{ y: '60vh' }}
          dataSource={rows}
          columns={[
            { title: '#', key: 'rank', width: 48, render: (_, __, i) => i + 1 },
            { title: nameTitle, dataIndex: 'name', key: 'name', ellipsis: true },
            {
              title: <ArrowUpOutlined />,
              dataIndex: 'up',
              key: 'up',
              align: 'right',
              render: (v: number) => SizeFormatter.sizeFormat(v),
            },
            {
              title: <ArrowDownOutlined />,
              dataIndex: 'down',
              key: 'down',
              align: 'right',
              render: (v: number) => SizeFormatter.sizeFormat(v),
            },
            {
              title: t('pages.index.traffic.periodTotal'),
              key: 'total',
              align: 'right',
              render: (_, row) => SizeFormatter.sizeFormat(row.up + row.down),
            },
          ]}
        />
      </Modal>
    </Card>
  );
}
