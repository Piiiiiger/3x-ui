import { useEffect, useMemo, useRef, useState } from 'react';
import { parseDocument } from 'yaml';
import {
  Alert,
  Button,
  Card,
  Collapse,
  Empty,
  Modal,
  Divider,
  Input,
  Popconfirm,
  Select,
  Space,
  Spin,
  Tag,
  Tabs,
  message,
} from 'antd';
import {
  DeleteOutlined,
  DownloadOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
  UndoOutlined,
  UploadOutlined,
} from '@ant-design/icons';

import { keys } from '@/api/queryKeys';
import { YamlEditor } from '@/components/form';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  PortalCustomizationSchema,
  PortalCustomizationGroupSchema,
  type PortalCustomization,
  type PortalCustomizationGroup,
  type PortalCustomizationLink,
} from '@/schemas/portal';

interface PortalCustomizeProps {
  base: string;
  onSessionEnded: () => void;
  onDirtyChange?: (dirty: boolean) => void;
}

type EditableGroup = {
  name: string;
  type: string;
  proxies: string[];
  filter?: string;
  extra: Record<string, unknown>;
};

const groupTypes = ['select', 'url-test', 'fallback', 'load-balance', 'relay', 'smart'];

function normalizeGroup(group: PortalCustomizationGroup): EditableGroup {
  const { name, type, proxies, filter, ...extra } = group;
  return {
    name,
    type,
    proxies: Array.isArray(proxies) ? proxies : [],
    filter: typeof filter === 'string' ? filter : '',
    extra,
  };
}

// Preserve provider definitions and advanced options when visual controls edit routing.
export function editRouteYaml(source: string, groups: EditableGroup[], rules: string[]) {
  const doc = parseDocument(source || '{}');
  if (doc.errors.length) throw new Error('规则 YAML 格式有误，请先在高级编辑中修正。');
  doc.set(
    'proxy-groups',
    groups.map(({ extra, filter, ...group }) => ({
      ...extra,
      ...group,
      ...(filter?.trim() ? { filter: filter.trim() } : {}),
    })),
  );
  doc.set('rules', rules);
  return doc.toString();
}

export function readRouteYaml(source: string, fallback = '') {
  const doc = parseDocument(source);
  if (doc.errors.length) throw new Error('规则 YAML 格式有误，请先修正后再切换。');
  const raw = doc.toJS();
  if (raw !== null && (typeof raw !== 'object' || Array.isArray(raw)))
    throw new Error('规则 YAML 顶层必须是配置对象。');
  const defaults = fallback ? parseDocument(fallback).toJS() : {};
  const value = { ...defaults, ...(raw ?? {}) };
  if (
    !value ||
    typeof value !== 'object' ||
    !Array.isArray(value['proxy-groups']) ||
    !Array.isArray(value.rules)
  ) {
    throw new Error('可视化编辑需要 proxy-groups 和 rules 列表。');
  }
  if (!value.rules.every((rule: unknown) => typeof rule === 'string'))
    throw new Error('每条规则必须是文本。');
  return {
    groups: value['proxy-groups'].map((g: unknown) =>
      normalizeGroup(PortalCustomizationGroupSchema.parse(g)),
    ),
    rules: value.rules as string[],
  };
}

async function fetchCustomization(base: string, onSessionEnded: () => void) {
  const response = await fetch(`${base}/customization`, {
    credentials: 'same-origin',
    headers: { Accept: 'application/json' },
  });
  if (response.status === 401) {
    onSessionEnded();
    throw new Error('session');
  }
  if (!response.ok) throw new Error(`customization: HTTP ${response.status}`);
  return PortalCustomizationSchema.parse(await response.json());
}

function editorValue(data: PortalCustomization) {
  return data.rulesYaml || data.effectiveYaml;
}

export default function PortalCustomize({
  base,
  onSessionEnded,
  onDirtyChange,
}: PortalCustomizeProps) {
  const queryClient = useQueryClient();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [busy, setBusy] = useState(false);
  const [modal, modalContextHolder] = Modal.useModal();
  const [historyVersion, setHistoryVersion] = useState<number>();
  const [ruleDomain, setRuleDomain] = useState('');
  const [ruleTarget, setRuleTarget] = useState<string>();
  const [nodesYaml, setNodesYaml] = useState('');
  const [rulesYaml, setRulesYaml] = useState('');
  const [groups, setGroups] = useState<EditableGroup[]>([]);
  const [rules, setRules] = useState<string[]>([]);
  const [rulesDirty, setRulesDirty] = useState(false);
  const [links, setLinks] = useState<PortalCustomizationLink[]>([]);
  const [linkKind, setLinkKind] = useState<PortalCustomizationLink['kind']>('link');
  const [linkValue, setLinkValue] = useState('');
  const [linkRemark, setLinkRemark] = useState('');
  const [activeTab, setActiveTab] = useState('nodes');
  const fileRef = useRef<HTMLInputElement | null>(null);
  const customization = useQuery({
    queryKey: keys.portal.customization(base),
    queryFn: () => fetchCustomization(base, onSessionEnded),
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    staleTime: Infinity,
  });

  const rulesBytes = useMemo(() => new TextEncoder().encode(rulesYaml).length, [rulesYaml]);
  const maxRulesBytes = customization.data?.maxRulesBytes ?? 256 * 1024;
  const rulesLimitLabel = `${maxRulesBytes / (1024 * 1024)} MB`;

  const dirty =
    !!customization.data &&
    (nodesYaml !== customization.data.nodesYaml ||
      JSON.stringify(links) !== JSON.stringify(customization.data.links) ||
      rulesDirty);
  const hydrated = useRef(false);
  useEffect(() => {
    onDirtyChange?.(dirty || !!linkValue.trim());
  }, [dirty, linkValue, onDirtyChange]);
  const draftRef = useRef(false);
  useEffect(() => {
    draftRef.current = dirty;
  }, [dirty]);
  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => {
      if (draftRef.current || linkValue.trim()) {
        event.preventDefault();
        event.returnValue = '';
      }
    };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, [linkValue]);
  const loadSnapshot = (snapshot: PortalCustomization) => {
    setNodesYaml(snapshot.nodesYaml);
    setRulesYaml(editorValue(snapshot));
    setGroups(snapshot.groups.map(normalizeGroup));
    setRules(snapshot.rules);
    setRulesDirty(false);
    setLinks(snapshot.links);
    setLinkValue('');
    setLinkRemark('');
    draftRef.current = false;
  };
  useEffect(() => {
    if (!customization.data || (hydrated.current && draftRef.current)) return;
    hydrated.current = true;
    // The query is an external source; copy its snapshot into the editor only
    // when it changes (after save, reset, or rollback).
    // oxlint-disable-next-line react/set-state-in-effect
    setNodesYaml(customization.data.nodesYaml);
    // oxlint-disable-next-line react/set-state-in-effect
    setRulesYaml(editorValue(customization.data));
    // oxlint-disable-next-line react/set-state-in-effect
    setGroups(customization.data.groups.map(normalizeGroup));
    // oxlint-disable-next-line react/set-state-in-effect
    setRules(customization.data.rules);
    // oxlint-disable-next-line react/set-state-in-effect
    setRulesDirty(false);
    // oxlint-disable-next-line react/set-state-in-effect
    setLinks(customization.data.links);
  }, [customization.data]);

  const updateRoute = (nextGroups: EditableGroup[], nextRules = rules) => {
    const yaml = editRouteYaml(rulesYaml, nextGroups, nextRules);
    setGroups(nextGroups);
    setRules(nextRules);
    setRulesYaml(yaml);
    setRulesDirty(true);
  };

  const save = async () => {
    if (linkValue.trim()) {
      messageApi.warning('输入框中还有未添加的链接，请先点击“添加”。');
      return;
    }
    const savedRules = rulesDirty ? rulesYaml : (customization.data?.rulesYaml ?? '');
    if (new TextEncoder().encode(savedRules).length > maxRulesBytes)
      throw new Error(
        `规则文件超过 ${rulesLimitLabel}，请精简规则或使用 rule-providers 引用规则集。`,
      );
    const parsed = readRouteYaml(rulesYaml, customization.data?.effectiveYaml);
    const names = parsed.groups.map((g: EditableGroup) => g.name.trim());
    if (names.some((name: string) => !name) || new Set(names).size !== names.length)
      throw new Error('代理组名称不能为空或重复。');
    const response = await fetch(`${base}/customization`, {
      method: 'PUT',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({
        nodesYaml,
        links,
        rulesYaml: savedRules,
      }),
    });
    if (response.status === 401) {
      onSessionEnded();
      return;
    }
    if (!response.ok) {
      const body = await response.json().catch(() => null);
      throw new Error(body?.error || `HTTP ${response.status}`);
    }
    const next = PortalCustomizationSchema.parse(await response.json());
    loadSnapshot(next);
    queryClient.setQueryData(keys.portal.customization(base), next);
    messageApi.success('已保存。请在 Clash / Mihomo 客户端更新订阅后使用。');
  };

  const reset = async () => {
    const response = await fetch(`${base}/customization`, {
      method: 'DELETE',
      credentials: 'same-origin',
      headers: { Accept: 'application/json' },
    });
    if (response.status === 401) {
      onSessionEnded();
      return;
    }
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const next = PortalCustomizationSchema.parse(await response.json());
    loadSnapshot(next);
    queryClient.setQueryData(keys.portal.customization(base), next);
    messageApi.success('已恢复套餐默认配置');
  };

  const rollback = async (versionId: number) => {
    const response = await fetch(`${base}/customization/rollback`, {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ versionId }),
    });
    if (response.status === 401) {
      onSessionEnded();
      return;
    }
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const next = PortalCustomizationSchema.parse(await response.json());
    loadSnapshot(next);
    queryClient.setQueryData(keys.portal.customization(base), next);
    messageApi.success('已回退到历史版本');
  };

  const importFile = (file: File) => {
    void file
      .text()
      .then(setNodesYaml)
      .catch(() => messageApi.error('读取 YAML 文件失败'));
  };

  const run = async (action: () => Promise<void>) => {
    if (busy) return;
    setBusy(true);
    try {
      await action();
    } catch (error) {
      reportError(error);
    } finally {
      setBusy(false);
    }
  };
  const changeTab = (tab: string) => {
    if (tab === 'groups') {
      try {
        const parsed = readRouteYaml(rulesYaml, customization.data?.effectiveYaml);
        setGroups(parsed.groups);
        setRules(parsed.rules);
      } catch (error) {
        reportError(error);
        return;
      }
    }
    setActiveTab(tab);
  };
  const addLink = () => {
    const value = linkValue.trim();
    if (!value) return;
    if (links.some((link) => link.value === value)) {
      messageApi.warning('这个链接已经添加了。');
      return;
    }
    if (
      linkKind === 'subscription'
        ? !/^https?:\/\/\S+$/i.test(value)
        : !/^(vless|vmess|trojan|ss|ssr|hysteria2?|hy2|tuic|wireguard|snell):\/\/\S+$/i.test(value)
    ) {
      messageApi.warning(
        linkKind === 'subscription'
          ? '请输入完整的 HTTP / HTTPS 订阅地址。'
          : '请输入有效的节点分享链接；订阅网址请切换为“订阅地址”。',
      );
      return;
    }
    setLinks([...links, { kind: linkKind, value, remark: linkRemark.trim() }]);
    setLinkValue('');
    setLinkRemark('');
    messageApi.info('已加入待保存列表。保存后会解析节点。');
  };
  const reportError = (error: unknown) => {
    messageApi.error(error instanceof Error ? error.message : '操作失败，请稍后重试');
  };

  const nodeHelp = useMemo(
    () =>
      '粘贴 Clash/Mihomo YAML（必须包含 proxies 列表）。支持 vmess、vless、trojan、ss、hysteria、tuic、wireguard 等 Mihomo 节点类型。',
    [],
  );
  const proxyOptions = useMemo(() => {
    const names = [
      ...(customization.data?.proxyNames ?? []),
      ...groups.map((group) => group.name).filter(Boolean),
      'DIRECT',
      'REJECT',
    ];
    return Array.from(new Set(names)).map((value) => ({ value, label: value }));
  }, [customization.data?.proxyNames, groups]);

  if (customization.isPending)
    return (
      <div className="portal-status">
        <Spin size="large" />
      </div>
    );
  if (customization.isError) {
    return <Alert type="error" showIcon title="无法加载自定义订阅" description="请刷新后重试。" />;
  }
  const data = customization.data;
  if (!data) return null;

  const versionOptions = data.versions.map((version) => ({
    value: version.id,
    label: new Date(version.savedAt).toLocaleString(),
  }));

  return (
    <div className="portal-customize">
      {messageContextHolder}
      {modalContextHolder}
      <fieldset className="portal-customize-fieldset" disabled={busy}>
        <div className="portal-section-head">
          <div>
            <div className="portal-section-title">自定义订阅</div>
            <div className="portal-customize-help">
              添加自己的节点，选择使用方式，再保存到个人订阅。
            </div>
          </div>
          <Space wrap className="portal-customize-actions">
            <Tag color={dirty ? 'orange' : 'green'}>{dirty ? '有未保存的修改' : '已保存'}</Tag>
            <Tag color={data.nodeCount > 0 ? 'blue' : 'default'}>
              已保存 {data.nodeCount} 个节点
            </Tag>
            <Tag color={links.length > 0 ? 'cyan' : 'default'}>{links.length} 个导入来源</Tag>
            <Button
              icon={<SaveOutlined />}
              type="primary"
              loading={busy}
              disabled={!dirty || busy}
              onClick={() => void run(save)}
            >
              保存并应用
            </Button>
            <Popconfirm title="清除节点和规则覆盖，恢复套餐默认配置？" onConfirm={() => run(reset)}>
              <Button icon={<UndoOutlined />}>恢复默认</Button>
            </Popconfirm>
          </Space>
        </div>
        <Alert
          className="portal-workflow-hint"
          type="info"
          showIcon
          title="已有套餐节点？可以直接到第 2 步调整代理组，无需重新导入。"
          description="新增节点：添加来源 → 保存解析 → 分配到代理组 → 保存。客户端更新订阅后生效。"
        />
        <Tabs
          activeKey={activeTab}
          onChange={changeTab}
          items={[
            {
              key: 'nodes',
              label: '1. 添加节点',
              icon: <UploadOutlined />,
              children: (
                <Card size="small">
                  <p className="portal-customize-help">
                    粘贴节点分享链接或订阅地址，添加后点击“保存并应用”解析节点。
                  </p>
                  <div className="portal-link-import">
                    <div className="portal-import-form">
                      <Select
                        value={linkKind}
                        options={[
                          { value: 'link', label: '分享链接' },
                          { value: 'subscription', label: '订阅地址' },
                        ]}
                        onChange={setLinkKind}
                        style={{ width: 140 }}
                      />
                      <Input
                        value={linkValue}
                        aria-label="节点链接或订阅地址"
                        onPressEnter={addLink}
                        placeholder={
                          linkKind === 'link'
                            ? 'vless://、vmess://、trojan://…'
                            : 'https://example.com/sub'
                        }
                        onChange={(event) => setLinkValue(event.target.value)}
                      />
                      <Input
                        value={linkRemark}
                        placeholder="备注（可选）"
                        onChange={(event) => setLinkRemark(event.target.value)}
                      />
                      <Button
                        icon={<PlusOutlined />}
                        disabled={!linkValue.trim()}
                        onClick={addLink}
                      >
                        添加
                      </Button>
                    </div>
                    {links.length === 0 && (
                      <Empty
                        image={Empty.PRESENTED_IMAGE_SIMPLE}
                        description="尚未添加来源；只用套餐节点时可以跳过。"
                      />
                    )}
                    {links.map((link, index) => (
                      <div className="portal-link-row" key={`${link.kind}-${link.value}-${index}`}>
                        <Tag>{link.kind === 'link' ? '分享链接' : '订阅地址'}</Tag>
                        <Input
                          value={link.remark}
                          placeholder={
                            link.kind === 'link' ? '节点名称（可选）' : '节点名前缀（可选）'
                          }
                          onChange={(event) => {
                            const next = links.map((item, itemIndex) =>
                              itemIndex === index ? { ...item, remark: event.target.value } : item,
                            );
                            setLinks(next);
                          }}
                        />
                        <span className="portal-link-value" title={link.value}>
                          {link.value}
                        </span>
                        <Button
                          type="text"
                          danger
                          icon={<DeleteOutlined />}
                          onClick={() =>
                            setLinks(links.filter((_, linkIndex) => linkIndex !== index))
                          }
                        />
                      </div>
                    ))}
                  </div>
                  {data.nodes.length > 0 && (
                    <>
                      <Divider />
                      <div className="portal-customize-help">
                        上次保存后解析的节点（当前修改需保存后更新）
                      </div>
                      <div className="portal-node-list">
                        {data.nodes.map((node) => (
                          <div
                            className="portal-node-row"
                            key={node.name + '-' + (node.linkIndex ?? 'yaml')}
                          >
                            <Tag color={node.source === 'yaml' ? 'blue' : 'cyan'}>
                              {node.source === 'yaml'
                                ? 'YAML'
                                : node.source === 'subscription'
                                  ? '订阅'
                                  : '分享链接'}
                            </Tag>
                            <span className="portal-node-name">{node.name}</span>
                            <span className="portal-node-type">{node.type}</span>
                          </div>
                        ))}
                      </div>
                    </>
                  )}
                  <Collapse
                    className="portal-yaml-import"
                    items={[
                      {
                        key: 'yaml',
                        label: '从 YAML 文件或文本导入（高级）',
                        children: (
                          <>
                            <p className="portal-customize-help">{nodeHelp}</p>
                            <Space wrap>
                              <Button
                                icon={<UploadOutlined />}
                                onClick={() => fileRef.current?.click()}
                              >
                                选择 YAML 文件
                              </Button>
                              <input
                                ref={fileRef}
                                type="file"
                                accept=".yaml,.yml,text/yaml"
                                hidden
                                onChange={(event) => {
                                  const file = event.currentTarget.files?.[0];
                                  if (file) importFile(file);
                                  event.currentTarget.value = '';
                                }}
                              />
                              <Button
                                icon={<DownloadOutlined />}
                                onClick={() =>
                                  modal.confirm({
                                    title: '清空 YAML 导入的节点？链接来源会保留，保存后生效。',
                                    onOk: () => setNodesYaml('proxies: []\n'),
                                  })
                                }
                              >
                                清空节点
                              </Button>
                            </Space>

                            <YamlEditor
                              value={nodesYaml}
                              onChange={setNodesYaml}
                              minHeight="180px"
                              maxHeight="420px"
                            />
                          </>
                        ),
                      },
                    ]}
                  />
                  <Button className="portal-next-step" onClick={() => changeTab('groups')}>
                    下一步：分配代理组 →
                  </Button>
                </Card>
              ),
            },
            {
              key: 'groups',
              label: '2. 代理组与规则',
              children: (
                <Card size="small">
                  <div className="portal-customize-help">
                    选择每个分组使用哪些节点。新导入的节点需先保存解析，才会出现在选项中。DIRECT
                    表示直连，REJECT 表示拦截。
                  </div>
                  <div className="portal-quick-rule">
                    <strong>让指定网站走某个代理组</strong>
                    <Space wrap>
                      <Input
                        aria-label="网站域名"
                        placeholder="例如 example.com"
                        value={ruleDomain}
                        onChange={(event) => setRuleDomain(event.target.value)}
                      />
                      <Select
                        aria-label="规则目标"
                        placeholder="选择代理组或直连"
                        style={{ minWidth: 180 }}
                        value={ruleTarget}
                        onChange={setRuleTarget}
                        options={proxyOptions}
                      />
                      <Button
                        disabled={!ruleDomain.trim() || !ruleTarget}
                        onClick={() => {
                          const domain = ruleDomain.trim().toLowerCase();
                          if (!/^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,63}$/.test(domain)) {
                            messageApi.warning(
                              '请输入域名，例如 example.com，不需要 https:// 或路径。',
                            );
                            return;
                          }
                          const rule = `DOMAIN-SUFFIX,${domain},${ruleTarget}`;
                          updateRoute(groups, [rule, ...rules.filter((item) => item !== rule)]);
                          setRuleDomain('');
                          messageApi.info('规则已置顶，保存后生效。');
                        }}
                      >
                        添加网站规则
                      </Button>
                    </Space>
                  </div>
                  {groups.map((group, index) => (
                    <div className="portal-group-editor" key={index}>
                      <Space.Compact block>
                        <Input
                          key={group.name}
                          defaultValue={group.name}
                          aria-label="代理组名称"
                          placeholder="代理组名称"
                          onBlur={(event) => {
                            const name = event.target.value.trim();
                            if (!name || groups.some((g, i) => i !== index && g.name === name)) {
                              event.target.value = group.name;
                              messageApi.warning('请使用不重复的代理组名称。');
                              return;
                            }
                            if (name === group.name) return;
                            const next = groups.map((item, i) => ({
                              ...item,
                              name: i === index ? name : item.name,
                              proxies: item.proxies.map((p) => (p === group.name ? name : p)),
                            }));
                            const nextRules = rules.map((rule) =>
                              rule
                                .split(',')
                                .map((part) => (part === group.name ? name : part))
                                .join(','),
                            );
                            updateRoute(next, nextRules);
                          }}
                        />
                        <Select
                          value={group.type}
                          options={groupTypes.map((type) => ({
                            value: type,
                            label:
                              (
                                {
                                  select: '手动选择',
                                  'url-test': '自动测速',
                                  fallback: '故障切换',
                                  'load-balance': '负载均衡',
                                  relay: '链式代理',
                                  smart: '智能选择',
                                } as Record<string, string>
                              )[type] ?? type,
                          }))}
                          onChange={(type) => {
                            const next = groups.map((item, itemIndex) =>
                              itemIndex === index
                                ? {
                                    ...item,
                                    type,
                                    extra: [
                                      'url-test',
                                      'fallback',
                                      'load-balance',
                                      'smart',
                                    ].includes(type)
                                      ? {
                                          url: 'https://www.gstatic.com/generate_204',
                                          interval: 300,
                                          ...item.extra,
                                        }
                                      : item.extra,
                                  }
                                : item,
                            );
                            updateRoute(next);
                          }}
                          style={{ width: 150 }}
                        />
                        <Button
                          danger
                          icon={<DeleteOutlined />}
                          aria-label={`删除代理组 ${group.name}`}
                          onClick={() => {
                            if (
                              groups.some((g) => g.proxies.includes(group.name)) ||
                              rules.some((rule) => rule.split(',').includes(group.name))
                            ) {
                              messageApi.warning('其他分组或规则正在使用这个组，请先调整引用。');
                              return;
                            }
                            modal.confirm({
                              title: `删除代理组“${group.name}”？`,
                              onOk: () => updateRoute(groups.filter((_, i) => i !== index)),
                            });
                          }}
                        />
                      </Space.Compact>
                      <Select
                        mode="multiple"
                        value={group.proxies}
                        options={proxyOptions.filter((option) => option.value !== group.name)}
                        placeholder="选择节点或其他代理组"
                        tokenSeparators={[',']}
                        style={{ width: '100%' }}
                        onChange={(proxies) => {
                          const next = groups.map((item, itemIndex) =>
                            itemIndex === index ? { ...item, proxies } : item,
                          );
                          updateRoute(next);
                        }}
                      />
                      <Input
                        value={group.filter}
                        placeholder="可选 filter 正则，例如 ^HK"
                        onChange={(event) => {
                          const next = groups.map((item, itemIndex) =>
                            itemIndex === index ? { ...item, filter: event.target.value } : item,
                          );
                          updateRoute(next);
                        }}
                      />
                    </div>
                  ))}
                  <Button
                    icon={<PlusOutlined />}
                    onClick={() => {
                      let number = groups.length + 1;
                      while (groups.some((group) => group.name === `自定义-${number}`)) number++;
                      updateRoute([
                        ...groups,
                        {
                          name: `自定义-${number}`,
                          type: 'select',
                          proxies: ['DIRECT'],
                          extra: {},
                        },
                      ]);
                    }}
                  >
                    添加代理组
                  </Button>
                </Card>
              ),
            },
            {
              key: 'rules',
              label: '高级 · YAML',
              children: (
                <Card size="small">
                  <p className="portal-customize-help">
                    可直接编辑 proxy-groups、rules 和
                    rule-providers。保存时服务器会拒绝代理节点、脚本等其他顶层配置。
                  </p>
                  <p className="portal-customize-help" role="status">
                    规则文件：{(rulesBytes / 1024).toFixed(1)} KB · 最大 {rulesLimitLabel}。
                    较大的规则集可使用 rule-providers 引用。
                  </p>
                  {rulesDirty && rulesBytes > maxRulesBytes && (
                    <Alert
                      type="error"
                      showIcon
                      title={`规则文件超过 ${rulesLimitLabel}，请精简后保存。`}
                      style={{ marginBottom: 12 }}
                    />
                  )}
                  <YamlEditor
                    value={rulesYaml}
                    onChange={(value) => {
                      setRulesYaml(value);
                      setRulesDirty(true);
                    }}
                    minHeight="360px"
                    maxHeight="680px"
                  />
                </Card>
              ),
            },
          ]}
        />
        {rules.length > 0 && activeTab === 'groups' && (
          <Card size="small" className="portal-rules-preview">
            <div className="portal-section-title">规则预览</div>
            <Input.TextArea
              value={rules.join('\n')}
              onChange={(event) => updateRoute(groups, event.target.value.split(/\r?\n/))}
              autoSize={{ minRows: 2, maxRows: 8 }}
            />
          </Card>
        )}
        {data.versions.length > 0 && (
          <div className="portal-customize-history">
            <Space wrap>
              <span className="portal-customize-help">历史版本</span>
              <Select
                placeholder="选择版本回退"
                options={versionOptions}
                value={historyVersion}
                onChange={setHistoryVersion}
                style={{ minWidth: 210 }}
              />
              <Popconfirm
                title="恢复此版本并替换当前配置？未保存修改将丢失。"
                onConfirm={() =>
                  historyVersion !== undefined ? run(() => rollback(historyVersion)) : undefined
                }
              >
                <Button disabled={historyVersion === undefined || busy}>恢复所选版本</Button>
              </Popconfirm>
              <Popconfirm
                title="放弃未保存修改并重新加载？"
                onConfirm={() =>
                  run(async () => {
                    const next = await fetchCustomization(base, onSessionEnded);
                    loadSnapshot(next);
                    queryClient.setQueryData(keys.portal.customization(base), next);
                  })
                }
              >
                <Button icon={<ReloadOutlined />}>重新加载</Button>
              </Popconfirm>
            </Space>
          </div>
        )}
      </fieldset>
      <div className="portal-save-bar">
        <span>{dirty ? '修改尚未保存' : '配置已保存'} · 保存后请在客户端更新订阅</span>
        <Button
          type="primary"
          loading={busy}
          disabled={!dirty || busy}
          onClick={() => void run(save)}
        >
          保存并应用
        </Button>
      </div>
    </div>
  );
}
