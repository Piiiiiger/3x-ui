import type { ReactNode } from 'react';
import { act, renderHook } from '@testing-library/react';
import { QueryClientProvider } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { usePlanMutations } from '@/api/queries/usePlanMutations';
import { keys } from '@/api/queryKeys';
import type { PlanFormValues } from '@/schemas/plan';
import { makeTestQueryClient } from '@/test/test-utils';
import { HttpUtil, Msg } from '@/utils';

const values: PlanFormValues = {
  name: 'Monthly',
  limitIp: 2,
  remark: '',
  templateId: 0,
  inboundIds: [1, 2],
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe('usePlanMutations update', () => {
  // Every save moves the plan's clients onto or off servers, so their views go stale either way.
  it.each([false, true])(
    'posts applyToMembers=%s and refreshes the clients and inbounds',
    async (reapplyLimits) => {
      const post = vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', { id: 7 }));
      const queryClient = makeTestQueryClient();
      queryClient.setQueryData(keys.clients.all(), []);
      queryClient.setQueryData(keys.inbounds.slim(), []);
      const wrapper = ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      );
      const { result } = renderHook(() => usePlanMutations(), { wrapper });

      await act(async () => {
        await result.current.update(7, values, reapplyLimits);
      });

      // The handler binds applyToMembers; a body under any other name re-applies nothing.
      expect(post).toHaveBeenCalledWith(
        '/panel/api/plans/update/7',
        expect.objectContaining({ inboundIds: [1, 2], applyToMembers: reapplyLimits }),
        { headers: { 'Content-Type': 'application/json' } },
      );
      expect(queryClient.getQueryState(keys.clients.all())?.isInvalidated).toBe(true);
      expect(queryClient.getQueryState(keys.inbounds.slim())?.isInvalidated).toBe(true);
    },
  );
});
