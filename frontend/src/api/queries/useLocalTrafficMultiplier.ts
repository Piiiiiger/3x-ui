import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { parseRequired } from '@/utils/zodValidate';
import { keys } from '@/api/queryKeys';
import { TrafficMultiplierViewSchema } from '@/generated/zod';

// The handler binds JSON only; the default form encoding is refused.
const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

async function fetchLocalTrafficMultiplier(): Promise<number> {
  const msg = await HttpUtil.get('/panel/api/server/trafficMultiplier', undefined, {
    silent: true,
  });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch the traffic multiplier');
  return parseRequired(msg, TrafficMultiplierViewSchema, 'server/trafficMultiplier').multiplier;
}

/** The traffic multiplier of this panel's own host; agent hosts carry theirs on the node. */
export function useLocalTrafficMultiplier() {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: keys.server.trafficMultiplier(),
    queryFn: fetchLocalTrafficMultiplier,
  });
  const saveMut = useMutation({
    mutationFn: (multiplier: number) =>
      HttpUtil.post('/panel/api/server/trafficMultiplier', { multiplier }, JSON_HEADERS),
    onSuccess: (msg) => {
      if (msg?.success) {
        queryClient.invalidateQueries({ queryKey: keys.server.trafficMultiplier() });
      }
    },
  });
  return { multiplier: query.data, save: saveMut.mutateAsync };
}
