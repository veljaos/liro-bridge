/**
 * Where the device secret lives, and what stops it going anywhere else.
 *
 * The device secret is the whole of an application's authority to ask
 * for a signature. `PROTOCOL.md` §2.4 is blunt about where it belongs —
 * on your server, and nowhere a browser can reach — and this SDK does
 * not choose for you: {@link ConnectOptions.secretStore} is required and
 * has no default. Not a file path with a sensible fallback, not
 * `~/.liro`. Somebody has to decide where a signing secret is kept, and
 * it is not this library.
 */
/**
 * The marker {@link StoredPairing.toJSON} puts where the secret would
 * be.
 *
 * A `SecretStore` that persists `JSON.stringify(pairing)` — which is
 * the first thing anybody writes — stores this string instead of a
 * secret. {@link StoredPairing.deserialise} recognises it and throws a
 * sentence naming `serialise()`, so the mistake surfaces the first time
 * the store is read back rather than as `AUTH_FAILED` on some later
 * request, which is the least diagnosable answer this protocol has.
 */
export declare const REDACTED_SECRET = "[redacted: persist pairing.serialise(), not JSON.stringify(pairing)]";
/** The plain object a {@link StoredPairing} serialises to and from. */
export interface SerialisedPairing {
    readonly appId: string;
    readonly applicationName: string;
    readonly origin: string;
    /** The 32-byte device secret, base64, exactly as `/v2/pair/confirm` returned it. */
    readonly deviceSecret: string;
}
declare const inspectCustom: unique symbol;
/**
 * One paired application: what the agent knows it as, and the secret it
 * authenticates with.
 *
 * The secret is deliberately awkward to leak and deliberately easy to
 * store on purpose:
 *
 * - It is a **non-enumerable** property, so `{...pairing}`,
 *   `Object.keys(pairing)` and a `for...in` do not see it.
 * - `JSON.stringify(pairing)` produces {@link REDACTED_SECRET} in its
 *   place.
 * - `console.log(pairing)` prints the redacted form, because Node's
 *   inspector is given one.
 * - {@link serialise} is the one way to get it out, and it is named so
 *   that using it is a decision.
 */
export declare class StoredPairing {
    /** The identifier the agent issued at pairing; the `X-Liro-App-Id` header. */
    readonly appId: string;
    /** The display name bound at pairing — what the person sees above every signature. */
    readonly applicationName: string;
    /** The origin bound at pairing, shown verbatim on the agent's window. */
    readonly origin: string;
    constructor(appId: string, applicationName: string, origin: string, deviceSecret: Uint8Array);
    /**
     * The device secret's raw bytes, for signing a request.
     *
     * A copy, so that a caller holding the result cannot reach back into
     * the pairing and change what it authenticates with.
     */
    deviceSecretBytes(): Uint8Array;
    /**
     * The plain object to persist, secret included.
     *
     * This is the only place the secret leaves this class, and it is
     * named so that a `SecretStore` implementation has to mean it. Pair it
     * with {@link StoredPairing.deserialise} on the way back.
     */
    serialise(): SerialisedPairing;
    /** Rebuilds a pairing from {@link serialise}'s output. */
    static deserialise(record: unknown): StoredPairing;
    /** What `JSON.stringify` sees: everything but the secret. */
    toJSON(): Record<string, string>;
    /** What `console.log` and `util.inspect` see. */
    [inspectCustom](): string;
    /** Never the secret, in any interpolation anywhere. */
    toString(): string;
}
/**
 * Where a paired application's secret is kept between runs.
 *
 * Implement this against whatever your application already trusts with
 * credentials — a secrets manager, a database column your ops team knows
 * about, an encrypted config service. {@link FileSecretStore} exists so
 * that a first integration works, not because a file is the right
 * answer for a server.
 *
 * A store is keyed by the application name so that one machine, and one
 * file, can hold several. Two different applications must not share a
 * name; if they do they share a pairing.
 */
export interface SecretStore {
    /** The stored pairing for `appName`, or `null` when there is none. */
    get(appName: string): Promise<StoredPairing | null>;
    /** Persists `pairing` under `appName`, replacing whatever was there. */
    set(appName: string, pairing: StoredPairing): Promise<void>;
    /** Forgets `appName`'s pairing. Does nothing when there is none. */
    clear(appName: string): Promise<void>;
}
/**
 * A {@link SecretStore} that keeps pairings in one JSON file at a path
 * the caller names.
 *
 * There is no default path on purpose. A device secret's location is a
 * decision, and a library that picks one for you has made it on your
 * behalf in every deployment that never thought about it.
 *
 * Permissions are set as tightly as the platform allows, and a failure
 * to set them is an error rather than a warning: a signing secret in a
 * world-readable file is the thing this class exists to avoid.
 *
 * - **POSIX** — the file is `0600`, owner read/write and nothing else.
 * - **Windows** — the file's inherited ACL is replaced with a single
 *   entry granting the current user full control, via `icacls`. That is
 *   the equivalent of `0600` on a filesystem that has no mode bits.
 *
 * For a server application, prefer your platform's secrets manager and
 * implement {@link SecretStore} against it.
 */
export declare class FileSecretStore implements SecretStore {
    private readonly path;
    /** @param path the file to keep pairings in. Its directory is created if it does not exist. */
    constructor(path: string);
    get(appName: string): Promise<StoredPairing | null>;
    set(appName: string, pairing: StoredPairing): Promise<void>;
    clear(appName: string): Promise<void>;
    private read;
    private write;
}
export {};
