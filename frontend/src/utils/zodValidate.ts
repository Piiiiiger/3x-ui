import type { z } from 'zod';
import { Msg } from '@/utils';

interface ParseMsgOptions {
  strict?: boolean;
}

export function parseMsg<T extends z.ZodType>(
  msg: Msg<unknown>,
  schema: T,
  context: string,
  options: ParseMsgOptions = {},
): Msg<z.infer<T>> {
  if (!msg.success || msg.obj == null) {
    return msg as Msg<z.infer<T>>;
  }
  const result = schema.safeParse(msg.obj);
  if (!result.success) {
    console.warn(`[zod] ${context} response failed validation`, result.error.issues);
    if (options.strict) throw new Error(`${context} response failed validation`);
    return msg as Msg<z.infer<T>>;
  }
  return new Msg<z.infer<T>>(msg.success, msg.msg, result.data);
}

// For a response whose payload a form or a page is built from: the validated
// payload itself, or an error when it is missing or malformed.
export function parseRequired<T extends z.ZodType>(
  msg: Msg<unknown>,
  schema: T,
  context: string,
): NonNullable<z.infer<T>> {
  const obj = parseMsg(msg, schema, context, { strict: true }).obj;
  if (obj == null) throw new Error(`${context} response is empty`);
  return obj;
}
