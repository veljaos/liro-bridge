/**
 * One request to the agent: built, signed, sent, and turned back into
 * either a body or a typed error.
 *
 * Everything about authentication lives here — `PROTOCOL.md` §3 — so
 * that no other file in this SDK has to know what an HMAC is, which is
 * the whole point of the SDK existing.
 */
import type { StoredPairing } from './secrets.js';
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
export declare class Transport {
    private readonly baseUrl;
    private readonly defaultTimeoutMs;
    private pairing;
    constructor(baseUrl: string, defaultTimeoutMs: number, pairing?: StoredPairing | null);
    /** The pairing requests are signed with, once there is one. */
    setPairing(pairing: StoredPairing | null): void;
    /** The pairing currently in use, or `null`. */
    currentPairing(): StoredPairing | null;
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
    send(spec: RequestSpec): Promise<RawResponse>;
    /** Sends one request, without any retrying. */
    sendOnce(spec: RequestSpec): Promise<RawResponse>;
    /**
     * Opens a streaming response — the event stream — and hands back the
     * raw body reader. Not retried here; reconnecting is the caller's
     * decision, because it is the caller that knows whether the job is
     * still worth following.
     */
    open(spec: RequestSpec): Promise<Response>;
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
    private buildRequest;
    /** Turns whatever `fetch` threw into a typed error. */
    private networkError;
}
/** Parses a JSON body, or throws `PROTOCOL_VIOLATION`. */
export declare function parseJSON<T>(response: RawResponse, what: string): T;
/** JSON-encodes a body as the bytes that will be hashed and sent. */
export declare function encodeJSON(value: unknown): Uint8Array;
