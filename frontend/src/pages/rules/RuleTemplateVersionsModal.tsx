import { useTranslation } from 'react-i18next';
import { Button, Empty, List, Modal, Popconfirm, Spin, Tag } from 'antd';
import { RollbackOutlined } from '@ant-design/icons';

import { IntlUtil, SizeFormatter } from '@/utils';
import { useRuleTemplateMutations, useRuleTemplateVersions } from '@/api/queries/useRuleTemplates';

interface RuleTemplateVersionsModalProps {
  template: { id: number; name: string } | null;
  onClose: () => void;
}

/** A template's kept saves, newest first; restoring one is itself a new save. */
export default function RuleTemplateVersionsModal({
  template,
  onClose,
}: RuleTemplateVersionsModalProps) {
  const { t } = useTranslation();
  const { restore } = useRuleTemplateMutations();
  const { data: versions = [], isFetching: loading } = useRuleTemplateVersions(
    template?.id ?? null,
  );

  async function restoreVersion(versionId: number) {
    const msg = await restore(versionId);
    if (msg?.success) onClose();
  }

  return (
    <Modal
      open={template !== null}
      title={t('pages.rules.versionsTitle', { name: template?.name ?? '' })}
      footer={null}
      onCancel={onClose}
    >
      <Spin spinning={loading}>
        {versions.length === 0 && !loading ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={t('pages.rules.versionsEmpty')}
          />
        ) : (
          <List
            dataSource={versions}
            renderItem={(version, index) => (
              <List.Item
                actions={
                  index === 0
                    ? [<Tag key="current">{t('pages.rules.versionCurrent')}</Tag>]
                    : [
                        <Popconfirm
                          key="restore"
                          title={t('pages.rules.versionRestoreConfirm')}
                          okText={t('pages.rules.versionRestore')}
                          cancelText={t('cancel')}
                          onConfirm={() => restoreVersion(version.id)}
                        >
                          <Button size="small" icon={<RollbackOutlined />}>
                            {t('pages.rules.versionRestore')}
                          </Button>
                        </Popconfirm>,
                      ]
                }
              >
                <List.Item.Meta
                  title={IntlUtil.formatDate(version.savedAt)}
                  description={SizeFormatter.sizeFormat(version.size)}
                />
              </List.Item>
            )}
          />
        )}
      </Spin>
    </Modal>
  );
}
