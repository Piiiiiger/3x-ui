import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Form, Input, Modal, Typography, message } from 'antd';

import { ClipboardManager, HttpUtil } from '@/utils';

interface CreatedToken {
  token?: string;
}

/** Where Pigger Switch posts its reports: this panel's address with its secret base path. */
export function panelUrl(): string {
  const base = (window.X_UI_BASE_PATH || '/').replace(/\/+$/, '');
  return `${window.location.origin}${base}`;
}

interface ConnectDeviceModalProps {
  open: boolean;
  onClose: () => void;
}

/** Mints an upload-only token for one computer and shows what to paste into Pigger Switch. */
export default function ConnectDeviceModal({ open, onClose }: ConnectDeviceModalProps) {
  const { t } = useTranslation();
  const [messageApi, contextHolder] = message.useMessage();
  const [name, setName] = useState('');
  const [creating, setCreating] = useState(false);
  const [token, setToken] = useState('');

  const close = () => {
    setName('');
    setToken('');
    onClose();
  };

  const create = async () => {
    const label = name.trim();
    if (!label) {
      messageApi.error(t('pages.aiUsage.connectNameRequired'));
      return;
    }
    setCreating(true);
    try {
      const msg = await HttpUtil.post<CreatedToken>('/panel/api/setting/apiTokens/create', {
        name: `ai-usage-${label}`,
        scope: 'ai-usage',
      });
      if (msg?.success && msg.obj?.token) setToken(msg.obj.token);
    } finally {
      setCreating(false);
    }
  };

  const copy = async (text: string) => {
    if (await ClipboardManager.copyText(text)) messageApi.success(t('copySuccess'));
  };

  return (
    <Modal
      open={open}
      title={t('pages.aiUsage.connect')}
      okText={token ? t('done') : t('confirm')}
      cancelButtonProps={token ? { style: { display: 'none' } } : undefined}
      cancelText={t('cancel')}
      confirmLoading={creating}
      onOk={token ? close : create}
      onCancel={close}
    >
      {contextHolder}
      {!token ? (
        <Form layout="vertical">
          <p className="ai-connect-hint">{t('pages.aiUsage.connectHint')}</p>
          <Form.Item label={t('pages.aiUsage.connectName')} required>
            <Input
              value={name}
              maxLength={40}
              placeholder={t('pages.aiUsage.connectNamePlaceholder')}
              onChange={(e) => setName(e.target.value)}
              onPressEnter={create}
            />
          </Form.Item>
        </Form>
      ) : (
        <div className="ai-connect-done">
          <p>{t('pages.aiUsage.connectDone')}</p>
          <div className="ai-connect-field">
            <Typography.Text type="secondary">{t('pages.aiUsage.connectUrl')}</Typography.Text>
            <div className="ai-connect-value">
              <code>{panelUrl()}</code>
              <Button size="small" onClick={() => copy(panelUrl())}>
                {t('copy')}
              </Button>
            </div>
          </div>
          <div className="ai-connect-field">
            <Typography.Text type="secondary">{t('pages.aiUsage.connectToken')}</Typography.Text>
            <div className="ai-connect-value">
              <code>{token}</code>
              <Button size="small" type="primary" onClick={() => copy(token)}>
                {t('copy')}
              </Button>
            </div>
          </div>
        </div>
      )}
    </Modal>
  );
}
