import { useMutation, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { keys } from '@/api/queryKeys';
import { markLocalInvalidate } from '@/api/invalidationTracker';
import type { PlanFormValues } from '@/schemas/plan';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

export interface PlanAssignRequest {
  emails: string[];
  planId: number;
}

export function usePlanMutations() {
  const queryClient = useQueryClient();
  const invalidatePlans = () => queryClient.invalidateQueries({ queryKey: keys.plans.root() });
  // Plan changes rewrite clients and their inbound entries, so those views refresh too.
  const invalidateMembers = () => {
    markLocalInvalidate();
    return Promise.all([
      invalidatePlans(),
      queryClient.invalidateQueries({ queryKey: keys.clients.root() }),
      queryClient.invalidateQueries({ queryKey: keys.inbounds.root() }),
      queryClient.invalidateQueries({ queryKey: keys.xray.config() }),
      queryClient.invalidateQueries({ queryKey: keys.traffic.root() }),
    ]);
  };

  const createMut = useMutation({
    mutationFn: (values: PlanFormValues) =>
      HttpUtil.post('/panel/api/plans/add', values, JSON_HEADERS),
    onSuccess: (msg) => {
      if (msg?.success) invalidatePlans();
    },
  });

  const updateMut = useMutation({
    mutationFn: (args: { id: number; values: PlanFormValues; reapplyLimits: boolean }) =>
      HttpUtil.post(
        `/panel/api/plans/update/${args.id}`,
        { ...args.values, applyToMembers: args.reapplyLimits },
        JSON_HEADERS,
      ),
    onSuccess: (msg) => {
      if (msg?.success) invalidateMembers();
    },
  });

  const removeMut = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`/panel/api/plans/del/${id}`),
    onSuccess: (msg) => {
      if (msg?.success) invalidatePlans();
    },
  });

  const assignMut = useMutation({
    mutationFn: (req: PlanAssignRequest) =>
      HttpUtil.post('/panel/api/plans/assign', req, JSON_HEADERS),
    onSuccess: (msg) => {
      if (msg?.success) invalidateMembers();
    },
  });

  const unassignMut = useMutation({
    mutationFn: (emails: string[]) =>
      HttpUtil.post('/panel/api/plans/unassign', { emails }, JSON_HEADERS),
    onSuccess: (msg) => {
      if (msg?.success) invalidateMembers();
    },
  });

  return {
    create: (values: PlanFormValues) => createMut.mutateAsync(values),
    update: (id: number, values: PlanFormValues, reapplyLimits: boolean) =>
      updateMut.mutateAsync({ id, values, reapplyLimits }),
    remove: (id: number) => removeMut.mutateAsync(id),
    assign: (req: PlanAssignRequest) => assignMut.mutateAsync(req),
    unassign: (emails: string[]) => unassignMut.mutateAsync(emails),
  };
}
