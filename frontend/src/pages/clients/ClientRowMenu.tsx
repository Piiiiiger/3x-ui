import { memo } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Dropdown } from 'antd';
import {
  CheckCircleOutlined,
  DeleteOutlined,
  EditOutlined,
  FieldTimeOutlined,
  InfoCircleOutlined,
  KeyOutlined,
  MoreOutlined,
  QrcodeOutlined,
  RetweetOutlined,
  StopOutlined,
} from '@ant-design/icons';

export interface ClientRowHandlers {
  onShowQr: (email: string) => void;
  onShowInfo: (email: string) => void;
  onEdit: (email: string) => void;
  onResetTraffic: (email: string) => void;
  onRenew: (email: string) => void;
  onSetEnable: (email: string, enable: boolean) => void;
  onPortal: (email: string) => void;
  onDelete: (email: string) => void;
}

interface ClientRowMenuProps extends ClientRowHandlers {
  email: string;
  enabled: boolean;
  hasPlan: boolean;
}

/** 操作: one "…" menu per row instead of a column of icon buttons. */
const ClientRowMenu = memo(function ClientRowMenu({
  email,
  enabled,
  hasPlan,
  ...on
}: ClientRowMenuProps) {
  const { t } = useTranslation();
  return (
    <Dropdown
      trigger={['click']}
      placement="bottomRight"
      menu={{
        items: [
          {
            key: 'qr',
            icon: <QrcodeOutlined />,
            label: t('pages.clients.qrCode'),
            onClick: () => on.onShowQr(email),
          },
          {
            key: 'info',
            icon: <InfoCircleOutlined />,
            label: t('pages.clients.clientInfo'),
            onClick: () => on.onShowInfo(email),
          },
          {
            key: 'edit',
            icon: <EditOutlined />,
            label: t('edit'),
            onClick: () => on.onEdit(email),
          },
          {
            key: 'reset',
            icon: <RetweetOutlined />,
            label: t('pages.inbounds.resetTraffic'),
            onClick: () => on.onResetTraffic(email),
          },
          {
            key: 'renew',
            icon: <FieldTimeOutlined />,
            label: hasPlan ? t('pages.plans.renew') : t('pages.clients.renewNeedsPlan'),
            disabled: !hasPlan,
            onClick: () => on.onRenew(email),
          },
          enabled
            ? {
                key: 'disable',
                icon: <StopOutlined />,
                label: t('pages.clients.disable'),
                onClick: () => on.onSetEnable(email, false),
              }
            : {
                key: 'enable',
                icon: <CheckCircleOutlined />,
                label: t('pages.clients.enable'),
                onClick: () => on.onSetEnable(email, true),
              },
          {
            key: 'portal',
            icon: <KeyOutlined />,
            label: t('pages.clients.portal.title'),
            onClick: () => on.onPortal(email),
          },
          { type: 'divider' },
          {
            key: 'delete',
            icon: <DeleteOutlined />,
            label: t('delete'),
            danger: true,
            onClick: () => on.onDelete(email),
          },
        ],
      }}
    >
      <Button size="small" icon={<MoreOutlined />} aria-label={t('more')} />
    </Dropdown>
  );
});

export default ClientRowMenu;
