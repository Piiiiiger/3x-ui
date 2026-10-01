import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import {
  Button,
  Card,
  Col,
  ConfigProvider,
  Empty,
  Layout,
  Modal,
  Result,
  Row,
  Spin,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  TeamOutlined,
  UserAddOutlined,
} from '@ant-design/icons';

import AppNav from '@/layouts/AppNav';
import { PageHeader } from '@/components/ui';
import { useTheme } from '@/hooks/useTheme';
import { usePlansQuery } from '@/api/queries/usePlansQuery';
import { usePlanMutations } from '@/api/queries/usePlanMutations';
import type { PlanSummary } from '@/generated/zod';
import type { PlanFormValues } from '@/schemas/plan';
import AssignPlanModal from './AssignPlanModal';
import PlanFormModal from './PlanFormModal';
import { usePlanText } from '@/lib/plans/planText';
import { useInboundChoices } from './planText';
import './PlansPage.css';

export default function PlansPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [modal, modalContextHolder] = Modal.useModal();
  const { plans, fetched, fetchError, loading, refetch } = usePlansQuery();
  const { create, update, remove } = usePlanMutations();
  const planText = usePlanText();
  const { labelOf } = useInboundChoices();

  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<PlanSummary | null>(null);
  const [assignPlanId, setAssignPlanId] = useState<number | null>(null);

  const pageClass = useMemo(() => {
    const classes = ['plans-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  function openForm(plan: PlanSummary | null) {
    setEditing(plan);
    setFormOpen(true);
  }

  async function savePlan(values: PlanFormValues, applyToMembers: boolean) {
    const msg = editing ? await update(editing.id, values, applyToMembers) : await create(values);
    if (msg?.success) {
      messageApi.success(t('pages.plans.toasts.saved'));
      setFormOpen(false);
    }
  }

  function confirmDelete(plan: PlanSummary) {
    if (plan.memberCount > 0) {
      messageApi.warning(t('pages.plans.deleteBlocked', { count: plan.memberCount }));
      return;
    }
    modal.confirm({
      title: t('pages.plans.deleteConfirm', { name: plan.name }),
      okText: t('delete'),
      okType: 'danger',
      cancelText: t('cancel'),
      onOk: async () => {
        const msg = await remove(plan.id);
        if (msg?.success) messageApi.success(t('pages.plans.toasts.deleted'));
      },
    });
  }

  const addButton = (
    <Button type="primary" icon={<PlusOutlined />} onClick={() => openForm(null)}>
      {t('pages.plans.add')}
    </Button>
  );

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      {modalContextHolder}
      <Layout className={pageClass}>
        <AppNav />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <PageHeader
              title={t('menu.plans')}
              description={t('pages.plans.intro')}
              extra={plans.length > 0 ? addButton : undefined}
            />
            <Spin spinning={!fetched} delay={200} description={t('loading')} size="large">
              {!fetched ? (
                <div className="loading-spacer" />
              ) : fetchError ? (
                <Result
                  status="error"
                  title={t('somethingWentWrong')}
                  subTitle={fetchError}
                  extra={
                    <Button type="primary" loading={loading} onClick={() => refetch()}>
                      {t('refresh')}
                    </Button>
                  }
                />
              ) : plans.length === 0 ? (
                <Card>
                  <Empty description={t('pages.plans.empty')}>{addButton}</Empty>
                </Card>
              ) : (
                <Row gutter={[16, 16]}>
                  {plans.map((plan) => {
                    const text = planText(plan);
                    return (
                      <Col xs={24} md={12} xl={8} key={plan.id}>
                        <Card
                          className="plan-card"
                          title={plan.name}
                          extra={
                            <Button
                              type="text"
                              size="small"
                              icon={<TeamOutlined />}
                              onClick={() => navigate(`/clients?plan=${plan.id}`)}
                            >
                              {t('pages.plans.members', { count: plan.memberCount })}
                            </Button>
                          }
                          actions={[
                            <Button
                              key="assign"
                              type="text"
                              icon={<UserAddOutlined />}
                              onClick={() => setAssignPlanId(plan.id)}
                            >
                              {t('pages.plans.addPeople')}
                            </Button>,
                            <Button
                              key="edit"
                              type="text"
                              icon={<EditOutlined />}
                              onClick={() => openForm(plan)}
                            >
                              {t('edit')}
                            </Button>,
                            <Button
                              key="delete"
                              type="text"
                              danger
                              icon={<DeleteOutlined />}
                              onClick={() => confirmDelete(plan)}
                            >
                              {t('delete')}
                            </Button>,
                          ]}
                        >
                          <div className="plan-card-headline">
                            <span className="plan-card-quota">{text.quota}</span>
                            <span className="plan-card-duration">/ {text.duration}</span>
                          </div>
                          <dl className="plan-card-facts">
                            <dt>{t('pages.inbounds.periodicTrafficResetTitle')}</dt>
                            <dd>{text.reset}</dd>
                            <dt>{t('pages.clients.limitIp')}</dt>
                            <dd>{text.ipLimit}</dd>
                            <dt>{t('pages.plans.clashRules')}</dt>
                            <dd>
                              {plan.clashRules
                                ? t('pages.plans.clashCustom')
                                : t('pages.plans.clashInherit')}
                            </dd>
                            <dt>{t('pages.plans.servers')}</dt>
                            <dd>
                              {plan.inboundIds.length > 0 ? (
                                plan.inboundIds.map((id) => (
                                  <Tag key={id} className="plan-card-server">
                                    {labelOf(id)}
                                  </Tag>
                                ))
                              ) : (
                                <Typography.Text type="secondary">
                                  {t('pages.plans.noServers')}
                                </Typography.Text>
                              )}
                            </dd>
                          </dl>
                          {plan.remark && (
                            <Typography.Paragraph type="secondary" className="plan-card-remark">
                              {plan.remark}
                            </Typography.Paragraph>
                          )}
                        </Card>
                      </Col>
                    );
                  })}
                </Row>
              )}
            </Spin>
          </Layout.Content>
        </Layout>
      </Layout>
      <PlanFormModal
        open={formOpen}
        plan={editing}
        onClose={() => setFormOpen(false)}
        onConfirm={savePlan}
      />
      <AssignPlanModal
        open={assignPlanId !== null}
        plans={plans}
        emails={[]}
        planId={assignPlanId ?? undefined}
        onClose={() => setAssignPlanId(null)}
      />
    </ConfigProvider>
  );
}
