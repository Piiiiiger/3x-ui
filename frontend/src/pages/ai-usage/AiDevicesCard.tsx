import { useTranslation } from 'react-i18next';
import { Button, Card, Modal, Table } from 'antd';
import { DesktopOutlined, PlusOutlined } from '@ant-design/icons';

import type { AiUsageDeviceView } from '@/generated/zod';
import { formatUsd } from './aiUsageFormat';

// Pigger Switch pushes every 10 minutes while it runs; an hour without one is worth a look.
const STALE_AFTER_SECONDS = 3600;

interface AiDevicesCardProps {
  devices: AiUsageDeviceView[];
  nowMs: number;
  relativeTime: (unixSeconds?: number) => string;
  onConnect: () => void;
  onDelete: (device: AiUsageDeviceView) => void;
}

/** The computers that report here, how fresh their data is and what each has cost so far. */
export default function AiDevicesCard({
  devices,
  nowMs,
  relativeTime,
  onConnect,
  onDelete,
}: AiDevicesCardProps) {
  const { t } = useTranslation();
  const [modal, contextHolder] = Modal.useModal();
  const now = Math.floor(nowMs / 1000);

  const confirmDelete = (device: AiUsageDeviceView) =>
    modal.confirm({
      title: t('pages.aiUsage.deleteDeviceConfirm', { name: device.name }),
      okText: t('pages.aiUsage.deleteDevice'),
      okType: 'danger',
      cancelText: t('cancel'),
      onOk: () => onDelete(device),
    });

  return (
    <Card styles={{ body: { padding: 0 } }}>
      {contextHolder}
      <div className="ov-wide-head">
        <div className="ov-wide-head-stack">
          <div className="ov-kicker ov-card-title ov-kicker-icon">
            <DesktopOutlined />
            {t('pages.aiUsage.devices')}
          </div>
          <div className="ov-sub">{t('pages.aiUsage.devicesSub')}</div>
        </div>
        <Button className="ov-rank-expand" icon={<PlusOutlined />} onClick={onConnect}>
          {t('pages.aiUsage.connect')}
        </Button>
      </div>
      <div className="ov-hosts-table">
        <Table<AiUsageDeviceView>
          size="small"
          rowKey="id"
          pagination={false}
          scroll={{ x: 'max-content' }}
          dataSource={devices}
          columns={[
            { title: t('pages.aiUsage.colDevice'), dataIndex: 'name', key: 'name' },
            { title: t('pages.aiUsage.colVersion'), dataIndex: 'appVersion', key: 'version' },
            {
              title: t('pages.aiUsage.colLastSync'),
              key: 'lastSync',
              render: (_, d) => (
                <span className={now - d.lastSyncAt > STALE_AFTER_SECONDS ? 'ai-stale' : undefined}>
                  {relativeTime(d.lastSyncAt)}
                </span>
              ),
            },
            {
              title: t('pages.aiUsage.colRange'),
              key: 'range',
              render: (_, d) => (d.firstDay ? `${d.firstDay} → ${d.lastDay}` : '—'),
            },
            {
              title: t('pages.aiUsage.colAllTime'),
              dataIndex: 'costUsd',
              key: 'cost',
              align: 'right',
              render: (v: number) => formatUsd(v),
            },
            {
              key: 'actions',
              align: 'right',
              render: (_, d) => (
                <Button size="small" type="text" danger onClick={() => confirmDelete(d)}>
                  {t('pages.aiUsage.deleteDevice')}
                </Button>
              ),
            },
          ]}
        />
      </div>
    </Card>
  );
}
