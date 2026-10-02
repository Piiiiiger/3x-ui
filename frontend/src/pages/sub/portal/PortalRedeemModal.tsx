import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Form, Input, Modal } from 'antd';
import { FormProvider, useForm, useWatch } from 'react-hook-form';

import { FormField, rhfZodValidate } from '@/components/form/rhf';
import { PortalRegistrationSchema } from '@/schemas/portal';

export default function PortalRedeemModal({
  base,
  onClose,
  onActivated,
  onSessionEnded,
}: {
  base: string;
  onClose: () => void;
  onActivated: () => void;
  onSessionEnded: () => void;
}) {
  const { t } = useTranslation();
  const methods = useForm<{ code: string }>({ defaultValues: { code: '' } });
  const code = useWatch({ control: methods.control, name: 'code' });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  async function redeem({ code }: { code: string }) {
    setSaving(true);
    setError('');
    try {
      const res = await fetch(`${base}/redeem`, {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code: code.trim() }),
      });
      const body = await res.json().catch(() => ({}));
      if (res.status === 401) {
        onSessionEnded();
        onClose();
        return;
      }
      if (!res.ok) {
        setError(
          res.status === 429
            ? 'subscription.portal.blocked'
            : body.error === 'code'
              ? 'subscription.portal.badCode'
              : 'subscription.portal.failed',
        );
        return;
      }
      onActivated();
      onClose();
    } catch {
      setError('subscription.portal.failed');
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      open
      title={t('subscription.portal.redeem')}
      onCancel={onClose}
      onOk={methods.handleSubmit(redeem)}
      okText={t('subscription.portal.redeem')}
      okButtonProps={{ disabled: !code.trim() }}
      confirmLoading={saving}
    >
      {error && <Alert type="error" showIcon title={t(error)} />}
      <FormProvider {...methods}>
        <Form layout="vertical" onFinish={methods.handleSubmit(redeem)}>
          <FormField
            name="code"
            label={t('subscription.portal.code')}
            rules={{ validate: rhfZodValidate(PortalRegistrationSchema.shape.code) }}
          >
            <Input autoComplete="off" maxLength={64} />
          </FormField>
        </Form>
      </FormProvider>
    </Modal>
  );
}
