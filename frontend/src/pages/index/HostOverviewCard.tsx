import { useTranslation } from 'react-i18next';
import { Card, Table } from 'antd';
import { ArrowDownOutlined, ArrowUpOutlined, CloudServerOutlined } from '@ant-design/icons';

import { RainbowBar } from '@/components/ui';
import { SizeFormatter } from '@/utils';
import type { TrafficHost } from '@/generated/zod';
import type { Speed } from './trafficOverview';

interface HostOverviewCardProps {
  hosts: TrafficHost[];
  speeds: Map<number, Speed>;
  hostName: (host: TrafficHost) => string;
}

/** Each host's live speed and where it stands against its quota this billing cycle. */
export default function HostOverviewCard({ hosts, speeds, hostName }: HostOverviewCardProps) {
  const { t } = useTranslation();
  const dash = <span className="ov-faint">—</span>;

  return (
    <Card className="ov-hosts" styles={{ body: { padding: 0 } }}>
      <div className="ov-wide-head ov-wide-head-stack">
        <div className="ov-kicker ov-card-title ov-kicker-icon">
          <CloudServerOutlined />
          {t('pages.index.traffic.hosts')}
        </div>
        <div className="ov-sub">{t('pages.index.traffic.hostsSub')}</div>
      </div>
      <Table<TrafficHost>
        className="ov-hosts-table"
        size="small"
        rowKey="nodeId"
        pagination={false}
        scroll={{ x: 'max-content' }}
        dataSource={hosts}
        columns={[
          {
            title: t('pages.index.traffic.colHost'),
            key: 'host',
            render: (_, host) => hostName(host),
          },
          {
            title: t('pages.index.traffic.colSpeed'),
            key: 'speed',
            render: (_, host) => {
              const speed = speeds.get(host.nodeId) ?? { up: 0, down: 0 };
              return (
                <span className="ov-rank-usage">
                  <span className="ov-up">
                    <ArrowUpOutlined /> {SizeFormatter.speedFormat(speed.up)}
                  </span>
                  <span className="ov-down">
                    <ArrowDownOutlined /> {SizeFormatter.speedFormat(speed.down)}
                  </span>
                </span>
              );
            },
          },
          {
            title: t('pages.index.traffic.colUsed'),
            key: 'used',
            align: 'right',
            render: (_, host) => (host.linked ? SizeFormatter.sizeFormat(host.usedBytes) : dash),
          },
          {
            title: t('pages.index.traffic.colQuota'),
            key: 'quota',
            align: 'right',
            render: (_, host) => {
              if (!host.linked)
                return <span className="ov-faint">{t('pages.index.traffic.unlinked')}</span>;
              return host.quotaBytes > 0
                ? SizeFormatter.sizeFormat(host.quotaBytes)
                : t('pages.index.traffic.unlimited');
            },
          },
          {
            title: t('pages.index.traffic.colRemaining'),
            key: 'remaining',
            align: 'right',
            render: (_, host) =>
              host.linked && host.quotaBytes > 0
                ? SizeFormatter.sizeFormat(Math.max(host.quotaBytes - host.usedBytes, 0))
                : dash,
          },
          {
            title: t('pages.index.traffic.usageRate'),
            key: 'usage',
            width: 160,
            render: (_, host) => {
              if (!host.linked || host.quotaBytes <= 0) return dash;
              const rate = Math.min((host.usedBytes / host.quotaBytes) * 100, 100);
              return (
                <div className="ov-usage">
                  <RainbowBar
                    percent={rate}
                    label={t('pages.index.traffic.usageRate')}
                    valueText={`${SizeFormatter.sizeFormat(host.usedBytes)} / ${SizeFormatter.sizeFormat(host.quotaBytes)}`}
                  />
                  <span className="ov-usage-value">{rate.toFixed(1)}%</span>
                </div>
              );
            },
          },
        ]}
      />
    </Card>
  );
}
