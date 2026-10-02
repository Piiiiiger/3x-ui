import { keepPreviousData, useQuery } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { keys } from '@/api/queryKeys';
import { TrafficOverviewSchema, type TrafficOverview } from '@/generated/zod';

export type TrafficPeriod = TrafficOverview['period'];

const REFRESH_MS = 60_000;

async function fetchTrafficOverview(period: TrafficPeriod): Promise<TrafficOverview | null> {
  const msg = await HttpUtil.get('/panel/api/traffic/overview', { period }, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch the traffic overview');
  return parseMsg(msg, TrafficOverviewSchema, 'traffic/overview').obj ?? null;
}

/** The home page's overview for one period; switching keeps the last answer up meanwhile. */
export function useTrafficOverviewQuery(period: TrafficPeriod) {
  const query = useQuery({
    queryKey: keys.traffic.overview(period),
    queryFn: () => fetchTrafficOverview(period),
    refetchInterval: REFRESH_MS,
    placeholderData: keepPreviousData,
  });
  return {
    overview: query.data ?? null,
    fetched: query.data !== undefined || query.isError,
    fetchError: query.error ? (query.error as Error).message : '',
    refetch: query.refetch,
  };
}
