import { lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useLocation, useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import {
  Badge,
  Button,
  Card,
  Checkbox,
  ConfigProvider,
  Dropdown,
  Input,
  Layout,
  Modal,
  Pagination,
  Result,
  Segmented,
  Select,
  Spin,
  Table,
  Tag,
  Tooltip,
  message,
} from 'antd';
import type { MenuProps } from 'antd';
import type { ColumnsType, TableProps } from 'antd/es/table';
import {
  AppstoreOutlined,
  CheckCircleOutlined,
  CheckSquareOutlined,
  ClockCircleOutlined,
  CopyOutlined,
  DeleteOutlined,
  DisconnectOutlined,
  DownloadOutlined,
  FieldTimeOutlined,
  FilterOutlined,
  LinkOutlined,
  MinusCircleOutlined,
  MoreOutlined,
  PlusOutlined,
  ProfileOutlined,
  RestOutlined,
  RetweetOutlined,
  ScheduleOutlined,
  SearchOutlined,
  SortAscendingOutlined,
  StopOutlined,
  TeamOutlined,
  UploadOutlined,
  UsergroupAddOutlined,
  UsergroupDeleteOutlined,
} from '@ant-design/icons';

import { useTheme } from '@/hooks/useTheme';
import { formatInboundLabel } from '@/lib/inbounds/label';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useClients } from '@/hooks/useClients';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import { usePlansQuery } from '@/api/queries/usePlansQuery';
import { usePlanMutations } from '@/api/queries/usePlanMutations';
import AssignPlanModal from '@/pages/plans/AssignPlanModal';
import ClientRenewModal from './ClientRenewModal';
import { useDatepicker } from '@/hooks/useDatepicker';
import type {
  ClientRecord,
  InboundOption,
  ExternalLink,
  ExternalLinkInput,
} from '@/hooks/useClients';
import ClientCardComment from '@/components/clients/ClientCardComment';
import AppNav from '@/layouts/AppNav';
import { PageHeader } from '@/components/ui';
import { ClipboardManager, IntlUtil } from '@/utils';
import { setMessageInstance } from '@/utils/messageBus';
import { LazyMount } from '@/components/utility';
const ClientFormModal = lazy(() => import('./ClientFormModal'));
const ClientInfoModal = lazy(() => import('./ClientInfoModal'));
const ClientQrModal = lazy(() => import('./ClientQrModal'));
const ClientBulkAddModal = lazy(() => import('./ClientBulkAddModal'));
const ClientBulkAdjustModal = lazy(() => import('./ClientBulkAdjustModal'));
const FilterDrawer = lazy(() => import('./FilterDrawer'));
const SubLinksModal = lazy(() => import('./SubLinksModal'));
const ClientPortalModal = lazy(() => import('./ClientPortalModal'));
const BulkAttachInboundsModal = lazy(() => import('./BulkAttachInboundsModal'));
const BulkDetachInboundsModal = lazy(() => import('./BulkDetachInboundsModal'));
const TextModal = lazy(() => import('@/components/feedback/TextModal'));
const PromptModal = lazy(() => import('@/components/feedback/PromptModal'));
import ClientChips from './ClientChips';
import ClientCommentCell from './ClientCommentCell';
import ClientRowMenu from './ClientRowMenu';
import PlanUsageCell from './PlanUsageCell';
import { ClientStateTag, DaysLeftText, OnlineDot } from './ClientCells';
import { clientState, daysToExpiry, usedBytes } from './clientState';
import { emptyFilters, activeFilterCount } from './filters';
import type { ClientFilters } from './filters';
import './ClientsPage.css';

const FILTER_STATE_KEY = 'clientsFilterState';
const DISABLED_PAGE_SIZE = 200;
const DEFAULT_TABLE_PAGE_SIZE = 25;

interface PersistedFilterState {
  searchKey: string;
  filters: ClientFilters;
  sort: string;
  // The page size resolved on the previous visit. Without it the first list
  // request has to wait for /setting/defaultSettings just to learn how many rows
  // to ask for, which serialises two round trips on every load.
  pageSize: number | null;
}

type ClientsView = 'full' | 'renewal';
const VIEW_KEY = 'clientsView';

function readView(): ClientsView {
  try {
    return localStorage.getItem(VIEW_KEY) === 'renewal' ? 'renewal' : 'full';
  } catch {
    return 'full';
  }
}

function readFilterState(): PersistedFilterState {
  try {
    const raw = JSON.parse(localStorage.getItem(FILTER_STATE_KEY) || '{}');
    const fromRaw = (raw.filters ?? {}) as Partial<ClientFilters>;
    return {
      searchKey: typeof raw.searchKey === 'string' ? raw.searchKey : '',
      filters: {
        ...emptyFilters(),
        ...fromRaw,
        buckets: Array.isArray(fromRaw.buckets) ? fromRaw.buckets : [],
        protocols: Array.isArray(fromRaw.protocols) ? fromRaw.protocols : [],
        inboundIds: Array.isArray(fromRaw.inboundIds) ? fromRaw.inboundIds : [],
        nodeIds: Array.isArray(fromRaw.nodeIds) ? fromRaw.nodeIds : [],
        plans: Array.isArray(fromRaw.plans) ? fromRaw.plans : [],
      },
      sort: typeof raw.sort === 'string' ? raw.sort : '',
      pageSize: typeof raw.pageSize === 'number' && raw.pageSize > 0 ? raw.pageSize : null,
    };
  } catch {
    return { searchKey: '', filters: emptyFilters(), sort: '', pageSize: null };
  }
}

function gbToBytes(gb: number | undefined): number {
  if (!gb || gb <= 0) return 0;
  return Math.round(gb * 1024 * 1024 * 1024);
}

const SORT_OPTIONS: {
  value: string;
  column: string;
  order: 'ascend' | 'descend';
  labelKey: string;
}[] = [
  {
    value: 'createdAt:ascend',
    column: 'createdAt',
    order: 'ascend',
    labelKey: 'pages.clients.sortOldest',
  },
  {
    value: 'createdAt:descend',
    column: 'createdAt',
    order: 'descend',
    labelKey: 'pages.clients.sortNewest',
  },
  {
    value: 'updatedAt:descend',
    column: 'updatedAt',
    order: 'descend',
    labelKey: 'pages.clients.sortRecentlyUpdated',
  },
  {
    value: 'lastOnline:descend',
    column: 'lastOnline',
    order: 'descend',
    labelKey: 'pages.clients.sortRecentlyOnline',
  },
  {
    value: 'email:ascend',
    column: 'email',
    order: 'ascend',
    labelKey: 'pages.clients.sortEmailAZ',
  },
  {
    value: 'email:descend',
    column: 'email',
    order: 'descend',
    labelKey: 'pages.clients.sortEmailZA',
  },
  {
    value: 'traffic:descend',
    column: 'traffic',
    order: 'descend',
    labelKey: 'pages.clients.sortMostTraffic',
  },
  {
    value: 'remaining:descend',
    column: 'remaining',
    order: 'descend',
    labelKey: 'pages.clients.sortHighestRemaining',
  },
  {
    value: 'expiryTime:ascend',
    column: 'expiryTime',
    order: 'ascend',
    labelKey: 'pages.clients.sortExpiringSoonest',
  },
];

const DEFAULT_SORT = SORT_OPTIONS[0];

function sortValueFor(column: string | null, order: 'ascend' | 'descend' | null): string {
  if (!column || !order) return DEFAULT_SORT.value;
  return `${column}:${order}`;
}

export default function ClientsPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { datepicker } = useDatepicker();
  const { isMobile } = useMediaQuery();
  const [modal, modalContextHolder] = Modal.useModal();
  const [messageApi, messageContextHolder] = message.useMessage();
  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const {
    clients,
    total,
    filtered,
    summary,
    setQuery,
    inbounds,
    onlines,
    transitioning,
    fetched,
    fetchError,
    subSettings,
    tgBotEnable,
    pageSize,
    settingsReady,
    create,
    update,
    setComment,
    remove,
    bulkDelete,
    bulkAdjust,
    bulkEnable,
    bulkDisable,
    attach,
    setExternalLinks,
    bulkAttach,
    detach,
    bulkDetach,
    resetTraffic,
    resetAllTraffics,
    delDepleted,
    delOrphans,
    exportClients,
    importClients,
    setEnable,
    applyTrafficEvent,
    applyClientStatsEvent,
    refresh,
    hydrate,
  } = useClients();

  useWebSocket({
    traffic: applyTrafficEvent,
    client_stats: applyClientStatsEvent,
  });

  // Node list for the Nodes filter; the section only renders when the panel
  // actually manages nodes (#4997).
  const { nodes } = useNodesQuery();

  const [view, setView] = useState<ClientsView>(readView);
  // The checkbox column only shows while 批量操作 is on, as on 妙妙屋X.
  const [selecting, setSelecting] = useState(false);
  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState<'add' | 'edit'>('add');
  const [editingClient, setEditingClient] = useState<ClientRecord | null>(null);
  const [editingAttachedIds, setEditingAttachedIds] = useState<number[]>([]);
  const [editingExternalLinks, setEditingExternalLinks] = useState<ExternalLink[]>([]);
  const [editingTunnelAllowedIPs, setEditingTunnelAllowedIPs] = useState<Record<number, string>>(
    {},
  );
  const [infoOpen, setInfoOpen] = useState(false);
  const [infoClient, setInfoClient] = useState<ClientRecord | null>(null);
  const [qrOpen, setQrOpen] = useState(false);
  const [portalEmail, setPortalEmail] = useState<string | null>(null);
  const onPortal = useCallback((email: string) => setPortalEmail(email), []);
  const [qrClient, setQrClient] = useState<ClientRecord | null>(null);
  const [viewingTunnelAllowedIPs, setViewingTunnelAllowedIPs] = useState<Record<number, string>>(
    {},
  );
  const [bulkAddOpen, setBulkAddOpen] = useState(false);
  const [bulkAdjustOpen, setBulkAdjustOpen] = useState(false);
  const [subLinksOpen, setSubLinksOpen] = useState(false);
  const [bulkAttachOpen, setBulkAttachOpen] = useState(false);
  const [bulkDetachOpen, setBulkDetachOpen] = useState(false);
  const [selectedRowKeys, setSelectedRowKeys] = useState<string[]>([]);

  const [textOpen, setTextOpen] = useState(false);
  const [textTitle, setTextTitle] = useState('');
  const [textContent, setTextContent] = useState('');
  const [textFileName, setTextFileName] = useState('');
  const [promptOpen, setPromptOpen] = useState(false);
  const [promptTitle, setPromptTitle] = useState('');
  const [promptOkText, setPromptOkText] = useState('');
  const [promptInitial, setPromptInitial] = useState('');
  const [promptLoading, setPromptLoading] = useState(false);
  const [promptHandler, setPromptHandler] = useState<
    ((value: string) => Promise<boolean | void> | boolean | void) | null
  >(null);

  const initial = readFilterState();
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const searchParam = searchParams.get('search');
  const [searchKey, setSearchKey] = useState(
    searchParam !== null ? searchParam : initial.searchKey,
  );
  // ?plan=<id> (the plans page links here) opens the list on that plan's people.
  const planParam = searchParams.get('plan');
  const [filters, setFilters] = useState<ClientFilters>(() =>
    planParam !== null && /^\d+$/.test(planParam)
      ? { ...initial.filters, plans: [Number(planParam)] }
      : initial.filters,
  );
  const [filterDrawerOpen, setFilterDrawerOpen] = useState(false);
  const { plans } = usePlansQuery();
  const { unassign: unassignPlan } = usePlanMutations();
  const [renewTarget, setRenewTarget] = useState<{ emails: string[]; bulk: boolean } | null>(null);
  const planNames = useMemo(() => new Map(plans.map((p) => [p.id, p.name])), [plans]);
  const [planTarget, setPlanTarget] = useState<{ emails: string[]; planId?: number } | null>(null);

  const initialSort = SORT_OPTIONS.find((o) => o.value === initial.sort) ?? DEFAULT_SORT;
  const [sortColumn, setSortColumn] = useState<string | null>(initialSort.column);
  const [sortOrder, setSortOrder] = useState<'ascend' | 'descend' | null>(initialSort.order);
  const [currentPage, setCurrentPage] = useState(1);
  // Derived, not mirrored into state by an effect: an effect lags one render
  // behind the settings arriving, and that lag is what made the page fetch the
  // list once with the placeholder size and again with the real one.
  const [pageSizeChoice, setPageSizeChoice] = useState<number | null>(null);
  const settingsPageSize = settingsReady ? (pageSize > 0 ? pageSize : DISABLED_PAGE_SIZE) : null;
  // Last visit's resolved size stands in until the settings land, so the list
  // request goes out with the page mount instead of queueing behind them. If the
  // admin has since changed the setting the authoritative value replaces it and
  // costs one refetch — only on the load that follows the change. Null means
  // nothing is known yet, which is the one case worth waiting for.
  const resolvedPageSize = pageSizeChoice ?? settingsPageSize ?? initial.pageSize;
  const tablePageSize = resolvedPageSize ?? DEFAULT_TABLE_PAGE_SIZE;
  // debouncedSearch lags behind the input so we don't spam the server on every
  // keystroke; the search box still feels instant locally.
  const [debouncedSearch, setDebouncedSearch] = useState(searchKey);
  const [prevLocationKey, setPrevLocationKey] = useState(location.key);

  if (location.key !== prevLocationKey) {
    setPrevLocationKey(location.key);
    if (searchParam !== null) {
      setSearchKey(searchParam);
      setDebouncedSearch(searchParam);
    }
  }

  useEffect(() => {
    localStorage.setItem(
      FILTER_STATE_KEY,
      JSON.stringify({
        searchKey,
        filters,
        sort: sortValueFor(sortColumn, sortOrder),
        // Only ever persist a size we actually resolved, never the render fallback.
        pageSize: resolvedPageSize,
      }),
    );
  }, [searchKey, filters, sortColumn, sortOrder, resolvedPageSize]);

  useEffect(() => {
    const handle = window.setTimeout(() => setDebouncedSearch(searchKey), 300);
    return () => window.clearTimeout(handle);
  }, [searchKey]);

  useEffect(() => {
    // Reset to page 1 whenever a filter or sort changes — otherwise an empty
    // result set on a high page number leaves the user staring at "no clients".
    setCurrentPage(1);
  }, [debouncedSearch, filters, sortColumn, sortOrder]);

  // The node filter maps onto inbound ids client-side (#4997): the paging API
  // already accepts an inbound CSV, so nodes never have to reach the backend.
  // Sentinel 0 = "local panel" (inbounds without a nodeId).
  const effectiveInboundCsv = useMemo(() => {
    if (!filters.nodeIds.length) return filters.inboundIds.join(',');
    const nodeSet = new Set(filters.nodeIds);
    const nodeInboundIds = inbounds.filter((ib) => nodeSet.has(ib.nodeId ?? 0)).map((ib) => ib.id);
    const pool = filters.inboundIds.length
      ? nodeInboundIds.filter((id) => filters.inboundIds.includes(id))
      : nodeInboundIds;
    // Nothing matches the selected nodes: send an impossible id so the filter
    // yields an honest empty result instead of being silently ignored.
    return pool.length ? pool.join(',') : '-1';
  }, [filters.nodeIds, filters.inboundIds, inbounds]);

  useEffect(() => {
    // With no remembered size and no settings yet, any query we build would be a
    // guess, and issuing it costs a full server round trip that is thrown away as
    // soon as the real size arrives.
    if (resolvedPageSize === null) return;
    setQuery({
      page: currentPage,
      pageSize: tablePageSize,
      search: debouncedSearch,
      filter: filters.buckets.join(','),
      protocol: filters.protocols.join(','),
      inbound: effectiveInboundCsv,
      expiryFrom: filters.expiryFrom,
      expiryTo: filters.expiryTo,
      usageFrom: gbToBytes(filters.usageFromGB),
      usageTo: gbToBytes(filters.usageToGB),
      autoRenew: filters.autoRenew || undefined,
      hasTgId: filters.hasTgId || undefined,
      hasComment: filters.hasComment || undefined,
      plan: filters.plans.join(',') || undefined,
      sort: sortColumn || undefined,
      order: sortOrder || undefined,
    });
  }, [
    setQuery,
    resolvedPageSize,
    currentPage,
    tablePageSize,
    debouncedSearch,
    filters,
    effectiveInboundCsv,
    sortColumn,
    sortOrder,
  ]);

  const activeCount = activeFilterCount(filters);

  // Row handlers take an email and look the row up here at call time. Keying
  // them on the record object instead would defeat the memoised cells: every
  // traffic push replaces the row object of every client whose counters moved,
  // so the memo would miss on exactly the rows that are busy. Reading through
  // the ref also means a modal opened mid-poll shows current usage.
  const rowsByEmail = useRef(new Map<string, ClientRecord>());
  rowsByEmail.current = useMemo(() => {
    const map = new Map<string, ClientRecord>();
    for (const c of clients) map.set(c.email, c);
    return map;
  }, [clients]);

  const onlineSet = useMemo(() => new Set(onlines || []), [onlines]);
  const inboundsById = useMemo(() => {
    const out: Record<number, InboundOption> = {};
    for (const ib of inbounds) out[ib.id] = ib;
    return out;
  }, [inbounds]);

  const protocolOptions = useMemo(() => {
    const values = new Set<string>(
      (inbounds || []).map((i) => i.protocol).filter((x): x is string => !!x),
    );
    return [...values].sort();
  }, [inbounds]);

  const isOnline = useCallback((email: string) => !!email && onlineSet.has(email), [onlineSet]);

  function inboundLabel(id: number) {
    const ib = inboundsById[id];
    return formatInboundLabel(ib?.tag, ib?.remark);
  }

  // The list page renders rows the server already sorted, filtered, and
  // paginated. Local filtering is gone — keep the variable name so the rest
  // of the file (table dataSource, mobile cards, select-all) doesn't need
  // a rename.
  const filteredClients = clients;

  // Sort is server-side now; the page already arrives in the requested
  // order, so we just hand it through.
  const sortedClients = filteredClients;

  function expiryLabel(row: ClientRecord) {
    if (!row.expiryTime) return t('pages.plans.permanent');
    if (row.expiryTime < 0) {
      const days = Math.round(row.expiryTime / -86400000);
      return `${t('pages.clients.delayedStart')}: ${days}d`;
    }
    return IntlUtil.formatDate(row.expiryTime, datepicker);
  }

  // Read at call time, so a row's state follows the clock between polls.
  const stateOf = useCallback((row: ClientRecord) => clientState(row, Date.now()), []);
  const daysLeftOf = useCallback((row: ClientRecord) => daysToExpiry(row, Date.now()), []);

  const onSetEnable = useCallback(
    async (email: string, next: boolean) => {
      const row = rowsByEmail.current.get(email);
      if (!row) return;
      const msg = await setEnable(row, next);
      if (!msg?.success) messageApi.error(msg?.msg || t('somethingWentWrong'));
    },
    [setEnable, messageApi, t],
  );

  const onSaveComment = useCallback(
    async (email: string, comment: string) => !!(await setComment(email, comment))?.success,
    [setComment],
  );

  const onPlanClick = useCallback((email: string) => {
    const row = rowsByEmail.current.get(email);
    setPlanTarget({ emails: [email], planId: row?.planId || undefined });
  }, []);

  const subLinkOf = useCallback(
    (row: ClientRecord) =>
      subSettings.enable && subSettings.subURI && row.subId ? subSettings.subURI + row.subId : '',
    [subSettings.enable, subSettings.subURI],
  );

  const onCopySub = useCallback(
    async (email: string) => {
      const row = rowsByEmail.current.get(email);
      const link = row ? subLinkOf(row) : '';
      if (link && (await ClipboardManager.copyText(link))) messageApi.success(t('copied'));
    },
    [subLinkOf, messageApi, t],
  );

  const onRenew = useCallback(
    (email: string) => setRenewTarget({ emails: [email], bulk: false }),
    [],
  );

  function onAdd() {
    setFormMode('add');
    setEditingClient(null);
    setEditingAttachedIds([]);
    setEditingExternalLinks([]);
    setEditingTunnelAllowedIPs({});
    setFormOpen(true);
  }

  const onEdit = useCallback(
    async (email: string) => {
      const row = rowsByEmail.current.get(email);
      if (!row) return;
      setFormMode('edit');
      // Paged list omits per-client secrets to keep the row payload tiny;
      // edit needs them, so fetch the full record first.
      const full = await hydrate(row.email);
      const merged: ClientRecord = full ? { ...row, ...full.client } : { ...row };
      setEditingClient(merged);
      const ids = full?.inboundIds ?? (Array.isArray(row.inboundIds) ? row.inboundIds : []);
      setEditingAttachedIds([...ids]);
      setEditingExternalLinks(Array.isArray(full?.externalLinks) ? [...full.externalLinks] : []);
      setEditingTunnelAllowedIPs(full?.tunnelAllowedIPs ?? {});
      setFormOpen(true);
    },
    [hydrate],
  );

  const onDelete = useCallback(
    (email: string) => {
      const row = rowsByEmail.current.get(email);
      if (!row) return;
      modal.confirm({
        title: t('pages.clients.deleteConfirmTitle', { email: row.email }),
        content: t('pages.clients.deleteConfirmContent'),
        okText: t('delete'),
        okType: 'danger',
        cancelText: t('cancel'),
        onOk: async () => {
          const msg = await remove(row.email);
          if (msg?.success) messageApi.success(t('pages.clients.toasts.deleted'));
        },
      });
    },
    [modal, t, remove, messageApi],
  );

  const onResetTraffic = useCallback(
    (email: string) => {
      const row = rowsByEmail.current.get(email);
      if (!row?.email) {
        messageApi.warning(t('pages.clients.resetNotPossible'));
        return;
      }
      modal.confirm({
        title: `${t('pages.inbounds.resetTraffic')} — ${row.email}`,
        content: t('pages.inbounds.resetTrafficContent'),
        okText: t('reset'),
        cancelText: t('cancel'),
        onOk: async () => {
          const msg = await resetTraffic(row);
          if (msg?.success) messageApi.success(t('pages.clients.toasts.trafficReset'));
        },
      });
    },
    [modal, t, resetTraffic, messageApi],
  );

  const onShowInfo = useCallback(
    async (email: string) => {
      const row = rowsByEmail.current.get(email);
      if (!row) return;
      const full = await hydrate(row.email);
      setInfoClient(full ? { ...row, ...full.client, inboundIds: full.inboundIds } : row);
      setViewingTunnelAllowedIPs(full?.tunnelAllowedIPs ?? {});
      setInfoOpen(true);
    },
    [hydrate],
  );

  const onShowQr = useCallback(
    async (email: string) => {
      const row = rowsByEmail.current.get(email);
      if (!row) return;
      const full = await hydrate(row.email);
      setQrClient(full ? { ...row, ...full.client, inboundIds: full.inboundIds } : row);
      setViewingTunnelAllowedIPs(full?.tunnelAllowedIPs ?? {});
      setQrOpen(true);
    },
    [hydrate],
  );

  const [refreshing, setRefreshing] = useState(false);
  const onRefreshClick = useCallback(async () => {
    setRefreshing(true);
    try {
      await refresh();
    } finally {
      setRefreshing(false);
    }
  }, [refresh]);

  const openText = useCallback((opts: { title: string; content: string; fileName?: string }) => {
    setTextTitle(opts.title);
    setTextContent(opts.content);
    setTextFileName(opts.fileName || '');
    setTextOpen(true);
  }, []);

  const openPrompt = useCallback(
    (opts: {
      title: string;
      okText?: string;
      value?: string;
      confirm: (value: string) => Promise<boolean | void> | boolean | void;
    }) => {
      setPromptTitle(opts.title);
      setPromptOkText(opts.okText || t('confirm'));
      setPromptInitial(opts.value || '');
      setPromptHandler(() => opts.confirm);
      setPromptOpen(true);
    },
    [t],
  );

  const onPromptConfirm = useCallback(
    async (value: string) => {
      if (!promptHandler) {
        setPromptOpen(false);
        return;
      }
      setPromptLoading(true);
      try {
        const ok = await promptHandler(value);
        if (ok !== false) setPromptOpen(false);
      } finally {
        setPromptLoading(false);
      }
    },
    [promptHandler],
  );

  function onResetAllTraffics() {
    modal.confirm({
      title: t('pages.clients.resetAllTrafficsTitle'),
      content: t('pages.clients.resetAllTrafficsContent'),
      okText: t('reset'),
      okType: 'danger',
      cancelText: t('cancel'),
      onOk: async () => {
        const msg = await resetAllTraffics();
        if (msg?.success) messageApi.success(t('pages.clients.toasts.allTrafficsReset'));
      },
    });
  }

  function onDelDepleted() {
    modal.confirm({
      title: t('pages.clients.delDepletedConfirmTitle'),
      content: t('pages.clients.delDepletedConfirmContent'),
      okText: t('delete'),
      okType: 'danger',
      cancelText: t('cancel'),
      onOk: async () => {
        const msg = await delDepleted();
        if (msg?.success) {
          const deleted = msg.obj?.deleted ?? 0;
          messageApi.success(t('pages.clients.toasts.delDepleted', { count: deleted }));
        }
      },
    });
  }

  function onDeleteOrphans() {
    modal.confirm({
      title: t('pages.clients.delOrphansConfirmTitle'),
      content: t('pages.clients.delOrphansConfirmContent'),
      okText: t('delete'),
      okType: 'danger',
      cancelText: t('cancel'),
      onOk: async () => {
        const msg = await delOrphans();
        if (msg?.success) {
          const deleted = msg.obj?.deleted ?? 0;
          messageApi.success(t('pages.clients.toasts.delOrphans', { count: deleted }));
        }
      },
    });
  }

  async function onExportClients() {
    const items = await exportClients();
    if (!items) return;
    openText({
      title: t('pages.clients.exportClients'),
      content: JSON.stringify(items, null, 2),
      fileName: 'clients-export.json',
    });
  }

  function onImportClients() {
    openPrompt({
      title: t('pages.clients.importClients'),
      okText: t('pages.clients.import'),
      value: '',
      confirm: async (value) => {
        const msg = await importClients(value);
        if (!msg?.success) return false;
        const created = msg.obj?.created ?? 0;
        const skipped = msg.obj?.skipped ?? [];
        if (skipped.length === 0) {
          messageApi.success(t('pages.clients.toasts.imported', { count: created }));
        } else {
          const firstError = skipped[0]?.reason ?? '';
          messageApi.warning(
            firstError
              ? `${t('pages.clients.toasts.importedMixed', { ok: created, failed: skipped.length })} — ${firstError}`
              : t('pages.clients.toasts.importedMixed', { ok: created, failed: skipped.length }),
          );
        }
        return true;
      },
    });
  }

  function onBulkRenew() {
    const emails = [...selectedRowKeys];
    if (emails.length > 0) setRenewTarget({ emails, bulk: true });
  }

  function onBulkUnassignPlan() {
    const emails = [...selectedRowKeys];
    if (emails.length === 0) return;
    modal.confirm({
      title: t('pages.plans.unassignConfirm', { count: emails.length }),
      okText: t('confirm'),
      okType: 'danger',
      cancelText: t('cancel'),
      onOk: async () => {
        const msg = await unassignPlan(emails);
        if (msg?.success) {
          setSelectedRowKeys([]);
          messageApi.success(t('pages.plans.toasts.unassigned', { count: emails.length }));
        }
      },
    });
  }

  function onBulkSetEnable(enable: boolean) {
    const emails = [...selectedRowKeys];
    if (emails.length === 0) return;
    modal.confirm({
      title: t(
        enable ? 'pages.clients.bulkEnableConfirmTitle' : 'pages.clients.bulkDisableConfirmTitle',
        { count: emails.length },
      ),
      content: t(
        enable
          ? 'pages.clients.bulkEnableConfirmContent'
          : 'pages.clients.bulkDisableConfirmContent',
      ),
      okText: t('confirm'),
      okType: enable ? 'primary' : 'danger',
      cancelText: t('cancel'),
      onOk: async () => {
        const msg = enable ? await bulkEnable(emails) : await bulkDisable(emails);
        setSelectedRowKeys([]);
        const changed = msg?.obj?.changed ?? 0;
        const skipped = msg?.obj?.skipped ?? [];
        const failed = skipped.length;
        const firstError = skipped[0]?.reason ?? msg?.msg ?? '';
        const okKey = enable
          ? 'pages.clients.toasts.bulkEnabled'
          : 'pages.clients.toasts.bulkDisabled';
        const mixedKey = enable
          ? 'pages.clients.toasts.bulkEnabledMixed'
          : 'pages.clients.toasts.bulkDisabledMixed';
        if (failed === 0 && msg?.success) {
          messageApi.success(t(okKey, { count: changed }));
        } else {
          messageApi.warning(
            firstError
              ? `${t(mixedKey, { ok: changed, failed })} — ${firstError}`
              : t(mixedKey, { ok: changed, failed }),
          );
        }
      },
    });
  }

  function onBulkDelete() {
    const emails = [...selectedRowKeys];
    if (emails.length === 0) return;
    modal.confirm({
      title: t('pages.clients.bulkDeleteConfirmTitle', { count: emails.length }),
      content: t('pages.clients.bulkDeleteConfirmContent'),
      okText: t('delete'),
      okType: 'danger',
      cancelText: t('cancel'),
      onOk: async () => {
        const msg = await bulkDelete(emails);
        setSelectedRowKeys([]);
        const ok = msg?.obj?.deleted ?? 0;
        const skipped = msg?.obj?.skipped ?? [];
        const failed = skipped.length;
        const firstError = skipped[0]?.reason ?? msg?.msg ?? '';
        if (failed === 0 && msg?.success) {
          messageApi.success(t('pages.clients.toasts.bulkDeleted', { count: ok }));
        } else {
          messageApi.warning(
            firstError
              ? `${t('pages.clients.toasts.bulkDeletedMixed', { ok, failed })} — ${firstError}`
              : t('pages.clients.toasts.bulkDeletedMixed', { ok, failed }),
          );
        }
      },
    });
  }

  const onSave = useCallback(
    async (
      payload: Record<string, unknown> | { client: Record<string, unknown>; inboundIds: number[] },
      meta:
        | { isEdit: false; email: string; externalLinks: ExternalLinkInput[] }
        | {
            isEdit: true;
            email: string;
            attach: number[];
            detach: number[];
            externalLinks: ExternalLinkInput[];
          },
    ) => {
      if (!meta.isEdit) {
        const createMsg = await create(payload);
        if (!createMsg?.success) return createMsg;
        if (meta.email && meta.externalLinks.length > 0) {
          const r = await setExternalLinks(meta.email, meta.externalLinks);
          if (!r?.success) return r;
        }
        return createMsg;
      }
      const updateMsg = await update(meta.email, payload);
      if (!updateMsg?.success) return updateMsg;
      const rawEmail = (payload as { email?: unknown }).email;
      const emailKey =
        typeof rawEmail === 'string' && rawEmail.trim() ? rawEmail.trim() : meta.email;
      if (Array.isArray(meta.attach) && meta.attach.length > 0) {
        const r = await attach(emailKey, meta.attach);
        if (!r?.success) return r;
      }
      if (Array.isArray(meta.detach) && meta.detach.length > 0) {
        const r = await detach(emailKey, meta.detach);
        if (!r?.success) return r;
      }
      // Always replace the client's external links (an empty set clears them).
      const r = await setExternalLinks(emailKey, meta.externalLinks);
      if (!r?.success) return r;
      return updateMsg;
    },
    [create, update, attach, detach, setExternalLinks],
  );

  const pageClass = useMemo(() => {
    const classes = ['clients-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const onTableChange: NonNullable<TableProps<ClientRecord>['onChange']> = (pag) => {
    if (pag?.current) setCurrentPage(pag.current);
    if (pag?.pageSize) setPageSizeChoice(pag.pageSize);
  };

  const rowHandlers = useMemo(
    () => ({
      onShowQr,
      onShowInfo,
      onEdit,
      onResetTraffic,
      onRenew,
      onSetEnable,
      onPortal,
      onDelete,
    }),
    [onShowQr, onShowInfo, onEdit, onResetTraffic, onRenew, onSetEnable, onPortal, onDelete],
  );

  const columns = useMemo<ColumnsType<ClientRecord>>(
    () => {
      const emailColumn = {
        title: t('pages.clients.client'),
        key: 'email',
        render: (_v: unknown, record: ClientRecord) => (
          <span className="client-email">{record.email}</span>
        ),
      };
      if (view === 'renewal') {
        return [
          emailColumn,
          {
            title: t('menu.plans'),
            key: 'plan',
            render: (_v, record) =>
              record.planId ? (
                (planNames.get(record.planId) ?? `#${record.planId}`)
              ) : (
                <span className="client-muted">{t('pages.plans.noPlan')}</span>
              ),
          },
          {
            title: t('pages.clients.expiryTime'),
            key: 'expiryTime',
            render: (_v, record) => expiryLabel(record),
          },
          {
            title: t('pages.clients.daysLeft'),
            key: 'daysLeft',
            render: (_v, record) => (
              <DaysLeftText days={daysLeftOf(record)} delayed={(record.expiryTime ?? 0) < 0} />
            ),
          },
          {
            title: t('pages.clients.nextReset'),
            key: 'nextReset',
            render: (_v, record) =>
              record.nextReset ? (
                <Tooltip title={IntlUtil.formatRelativeTime(record.nextReset)}>
                  <span>{IntlUtil.formatDate(record.nextReset, datepicker)}</span>
                </Tooltip>
              ) : (
                '—'
              ),
          },
          {
            title: t('pages.plans.renew'),
            key: 'renew',
            align: 'right',
            render: (_v, record) => (
              <Button
                size="small"
                icon={<FieldTimeOutlined />}
                onClick={() => onRenew(record.email)}
              >
                {t('pages.plans.renew')}
              </Button>
            ),
          },
        ];
      }
      return [
        emailColumn,
        {
          title: t('pages.clients.comment'),
          key: 'comment',
          width: 200,
          render: (_v, record) => (
            <ClientCommentCell
              email={record.email}
              comment={record.comment}
              onSave={onSaveComment}
            />
          ),
        },
        {
          title: t('pages.clients.subId'),
          key: 'subId',
          render: (_v, record) =>
            record.subId ? <span className="client-sub-id">{record.subId}</span> : '—',
        },
        {
          title: t('pages.clients.subscription'),
          key: 'subscription',
          render: (_v, record) =>
            subLinkOf(record) ? (
              <Button size="small" icon={<CopyOutlined />} onClick={() => onCopySub(record.email)}>
                {t('pages.clients.copySub')}
              </Button>
            ) : (
              '—'
            ),
        },
        {
          title: t('pages.clients.planUsage'),
          key: 'plan',
          width: 240,
          render: (_v, record) => (
            <PlanUsageCell
              email={record.email}
              planName={record.planId ? planNames.get(record.planId) : undefined}
              used={usedBytes(record)}
              total={record.totalGB ?? 0}
              onPlanClick={onPlanClick}
            />
          ),
        },
        {
          title: t('pages.clients.online'),
          key: 'online',
          align: 'center',
          render: (_v, record) => (
            <OnlineDot
              online={!!record.enable && isOnline(record.email)}
              lastOnline={record.traffic?.lastOnline ?? 0}
            />
          ),
        },
        {
          title: t('pages.clients.statusTitle'),
          key: 'status',
          render: (_v, record) => <ClientStateTag state={stateOf(record)} />,
        },
        {
          title: t('pages.clients.actions'),
          key: 'actions',
          align: 'right',
          render: (_v, record) => (
            <ClientRowMenu email={record.email} enabled={!!record.enable} {...rowHandlers} />
          ),
        },
      ];
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [
      t,
      view,
      planNames,
      datepicker,
      isOnline,
      stateOf,
      daysLeftOf,
      subLinkOf,
      onCopySub,
      onSaveComment,
      onPlanClick,
      onRenew,
      rowHandlers,
    ],
  );

  const tablePagination = {
    current: currentPage,
    pageSize: tablePageSize,
    total: filtered,
    showSizeChanger: filtered > 10,
    pageSizeOptions: ['10', '25', '50', '100', '200'],
    hideOnSinglePage: filtered <= tablePageSize,
    showTotal: (n: number) => `${n}`,
  };

  const rowSelection = {
    selectedRowKeys,
    onChange: (keys: React.Key[]) => setSelectedRowKeys(keys as string[]),
  };

  function toggleSelect(email: string, checked: boolean) {
    setSelectedRowKeys((prev) => {
      const next = new Set(prev);
      if (checked) next.add(email);
      else next.delete(email);
      return Array.from(next);
    });
  }

  function selectAll(checked: boolean) {
    setSelectedRowKeys(checked ? filteredClients.map((c) => c.email) : []);
  }

  const allSelected =
    filteredClients.length > 0 && selectedRowKeys.length === filteredClients.length;
  const someSelected =
    selectedRowKeys.length > 0 && selectedRowKeys.length < filteredClients.length;

  const isOnlyBucket = (bucket: string) =>
    filters.buckets.length === 1 && filters.buckets[0] === bucket;

  // A chip filters on its own; clicking the selected chip again clears it.
  function onBucketChip(bucket: string) {
    setFilters({ ...filters, buckets: isOnlyBucket(bucket) ? [] : [bucket] });
  }

  function onPlanChip(planId: number) {
    const only = filters.plans.length === 1 && filters.plans[0] === planId;
    setFilters({ ...filters, plans: only ? [] : [planId] });
  }

  function toggleSelecting() {
    if (selecting) setSelectedRowKeys([]);
    setSelecting(!selecting);
  }

  function changeView(next: ClientsView) {
    setView(next);
    try {
      localStorage.setItem(VIEW_KEY, next);
    } catch {
      // The view choice is a convenience; private windows may refuse storage.
    }
  }

  function clearOneFilter<K extends keyof ClientFilters>(key: K) {
    if (key === 'expiryFrom' || key === 'expiryTo') {
      setFilters({ ...filters, expiryFrom: undefined, expiryTo: undefined });
      return;
    }
    if (key === 'usageFromGB' || key === 'usageToGB') {
      setFilters({ ...filters, usageFromGB: undefined, usageToGB: undefined });
      return;
    }
    setFilters({ ...filters, [key]: emptyFilters()[key] });
  }

  const moreMenuItems: MenuProps['items'] = [
    {
      key: 'bulk',
      icon: <UsergroupAddOutlined />,
      label: t('pages.clients.bulk'),
      onClick: () => setBulkAddOpen(true),
    },
    {
      key: 'export',
      icon: <DownloadOutlined />,
      label: t('pages.clients.exportClients'),
      onClick: onExportClients,
    },
    {
      key: 'import',
      icon: <UploadOutlined />,
      label: t('pages.clients.importClients'),
      onClick: onImportClients,
    },
    {
      key: 'resetAll',
      icon: <RetweetOutlined />,
      label: t('pages.clients.resetAllTraffics'),
      onClick: onResetAllTraffics,
    },
    { type: 'divider' },
    {
      key: 'delDepleted',
      icon: <RestOutlined />,
      label: t('pages.clients.delDepleted'),
      danger: true,
      onClick: onDelDepleted,
    },
    {
      key: 'delOrphans',
      icon: <DisconnectOutlined />,
      label: t('pages.clients.delOrphans'),
      danger: true,
      onClick: onDeleteOrphans,
    },
  ];

  const bulkMenuItems: MenuProps['items'] = [
    {
      key: 'attach',
      icon: <UsergroupAddOutlined />,
      label: t('pages.clients.attach'),
      onClick: () => setBulkAttachOpen(true),
    },
    {
      key: 'detach',
      icon: <UsergroupDeleteOutlined />,
      label: t('pages.clients.detach'),
      danger: true,
      onClick: () => setBulkDetachOpen(true),
    },
    ...(plans.length > 0
      ? [
          { type: 'divider' as const },
          {
            key: 'assignPlan',
            icon: <ProfileOutlined />,
            label: t('pages.plans.assign'),
            onClick: () => setPlanTarget({ emails: [...selectedRowKeys] }),
          },
          {
            key: 'renewPlan',
            icon: <FieldTimeOutlined />,
            label: t('pages.plans.renew'),
            onClick: onBulkRenew,
          },
          {
            key: 'unassignPlan',
            icon: <MinusCircleOutlined />,
            label: t('pages.plans.unassign'),
            danger: true,
            onClick: onBulkUnassignPlan,
          },
        ]
      : []),
    { type: 'divider' },
    {
      key: 'enable',
      icon: <CheckCircleOutlined />,
      label: t('pages.clients.enable'),
      onClick: () => onBulkSetEnable(true),
    },
    {
      key: 'disable',
      icon: <StopOutlined />,
      label: t('pages.clients.disable'),
      danger: true,
      onClick: () => onBulkSetEnable(false),
    },
    {
      key: 'adjust',
      icon: <ClockCircleOutlined />,
      label: t('pages.clients.adjust'),
      onClick: () => setBulkAdjustOpen(true),
    },
    {
      key: 'subLinks',
      icon: <LinkOutlined />,
      label: t('pages.clients.subLinks'),
      onClick: () => setSubLinksOpen(true),
    },
  ];

  // A single bucket or plan shows as a highlighted chip, so only the rest need tags.
  const filterTagCount =
    activeCount - (filters.buckets.length === 1 ? 1 : 0) - (filters.plans.length === 1 ? 1 : 0);

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      {modalContextHolder}
      <Layout className={pageClass}>
        <AppNav />

        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <PageHeader title={t('menu.clients')} description={t('pages.clients.intro')} />
            <Spin spinning={!fetched} delay={200} description={t('loading')} size="large">
              {!fetched ? (
                <div className="loading-spacer" />
              ) : fetchError ? (
                <Result
                  status="error"
                  title={t('somethingWentWrong')}
                  subTitle={fetchError}
                  extra={
                    <Button type="primary" loading={refreshing} onClick={onRefreshClick}>
                      {t('refresh')}
                    </Button>
                  }
                />
              ) : (
                <Card
                  size="small"
                  className="clients-card"
                  title={t('pages.clients.listTitle')}
                  extra={
                    <div className="clients-card-extra">
                      <Segmented<ClientsView>
                        value={view}
                        onChange={changeView}
                        options={[
                          {
                            value: 'full',
                            icon: <AppstoreOutlined />,
                            label: isMobile ? undefined : t('pages.clients.viewFull'),
                            title: t('pages.clients.viewFull'),
                          },
                          {
                            value: 'renewal',
                            icon: <ScheduleOutlined />,
                            label: isMobile ? undefined : t('pages.clients.viewRenewal'),
                            title: t('pages.clients.viewRenewal'),
                          },
                        ]}
                      />
                      <Button
                        icon={<CheckSquareOutlined />}
                        type={selecting ? 'primary' : 'default'}
                        aria-pressed={selecting}
                        aria-label={t('pages.clients.bulkMode')}
                        onClick={toggleSelecting}
                      >
                        {!isMobile && t('pages.clients.bulkMode')}
                      </Button>
                      <Button
                        type="primary"
                        icon={<PlusOutlined />}
                        onClick={onAdd}
                        aria-label={t('pages.clients.addClients')}
                      >
                        {!isMobile && t('pages.clients.addClients')}
                      </Button>
                      <Dropdown
                        trigger={['click']}
                        placement="bottomRight"
                        menu={{ items: moreMenuItems }}
                      >
                        <Button icon={<MoreOutlined />} aria-label={t('more')} />
                      </Dropdown>
                    </div>
                  }
                >
                  <div className={isMobile ? 'filter-bar mobile' : 'filter-bar'}>
                    <Input
                      value={searchKey}
                      onChange={(e) => setSearchKey(e.target.value)}
                      placeholder={t('pages.clients.searchPlaceholder')}
                      allowClear
                      prefix={<SearchOutlined />}
                      size={isMobile ? 'small' : 'middle'}
                      style={{ maxWidth: 320 }}
                      aria-label={t('search')}
                    />
                    <Badge count={activeCount} size="small" offset={[-4, 4]}>
                      <Button
                        icon={<FilterOutlined />}
                        size={isMobile ? 'small' : 'middle'}
                        onClick={() => setFilterDrawerOpen(true)}
                        type={activeCount > 0 ? 'primary' : 'default'}
                        aria-label={t('filter')}
                      >
                        {!isMobile && t('filter')}
                      </Button>
                    </Badge>
                    <Select
                      value={sortValueFor(sortColumn, sortOrder)}
                      aria-label={t('sort')}
                      size={isMobile ? 'small' : 'middle'}
                      suffix={<SortAscendingOutlined />}
                      style={{ minWidth: isMobile ? 130 : 200 }}
                      onChange={(value) => {
                        const opt = SORT_OPTIONS.find((o) => o.value === value);
                        setSortColumn(opt?.column ?? null);
                        setSortOrder(opt?.order ?? null);
                      }}
                      options={SORT_OPTIONS.map((o) => ({
                        value: o.value,
                        label: t(o.labelKey),
                      }))}
                    />
                    {activeCount > 0 && (
                      <Button
                        size={isMobile ? 'small' : 'middle'}
                        onClick={() => setFilters(emptyFilters())}
                      >
                        {t('pages.clients.clearAllFilters')}
                      </Button>
                    )}
                    {(activeCount > 0 || debouncedSearch.trim().length > 0) && (
                      <span className="filter-count">
                        {t('pages.clients.showingCount', { shown: filtered, total })}
                      </span>
                    )}
                  </div>

                  <ClientChips
                    plans={plans}
                    summary={summary}
                    planFilter={filters.plans}
                    bucketFilter={filters.buckets}
                    onShowAll={() => setFilters({ ...filters, plans: [], buckets: [] })}
                    onPlan={onPlanChip}
                    onBucket={onBucketChip}
                  />

                  {filterTagCount > 0 && (
                    <div className="filter-chips">
                      {filters.buckets.length > 1 &&
                        filters.buckets.map((b) => (
                          <Tag
                            key={`b-${b}`}
                            closable
                            onClose={() =>
                              setFilters({
                                ...filters,
                                buckets: filters.buckets.filter((x) => x !== b),
                              })
                            }
                          >
                            {bucketChipLabel(b, t)}
                          </Tag>
                        ))}
                      {filters.protocols.map((p) => (
                        <Tag
                          key={`p-${p}`}
                          closable
                          color="blue"
                          onClose={() =>
                            setFilters({
                              ...filters,
                              protocols: filters.protocols.filter((x) => x !== p),
                            })
                          }
                        >
                          {p}
                        </Tag>
                      ))}
                      {filters.inboundIds.map((id) => (
                        <Tag
                          key={`i-${id}`}
                          closable
                          color="cyan"
                          onClose={() =>
                            setFilters({
                              ...filters,
                              inboundIds: filters.inboundIds.filter((x) => x !== id),
                            })
                          }
                        >
                          {inboundLabel(id)}
                        </Tag>
                      ))}
                      {filters.plans.length > 1 &&
                        filters.plans.map((id) => (
                          <Tag
                            key={`p-${id}`}
                            closable
                            color="volcano"
                            onClose={() =>
                              setFilters({
                                ...filters,
                                plans: filters.plans.filter((x) => x !== id),
                              })
                            }
                          >
                            {t('menu.plans')}:{' '}
                            {id === 0 ? t('pages.plans.noPlan') : (planNames.get(id) ?? `#${id}`)}
                          </Tag>
                        ))}
                      {(filters.expiryFrom || filters.expiryTo) && (
                        <Tag closable color="purple" onClose={() => clearOneFilter('expiryFrom')}>
                          {t('pages.clients.expiryTime')}:{' '}
                          {filters.expiryFrom
                            ? IntlUtil.formatDate(filters.expiryFrom, datepicker)
                            : '…'}
                          {' → '}
                          {filters.expiryTo
                            ? IntlUtil.formatDate(filters.expiryTo, datepicker)
                            : '…'}
                        </Tag>
                      )}
                      {(filters.usageFromGB || filters.usageToGB) && (
                        <Tag closable color="orange" onClose={() => clearOneFilter('usageFromGB')}>
                          {t('pages.clients.traffic')}: {filters.usageFromGB ?? 0}
                          {filters.usageToGB ? `–${filters.usageToGB}` : '+'} GB
                        </Tag>
                      )}
                      {filters.autoRenew && (
                        <Tag closable color="gold" onClose={() => clearOneFilter('autoRenew')}>
                          {t('pages.clients.renew')}:{' '}
                          {filters.autoRenew === 'on' ? t('enabled') : t('disabled')}
                        </Tag>
                      )}
                      {filters.hasTgId && (
                        <Tag closable onClose={() => clearOneFilter('hasTgId')}>
                          {t('pages.clients.telegramId')}:{' '}
                          {filters.hasTgId === 'yes'
                            ? t('pages.clients.has')
                            : t('pages.clients.hasNot')}
                        </Tag>
                      )}
                      {filters.hasComment && (
                        <Tag closable onClose={() => clearOneFilter('hasComment')}>
                          {t('pages.clients.comment')}:{' '}
                          {filters.hasComment === 'yes'
                            ? t('pages.clients.has')
                            : t('pages.clients.hasNot')}
                        </Tag>
                      )}
                    </div>
                  )}

                  {selecting && (
                    <div className="bulk-bar">
                      <Tag color="blue">
                        {t('pages.clients.selectedCount', { count: selectedRowKeys.length })}
                      </Tag>
                      <Dropdown
                        trigger={['click']}
                        disabled={selectedRowKeys.length === 0}
                        menu={{ items: bulkMenuItems }}
                      >
                        <Button icon={<MoreOutlined />}>{t('pages.clients.actions')}</Button>
                      </Dropdown>
                      <Button
                        danger
                        icon={<DeleteOutlined />}
                        disabled={selectedRowKeys.length === 0}
                        onClick={onBulkDelete}
                      >
                        {t('delete')}
                      </Button>
                    </div>
                  )}

                  {!isMobile ? (
                    <Table<ClientRecord>
                      columns={columns}
                      dataSource={sortedClients}
                      loading={transitioning}
                      rowKey="email"
                      rowSelection={selecting ? rowSelection : undefined}
                      pagination={tablePagination}
                      size="small"
                      scroll={{ x: 'max-content' }}
                      onChange={onTableChange}
                      locale={{
                        emptyText: (
                          <div className="clients-empty">
                            <TeamOutlined style={{ fontSize: 32, marginBottom: 8 }} />
                            <div>{t('noData')}</div>
                          </div>
                        ),
                      }}
                    />
                  ) : (
                    <Spin spinning={transitioning}>
                      <div className="client-cards">
                        {selecting && filteredClients.length > 0 && (
                          <div className="card-bulk-bar">
                            <Checkbox
                              checked={allSelected}
                              indeterminate={someSelected}
                              onChange={(e) => selectAll(e.target.checked)}
                            >
                              {t('pages.clients.selectAll')}
                            </Checkbox>
                          </div>
                        )}
                        {filteredClients.length === 0 && (
                          <div className="card-empty">
                            <TeamOutlined style={{ fontSize: 28, opacity: 0.5 }} />
                            <div>{t('noData')}</div>
                          </div>
                        )}
                        {filteredClients.length > 0 && (
                          <div className="card-pagination">
                            <Pagination
                              current={currentPage}
                              pageSize={tablePageSize}
                              total={filtered}
                              showSizeChanger={filtered > 10}
                              pageSizeOptions={['10', '25', '50', '100', '200']}
                              hideOnSinglePage={filtered <= tablePageSize}
                              size="small"
                              showTotal={(n) => `${n}`}
                              onChange={(p, s) => {
                                setCurrentPage(p);
                                if (s && s !== tablePageSize) setPageSizeChoice(s);
                              }}
                            />
                          </div>
                        )}
                        {filteredClients.map((row) => (
                          <div
                            key={row.email}
                            className={`client-card${selectedRowKeys.includes(row.email) ? ' is-selected' : ''}`}
                          >
                            <div className="card-head">
                              {selecting && (
                                <Checkbox
                                  checked={selectedRowKeys.includes(row.email)}
                                  onChange={(e) => toggleSelect(row.email, e.target.checked)}
                                />
                              )}
                              <OnlineDot
                                online={!!row.enable && isOnline(row.email)}
                                lastOnline={row.traffic?.lastOnline ?? 0}
                              />
                              <span className="tag-name">{row.email}</span>
                              <ClientStateTag state={stateOf(row)} />
                              <div className="card-actions">
                                <ClientRowMenu
                                  email={row.email}
                                  enabled={!!row.enable}
                                  {...rowHandlers}
                                />
                              </div>
                            </div>
                            {row.comment ? (
                              <ClientCardComment comment={row.comment} />
                            ) : (
                              <span className="client-card-comment client-muted">—</span>
                            )}
                            {view === 'renewal' ? (
                              <div className="client-card-renewal">
                                <span>
                                  {t('pages.clients.expiryTime')}: {expiryLabel(row)}
                                </span>
                                <DaysLeftText
                                  days={daysLeftOf(row)}
                                  delayed={(row.expiryTime ?? 0) < 0}
                                />
                                <span>
                                  {t('pages.clients.nextReset')}:{' '}
                                  {row.nextReset
                                    ? IntlUtil.formatDate(row.nextReset, datepicker)
                                    : '—'}
                                </span>
                              </div>
                            ) : (
                              <PlanUsageCell
                                email={row.email}
                                planName={row.planId ? planNames.get(row.planId) : undefined}
                                used={usedBytes(row)}
                                total={row.totalGB ?? 0}
                                onPlanClick={onPlanClick}
                              />
                            )}
                          </div>
                        ))}
                      </div>
                    </Spin>
                  )}
                </Card>
              )}
            </Spin>
          </Layout.Content>
        </Layout>

        <LazyMount when={formOpen}>
          <ClientFormModal
            open={formOpen}
            mode={formMode}
            client={editingClient}
            attachedIds={editingAttachedIds}
            attachedExternalLinks={editingExternalLinks}
            tunnelAllowedIPs={editingTunnelAllowedIPs}
            inbounds={inbounds}
            tgBotEnable={tgBotEnable}
            save={onSave}
            resetTraffic={resetTraffic}
            onOpenChange={setFormOpen}
          />
        </LazyMount>
        <LazyMount when={infoOpen}>
          <ClientInfoModal
            open={infoOpen}
            client={infoClient}
            inboundsById={inboundsById}
            tunnelAllowedIPs={viewingTunnelAllowedIPs}
            isOnline={infoClient ? isOnline(infoClient.email) : false}
            subSettings={subSettings}
            onOpenChange={setInfoOpen}
          />
        </LazyMount>
        <LazyMount when={qrOpen}>
          <ClientQrModal
            open={qrOpen}
            client={qrClient}
            inboundsById={inboundsById}
            tunnelAllowedIPs={viewingTunnelAllowedIPs}
            subSettings={subSettings}
            onOpenChange={setQrOpen}
          />
        </LazyMount>
        <LazyMount when={bulkAddOpen}>
          <ClientBulkAddModal
            open={bulkAddOpen}
            inbounds={inbounds}
            onOpenChange={setBulkAddOpen}
            onSaved={() => setBulkAddOpen(false)}
          />
        </LazyMount>
        <LazyMount when={bulkAdjustOpen}>
          <ClientBulkAdjustModal
            open={bulkAdjustOpen}
            count={selectedRowKeys.length}
            onOpenChange={setBulkAdjustOpen}
            onSubmit={async (addDays, addBytes, flow, limitHwid, adTag) => {
              const msg = await bulkAdjust(
                [...selectedRowKeys],
                addDays,
                addBytes,
                flow,
                limitHwid,
                adTag,
              );
              if (msg?.success) {
                setSelectedRowKeys([]);
                return msg.obj ?? { adjusted: 0 };
              }
              return null;
            }}
          />
        </LazyMount>
        <LazyMount when={portalEmail !== null}>
          <ClientPortalModal
            email={portalEmail}
            portalUrl={
              subSettings.enable && subSettings.subURI ? `${subSettings.subURI}portal` : ''
            }
            onClose={() => setPortalEmail(null)}
          />
        </LazyMount>
        <LazyMount when={subLinksOpen}>
          <SubLinksModal
            open={subLinksOpen}
            emails={selectedRowKeys}
            clients={clients}
            subSettings={subSettings}
            onOpenChange={setSubLinksOpen}
          />
        </LazyMount>
        <LazyMount when={bulkAttachOpen}>
          <BulkAttachInboundsModal
            open={bulkAttachOpen}
            count={selectedRowKeys.length}
            inbounds={inbounds}
            onOpenChange={setBulkAttachOpen}
            onSubmit={async (inboundIds) => {
              const msg = await bulkAttach([...selectedRowKeys], inboundIds);
              if (msg?.success) {
                setSelectedRowKeys([]);
                return msg.obj ?? { attached: [], skipped: [], errors: [] };
              }
              return null;
            }}
          />
        </LazyMount>
        <LazyMount when={bulkDetachOpen}>
          <BulkDetachInboundsModal
            open={bulkDetachOpen}
            count={selectedRowKeys.length}
            inbounds={inbounds}
            onOpenChange={setBulkDetachOpen}
            onSubmit={async (inboundIds) => {
              const msg = await bulkDetach([...selectedRowKeys], inboundIds);
              if (msg?.success) {
                setSelectedRowKeys([]);
                return msg.obj ?? { detached: [], skipped: [], errors: [] };
              }
              return null;
            }}
          />
        </LazyMount>
        <LazyMount when={filterDrawerOpen}>
          <FilterDrawer
            open={filterDrawerOpen}
            onOpenChange={setFilterDrawerOpen}
            filters={filters}
            onChange={setFilters}
            inbounds={inbounds}
            protocols={protocolOptions}
            nodes={nodes}
            plans={plans}
          />
        </LazyMount>
        <AssignPlanModal
          open={planTarget !== null}
          plans={plans}
          emails={planTarget?.emails ?? []}
          planId={planTarget?.planId}
          onClose={() => setPlanTarget(null)}
          onAssigned={() => setSelectedRowKeys([])}
        />
        <ClientRenewModal
          open={renewTarget !== null}
          emails={renewTarget?.emails ?? []}
          onClose={() => setRenewTarget(null)}
          onRenewed={() => {
            if (renewTarget?.bulk) setSelectedRowKeys([]);
          }}
        />
        <LazyMount when={textOpen}>
          <TextModal
            open={textOpen}
            onClose={() => setTextOpen(false)}
            title={textTitle}
            content={textContent}
            fileName={textFileName}
            json
          />
        </LazyMount>
        <LazyMount when={promptOpen}>
          <PromptModal
            open={promptOpen}
            onClose={() => setPromptOpen(false)}
            title={promptTitle}
            okText={promptOkText}
            initialValue={promptInitial}
            loading={promptLoading}
            json
            onConfirm={onPromptConfirm}
          />
        </LazyMount>
      </Layout>
    </ConfigProvider>
  );
}

function bucketChipLabel(b: string, t: (k: string) => string): string {
  switch (b) {
    case 'active':
      return t('subscription.active');
    case 'expiring':
      return t('depletingSoon');
    case 'depleted':
      return t('depleted');
    case 'deactive':
      return t('disabled');
    case 'online':
      return t('online');
    default:
      return b;
  }
}
