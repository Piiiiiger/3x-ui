import { useQuery } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { parseRequired } from '@/utils/zodValidate';
import { keys } from '@/api/queryKeys';
import { ProbeOverviewSchema, type ProbeOverview } from '@/generated/zod';

const POLL_INTERVAL_MS = 3000;

async function fetchProbeOverview(): Promise<ProbeOverview> {
  const msg = await HttpUtil.get('/panel/api/probe/servers', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch the probe servers');
  return parseRequired(msg, ProbeOverviewSchema, 'probe/servers');
}

export function useProbeQuery() {
  const query = useQuery({
    queryKey: keys.probe.servers(),
    queryFn: fetchProbeOverview,
    refetchInterval: POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
    staleTime: 0,
  });
  return {
    // The answer of the last poll that worked stays here while a later one fails.
    overview: query.data ?? null,
    loading: query.isFetching,
    fetched: query.data !== undefined || query.isError,
    fetchError: query.error ? (query.error as Error).message : '',
    refetch: query.refetch,
  };
}
