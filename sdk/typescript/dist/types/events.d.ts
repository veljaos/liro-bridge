/**
 * Reading the job event stream — `PROTOCOL.md` §6.2.
 *
 * Server-sent events, one per state change, until the job ends and the
 * stream closes. This is a small hand-written parser rather than
 * `EventSource`, for two reasons: `EventSource` cannot set headers, and
 * this endpoint needs the four from §3 — and `EventSource` exists in the
 * browser, which is the one place a device secret must never be.
 */
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
export declare function readEventStream(response: Response): AsyncGenerator<RawEvent>;
