import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Modal, Select, Space, Spin, Typography } from 'antd';

import { YamlEditor } from '@/components/form';
import { usePlansQuery } from '@/api/queries/usePlansQuery';
import {
  useRuleTemplatePreview,
  type RuleTemplatePreviewQuery,
} from '@/api/queries/useRuleTemplates';

export interface RuleTemplatePreviewRequest extends RuleTemplatePreviewQuery {
  name: string;
}

interface RuleTemplatePreviewModalProps {
  request: RuleTemplatePreviewRequest | null;
  onClose: () => void;
}

/** What a plan's first user would get with the template, rendered by the subscription code. */
export default function RuleTemplatePreviewModal({
  request,
  onClose,
}: RuleTemplatePreviewModalProps) {
  const { t } = useTranslation();
  const { plans } = usePlansQuery();
  const [planId, setPlanId] = useState<number | null>(null);

  // The first plan with users is the useful default; any plan can be picked.
  const chosen = planId ?? plans.find((p) => p.memberCount > 0)?.id ?? null;
  const preview = useRuleTemplatePreview(request, chosen);

  function close() {
    setPlanId(null);
    onClose();
  }

  return (
    <Modal
      open={request !== null}
      title={t('pages.rules.previewTitle', { name: request?.name || t('pages.rules.add') })}
      width={960}
      style={{ top: 24 }}
      footer={null}
      onCancel={close}
    >
      <Space orientation="vertical" style={{ width: '100%' }} size="middle">
        <Space wrap>
          <Typography.Text>{t('pages.rules.previewPlan')}</Typography.Text>
          <Select
            style={{ minWidth: 220 }}
            value={chosen ?? undefined}
            placeholder={t('pages.rules.previewPick')}
            onChange={setPlanId}
            options={plans.map((p) => ({
              value: p.id,
              label: `${p.name} (${t('pages.plans.members', { count: p.memberCount })})`,
            }))}
          />
          <Typography.Text type="secondary">{t('pages.rules.previewHint')}</Typography.Text>
        </Space>
        {preview.data && (
          <Typography.Text>
            {t('subscription.portal.username')}: <strong>{preview.data.username}</strong>
          </Typography.Text>
        )}
        {preview.error && <Alert type="error" showIcon title={(preview.error as Error).message} />}
        <Spin spinning={preview.isFetching}>
          <YamlEditor
            value={preview.data?.content ?? ''}
            readOnly
            minHeight="50vh"
            maxHeight="60vh"
          />
        </Spin>
      </Space>
    </Modal>
  );
}
