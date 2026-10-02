import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Checkbox, Form, Modal, Select, Space, Spin, Typography } from 'antd';

import type { RuleTemplateSummary } from '@/generated/zod';
import {
  useRuleTemplateConversion,
  useRuleTemplateMutations,
} from '@/api/queries/useRuleTemplates';
import { SizeFormatter } from '@/utils';
import RuleTemplateChanges from './RuleTemplateChanges';

interface RuleTemplateVariantModalProps {
  template: { id: number; name: string } | null;
  /** The full YAML templates it can become a variant of. */
  bases: RuleTemplateSummary[];
  onClose: () => void;
}

/** 改为变体: keeps a template as only what differs from a base, after saying what that does. */
export default function RuleTemplateVariantModal({
  template,
  bases,
  onClose,
}: RuleTemplateVariantModalProps) {
  const { t } = useTranslation();
  const { convert } = useRuleTemplateMutations();
  const [baseId, setBaseId] = useState<number | null>(null);
  const [allowMove, setAllowMove] = useState(false);
  const [saving, setSaving] = useState(false);
  const check = useRuleTemplateConversion(template?.id ?? null, baseId);
  const result = check.isFetching ? undefined : check.data;
  const name = template?.name ?? '';
  const base = bases.find((b) => b.id === baseId)?.name ?? '';

  function close() {
    setBaseId(null);
    setAllowMove(false);
    onClose();
  }

  async function apply() {
    if (!template || baseId === null) return;
    setSaving(true);
    try {
      const msg = await convert({ id: template.id, baseId, allowReorder: allowMove });
      if (msg?.success) close();
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      open={template !== null}
      title={t('pages.rules.makeVariantTitle', { name })}
      width={640}
      onCancel={close}
      footer={[
        <Button key="cancel" onClick={close}>
          {t('cancel')}
        </Button>,
        <Button
          key="convert"
          type="primary"
          loading={saving}
          disabled={!result || (result.moved > 0 && !allowMove)}
          onClick={apply}
        >
          {t('pages.rules.convert')}
        </Button>,
      ]}
    >
      <Form layout="vertical" component="div">
        <Form.Item label={t('pages.rules.baseTemplate')} htmlFor="variant-base">
          <Select
            id="variant-base"
            value={baseId ?? undefined}
            placeholder={t('pages.rules.pickBase')}
            onChange={(value: number) => {
              setBaseId(value);
              setAllowMove(false);
            }}
            options={bases.map((b) => ({ value: b.id, label: b.name }))}
          />
        </Form.Item>
      </Form>
      <Spin spinning={check.isFetching}>
        {check.error && !check.isFetching && (
          <Alert type="error" showIcon title={(check.error as Error).message} />
        )}
        {result &&
          (result.identical ? (
            <Alert
              type="info"
              showIcon
              title={t('pages.rules.conversion.identical', {
                name,
                base,
                count: result.planCount,
              })}
            />
          ) : (
            <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
              <Typography.Text>
                {t('pages.rules.conversion.keeps', {
                  name,
                  base,
                  size: SizeFormatter.sizeFormat(result.size),
                })}
              </Typography.Text>
              <RuleTemplateChanges changes={result.changes} />
              {result.moved > 0 && (
                <Alert
                  type="warning"
                  showIcon
                  title={t('pages.rules.conversion.moved', { count: result.moved, base })}
                  description={
                    <Checkbox
                      checked={allowMove}
                      onChange={(event) => setAllowMove(event.target.checked)}
                    >
                      {t('pages.rules.conversion.allowMove')}
                    </Checkbox>
                  }
                />
              )}
            </Space>
          ))}
      </Spin>
    </Modal>
  );
}
