import { useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  ConfigProvider,
  Empty,
  Layout,
  Popconfirm,
  Result,
  Space,
  Spin,
  Table,
  Tag,
  Tooltip,
} from 'antd';
import {
  DeleteOutlined,
  EditOutlined,
  EyeOutlined,
  HistoryOutlined,
  PlusOutlined,
  StarFilled,
  StarOutlined,
} from '@ant-design/icons';

import AppNav from '@/layouts/AppNav';
import { PageHeader } from '@/components/ui';
import { useTheme } from '@/hooks/useTheme';
import { IntlUtil, SizeFormatter } from '@/utils';
import type { RuleTemplateSummary } from '@/generated/zod';
import {
  fetchRuleTemplate,
  useRuleTemplateMutations,
  useRuleTemplatesQuery,
} from '@/api/queries/useRuleTemplates';
import RuleTemplateEditorModal from './RuleTemplateEditorModal';
import RuleTemplatePreviewModal, {
  type RuleTemplatePreviewRequest,
} from './RuleTemplatePreviewModal';
import RuleTemplateVersionsModal from './RuleTemplateVersionsModal';
import './RulesPage.css';

const KIND_COLORS: Record<RuleTemplateSummary['kind'], string> = {
  yaml: 'blue',
  rules: 'default',
  remote: 'purple',
};

/** 规则: the Clash rule templates plans share, after 妙妙屋X's 模板管理. */
export default function RulesPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { templates, fetched, fetchError, loading, refetch } = useRuleTemplatesQuery();
  const { remove, setDefault } = useRuleTemplateMutations();

  const [editing, setEditing] = useState<{ id: number | null } | null>(null);
  const [preview, setPreview] = useState<RuleTemplatePreviewRequest | null>(null);
  const [versionsOf, setVersionsOf] = useState<{ id: number; name: string } | null>(null);
  const previewCount = useRef(0);

  function openPreview(name: string, content: string) {
    previewCount.current += 1;
    setPreview({ requestId: previewCount.current, name, content });
  }

  const pageClass = useMemo(() => {
    const classes = ['rules-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  async function previewSaved(tpl: RuleTemplateSummary) {
    const full = await fetchRuleTemplate(tpl.id);
    if (full) openPreview(full.name, full.content);
  }

  const addButton = (
    <Button type="primary" icon={<PlusOutlined />} onClick={() => setEditing({ id: null })}>
      {t('pages.rules.add')}
    </Button>
  );

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={pageClass}>
        <AppNav />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <PageHeader
              title={t('menu.rules')}
              description={t('pages.rules.intro')}
              extra={templates.length > 0 ? addButton : undefined}
            />
            <Spin spinning={!fetched} delay={200} description={t('loading')} size="large">
              {!fetched ? (
                <div className="loading-spacer" />
              ) : fetchError ? (
                <Result
                  status="error"
                  title={t('somethingWentWrong')}
                  subTitle={fetchError}
                  extra={
                    <Button type="primary" loading={loading} onClick={() => refetch()}>
                      {t('refresh')}
                    </Button>
                  }
                />
              ) : templates.length === 0 ? (
                <Card>
                  <Empty description={t('pages.rules.empty')}>{addButton}</Empty>
                </Card>
              ) : (
                <Card styles={{ body: { padding: 0 } }}>
                  <Table<RuleTemplateSummary>
                    rowKey="id"
                    pagination={false}
                    scroll={{ x: 'max-content' }}
                    dataSource={templates}
                    columns={[
                      {
                        title: t('pages.rules.name'),
                        key: 'name',
                        render: (_, tpl) => (
                          <span className="rule-template-name">
                            {tpl.name}
                            {tpl.isDefault && (
                              <Tooltip title={t('pages.rules.isDefault')}>
                                <StarFilled
                                  className="rule-template-star"
                                  aria-label={t('pages.rules.isDefault')}
                                />
                              </Tooltip>
                            )}
                          </span>
                        ),
                      },
                      {
                        title: t('pages.rules.kind'),
                        key: 'kind',
                        render: (_, tpl) => (
                          <Tag color={KIND_COLORS[tpl.kind]}>
                            {t(`pages.rules.kinds.${tpl.kind}`)}
                          </Tag>
                        ),
                      },
                      {
                        title: t('pages.rules.size'),
                        key: 'size',
                        align: 'right',
                        render: (_, tpl) => SizeFormatter.sizeFormat(tpl.size),
                      },
                      {
                        title: t('pages.rules.usage'),
                        key: 'usage',
                        render: (_, tpl) => t('pages.rules.usedBy', { count: tpl.planCount }),
                      },
                      {
                        title: t('pages.rules.updated'),
                        key: 'updated',
                        render: (_, tpl) => IntlUtil.formatDate(tpl.updatedAt),
                      },
                      {
                        title: t('pages.rules.actions'),
                        key: 'actions',
                        render: (_, tpl) => (
                          <Space size={0} wrap>
                            <Button
                              type="text"
                              icon={<StarOutlined />}
                              disabled={tpl.isDefault}
                              onClick={() => setDefault(tpl.id)}
                            >
                              {t('pages.rules.setDefault')}
                            </Button>
                            <Button
                              type="text"
                              icon={<EditOutlined />}
                              onClick={() => setEditing({ id: tpl.id })}
                            >
                              {t('edit')}
                            </Button>
                            <Button
                              type="text"
                              icon={<EyeOutlined />}
                              onClick={() => previewSaved(tpl)}
                            >
                              {t('pages.rules.preview')}
                            </Button>
                            <Button
                              type="text"
                              icon={<HistoryOutlined />}
                              onClick={() => setVersionsOf({ id: tpl.id, name: tpl.name })}
                            >
                              {t('pages.rules.versions')}
                            </Button>
                            <Popconfirm
                              title={t('pages.rules.deleteConfirm', { name: tpl.name })}
                              okText={t('delete')}
                              okType="danger"
                              cancelText={t('cancel')}
                              onConfirm={() => remove(tpl.id)}
                            >
                              <Button type="text" danger icon={<DeleteOutlined />}>
                                {t('delete')}
                              </Button>
                            </Popconfirm>
                          </Space>
                        ),
                      },
                    ]}
                  />
                </Card>
              )}
            </Spin>
          </Layout.Content>
        </Layout>
      </Layout>
      <RuleTemplateEditorModal
        open={editing !== null}
        templateId={editing?.id ?? null}
        onClose={() => setEditing(null)}
        onPreview={openPreview}
      />
      <RuleTemplatePreviewModal request={preview} onClose={() => setPreview(null)} />
      <RuleTemplateVersionsModal template={versionsOf} onClose={() => setVersionsOf(null)} />
    </ConfigProvider>
  );
}
