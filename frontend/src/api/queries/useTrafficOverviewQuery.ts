import { useQuery } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { keys } from '@/api/queryKeys';
import { TrafficOverviewSchema, type TrafficOverview } from '@/generated/zod';

const REFRESH_MS = 60_000;

async function fetchTrafficOverview(): Promise<TrafficOverview | null> {
  const msg = await HttpUtil.get('/panel/api/traffic/overview', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch the traffic overview');
  return parseMsg(msg, TrafficOverviewSchema, 'traffic/overview').obj ?? null;
}

export function useTrafficOverviewQuery() {
  const query = useQuery({
    queryKey: keys.traffic.overview(),
    queryFn: fetchTrafficOverview,
    refetchInterval: REFRESH_MS,
  });
  return {
    overview: query.data ?? null,
    fetched: query.data !== undefined || query.isError,
    fetchError: query.error ? (query.error as Error).message : '',
    refetch: query.refetch,
  };
}
