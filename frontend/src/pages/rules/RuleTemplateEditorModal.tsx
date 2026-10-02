import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Form, Input, Modal, Spin, Typography } from 'antd';
import { EyeOutlined } from '@ant-design/icons';
import { FormProvider } from 'react-hook-form';

import { YamlEditor } from '@/components/form';
import { FormField, useZodForm } from '@/components/form/rhf';
import { useRuleTemplate, useRuleTemplateMutations } from '@/api/queries/useRuleTemplates';
import { RuleTemplateFormSchema, type RuleTemplateFormValues } from '@/schemas/ruleTemplate';

interface RuleTemplateEditorModalProps {
  open: boolean;
  /** The template to edit; null creates one. */
  templateId: number | null;
  /** For a new template, the template it is a variant of; 0 for a full one. */
  newBaseId: number;
  baseNameOf: (id: number) => string;
  onClose: () => void;
  onPreview: (name: string, content: string, baseId: number) => void;
}

const STARTER: RuleTemplateFormValues = {
  name: '',
  content:
    'proxy-groups:\n  - name: PROXY\n    type: select\n    proxies: [__PROXY_NODES__]\nrules:\n  - MATCH,PROXY\n',
};

// A variant starts as its base unchanged.
const VARIANT_STARTER: RuleTemplateFormValues = { name: '', content: 'prepend-rules: []\n' };

export default function RuleTemplateEditorModal({
  open,
  templateId,
  newBaseId,
  baseNameOf,
  onClose,
  onPreview,
}: RuleTemplateEditorModalProps) {
  const { t } = useTranslation();
  const { create, update } = useRuleTemplateMutations();
  const methods = useZodForm(RuleTemplateFormSchema, { defaultValues: STARTER });
  const [saving, setSaving] = useState(false);
  const { data: template, isFetching: loading } = useRuleTemplate(open ? templateId : null);
  // The form is filled once per opening, so nothing arriving later undoes an edit.
  const filledFor = useRef<string | null>(null);
  const baseId =
    templateId === null ? newBaseId : template?.id === templateId ? template.baseId : 0;

  useEffect(() => {
    if (!open) {
      filledFor.current = null;
      return;
    }
    const target = templateId === null ? `new-${newBaseId}` : String(templateId);
    if (filledFor.current === target) return;
    if (templateId === null) {
      methods.reset(newBaseId ? VARIANT_STARTER : STARTER);
    } else if (template && template.id === templateId) {
      methods.reset({ name: template.name, content: template.content });
    } else {
      return;
    }
    filledFor.current = target;
  }, [open, templateId, newBaseId, template, methods]);

  const save = methods.handleSubmit(async (values) => {
    setSaving(true);
    try {
      const input = { ...values, baseId };
      const msg = templateId === null ? await create(input) : await update(templateId, input);
      if (msg?.success) onClose();
    } finally {
      setSaving(false);
    }
  });

  const title =
    templateId !== null
      ? t('pages.rules.editTitle')
      : newBaseId
        ? t('pages.rules.addVariantTitle', { name: baseNameOf(newBaseId) })
        : t('pages.rules.add');

  return (
    <Modal
      open={open}
      title={title}
      width={960}
      style={{ top: 24 }}
      mask={{ closable: false }}
      onCancel={onClose}
      footer={[
        <Button
          key="preview"
          icon={<EyeOutlined />}
          onClick={() => onPreview(methods.getValues('name'), methods.getValues('content'), baseId)}
        >
          {t('pages.rules.preview')}
        </Button>,
        <Button key="cancel" onClick={onClose}>
          {t('cancel')}
        </Button>,
        <Button key="save" type="primary" loading={saving} disabled={loading} onClick={save}>
          {t('save')}
        </Button>,
      ]}
    >
      <Spin spinning={loading}>
        {baseId !== 0 && (
          <Typography.Paragraph type="secondary">
            {t('pages.rules.basedOn', { name: baseNameOf(baseId) })}
          </Typography.Paragraph>
        )}
        <FormProvider {...methods}>
          <Form layout="vertical" component="div">
            <FormField name="name" label={t('pages.rules.name')} required>
              <Input maxLength={64} />
            </FormField>
            <FormField
              name="content"
              label={t('pages.rules.content')}
              extra={baseId ? t('pages.rules.variantHint') : t('pages.rules.contentHint')}
              required
            >
              <YamlEditor value="" minHeight="50vh" maxHeight="60vh" />
            </FormField>
          </Form>
        </FormProvider>
      </Spin>
    </Modal>
  );
}
