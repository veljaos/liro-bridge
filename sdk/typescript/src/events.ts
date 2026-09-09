/**
 * Reading the job event stream — `PROTOCOL.md` §6.2.
 *
 * Server-sent events, one per state change, until the job ends and the
 * stream closes. This is a small hand-written parser rather than
 * `EventSource`, for two reasons: `EventSource` cannot set headers, and
 * this endpoint needs the four from §3 — and `EventSource` exists in the
 * browser, which is the one place a device secret must never be.
 */

import { LiroError } from './errors.js';

/** One `data:` payload from the stream, already JSON-parsed. */
export interface RawEvent {
  readonly state?: string;
  readonly completed?: number;
  readonly total?: number;
  readonly failed?: number;
  readonly etaMs?: number;
  readonly consentRemainingMs?: number;
  readonly code?: string;
}

/**
 * Yields each event's parsed payload as it arrives.
 *
 * Only the `data:` field is read. The agent sends no event names, no
 * ids and no retry directives, and a parser that invented meanings for
 * fields the other side does not send would be describing a protocol
 * nobody implements.
 */
export async function* readEventStream(response: Response): AsyncGenerator<RawEvent> {
  const body = response.body;
  if (body === null) {
    throw new LiroError('PROTOCOL_VIOLATION', { detail: 'the event stream had no body' });
  }
  const decoder = new TextDecoder('utf-8');
  let buffer = '';

  const reader = body.getReader();
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (value !== undefined) {
        buffer += decoder.decode(value, { stream: true });
      }
      if (done) {
        buffer += decoder.decode();
      }

      // An event ends at a blank line. Both spellings of a line ending
      // are accepted: the agent writes "\n\n", and nothing should break
      // if something between it and here ever normalises.
      for (;;) {
        const boundary = findBoundary(buffer);
        if (boundary === null) {
          break;
        }
        const block = buffer.slice(0, boundary.index);
        buffer = buffer.slice(boundary.index + boundary.length);
        const event = parseBlock(block);
        if (event !== null) {
          yield event;
        }
      }

      if (done) {
        const tail = parseBlock(buffer);
        if (tail !== null) {
          yield tail;
        }
        return;
      }
    }
  } finally {
    // Releasing the lock lets the caller cancel the underlying response
    // without the reader holding it open.
    reader.releaseLock();
  }
}

/** The first blank line in `buffer`, or `null`. */
function findBoundary(buffer: string): { index: number; length: number } | null {
  const lf = buffer.indexOf('\n\n');
  const crlf = buffer.indexOf('\r\n\r\n');
  if (lf === -1 && crlf === -1) {
    return null;
  }
  if (crlf !== -1 && (lf === -1 || crlf < lf)) {
    return { index: crlf, length: 4 };
  }
  return { index: lf, length: 2 };
}

/** One event block's `data:` lines, JSON-parsed, or `null` when there are none. */
function parseBlock(block: string): RawEvent | null {
  const data: string[] = [];
  for (const rawLine of block.split(/\r?\n/)) {
    const line = rawLine.startsWith('﻿') ? rawLine.slice(1) : rawLine;
    if (line === '' || line.startsWith(':')) {
      continue;
    }
    const colon = line.indexOf(':');
    const field = colon === -1 ? line : line.slice(0, colon);
    if (field !== 'data') {
      continue;
    }
    const rest = colon === -1 ? '' : line.slice(colon + 1);
    data.push(rest.startsWith(' ') ? rest.slice(1) : rest);
  }
  if (data.length === 0) {
    return null;
  }
  const payload = data.join('\n');
  try {
    const parsed: unknown = JSON.parse(payload);
    if (typeof parsed !== 'object' || parsed === null) {
      return null;
    }
    return parsed as RawEvent;
  } catch {
    // A payload that is not JSON is not this SDK's to interpret, and it
    // is not worth failing a batch over: the result endpoint is the
    // authority on how the job ended, and it is asked either way.
    return null;
  }
}
