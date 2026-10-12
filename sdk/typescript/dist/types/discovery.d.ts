/**
 * Finding the agent — `PROTOCOL.md` §4.
 *
 * The agent binds loopback only, on the first free port in 17580–17590,
 * and writes what it chose to a per-user file. **Read that file. Never
 * scan ports.** Several people can be signed in to one machine at once —
 * an accounting firm over RDP is the ordinary case, not an edge one —
 * and each of them has their own agent on its own port. Scanning finds
 * somebody else's, which is exactly what the per-user file prevents.
 *
 * There is no port-scanning code in this SDK, and there is no option
 * that would turn some on.
 */
/** What the discovery file holds. */
export interface BridgeInfo {
    /** The loopback port the agent is listening on. */
    readonly port: number;
    /** The agent's own version. */
    readonly agentVersion: string;
    /** The protocol version it speaks. `2` for everything this SDK does. */
    readonly protocolVersion: number;
    /** Every version it speaks, when it says; empty from an agent older than the field. */
    readonly protocolVersions: readonly number[];
}
/** The newest protocol version this SDK is written against. */
export declare const PROTOCOL_VERSION = 2;
/**
 * Every protocol version this SDK speaks, ascending. It picks the
 * highest one the agent also speaks (`PROTOCOL.md` §4.1, F14 §8) and
 * refuses an agent only when the two lists share nothing.
 */
export declare const PROTOCOL_VERSIONS: readonly number[];
/**
 * The version both sides speak, or `null` when they share none. An
 * agent older than the list advertises only `protocolVersion`, read as
 * a list of one; one that advertises neither is read as version 2,
 * which is every agent before the field existed.
 */
export declare function negotiateProtocol(agentVersions: readonly number[], ours?: readonly number[]): number | null;
/**
 * The per-user discovery file for the current platform.
 *
 * The three paths are SPEC §14's, not this SDK's invention.
 * `PROTOCOL.md` documents the Windows one because Windows is where the
 * agent runs today; the other two are here so that an SDK reading a file
 * does not become the thing that has to change when the agent reaches
 * them.
 */
export declare function defaultBridgeFilePath(): string;
/**
 * Reads the discovery file.
 *
 * A missing file means the agent is not running, and so does a file
 * pointing at a port nothing answers on — a stale one left by a crash.
 * Both are the same fact to a caller, and both are `AGENT_NOT_RUNNING`.
 */
export declare function readBridgeFile(path: string): Promise<BridgeInfo>;
/** A `protocolVersions` value as sent, keeping only whole numbers. */
export declare function versionList(value: unknown): number[];
/** `http://127.0.0.1:<port>` — loopback, always, with no way to ask for anything else. */
export declare function baseUrlFor(port: number): string;
