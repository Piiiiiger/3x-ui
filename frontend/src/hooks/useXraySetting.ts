import { useCallback, useMemo, useRef, useState, useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';

import { HttpUtil, Msg } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { keys } from '@/api/queryKeys';
import { XrayConfigPayloadSchema, XraySettingsValueSchema } from '@/schemas/xray';

export type XraySettingsValue = z.infer<typeof XraySettingsValueSchema>;

export type SetTemplate = (
  next: XraySettingsValue | null | ((prev: XraySettingsValue | null) => XraySettingsValue | null),
) => void;

export interface UseXraySettingResult {
  fetched: boolean;
  spinning: boolean;
  saveDisabled: boolean;
  fetchError: string;
  xraySetting: string;
  setXraySetting: (next: string) => void;
  templateSettings: XraySettingsValue | null;
  setTemplateSettings: SetTemplate;
  fetchAll: () => Promise<void>;
  saveAll: () => Promise<void>;
  resetToDefault: () => Promise<void>;
}

type XrayConfigPayload = z.infer<typeof XrayConfigPayloadSchema>;

export async function fetchXrayConfig(): Promise<XrayConfigPayload> {
  const msg = await HttpUtil.post('/panel/api/xray/', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load xray config');
  if (typeof msg.obj !== 'string')
    throw new Error('Malformed xray config response: expected string');
  let parsed: unknown;
  try {
    parsed = JSON.parse(msg.obj);
  } catch (e) {
    const err = e as Error;
    throw new Error(`Malformed xray config response: ${err.message}`, { cause: e });
  }
  const result = XrayConfigPayloadSchema.safeParse(parsed);
  if (!result.success) {
    console.warn('[zod] xray/ config payload failed validation', result.error.issues);
    return parsed as XrayConfigPayload;
  }
  return result.data;
}

export function useXraySetting(): UseXraySettingResult {
  const queryClient = useQueryClient();

  const configQuery = useQuery({
    queryKey: keys.xray.config(),
    queryFn: fetchXrayConfig,
    staleTime: Infinity,
  });

  const [xraySetting, setXraySettingState] = useState('');
  const [templateSettings, setTemplateSettingsState] = useState<XraySettingsValue | null>(null);
  const [savedXraySetting, setSavedXraySetting] = useState('');
  const config = configQuery.data;

  const syncingRef = useRef(false);
  const xraySettingRef = useRef('');

  const [syncedConfig, setSyncedConfig] = useState<XrayConfigPayload | undefined>();

  useEffect(() => {
    xraySettingRef.current = xraySetting;
  });

  // Adopt a fetched config during render, so the editor never paints one frame
  // of the previous config after a refetch. Local edits win over the refetch.
  if (config && config !== syncedConfig) {
    setSyncedConfig(config);
    if (savedXraySetting === xraySetting) {
      const pretty = JSON.stringify(config.xraySetting, null, 2);
      setXraySettingState(pretty);
      setTemplateSettingsState(config.xraySetting);
      setSavedXraySetting(pretty);
    }
  }

  const fetched = configQuery.data !== undefined || configQuery.isError;
  const fetchError = configQuery.error ? (configQuery.error as Error).message : '';

  const setXraySetting = useCallback((next: string) => {
    setXraySettingState(next);
    if (syncingRef.current) return;
    try {
      const parsed = JSON.parse(next);
      syncingRef.current = true;
      setTemplateSettingsState(parsed);
      syncingRef.current = false;
    } catch {
      /* ignore — wait for user to finish */
    }
  }, []);

  const setTemplateSettings: SetTemplate = useCallback((nextOrFn) => {
    setTemplateSettingsState((prev) => {
      const next = typeof nextOrFn === 'function' ? nextOrFn(prev) : nextOrFn;
      if (next == null) return next;
      if (!syncingRef.current) {
        try {
          syncingRef.current = true;
          setXraySettingState(JSON.stringify(next, null, 2));
        } finally {
          syncingRef.current = false;
        }
      }
      return next;
    });
  }, []);

  const fetchAll = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: keys.xray.config() });
  }, [queryClient]);

  const saveMut = useMutation({
    mutationFn: async () => {
      const sentXraySetting = xraySettingRef.current;
      const msg = await HttpUtil.post('/panel/api/xray/update', { xraySetting: sentXraySetting });
      return { msg, sentXraySetting };
    },
    onSuccess: ({ msg, sentXraySetting }) => {
      if (!msg?.success) return;
      setSavedXraySetting(sentXraySetting);
      queryClient.invalidateQueries({ queryKey: keys.xray.config() });
    },
  });

  const resetDefaultMut = useMutation({
    mutationFn: async (): Promise<Msg<XraySettingsValue>> => {
      const raw = await HttpUtil.get('/panel/api/setting/getDefaultJsonConfig');
      return parseMsg(raw, XraySettingsValueSchema, 'setting/getDefaultJsonConfig');
    },
    onSuccess: (msg) => {
      if (msg?.success && msg.obj) {
        const cloned = JSON.parse(JSON.stringify(msg.obj));
        setTemplateSettings(cloned);
      }
    },
  });

  const saveAll = useCallback(async () => {
    await saveMut.mutateAsync();
  }, [saveMut]);
  const resetToDefault = useCallback(async () => {
    await resetDefaultMut.mutateAsync();
  }, [resetDefaultMut]);

  const spinning = saveMut.isPending || resetDefaultMut.isPending;
  const saveDisabled = savedXraySetting === xraySetting;

  return useMemo(
    () => ({
      fetched,
      spinning,
      saveDisabled,
      fetchError,
      xraySetting,
      setXraySetting,
      templateSettings,
      setTemplateSettings,
      fetchAll,
      saveAll,
      resetToDefault,
    }),
    [
      fetched,
      spinning,
      saveDisabled,
      fetchError,
      xraySetting,
      setXraySetting,
      templateSettings,
      setTemplateSettings,
      fetchAll,
      saveAll,
      resetToDefault,
    ],
  );
}
