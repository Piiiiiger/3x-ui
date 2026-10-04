import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal, Select, Typography, message } from 'antd';

import { SelectAllClearButtons } from '@/components/form';
import { pickClonePort, type CloneShare } from '@/lib/xray/inbound-clone';
import type { NodeRecord } from '@/api/queries/useNodesQuery';
import type { DBInbound } from '@/models/dbinbound';

import { postClone } from './realityKeys';

// 0 is the "local panel" sentinel (inbounds without a nodeId) — the same
// convention as the clients page node filter (#4997).
const LOCAL_PANEL = 0;

interface CloneInboundModalProps {
  open: boolean;
  dbInbound: DBInbound | null;
  nodes: NodeRecord[];
  portsInUse: Map<number, Set<number>>;
  sharesByHost: Map<number, CloneShare>;
  onClose: () => void;
  onCloned: () => void | Promise<void>;
}

export default function CloneInboundModal({
  open,
  dbInbound,
  nodes,
  portsInUse,
  sharesByHost,
  onClose,
  onCloned,
}: CloneInboundModalProps) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [targets, setTargets] = useState<number[]>([LOCAL_PANEL]);
  const [submitting, setSubmitting] = useState(false);

  const targetOptions = useMemo(
    () => [
      ...(dbInbound?.protocol === 'snell'
        ? []
        : [{ value: LOCAL_PANEL, label: t('pages.inbounds.localPanel'), disabled: false }]),
      ...(nodes || [])
        .filter((n) => n.enable && (dbInbound?.protocol !== 'snell' || n.kind === 'agent'))
        .map((n) => ({
          value: n.id,
          // Only online nodes are deployable targets: nodes report `unknown`
          // until their first heartbeat, and the backend refuses any status
          // other than online.
          label: `${n.name}${n.status === 'online' ? '' : ` (${n.status || 'offline'})`}`,
          disabled: n.status !== 'online',
        })),
    ],
    [nodes, t, dbInbound?.protocol],
  );

  // "Select all" must not pick targets the user can't pick manually —
  // offline nodes are disabled options in the dropdown.
  const selectableOptions = useMemo(
    () => targetOptions.filter((o) => !o.disabled),
    [targetOptions],
  );

  // Reset the selection when the dialog OPENS: pre-select the source
  // inbound's own node when it is a selectable target, otherwise the local
  // panel (the only destination the clone action had before this picker).
  // Deps are deliberately `[open]` only — `nodes` gets a new identity on every
  // background refetch (heartbeats bump latency/status), and keying the reset
  // on it would clobber the user's selection mid-dialog.
  useEffect(() => {
    if (!open || !dbInbound) return;
    const src = dbInbound.nodeId ?? LOCAL_PANEL;
    const srcNode = (nodes || []).find((n) => n.id === src);
    const selectable = !!srcNode && !!srcNode.enable && srcNode.status === 'online';
    setTargets(selectable ? [src] : dbInbound.protocol === 'snell' ? [] : [LOCAL_PANEL]);
    /* eslint-disable-next-line react-hooks/exhaustive-deps */
  }, [open]);

  async function submit() {
    if (!dbInbound || targets.length === 0) return;
    setSubmitting(true);
    try {
      // Sequential posts keep per-target results in selection order; every
      // target gets its own fresh port because ports are only node-scoped.
      const results: { ok: boolean; reason: string }[] = [];
      for (const target of targets) {
        results.push(
          await postClone(dbInbound, target, pickClonePort(portsInUse.get(target)), sharesByHost),
        );
      }
      const okCount = results.filter((r) => r.ok).length;
      const failed = results.length - okCount;
      if (failed === 0) {
        messageApi.success(
          okCount === 1
            ? t('pages.inbounds.toasts.inboundCreateSuccess')
            : t('pages.inbounds.toasts.clonedMany', { count: okCount }),
        );
      } else {
        const firstError = results.find((r) => !r.ok)?.reason ?? '';
        const base = t('pages.inbounds.toasts.clonedMixed', { ok: okCount, failed });
        messageApi.warning(firstError ? `${base} — ${firstError}` : base);
      }
      if (okCount > 0) await onCloned();
      onClose();
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <>
      {messageContextHolder}
      <Modal
        open={open}
        title={t('pages.inbounds.cloneConfirmTitle', { remark: dbInbound?.remark ?? '' })}
        okText={t('pages.inbounds.clone')}
        cancelText={t('cancel')}
        okButtonProps={{ disabled: targets.length === 0, loading: submitting }}
        onCancel={onClose}
        onOk={submit}
        destroyOnHidden
      >
        <Typography.Paragraph type="secondary">
          {t('pages.inbounds.cloneConfirmContent')}
        </Typography.Paragraph>
        <SelectAllClearButtons options={selectableOptions} value={targets} onChange={setTargets} />
        <Select
          aria-label={t('pages.inbounds.deployTo')}
          mode="multiple"
          style={{ width: '100%' }}
          value={targets}
          onChange={setTargets}
          options={targetOptions}
          placeholder={t('pages.inbounds.deployTo')}
          showSearch={{ optionFilterProp: 'label' }}
          autoFocus
        />
      </Modal>
    </>
  );
}
