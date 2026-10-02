import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Checkbox, Form, InputNumber, Modal, message } from 'antd';

import { useClientRenew } from '@/api/queries/useClientRenew';

interface ClientRenewModalProps {
  open: boolean;
  emails: string[];
  onClose: () => void;
  onRenewed?: () => void;
}

const DEFAULT_DAYS = 30;

/** 续期: days on top of the later of today and each user's expiry, the usage cleared or kept. */
export default function ClientRenewModal({
  open,
  emails,
  onClose,
  onRenewed,
}: ClientRenewModalProps) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const renew = useClientRenew();
  const [days, setDays] = useState<number | null>(DEFAULT_DAYS);
  const [resetUsage, setResetUsage] = useState(true);
  const [saving, setSaving] = useState(false);

  // Closed, it starts over: the next renewal opens on a month with the usage cleared.
  function close() {
    setDays(DEFAULT_DAYS);
    setResetUsage(true);
    onClose();
  }

  async function submit() {
    if (!days || days < 1) return;
    setSaving(true);
    try {
      const msg = await renew({ emails, days, resetUsage });
      if (msg?.success) {
        messageApi.success(t('pages.plans.toasts.renewed', { count: emails.length }));
        onRenewed?.();
        close();
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      open={open}
      title={
        emails.length === 1
          ? t('pages.clients.renewOneTitle', { email: emails[0] })
          : t('pages.clients.renewManyTitle', { count: emails.length })
      }
      okText={t('pages.plans.renew')}
      cancelText={t('cancel')}
      okButtonProps={{ disabled: !days || days < 1 }}
      confirmLoading={saving}
      width={440}
      onOk={submit}
      onCancel={close}
    >
      {messageContextHolder}
      <Form layout="vertical" component="div">
        <Form.Item
          label={t('pages.clients.renewAddDays')}
          htmlFor="client-renew-days"
          extra={t('pages.clients.renewHint')}
        >
          <InputNumber
            id="client-renew-days"
            min={1}
            precision={0}
            value={days}
            onChange={(value) => setDays(value)}
            suffix={t('pages.plans.daysUnit')}
            style={{ width: '100%' }}
          />
        </Form.Item>
        <Checkbox checked={resetUsage} onChange={(event) => setResetUsage(event.target.checked)}>
          {t('pages.plans.resetTraffic')}
        </Checkbox>
      </Form>
    </Modal>
  );
}
