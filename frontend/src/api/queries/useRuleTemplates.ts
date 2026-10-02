import { useMemo } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';

import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { keys } from '@/api/queryKeys';
import {
  RuleTemplateSchema,
  RuleTemplateSummarySchema,
  RuleTemplateVersionViewSchema,
  type RuleTemplate,
  type RuleTemplateInput,
  type RuleTemplateSummary,
  type RuleTemplateVersionView,
} from '@/generated/zod';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;
const BASE = '/panel/api/ruleTemplates';

const SummaryListSchema = z
  .array(RuleTemplateSummarySchema)
  .nullable()
  .transform((value) => value ?? []);
const VersionListSchema = z
  .array(RuleTemplateVersionViewSchema)
  .nullable()
  .transform((value) => value ?? []);

async function fetchRuleTemplates(): Promise<RuleTemplateSummary[]> {
  const msg = await HttpUtil.get(`${BASE}/list`, undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch rule templates');
  const list = parseMsg(msg, SummaryListSchema, 'ruleTemplates/list').obj;
  return Array.isArray(list) ? list : [];
}

export function useRuleTemplatesQuery() {
  const query = useQuery({ queryKey: keys.ruleTemplates.list(), queryFn: fetchRuleTemplates });
  const templates = useMemo(() => query.data ?? [], [query.data]);
  return {
    templates,
    loading: query.isFetching,
    fetched: query.data !== undefined || query.isError,
    fetchError: query.error ? (query.error as Error).message : '',
    refetch: query.refetch,
  };
}

/** One template with its content, for the editor and previews. */
export async function fetchRuleTemplate(id: number): Promise<RuleTemplate | null> {
  const msg = await HttpUtil.get(`${BASE}/get/${id}`);
  return msg?.success ? (parseMsg(msg, RuleTemplateSchema, 'ruleTemplates/get').obj ?? null) : null;
}

export function useRuleTemplate(id: number | null) {
  return useQuery({
    queryKey: keys.ruleTemplates.get(id ?? 0),
    queryFn: () => fetchRuleTemplate(id ?? 0),
    enabled: id !== null,
    staleTime: 0,
    // A refetch while the editor is open would look like lost edits.
    refetchOnWindowFocus: false,
  });
}

async function fetchRuleTemplateVersions(id: number): Promise<RuleTemplateVersionView[]> {
  const msg = await HttpUtil.get(`${BASE}/versions/${id}`, undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch template versions');
  const list = parseMsg(msg, VersionListSchema, 'ruleTemplates/versions').obj;
  return Array.isArray(list) ? list : [];
}

export function useRuleTemplateVersions(id: number | null) {
  return useQuery({
    queryKey: keys.ruleTemplates.versions(id ?? 0),
    queryFn: () => fetchRuleTemplateVersions(id ?? 0),
    enabled: id !== null,
    staleTime: 0,
  });
}

/** A preview the page asked for; requestId tells two previews on one plan apart. */
export interface RuleTemplatePreviewQuery {
  requestId: number;
  content: string;
}

async function fetchRuleTemplatePreview(planId: number, content: string): Promise<string> {
  const msg = await HttpUtil.post<string>(
    `${BASE}/preview`,
    { planId, content },
    { ...JSON_HEADERS, silent: true },
  );
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to preview the template');
  return typeof msg.obj === 'string' ? msg.obj : '';
}

/** The Clash config the plan's first member would get with this content, or why not. */
export function useRuleTemplatePreview(
  request: RuleTemplatePreviewQuery | null,
  planId: number | null,
) {
  return useQuery({
    queryKey: keys.ruleTemplates.preview(request?.requestId ?? 0, planId ?? 0),
    queryFn: () => fetchRuleTemplatePreview(planId ?? 0, request?.content ?? ''),
    enabled: request !== null && planId !== null,
    retry: false,
    staleTime: Infinity,
  });
}

export function useRuleTemplateMutations() {
  const queryClient = useQueryClient();
  // Plan cards show their template's name, so plans refresh with the templates.
  const invalidate = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: keys.ruleTemplates.root() }),
      queryClient.invalidateQueries({ queryKey: keys.plans.root() }),
    ]);
  const createMut = useMutation({
    mutationFn: (input: RuleTemplateInput) =>
      HttpUtil.post<RuleTemplate>(`${BASE}/add`, input, JSON_HEADERS),
    onSuccess: invalidate,
  });
  const updateMut = useMutation({
    mutationFn: ({ id, input }: { id: number; input: RuleTemplateInput }) =>
      HttpUtil.post<RuleTemplate>(`${BASE}/update/${id}`, input, JSON_HEADERS),
    onSuccess: invalidate,
  });
  const removeMut = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`${BASE}/del/${id}`),
    onSuccess: invalidate,
  });
  const setDefaultMut = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`${BASE}/setDefault/${id}`),
    onSuccess: invalidate,
  });
  const restoreMut = useMutation({
    mutationFn: (versionId: number) => HttpUtil.post<RuleTemplate>(`${BASE}/restore/${versionId}`),
    onSuccess: invalidate,
  });
  return {
    create: createMut.mutateAsync,
    update: (id: number, input: RuleTemplateInput) => updateMut.mutateAsync({ id, input }),
    remove: removeMut.mutateAsync,
    setDefault: setDefaultMut.mutateAsync,
    restore: restoreMut.mutateAsync,
  };
}
