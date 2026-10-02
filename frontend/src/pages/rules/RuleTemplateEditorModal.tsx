import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Form, Input, Modal, Spin } from 'antd';
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
  onClose: () => void;
  onPreview: (name: string, content: string) => void;
}

const STARTER: RuleTemplateFormValues = {
  name: '',
  content:
    'proxy-groups:\n  - name: PROXY\n    type: select\n    proxies: [__PROXY_NODES__]\nrules:\n  - MATCH,PROXY\n',
};

export default function RuleTemplateEditorModal({
  open,
  templateId,
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

  useEffect(() => {
    if (!open) {
      filledFor.current = null;
      return;
    }
    const target = templateId === null ? 'new' : String(templateId);
    if (filledFor.current === target) return;
    if (templateId === null) {
      methods.reset(STARTER);
    } else if (template && template.id === templateId) {
      methods.reset({ name: template.name, content: template.content });
    } else {
      return;
    }
    filledFor.current = target;
  }, [open, templateId, template, methods]);

  const save = methods.handleSubmit(async (values) => {
    setSaving(true);
    try {
      const msg = templateId === null ? await create(values) : await update(templateId, values);
      if (msg?.success) onClose();
    } finally {
      setSaving(false);
    }
  });

  return (
    <Modal
      open={open}
      title={templateId === null ? t('pages.rules.add') : t('pages.rules.editTitle')}
      width={960}
      style={{ top: 24 }}
      mask={{ closable: false }}
      onCancel={onClose}
      footer={[
        <Button
          key="preview"
          icon={<EyeOutlined />}
          onClick={() => onPreview(methods.getValues('name'), methods.getValues('content'))}
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
        <FormProvider {...methods}>
          <Form layout="vertical" component="div">
            <FormField name="name" label={t('pages.rules.name')} required>
              <Input maxLength={64} />
            </FormField>
            <FormField
              name="content"
              label={t('pages.rules.content')}
              extra={t('pages.rules.contentHint')}
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
