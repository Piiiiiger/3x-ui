import { HttpUtil, RandomUtil } from '@/utils';
import {
  buildClonePayload,
  cloneShareFor,
  type CloneShare,
  type FreshReality,
} from '@/lib/xray/inbound-clone';
import { coerceInboundJsonField, type DBInbound } from '@/models/dbinbound';

/*
 * New REALITY values for one inbound, made the way the inbound form makes them; a
 * failed key request throws, so no inbound goes out with keys it shares.
 */
export async function fetchFreshReality(withMldsa65: boolean): Promise<FreshReality> {
  const keys = await HttpUtil.get<{ privateKey: string; publicKey: string }>(
    '/panel/api/server/getNewX25519Cert',
    undefined,
    { silent: true },
  );
  if (!keys?.success || !keys.obj?.privateKey) throw new Error(keys?.msg || 'x25519 keys');
  const fresh: FreshReality = {
    privateKey: keys.obj.privateKey,
    publicKey: keys.obj.publicKey,
    shortIds: RandomUtil.randomShortIds()
      .split(',')
      .map((id) => id.trim())
      .filter(Boolean),
    spiderX: `/${RandomUtil.randomSeq(15)}`,
  };
  if (withMldsa65) {
    const pq = await HttpUtil.get<{ seed: string; verify: string }>(
      '/panel/api/server/getNewmldsa65',
      undefined,
      {
        silent: true,
      },
    );
    if (!pq?.success || !pq.obj?.seed) throw new Error(pq?.msg || 'mldsa65 seed');
    fresh.mldsa65 = { seed: pq.obj.seed, verify: pq.obj.verify };
  }
  return fresh;
}

/** Fresh REALITY values for a copy of source, or undefined when it is no REALITY inbound. */
export async function freshRealityFor(source: DBInbound): Promise<FreshReality | undefined> {
  let stream: { security?: string; realitySettings?: { mldsa65Seed?: string } };
  try {
    stream = coerceInboundJsonField(source.streamSettings) as typeof stream;
  } catch {
    return undefined;
  }
  if (stream.security !== 'reality') return undefined;
  return fetchFreshReality(!!stream.realitySettings?.mldsa65Seed);
}

/*
 * Posts one staged copy of source to target (0 is the local panel) with its own
 * REALITY values and its target's address; a copy whose keys failed is not sent.
 */
export async function postClone(
  source: DBInbound,
  target: number,
  port: number,
  sharesByHost: Map<number, CloneShare>,
): Promise<{ ok: boolean; reason: string }> {
  let fresh: FreshReality | undefined;
  try {
    fresh = await freshRealityFor(source);
  } catch (e) {
    return { ok: false, reason: e instanceof Error ? e.message : String(e) };
  }
  const payload = buildClonePayload(
    source,
    port,
    target === 0 ? null : target,
    cloneShareFor(source, target, sharesByHost),
    fresh,
  );
  const msg = await HttpUtil.post('/panel/api/inbounds/add', payload, { silent: true });
  return { ok: !!msg?.success, reason: msg?.success ? '' : msg?.msg || '' };
}
