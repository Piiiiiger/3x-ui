import type { TFunction } from 'i18next';

// Node id 0 is the panel's own host, which has no name of its own.
export function probeHostLabel(host: { nodeId: number; nodeName: string }, t: TFunction): string {
  return host.nodeId === 0 ? t('pages.probe.master') : host.nodeName;
}
