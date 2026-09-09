/**
 * Every error this SDK throws, and the codes the agent can answer with.
 *
 * The agent never sends a human-readable message, in any language
 * (`PROTOCOL.md` §7): an error body is a stable `SCREAMING_SNAKE_CASE`
 * code and optional structured `details`, and nothing else. Translating
 * that into something a person reads is the caller's job, because the
 * caller is the only layer that knows what language the person speaks.
 *
 * What this file adds is a *developer-facing* English sentence per code,
 * on `LiroError.message`, so a stack trace in a log says what happened.
 * It is not a user-facing string and it is not translated. If you show
 * something to a person, switch on `error.code` and write your own.
 */
/**
 * Every code the agent's own vocabulary contains, exactly as
 * `PROTOCOL.md` §7.1 lists them.
 *
 * The union is exhaustive, so a `switch` over `error.code` with a
 * `never` default is checked by the compiler.
 */
export type KnownLiroErrorCode = 'REQUEST_INVALID' | 'NOT_PAIRED' | 'AUTH_FAILED' | 'PAIRING_CODE_INCORRECT' | 'PAIRING_DENIED' | 'PAIRING_ORIGIN_MISMATCH' | 'PAIRING_IN_PROGRESS' | 'PAIRING_EXPIRED' | 'RATE_LIMITED' | 'DOCUMENT_SIGNING_DISABLED' | 'CERTIFICATE_LISTING_DISABLED' | 'JOB_IN_PROGRESS' | 'JOB_NOT_FOUND' | 'CONSENT_DENIED' | 'CONSENT_TIMEOUT' | 'NO_READER' | 'SMART_CARD_SERVICE_DOWN' | 'CARD_NOT_PRESENT' | 'PIN_REQUIRED' | 'PIN_INCORRECT' | 'PIN_LOCKED' | 'CERT_NOT_FOUND' | 'CERT_EXPIRED' | 'CERT_NOT_USABLE' | 'CERT_REVOKED' | 'PDF_INVALID' | 'PDF_ENCRYPTED' | 'TSA_UNAVAILABLE' | 'TSA_REJECTED' | 'TSA_CLIENT_CERT_UNREADABLE' | 'TSA_CLIENT_CERT_INVALID' | 'STAMP_GLYPH_MISSING' | 'SIGN_FAILED' | 'INPUT_UNREADABLE' | 'OUTPUT_EXISTS' | 'OUTPUT_WRITE_FAILED' | 'OUTPUT_IN_USE' | 'VERSION_TOO_OLD' | 'INTERNAL';
/**
 * Codes this SDK raises itself, for the things that go wrong on this
 * side of the socket. They are distinct strings from the agent's own so
 * that one `switch` covers everything without two vocabularies.
 */
export type ClientLiroErrorCode = 
/** No agent is running, or its discovery file is not where it should be. */
'AGENT_NOT_RUNNING'
/** The agent answered, but not with anything this protocol describes. */
 | 'PROTOCOL_VIOLATION'
/** The SDK is running somewhere a device secret must never be. */
 | 'UNSUPPORTED_ENVIRONMENT'
/** Something the caller passed cannot be used. */
 | 'CONFIGURATION_INVALID'
/** The secret store gave back something that is not a usable pairing. */
 | 'SECRET_STORE_INVALID'
/** A request, or a whole batch, took longer than it was given. */
 | 'TIMEOUT'
/** The connection failed outright. */
 | 'NETWORK'
/** The caller cancelled. */
 | 'CANCELLED';
/**
 * `UNKNOWN` is the forward-compatible case, and it exists on purpose.
 *
 * `PROTOCOL.md` §7 says codes are stable and that new situations get new
 * ones — and adding a code does not change the protocol version, because
 * a caller written against the old one still works. So an agent newer
 * than this SDK can answer with a code that is not in the union above.
 *
 * When that happens the SDK does not guess and does not silently
 * mislabel it as something it knows. `code` is `UNKNOWN` and
 * {@link LiroError.agentCode} carries the exact string the agent sent,
 * so it can be logged, reported and switched on by a caller that knows
 * about it before this SDK does.
 */
export type LiroErrorCode = KnownLiroErrorCode | ClientLiroErrorCode | 'UNKNOWN';
/** Reports whether `code` is one this SDK's `LiroErrorCode` union names. */
export declare function isKnownAgentCode(code: string): code is KnownLiroErrorCode;
/** Every agent code, for tests and for tooling that enumerates them. */
export declare function allKnownAgentCodes(): KnownLiroErrorCode[];
/** The English developer-facing sentence for a code. */
export declare function messageForCode(code: LiroErrorCode): string;
/** Options for constructing a {@link LiroError} directly. */
export interface LiroErrorOptions {
    /** Extra text appended to the code's own sentence. Never a secret. */
    readonly detail?: string;
    /** The `details` object from the agent's error body. */
    readonly details?: Readonly<Record<string, unknown>>;
    /** The HTTP status the error arrived with, when it arrived over HTTP. */
    readonly httpStatus?: number;
    /** The exact code string the agent sent, when it differs from `code`. */
    readonly agentCode?: string;
    /** The underlying error, for a network or filesystem failure. */
    readonly cause?: unknown;
}
/**
 * Every error this SDK throws.
 *
 * ```ts
 * try {
 *   await bridge.signPdf(pdf);
 * } catch (e) {
 *   if (e instanceof LiroError && e.code === 'CARD_NOT_PRESENT') {
 *     // tell the person to insert their card
 *   }
 * }
 * ```
 *
 * The device secret never appears in `message`, in `details`, in
 * `stack`, or in anything this class serialises. Nothing in the SDK
 * interpolates it into a string, and a test greps every string produced
 * along every error path to keep it that way.
 */
export declare class LiroError extends Error {
    /**
     * What went wrong, as a value a `switch` can be exhaustive over.
     *
     * `UNKNOWN` means the agent sent a code newer than this SDK;
     * {@link agentCode} then carries it verbatim.
     */
    readonly code: LiroErrorCode;
    /**
     * The exact code string that arrived, whether or not this SDK knows
     * it. Equal to {@link code} for every known agent code.
     */
    readonly agentCode: string;
    /** The agent's structured `details`, when it sent any. Never prose. */
    readonly details: Readonly<Record<string, unknown>> | undefined;
    /** The HTTP status this error arrived with, when it arrived over HTTP. */
    readonly httpStatus: number | undefined;
    constructor(code: LiroErrorCode, options?: LiroErrorOptions);
    /**
     * Builds the error for an agent's error body.
     *
     * A code this SDK does not recognise becomes `UNKNOWN` with
     * `agentCode` carrying it, rather than being folded into `INTERNAL` —
     * see {@link LiroErrorCode}.
     */
    static fromAgent(rawCode: string, details: Readonly<Record<string, unknown>> | undefined, httpStatus: number, detail?: string): LiroError;
    /**
     * `details.attemptsRemaining` as a number, for
     * `PAIRING_CODE_INCORRECT`, or `null` when the agent sent none.
     */
    get attemptsRemaining(): number | null;
    /**
     * `details.retryAfterSeconds` as a number, for `RATE_LIMITED`, or
     * `null` when the agent sent none.
     */
    get retryAfterSeconds(): number | null;
}
