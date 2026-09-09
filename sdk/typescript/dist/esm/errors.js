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
const KNOWN_CODES = new Set([
    'REQUEST_INVALID',
    'NOT_PAIRED',
    'AUTH_FAILED',
    'PAIRING_CODE_INCORRECT',
    'PAIRING_DENIED',
    'PAIRING_ORIGIN_MISMATCH',
    'PAIRING_IN_PROGRESS',
    'PAIRING_EXPIRED',
    'RATE_LIMITED',
    'DOCUMENT_SIGNING_DISABLED',
    'CERTIFICATE_LISTING_DISABLED',
    'JOB_IN_PROGRESS',
    'JOB_NOT_FOUND',
    'CONSENT_DENIED',
    'CONSENT_TIMEOUT',
    'NO_READER',
    'SMART_CARD_SERVICE_DOWN',
    'CARD_NOT_PRESENT',
    'PIN_REQUIRED',
    'PIN_INCORRECT',
    'PIN_LOCKED',
    'CERT_NOT_FOUND',
    'CERT_EXPIRED',
    'CERT_NOT_USABLE',
    'CERT_REVOKED',
    'PDF_INVALID',
    'PDF_ENCRYPTED',
    'TSA_UNAVAILABLE',
    'TSA_REJECTED',
    'TSA_CLIENT_CERT_UNREADABLE',
    'TSA_CLIENT_CERT_INVALID',
    'STAMP_GLYPH_MISSING',
    'SIGN_FAILED',
    'INPUT_UNREADABLE',
    'OUTPUT_EXISTS',
    'OUTPUT_WRITE_FAILED',
    'OUTPUT_IN_USE',
    'VERSION_TOO_OLD',
    'INTERNAL',
]);
/** Reports whether `code` is one this SDK's `LiroErrorCode` union names. */
export function isKnownAgentCode(code) {
    return KNOWN_CODES.has(code);
}
/** Every agent code, for tests and for tooling that enumerates them. */
export function allKnownAgentCodes() {
    return [...KNOWN_CODES];
}
/**
 * The English sentence `LiroError.message` carries for each code.
 *
 * Two of them are load-bearing rather than descriptive.
 *
 * `AUTH_FAILED` names the agent's own log and `POST /v2/echo`. The
 * protocol answers every authentication failure identically and always
 * will — telling a caller that its nonce was reused tells whoever is
 * probing that the timestamp and the signature were both fine
 * (`PROTOCOL.md` §3.3, decision D-175). The log is the one place that
 * says which check refused a request, and `/v2/echo` returns the
 * canonical string the agent built from it. Between them there is
 * nothing left to guess at, and the sentence that says so is worth a
 * morning to whoever reads it.
 *
 * `PIN_INCORRECT` says never to retry, because three wrong entries block
 * a card and unblocking a national identity card means a visit to the
 * Ministry.
 */
const MESSAGES = {
    // ---- pairing and authentication ----
    REQUEST_INVALID: 'The agent refused the request as malformed. `details.field` names the field, or `details.maxBytes` the limit.',
    NOT_PAIRED: 'This application is not paired with the agent, or the person disconnected it in the agent’s Settings window. Pair again.',
    AUTH_FAILED: 'The request did not authenticate, and the agent deliberately does not say which check failed. ' +
        'Two things will tell you: the agent’s own log names the check (signature_mismatch, clock_skew, ' +
        'unparseable_timestamp, nonce_reused, origin_mismatch, missing_header, nonce_too_long), and ' +
        'POST /v2/echo returns the canonical string the agent built from your request, to compare against your own.',
    PAIRING_CODE_INCORRECT: 'That six-digit code is wrong. The pairing request is still live; `details.attemptsRemaining` says how many tries are left.',
    PAIRING_DENIED: 'The person refused the pairing, or closed the window. This is an answer, not a failure — do not ask again automatically.',
    PAIRING_ORIGIN_MISMATCH: 'The `origin` sent with pair/confirm is not the one pair/request declared. Both calls must carry the same string, byte for byte.',
    PAIRING_IN_PROGRESS: 'Another application’s pairing window is already open on this machine. Wait and try again.',
    PAIRING_EXPIRED: 'The pairing request is gone: five minutes passed, five wrong codes voided it, it was already confirmed, or there is no such request. Start a new one.',
    RATE_LIMITED: 'Too many pairing requests from this origin. `details.retryAfterSeconds` says how long to wait.',
    // ---- jobs ----
    DOCUMENT_SIGNING_DISABLED: 'This machine is set up to serve only POST /v2/sign; the whole-document path is switched off. ' +
        'That is a setting in the agent’s Settings window, not a fault in your request — send digests instead, or ask the person to turn it on.',
    CERTIFICATE_LISTING_DISABLED: 'The certificate listing is switched off on this machine. That is a setting in the agent’s Settings window, not a fault in your request — ' +
        'ask the person for the thumbprint, or ask them to turn it back on. Signing is unaffected.',
    JOB_IN_PROGRESS: 'This application already has a signing job running. One at a time; wait for it to finish.',
    JOB_NOT_FOUND: 'No such job: it never existed, it belongs to another application, its result has already been collected, or it expired. Submit again.',
    // ---- signing ----
    CONSENT_DENIED: 'The person pressed Cancel, closed the window, or stopped the batch. This is an answer, not a failure.',
    CONSENT_TIMEOUT: 'Nobody answered the agent’s window within 120 seconds. Submit again when somebody is at the machine.',
    NO_READER: 'No smart card reader is attached to the machine.',
    SMART_CARD_SERVICE_DOWN: 'The operating system’s smart card service is not running. That is a service to start, not hardware to plug in.',
    CARD_NOT_PRESENT: 'The certificate’s card is not in a reader. Ask the person to insert it.',
    PIN_REQUIRED: 'The card needs its PIN. The operating system asks for it; there is nothing for you to send.',
    PIN_INCORRECT: 'The PIN was wrong. NEVER retry this automatically: three wrong entries block the card, and unblocking a national identity card means a visit to the Ministry. Show the person what happened and let them decide.',
    PIN_LOCKED: 'The card is blocked and needs its PUK. Stop.',
    CERT_NOT_FOUND: 'No certificate with that thumbprint is available on this machine. Check the thumbprint against GET /v2/certificates, or leave it out on /v2/sign/pdf.',
    CERT_EXPIRED: 'The certificate is outside its validity period.',
    CERT_NOT_USABLE: 'The certificate exists but cannot sign. Choose another.',
    CERT_REVOKED: 'Revocation checking says the certificate is revoked. Stop.',
    PDF_INVALID: 'A document is not a readable PDF. Check what you sent.',
    PDF_ENCRYPTED: 'A document is password-protected. Decrypt it first; the agent never will.',
    TSA_UNAVAILABLE: 'The timestamp authority did not answer after three attempts. Retry later, or ask for level "b-b".',
    TSA_REJECTED: 'The timestamp authority refused the request. Check the agent’s timestamp settings.',
    TSA_CLIENT_CERT_UNREADABLE: 'The TSA client certificate configured on this machine could not be read. That is the person’s Settings, not your request.',
    TSA_CLIENT_CERT_INVALID: 'The TSA client certificate would not open — almost always a wrong password. That is the person’s Settings, not your request.',
    STAMP_GLYPH_MISSING: 'The visible stamp needs a character the embedded font does not have. `details.character` and `details.codePoint` name it.',
    SIGN_FAILED: 'The card refused or failed to sign. Retry once; then stop.',
    INPUT_UNREADABLE: 'A document could not be read. This happens to batches the person started locally; you cannot cause it.',
    OUTPUT_EXISTS: 'About writing a file. This happens to batches the person started locally; a request over the protocol writes nothing.',
    OUTPUT_WRITE_FAILED: 'About writing a file. This happens to batches the person started locally; a request over the protocol writes nothing.',
    OUTPUT_IN_USE: 'About writing a file. This happens to batches the person started locally; a request over the protocol writes nothing.',
    VERSION_TOO_OLD: 'This client is older than the agent’s minimumClientVersion. Update the SDK.',
    INTERNAL: 'The agent hit something it could not classify. It has logged the details locally. Please report it.',
    // ---- raised by this SDK ----
    AGENT_NOT_RUNNING: 'Liro Bridge is not running, or its discovery file could not be read. Start the agent and try again; never scan ports for it.',
    PROTOCOL_VIOLATION: 'The agent answered with something this protocol does not describe.',
    UNSUPPORTED_ENVIRONMENT: 'This SDK cannot run here.',
    CONFIGURATION_INVALID: 'The SDK was given something it cannot use.',
    SECRET_STORE_INVALID: 'The secret store did not give back a usable pairing.',
    TIMEOUT: 'The agent did not answer in time.',
    NETWORK: 'The connection to the agent failed.',
    CANCELLED: 'The operation was cancelled.',
    UNKNOWN: 'The agent answered with a code this version of the SDK does not know. `agentCode` carries it verbatim; the agent is probably newer than this SDK.',
};
/** The English developer-facing sentence for a code. */
export function messageForCode(code) {
    return MESSAGES[code];
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
export class LiroError extends Error {
    /**
     * What went wrong, as a value a `switch` can be exhaustive over.
     *
     * `UNKNOWN` means the agent sent a code newer than this SDK;
     * {@link agentCode} then carries it verbatim.
     */
    code;
    /**
     * The exact code string that arrived, whether or not this SDK knows
     * it. Equal to {@link code} for every known agent code.
     */
    agentCode;
    /** The agent's structured `details`, when it sent any. Never prose. */
    details;
    /** The HTTP status this error arrived with, when it arrived over HTTP. */
    httpStatus;
    constructor(code, options = {}) {
        const base = messageForCode(code);
        super(options.detail ? `${base} (${options.detail})` : base, { cause: options.cause });
        this.name = 'LiroError';
        this.code = code;
        this.agentCode = options.agentCode ?? code;
        this.details = options.details;
        this.httpStatus = options.httpStatus;
    }
    /**
     * Builds the error for an agent's error body.
     *
     * A code this SDK does not recognise becomes `UNKNOWN` with
     * `agentCode` carrying it, rather than being folded into `INTERNAL` —
     * see {@link LiroErrorCode}.
     */
    static fromAgent(rawCode, details, httpStatus, detail) {
        const known = isKnownAgentCode(rawCode);
        const options = {
            details,
            httpStatus,
            agentCode: rawCode,
            ...(detail === undefined ? {} : { detail }),
        };
        return new LiroError(known ? rawCode : 'UNKNOWN', options);
    }
    /**
     * `details.attemptsRemaining` as a number, for
     * `PAIRING_CODE_INCORRECT`, or `null` when the agent sent none.
     */
    get attemptsRemaining() {
        const value = this.details?.['attemptsRemaining'];
        return typeof value === 'number' ? value : null;
    }
    /**
     * `details.retryAfterSeconds` as a number, for `RATE_LIMITED`, or
     * `null` when the agent sent none.
     */
    get retryAfterSeconds() {
        const value = this.details?.['retryAfterSeconds'];
        return typeof value === 'number' ? value : null;
    }
}
