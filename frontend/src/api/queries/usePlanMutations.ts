import { useMutation, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { keys } from '@/api/queryKeys';
import { markLocalInvalidate } from '@/api/invalidationTracker';
import type { PlanInput } from '@/generated/zod';
import type { PlanFormValues, PlanStart } from '@/schemas/plan';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;
const GIB = 1024 ** 3;

export function toPlanInput({ quotaGB, ...rest }: PlanFormValues): PlanInput {
  return { ...rest, totalGB: Math.round(quotaGB * GIB) };
}

export interface PlanAssignRequest {
  emails: string[];
  planId: number;
  start: PlanStart;
  resetTraffic: boolean;
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
    ]);
  };

  const createMut = useMutation({
    mutationFn: (values: PlanFormValues) =>
      HttpUtil.post('/panel/api/plans/add', toPlanInput(values), JSON_HEADERS),
    onSuccess: (msg) => {
      if (msg?.success) invalidatePlans();
    },
  });

  const updateMut = useMutation({
    mutationFn: (args: { id: number; values: PlanFormValues; applyToMembers: boolean }) =>
      HttpUtil.post(
        `/panel/api/plans/update/${args.id}`,
        { ...toPlanInput(args.values), applyToMembers: args.applyToMembers },
        JSON_HEADERS,
      ),
    onSuccess: (msg, args) => {
      if (msg?.success) (args.applyToMembers ? invalidateMembers : invalidatePlans)();
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

  const renewMut = useMutation({
    mutationFn: (emails: string[]) =>
      HttpUtil.post('/panel/api/plans/renew', { emails }, JSON_HEADERS),
    onSuccess: (msg) => {
      if (msg?.success) invalidateMembers();
    },
  });

  return {
    create: (values: PlanFormValues) => createMut.mutateAsync(values),
    update: (id: number, values: PlanFormValues, applyToMembers: boolean) =>
      updateMut.mutateAsync({ id, values, applyToMembers }),
    remove: (id: number) => removeMut.mutateAsync(id),
    assign: (req: PlanAssignRequest) => assignMut.mutateAsync(req),
    unassign: (emails: string[]) => unassignMut.mutateAsync(emails),
    renew: (emails: string[]) => renewMut.mutateAsync(emails),
  };
}
