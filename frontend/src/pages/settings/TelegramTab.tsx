import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Input, Select, Space, Switch, Tabs } from 'antd';
import { BellOutlined, SendOutlined, SettingOutlined } from '@ant-design/icons';
import { HttpUtil, LanguageManager } from '@/utils';
import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';
import { TelegramNotifications } from '@/components/ui/notifications/TelegramNotifications';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { catTabLabel } from './catTabLabel';
import { NotifyTimeField } from './NotifyTimeField';
import SecretInput from './SecretInput';

interface TelegramTabProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

export default function TelegramTab({ allSetting, updateSetting }: TelegramTabProps) {
  const { t } = useTranslation();
  const { isMobile } = useMediaQuery();
  const [testLoading, setTestLoading] = useState(false);
  const [testResult, setTestResult] = useState<{ success: boolean; msg: string } | null>(null);

  async function handleTestTgBot() {
    setTestLoading(true);
    setTestResult(null);
    try {
      const res = (await HttpUtil.post('/panel/api/setting/testTgBot')) as {
        success?: boolean;
        msg?: string;
      };
      setTestResult({ success: !!res.success, msg: res.msg || '' });
    } catch (e: unknown) {
      setTestResult({
        success: false,
        msg: e instanceof Error ? e.message : t('pages.settings.requestFailed'),
      });
    } finally {
      setTestLoading(false);
    }
  }

  const langOptions = useMemo(
    () =>
      LanguageManager.supportedLanguages.map(
        (l: { value: string; name: string; icon: string }) => ({
          value: l.value,
          label: (
            <>
              <span role="img" aria-label={l.name}>
                {l.icon}
              </span>
              &nbsp;&nbsp;<span>{l.name}</span>
            </>
          ),
        }),
      ),
    [],
  );

  return (
    <Tabs
      defaultActiveKey="1"
      items={[
        {
          key: '1',
          label: catTabLabel(<SettingOutlined />, t('pages.settings.panelSettings'), isMobile),
          children: (
            <>
              <Alert
                type="info"
                showIcon
                title="账户绑定与续费提醒"
                description="在 @BotFather 创建新机器人，将 Token 填入下方并启用、保存。用户私聊机器人发送自己的激活码即可绑定；默认北京时间 20:00 日报，可用 /daily 修改。账号和服务器在到期前 7、3、1 天提醒；服务器提醒发给已绑定的 pigger 账号，无需填写管理员聊天 ID。"
                style={{ marginBottom: 16 }}
              />
              <SettingListItem
                paddings="small"
                title={t('pages.settings.telegramBotEnable')}
                description={t('pages.settings.telegramBotEnableDesc')}
              >
                <Switch
                  checked={allSetting.tgBotEnable}
                  onChange={(v) => updateSetting({ tgBotEnable: v })}
                />
              </SettingListItem>

              <SettingListItem
                paddings="small"
                title={t('pages.settings.telegramToken')}
                description={
                  allSetting.hasTgBotToken && !allSetting.clearTgBotToken
                    ? t('pages.settings.telegramTokenConfigured')
                    : t('pages.settings.telegramTokenDesc')
                }
              >
                <SecretInput
                  value={allSetting.tgBotToken}
                  configured={allSetting.hasTgBotToken}
                  clearArmed={allSetting.clearTgBotToken}
                  placeholder={t('pages.settings.telegramTokenPlaceholder')}
                  onChange={(v) => updateSetting({ tgBotToken: v })}
                  onClearArmedChange={(armed) => updateSetting({ clearTgBotToken: armed })}
                />
              </SettingListItem>

              <SettingListItem
                paddings="small"
                title={t('pages.settings.telegramChatId')}
                description={t('pages.settings.telegramChatIdDesc')}
              >
                <Input
                  value={allSetting.tgBotChatId}
                  onChange={(e) => updateSetting({ tgBotChatId: e.target.value })}
                />
              </SettingListItem>

              <SettingListItem paddings="small" title={t('pages.settings.telegramBotLanguage')}>
                <Select
                  value={allSetting.tgLang}
                  onChange={(v) => updateSetting({ tgLang: v })}
                  style={{ width: '100%' }}
                  options={langOptions}
                />
              </SettingListItem>

              <SettingListItem
                paddings="small"
                title={t('pages.settings.telegramAPIServer')}
                description={t('pages.settings.telegramAPIServerDesc')}
              >
                <Input
                  value={allSetting.tgBotAPIServer}
                  placeholder="https://api.example.com"
                  onChange={(e) => updateSetting({ tgBotAPIServer: e.target.value })}
                />
              </SettingListItem>

              <Space orientation="vertical" size={8} style={{ width: '100%', marginTop: 16 }}>
                <Button
                  type="primary"
                  icon={<SendOutlined />}
                  loading={testLoading}
                  onClick={handleTestTgBot}
                >
                  {t('pages.settings.testTgBot')}
                </Button>
                {testResult && (
                  <Alert
                    type={testResult.success ? 'success' : 'error'}
                    title={testResult.msg}
                    showIcon
                    closable={{ onClose: () => setTestResult(null) }}
                  />
                )}
              </Space>
            </>
          ),
        },
        {
          key: '2',
          label: catTabLabel(<BellOutlined />, t('pages.settings.notifications'), isMobile),
          children: (
            <>
              <SettingListItem
                paddings="small"
                title={t('pages.settings.telegramNotifyTime')}
                description={t('pages.settings.telegramNotifyTimeDesc')}
              >
                <NotifyTimeField
                  value={allSetting.tgRunTime}
                  onChange={(v) => updateSetting({ tgRunTime: v })}
                />
              </SettingListItem>
              <SettingListItem
                paddings="small"
                title={t('pages.settings.tgNotifyBackup')}
                description={t('pages.settings.tgNotifyBackupDesc')}
              >
                <Switch
                  checked={allSetting.tgBotBackup}
                  onChange={(v) => updateSetting({ tgBotBackup: v })}
                />
              </SettingListItem>

              <SettingListItem
                paddings="small"
                title={t('pages.settings.tgEventBusNotify')}
                description={t('pages.settings.tgEventBusNotifyDesc')}
              >
                <TelegramNotifications allSetting={allSetting} updateSetting={updateSetting} />
              </SettingListItem>
            </>
          ),
        },
      ]}
    />
  );
}
