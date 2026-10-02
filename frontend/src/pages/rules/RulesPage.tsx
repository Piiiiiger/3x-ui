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
  BranchesOutlined,
  DeleteOutlined,
  EditOutlined,
  EyeOutlined,
  ForkOutlined,
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
import RuleTemplateChanges from './RuleTemplateChanges';
import RuleTemplateVariantModal from './RuleTemplateVariantModal';
import './RulesPage.css';

const KIND_COLORS: Record<RuleTemplateSummary['kind'], string> = {
  yaml: 'blue',
  rules: 'default',
  remote: 'purple',
};

/** A base before its variants, each variant right under its base. */
function inBaseOrder(templates: RuleTemplateSummary[]) {
  const ids = new Set(templates.map((tpl) => tpl.id));
  const variantsOf = new Map<number, RuleTemplateSummary[]>();
  for (const tpl of templates) {
    if (tpl.baseId === 0 || !ids.has(tpl.baseId)) continue;
    variantsOf.set(tpl.baseId, [...(variantsOf.get(tpl.baseId) ?? []), tpl]);
  }
  const rows: RuleTemplateSummary[] = [];
  for (const tpl of templates) {
    if (tpl.baseId !== 0 && ids.has(tpl.baseId)) continue;
    rows.push(tpl, ...(variantsOf.get(tpl.id) ?? []));
  }
  return { rows, variantsOf };
}

// Only a full YAML template can be a base, or turn into a variant of one.
const isFullYaml = (tpl: RuleTemplateSummary) => tpl.baseId === 0 && tpl.kind === 'yaml';

/** 规则: the Clash rule templates plans share, after 妙妙屋X's 模板管理. */
export default function RulesPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { templates, fetched, fetchError, loading, refetch } = useRuleTemplatesQuery();
  const { remove, setDefault } = useRuleTemplateMutations();

  const [editing, setEditing] = useState<{ id: number | null; baseId: number } | null>(null);
  const [preview, setPreview] = useState<RuleTemplatePreviewRequest | null>(null);
  const [versionsOf, setVersionsOf] = useState<{ id: number; name: string } | null>(null);
  const [converting, setConverting] = useState<{ id: number; name: string } | null>(null);
  const previewCount = useRef(0);
  const { rows, variantsOf } = useMemo(() => inBaseOrder(templates), [templates]);
  const nameOf = (id: number) => templates.find((tpl) => tpl.id === id)?.name ?? '';

  function openPreview(name: string, content: string, baseId: number) {
    previewCount.current += 1;
    setPreview({ requestId: previewCount.current, name, content, baseId });
  }

  const pageClass = useMemo(() => {
    const classes = ['rules-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  async function previewSaved(tpl: RuleTemplateSummary) {
    const full = await fetchRuleTemplate(tpl.id);
    if (full) openPreview(full.name, full.content, full.baseId);
  }

  const addButton = (
    <Button
      type="primary"
      icon={<PlusOutlined />}
      onClick={() => setEditing({ id: null, baseId: 0 })}
    >
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
                    dataSource={rows}
                    rowClassName={(tpl) => (tpl.baseId ? 'rule-template-variant-row' : '')}
                    columns={[
                      {
                        title: t('pages.rules.name'),
                        key: 'name',
                        render: (_, tpl) => (
                          <span className="rule-template-name">
                            {tpl.baseId !== 0 && (
                              <span className="rule-template-branch" aria-hidden="true">
                                └
                              </span>
                            )}
                            <span className="rule-template-name-text">{tpl.name}</span>
                            {(variantsOf.get(tpl.id)?.length ?? 0) > 0 && (
                              <Tag color="gold">{t('pages.rules.base')}</Tag>
                            )}
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
                        render: (_, tpl) =>
                          tpl.baseId !== 0 ? (
                            <RuleTemplateChanges changes={tpl.changes} />
                          ) : (
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
                              onClick={() => setEditing({ id: tpl.id, baseId: tpl.baseId })}
                            >
                              {t('edit')}
                            </Button>
                            {isFullYaml(tpl) && (
                              <Button
                                type="text"
                                icon={<ForkOutlined />}
                                onClick={() => setEditing({ id: null, baseId: tpl.id })}
                              >
                                {t('pages.rules.addVariant')}
                              </Button>
                            )}
                            {isFullYaml(tpl) &&
                              !variantsOf.has(tpl.id) &&
                              templates.some(
                                (other) => other.id !== tpl.id && isFullYaml(other),
                              ) && (
                                <Button
                                  type="text"
                                  icon={<BranchesOutlined />}
                                  onClick={() => setConverting({ id: tpl.id, name: tpl.name })}
                                >
                                  {t('pages.rules.makeVariant')}
                                </Button>
                              )}
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
        newBaseId={editing?.id === null ? editing.baseId : 0}
        baseNameOf={nameOf}
        onClose={() => setEditing(null)}
        onPreview={openPreview}
      />
      <RuleTemplateVariantModal
        template={converting}
        bases={templates.filter((tpl) => isFullYaml(tpl) && tpl.id !== converting?.id)}
        onClose={() => setConverting(null)}
      />
      <RuleTemplatePreviewModal request={preview} onClose={() => setPreview(null)} />
      <RuleTemplateVersionsModal template={versionsOf} onClose={() => setVersionsOf(null)} />
    </ConfigProvider>
  );
}
