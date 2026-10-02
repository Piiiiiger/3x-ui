import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Flex, Form, Input, Modal, Spin } from 'antd';
import { FormProvider } from 'react-hook-form';

import { FormField, useZodForm } from '@/components/form/rhf';
import { useProbeMutations } from '@/api/queries/useProbeMutations';
import { useProbeSettingsQuery } from '@/api/queries/useProbeSettingsQuery';
import type { ProbeSettings } from '@/generated/zod';
import { ProbeSettingsFormSchema, type ProbeSettingsFormValues } from '@/schemas/probe';
import { useOpenings } from './useOpenings';
import './ProbeModals.css';

const LITE_URL_EXAMPLE = 'http://127.0.0.1:27777';

interface ProbeSettingsFormProps {
  settings: ProbeSettings;
  onClose: () => void;
  onSaved: () => void;
}

function ProbeSettingsForm({ settings, onClose, onSaved }: ProbeSettingsFormProps) {
  const { t } = useTranslation();
  const { saveSettings } = useProbeMutations();
  const methods = useZodForm(ProbeSettingsFormSchema, { defaultValues: settings });
  const [saving, setSaving] = useState(false);

  async function submit(values: ProbeSettingsFormValues) {
    setSaving(true);
    try {
      const msg = await saveSettings(values);
      if (msg?.success) onSaved();
    } finally {
      setSaving(false);
    }
  }

  return (
    <FormProvider {...methods}>
      <Form layout="vertical" onFinish={methods.handleSubmit(submit)}>
        <FormField
          name="url"
          label={t('pages.probe.liteUrl')}
          extra={
            <>
              {t('pages.probe.liteUrlHint')}
              {/* On a line of its own: inside a right-to-left sentence a URL gets reordered. */}
              <code className="probe-settings-example" dir="ltr">
                {LITE_URL_EXAMPLE}
              </code>
            </>
          }
        >
          <Input placeholder={LITE_URL_EXAMPLE} autoComplete="off" spellCheck={false} />
        </FormField>
        <FormField
          name="publicUrl"
          label={t('pages.probe.publicUrl')}
          extra={t('pages.probe.publicUrlHint')}
        >
          <Input placeholder="https://" autoComplete="off" spellCheck={false} />
        </FormField>
        <Flex justify="end" gap={8}>
          <Button onClick={onClose}>{t('cancel')}</Button>
          <Button type="primary" htmlType="submit" loading={saving}>
            {t('save')}
          </Button>
        </Flex>
      </Form>
    </FormProvider>
  );
}

// Saving a form that failed to load would store its empty fields, so the form
// exists only once the stored settings have arrived.
function ProbeSettingsBody(props: Omit<ProbeSettingsFormProps, 'settings'>) {
  const { settings, fetchError } = useProbeSettingsQuery();
  if (fetchError) return <Alert type="error" showIcon title={fetchError} />;
  if (!settings) return <Spin />;
  return <ProbeSettingsForm settings={settings} {...props} />;
}

interface ProbeSettingsModalProps {
  open: boolean;
  onClose: () => void;
  onSaved: () => void;
}

export default function ProbeSettingsModal({ open, onClose, onSaved }: ProbeSettingsModalProps) {
  const { t } = useTranslation();
  const openings = useOpenings(open);
  return (
    <Modal
      open={open}
      title={t('pages.probe.settingsTitle')}
      width="520px"
      footer={null}
      destroyOnHidden
      onCancel={onClose}
    >
      <ProbeSettingsBody key={openings} onClose={onClose} onSaved={onSaved} />
    </Modal>
  );
}
