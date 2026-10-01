import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { Button, Card, Empty, Modal, Table, Tag, Typography, message } from 'antd';
import type { TableColumnsType } from 'antd';

import { IntlUtil, SizeFormatter } from '@/utils';
import { useDatepicker } from '@/hooks/useDatepicker';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { usePlansQuery } from '@/api/queries/usePlansQuery';
import { usePlanMutations } from '@/api/queries/usePlanMutations';
import type { AttentionClient } from '@/generated/zod';
import AssignPlanModal from '@/pages/plans/AssignPlanModal';

function isLowOnTraffic(row: AttentionClient) {
  return row.totalGB > 0 && (row.totalGB - row.used) * 10 < row.totalGB;
}

export default function AttentionCard({ clients }: { clients: AttentionClient[] }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { datepicker } = useDatepicker();
  const { isMobile } = useMediaQuery();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [modal, modalContextHolder] = Modal.useModal();
  const { plans } = usePlansQuery();
  const { renew } = usePlanMutations();
  const [assignEmail, setAssignEmail] = useState<string | null>(null);

  const planNames = useMemo(() => new Map(plans.map((p) => [p.id, p.name])), [plans]);

  function statusTag(row: AttentionClient) {
    if (row.status === 'expired') return <Tag color="red">{t('pages.index.traffic.expired')}</Tag>;
    if (row.status === 'usedUp') return <Tag color="red">{t('pages.index.traffic.usedUp')}</Tag>;
    if (isLowOnTraffic(row)) return <Tag color="gold">{t('pages.index.traffic.runningLow')}</Tag>;
    return <Tag color="orange">{t('pages.index.traffic.expiring')}</Tag>;
  }

  function confirmRenew(email: string) {
    modal.confirm({
      title: t('pages.plans.renewConfirm', { count: 1 }),
      content: (
        <>
          <Typography.Text strong>{email}</Typography.Text>
          <Typography.Paragraph type="secondary">{t('pages.plans.renewHint')}</Typography.Paragraph>
        </>
      ),
      okText: t('pages.plans.renew'),
      cancelText: t('cancel'),
      onOk: async () => {
        const msg = await renew([email]);
        if (msg?.success) messageApi.success(t('pages.plans.toasts.renewed', { count: 1 }));
      },
    });
  }

  const columns: TableColumnsType<AttentionClient> = [
    {
      title: t('pages.plans.people'),
      dataIndex: 'email',
      key: 'email',
      render: (email: string) => (
        <Button
          type="link"
          size="small"
          className="ov-attention-email"
          onClick={() => navigate(`/clients?search=${encodeURIComponent(email)}`)}
        >
          {email}
        </Button>
      ),
    },
    {
      title: t('menu.plans'),
      key: 'plan',
      render: (_v, row) => {
        const name = row.planId ? (planNames.get(row.planId) ?? `#${row.planId}`) : undefined;
        return (
          <Tag
            color={name ? 'volcano' : undefined}
            style={{ margin: 0, borderStyle: name ? undefined : 'dashed' }}
          >
            {name ?? t('pages.plans.noPlan')}
          </Tag>
        );
      },
    },
    { title: t('status'), key: 'status', render: (_v, row) => statusTag(row) },
    {
      title: t('pages.clients.expiryTime'),
      key: 'expiry',
      render: (_v, row) =>
        row.expiryTime > 0 ? (
          <>
            <div>{IntlUtil.formatDate(row.expiryTime, datepicker)}</div>
            <Typography.Text type="secondary">
              {IntlUtil.formatRelativeTime(row.expiryTime)}
            </Typography.Text>
          </>
        ) : (
          '∞'
        ),
    },
    {
      title: t('pages.clients.traffic'),
      key: 'traffic',
      render: (_v, row) =>
        `${SizeFormatter.sizeFormat(row.used)} / ${row.totalGB > 0 ? SizeFormatter.sizeFormat(row.totalGB) : '∞'}`,
    },
    {
      key: 'action',
      align: 'end',
      render: (_v, row) =>
        row.planId ? (
          <Button size="small" onClick={() => confirmRenew(row.email)}>
            {t('pages.plans.renew')}
          </Button>
        ) : plans.length > 0 ? (
          <Button size="small" onClick={() => setAssignEmail(row.email)}>
            {t('pages.plans.assign')}
          </Button>
        ) : null,
    },
  ];

  return (
    <Card hoverable styles={{ body: { padding: 0 } }}>
      {messageContextHolder}
      {modalContextHolder}
      <div className="ov-wide-head">
        <div>
          <div className="ov-kicker ov-card-title">
            {t('pages.index.traffic.attention')}
            {clients.length > 0 && <Tag className="ov-attention-count">{clients.length}</Tag>}
          </div>
          <div className="ov-sub">{t('pages.index.traffic.attentionSub')}</div>
        </div>
      </div>
      <div className="ov-attention-body">
        {clients.length === 0 ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={t('pages.index.traffic.attentionEmpty')}
          />
        ) : (
          <Table<AttentionClient>
            rowKey="email"
            size="small"
            columns={columns}
            dataSource={clients}
            pagination={clients.length > 10 ? { pageSize: 10, size: 'small' } : false}
            scroll={isMobile ? { x: 'max-content' } : undefined}
          />
        )}
      </div>
      <AssignPlanModal
        open={assignEmail !== null}
        plans={plans}
        emails={assignEmail ? [assignEmail] : []}
        onClose={() => setAssignEmail(null)}
      />
    </Card>
  );
}
