import { useTranslation } from 'react-i18next';
import { Alert, Button, Input, Modal, Space, Typography, message } from 'antd';
import { CopyOutlined } from '@ant-design/icons';

import { ClipboardManager } from '@/utils';
import { withBasePath } from '@/api/http-init';

// The agent dials this panel at the URL the admin is using right now.
export function agentMasterUrl(): string {
  return window.location.origin + withBasePath('/');
}

export function agentConfigFile(master: string, secret: string): string {
  return JSON.stringify({ master, secret }, null, 2);
}

export function agentInstallCommand(master: string, secret: string): string {
  return `sh install.sh ${master} ${secret}`;
}

interface AgentSecretModalProps {
  // A secret to show, or null when the dialog is closed.
  secret: string | null;
  nodeName: string;
  onClose: () => void;
}

export default function AgentSecretModal({ secret, nodeName, onClose }: AgentSecretModalProps) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const master = agentMasterUrl();
  const install = secret ? agentInstallCommand(master, secret) : '';
  const config = secret ? agentConfigFile(master, secret) : '';

  async function copy(text: string) {
    if (await ClipboardManager.copyText(text)) messageApi.success(t('copied'));
  }

  return (
    <Modal
      open={secret !== null}
      title={t('pages.nodes.agentSecretTitle', { name: nodeName })}
      onCancel={onClose}
      mask={{ closable: false }}
      width="640px"
      footer={
        <Button type="primary" onClick={onClose}>
          {t('close')}
        </Button>
      }
    >
      {messageContextHolder}
      <Alert
        type="warning"
        showIcon
        style={{ marginBottom: 16 }}
        title={t('pages.nodes.agentSecretOnce')}
      />
      <Typography.Text strong>{t('pages.nodes.agentInstallCommand')}</Typography.Text>
      <Space.Compact style={{ width: '100%', margin: '8px 0 16px' }}>
        <Input readOnly value={install} />
        <Button icon={<CopyOutlined />} aria-label={t('copy')} onClick={() => copy(install)} />
      </Space.Compact>
      <Typography.Text strong>/etc/pigger-agent/config.json</Typography.Text>
      <Input.TextArea
        readOnly
        autoSize
        value={config}
        style={{ marginTop: 8, fontFamily: 'monospace' }}
      />
      <Button icon={<CopyOutlined />} style={{ marginTop: 8 }} onClick={() => copy(config)}>
        {t('copy')}
      </Button>
    </Modal>
  );
}
