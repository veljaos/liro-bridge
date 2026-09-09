/**
 * One request to the agent: built, signed, sent, and turned back into
 * either a body or a typed error.
 *
 * Everything about authentication lives here — `PROTOCOL.md` §3 — so
 * that no other file in this SDK has to know what an HMAC is, which is
 * the whole point of the SDK existing.
 */

import {
  HEADER_APP_ID,
  HEADER_NONCE,
  HEADER_SIGNATURE,
  HEADER_TIMESTAMP,
  canonicalString,
  newNonce,
  signCanonical,
  unixSeconds,
} from './canonical.js';
import { LiroError } from './errors.js';
import type { StoredPairing } from './secrets.js';

const EMPTY_BODY = new Uint8Array(0);

/**
 * How many times a request that is safe to repeat is attempted, and how
 * long between attempts.
 *
 * Short, because this is a loopback socket: a request that is going to
 * connect connects in under a millisecond, and the only things worth
 * waiting out are a listener a moment mid-restart or a transient 5xx.
 */
const MAX_ATTEMPTS = 3;
const BACKOFF_MS = [200, 600];

/** What one request needs to be sent. */
export interface RequestSpec {
  readonly method: 'GET' | 'POST';
  /** The path, exactly as it goes in the request line. No query strings anywhere in this protocol. */
  readonly path: string;
  /** The body, or `undefined` for a request that has none. */
  readonly body?: Uint8Array;
  /**
   * Whether this request may be sent again after a failure that left no
   * response.
   *
   * The default is **false**, and that is the important half. A request
   * this SDK repeats is one that has been reasoned about; everything
   * else is sent once. See {@link Transport.send} for what "safe" means
   * here and why so little of this protocol qualifies.
   */
  readonly retryable?: boolean;
  /** Cancels the request. */
  readonly signal?: AbortSignal;
  /** How long this one request may take. */
  readonly timeoutMs?: number;
  /** Set for the two pairing calls, which have no pairing to sign with yet. */
  readonly unauthenticated?: boolean;
}

/** A response that arrived and was understood. */
export interface RawResponse {
  readonly status: number;
  readonly headers: Headers;
  readonly body: Uint8Array;
}

/** Sends signed requests to one agent. */
export class Transport {
  private readonly baseUrl: string;
  private readonly defaultTimeoutMs: number;
  private pairing: StoredPairing | null;

  constructor(baseUrl: string, defaultTimeoutMs: number, pairing: StoredPairing | null = null) {
    this.baseUrl = baseUrl;
    this.defaultTimeoutMs = defaultTimeoutMs;
    this.pairing = pairing;
  }

  /** The pairing requests are signed with, once there is one. */
  setPairing(pairing: StoredPairing | null): void {
    this.pairing = pairing;
  }

  /** The pairing currently in use, or `null`. */
  currentPairing(): StoredPairing | null {
    return this.pairing;
  }

  /**
   * Sends one request and returns its body, or throws a
   * {@link LiroError}.
   *
   * **What is retried, and what is not.** Only a failure that left no
   * response at all, or a 5xx, and only when the caller marked the
   * request `retryable`. Never a 401 — a retry of a request that did not
   * authenticate does not authenticate either, and the second failure
   * costs a nonce and tells nobody anything new. Never a submission: a
   * `POST /v2/sign` that timed out may already have created a job, and
   * sending it again is a hundred documents signed twice. Never
   * `POST /v2/pair/confirm`, which spends one of five code attempts.
   * Never `GET /v2/jobs/{id}/result`, which is delivered exactly once.
   *
   * **Every attempt gets a new nonce and a new timestamp.** Reusing
   * either on a retry produces `AUTH_FAILED`, and every authentication
   * failure looks identical from the outside, deliberately — so the
   * retry that was meant to recover from a hiccup instead reports the
   * least diagnosable answer the protocol has.
   */
  async send(spec: RequestSpec): Promise<RawResponse> {
    const attempts = spec.retryable === true ? MAX_ATTEMPTS : 1;
    let lastError: LiroError | undefined;

    for (let attempt = 0; attempt < attempts; attempt++) {
      if (attempt > 0) {
        await delay(BACKOFF_MS[attempt - 1] ?? 600, spec.signal);
      }
      let response: RawResponse;
      try {
        response = await this.sendOnce(spec);
      } catch (err) {
        const wrapped = err instanceof LiroError ? err : unexpected(err);
        if (wrapped.code === 'CANCELLED' || wrapped.code === 'CONFIGURATION_INVALID') {
          throw wrapped;
        }
        lastError = wrapped;
        continue;
      }
      if (response.status >= 500 && attempt < attempts - 1) {
        lastError = errorFromResponse(response, spec);
        continue;
      }
      if (response.status >= 400) {
        throw errorFromResponse(response, spec);
      }
      return response;
    }
    throw lastError ?? new LiroError('NETWORK', { detail: `${spec.method} ${spec.path} did not complete` });
  }

  /** Sends one request, without any retrying. */
  async sendOnce(spec: RequestSpec): Promise<RawResponse> {
    const request = this.buildRequest(spec);
    const timeoutMs = spec.timeoutMs ?? this.defaultTimeoutMs;
    const controller = new AbortController();
    const onAbort = (): void => controller.abort(spec.signal?.reason);
    spec.signal?.addEventListener('abort', onAbort, { once: true });
    const timer = timeoutMs > 0 ? setTimeout(() => controller.abort(new TimeoutMarker()), timeoutMs) : undefined;

    try {
      const response = await fetch(`${this.baseUrl}${spec.path}`, {
        method: spec.method,
        headers: request.headers,
        ...(request.body === undefined ? {} : { body: request.body }),
        signal: controller.signal,
        // Loopback, one agent, no redirects anywhere in this protocol.
        redirect: 'error',
      });
      const body = new Uint8Array(await response.arrayBuffer());
      return { status: response.status, headers: response.headers, body };
    } catch (err) {
      throw this.networkError(err, spec);
    } finally {
      if (timer !== undefined) {
        clearTimeout(timer);
      }
      spec.signal?.removeEventListener('abort', onAbort);
    }
  }

  /**
   * Opens a streaming response — the event stream — and hands back the
   * raw body reader. Not retried here; reconnecting is the caller's
   * decision, because it is the caller that knows whether the job is
   * still worth following.
   */
  async open(spec: RequestSpec): Promise<Response> {
    const request = this.buildRequest(spec);
    try {
      const response = await fetch(`${this.baseUrl}${spec.path}`, {
        method: spec.method,
        headers: request.headers,
        ...(request.body === undefined ? {} : { body: request.body }),
        ...(spec.signal === undefined ? {} : { signal: spec.signal }),
        redirect: 'error',
      });
      if (response.status >= 400) {
        const body = new Uint8Array(await response.arrayBuffer());
        throw errorFromResponse({ status: response.status, headers: response.headers, body }, spec);
      }
      return response;
    } catch (err) {
      if (err instanceof LiroError) {
        throw err;
      }
      throw this.networkError(err, spec);
    }
  }

  /**
   * Builds one request's headers and body.
   *
   * The rule that costs an afternoon otherwise: **a request with no body
   * does not send `Content-Type`.** `PROTOCOL.md` §2.5 says so and the
   * agent does not ask for one, because several HTTP clients cannot
   * attach a content type to a request with no content at all — .NET's
   * `HttpClient` puts the header on the content, so a `GET` has nowhere
   * to put it. The agent required one on every endpoint once, and every
   * request from a real client was refused.
   */
  private buildRequest(spec: RequestSpec): { headers: Record<string, string>; body: Uint8Array | undefined } {
    const body = spec.body;
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
    }

    if (spec.unauthenticated !== true) {
      const pairing = this.pairing;
      if (pairing === null) {
        throw new LiroError('NOT_PAIRED', {
          detail: `${spec.method} ${spec.path} needs a pairing and this client has none`,
        });
      }
      const timestamp = unixSeconds();
      const nonce = newNonce();
      const canonical = canonicalString(spec.method, spec.path, timestamp, nonce, body ?? EMPTY_BODY);
      headers[HEADER_APP_ID] = pairing.appId;
      headers[HEADER_TIMESTAMP] = timestamp;
      headers[HEADER_NONCE] = nonce;
      headers[HEADER_SIGNATURE] = signCanonical(pairing.deviceSecretBytes(), canonical);
    }
    return { headers, body };
  }

  /** Turns whatever `fetch` threw into a typed error. */
  private networkError(err: unknown, spec: RequestSpec): LiroError {
    if (err instanceof LiroError) {
      return err;
    }
    const reason = (err as { cause?: unknown })?.cause ?? err;
    if (reason instanceof TimeoutMarker || (err as { name?: string })?.name === 'TimeoutError') {
      return new LiroError('TIMEOUT', { detail: `${spec.method} ${spec.path} did not answer in time`, cause: err });
    }
    if (spec.signal?.aborted === true) {
      return new LiroError('CANCELLED', { detail: `${spec.method} ${spec.path} was cancelled`, cause: err });
    }
    if ((err as { name?: string })?.name === 'AbortError') {
      return new LiroError('TIMEOUT', { detail: `${spec.method} ${spec.path} did not answer in time`, cause: err });
    }
    const code = (reason as { code?: string })?.code;
    if (code === 'ECONNREFUSED' || code === 'ECONNRESET' || code === 'ENOTFOUND') {
      return new LiroError('AGENT_NOT_RUNNING', {
        detail:
          `nothing is listening at ${this.baseUrl}. The discovery file may be one a crash left behind; ` +
          'a refused connection means the same thing as no file at all — the agent is not running',
        cause: err,
      });
    }
    return new LiroError('NETWORK', { detail: `${spec.method} ${spec.path} failed`, cause: err });
  }
}

/** Marks an abort this SDK caused with a timer rather than the caller causing it. */
class TimeoutMarker extends Error {
  constructor() {
    super('timeout');
    this.name = 'TimeoutError';
  }
}

/** Parses a JSON body, or throws `PROTOCOL_VIOLATION`. */
export function parseJSON<T>(response: RawResponse, what: string): T {
  const text = new TextDecoder('utf-8').decode(response.body);
  try {
    return JSON.parse(text) as T;
  } catch (err) {
    throw new LiroError('PROTOCOL_VIOLATION', {
      detail: `${what} did not answer with JSON`,
      httpStatus: response.status,
      cause: err,
    });
  }
}

/** JSON-encodes a body as the bytes that will be hashed and sent. */
export function encodeJSON(value: unknown): Uint8Array {
  return new TextEncoder().encode(JSON.stringify(value));
}

/**
 * Turns an error response into a {@link LiroError}.
 *
 * The body is `{"code": "...", "details": {...}}` and nothing else — no
 * human-readable message, in any language. A body that is not that shape
 * is a `PROTOCOL_VIOLATION`, not something to guess at.
 */
function errorFromResponse(response: RawResponse, spec: RequestSpec): LiroError {
  const text = new TextDecoder('utf-8').decode(response.body).trim();
  let parsed: unknown;
  try {
    parsed = text === '' ? null : JSON.parse(text);
  } catch {
    parsed = null;
  }
  const body = parsed as { code?: unknown; details?: unknown } | null;
  const code = typeof body?.code === 'string' ? body.code : null;
  if (code === null) {
    return new LiroError('PROTOCOL_VIOLATION', {
      detail: `${spec.method} ${spec.path} answered ${response.status} with no error code`,
      httpStatus: response.status,
    });
  }
  const details =
    typeof body?.details === 'object' && body.details !== null
      ? (body.details as Record<string, unknown>)
      : undefined;
  return LiroError.fromAgent(code, details, response.status, `${spec.method} ${spec.path}`);
}

/** Anything thrown that was not already a `LiroError`. */
function unexpected(err: unknown): LiroError {
  return new LiroError('NETWORK', { detail: 'the request failed', cause: err });
}

/** Waits, and gives up early if the caller cancels. */
function delay(ms: number, signal: AbortSignal | undefined): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    if (signal?.aborted === true) {
      reject(new LiroError('CANCELLED', { detail: 'cancelled while waiting to retry' }));
      return;
    }
    const timer = setTimeout(() => {
      signal?.removeEventListener('abort', onAbort);
      resolve();
    }, ms);
    function onAbort(): void {
      clearTimeout(timer);
      reject(new LiroError('CANCELLED', { detail: 'cancelled while waiting to retry' }));
    }
    signal?.addEventListener('abort', onAbort, { once: true });
  });
}
