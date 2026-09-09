"use strict";
/**
 * The canonical string and the signature over it — `PROTOCOL.md` §3.1.
 *
 * This is the one piece of the protocol that is easy to get subtly
 * wrong and expensive to find wrong, so it is small, it is separate from
 * everything that uses it, and it is pinned by a test against the worked
 * example in `PROTOCOL.md` §3.2 — the same example the agent's own
 * `TestTheProtocolDocumentsWorkedExampleIsTrue` reads out of that
 * document. Both sides agree with the document rather than with each
 * other.
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.MAX_NONCE_LENGTH = exports.EMPTY_BODY_SHA256 = exports.HEADER_SIGNATURE = exports.HEADER_NONCE = exports.HEADER_TIMESTAMP = exports.HEADER_APP_ID = void 0;
exports.sha256Hex = sha256Hex;
exports.canonicalString = canonicalString;
exports.signCanonical = signCanonical;
exports.newNonce = newNonce;
exports.unixSeconds = unixSeconds;
const node_crypto_1 = require("node:crypto");
/** The four headers every authenticated request carries. */
exports.HEADER_APP_ID = 'X-Liro-App-Id';
exports.HEADER_TIMESTAMP = 'X-Liro-Timestamp';
exports.HEADER_NONCE = 'X-Liro-Nonce';
exports.HEADER_SIGNATURE = 'X-Liro-Signature';
/**
 * SHA-256 of no bytes at all, lowercase hex.
 *
 * The last line of the canonical string for a request with an empty
 * body — a GET, or a POST that sends nothing. `PROTOCOL.md` §3.1 states
 * it outright because it is the single thing every integrator gets
 * wrong first: an empty body still hashes to *something*, and this is
 * the something.
 *
 * It is not used as a shortcut anywhere in this SDK — {@link sha256Hex}
 * computes it from the bytes like any other body — it is exported so a
 * caller writing their own client can check theirs against it.
 */
exports.EMPTY_BODY_SHA256 = 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855';
/** The maximum length the agent accepts for a nonce. */
exports.MAX_NONCE_LENGTH = 128;
/** SHA-256 over `body`, lowercase hex. */
function sha256Hex(body) {
    return (0, node_crypto_1.createHash)('sha256').update(body).digest('hex');
}
/**
 * Builds the exact text a request's signature covers:
 *
 * ```
 * METHOD \n PATH \n TIMESTAMP \n NONCE \n sha256hex(BODY)
 * ```
 *
 * The separator is a single `\n`. There is no trailing newline, no
 * spaces around the parts, and no query string on the path.
 *
 * Only the method is upper-cased. `timestamp` and `nonce` are used
 * character for character, because a client signs what it sends:
 * `"1757260800"` and `"01757260800"` are the same instant and different
 * canonical strings.
 */
function canonicalString(method, path, timestamp, nonce, body) {
    return [method.toUpperCase(), path, timestamp, nonce, sha256Hex(body)].join('\n');
}
/**
 * `HMAC-SHA256(deviceSecret, canonical)`, lowercase hex — the value of
 * the `X-Liro-Signature` header.
 *
 * The key is the 32 raw bytes, which is the base64 from
 * `/v2/pair/confirm` *decoded*, not the base64 text.
 */
function signCanonical(deviceSecret, canonical) {
    return (0, node_crypto_1.createHmac)('sha256', deviceSecret).update(canonical, 'utf8').digest('hex');
}
/**
 * A nonce the agent has not seen before.
 *
 * `crypto.randomUUID()` and never `Math.random()`: a predictable nonce
 * is a replay window. Thirty-two hexadecimal characters, well inside
 * the agent's 128-character limit.
 *
 * Every request gets its own, **including a retry**. Reusing a nonce on
 * a retry produces `AUTH_FAILED`, which is the least helpful answer this
 * protocol has — every authentication failure looks identical from the
 * outside, deliberately, so a reused nonce is indistinguishable from a
 * wrong secret.
 */
function newNonce() {
    return (0, node_crypto_1.randomUUID)().replace(/-/g, '');
}
/** Unix seconds as the decimal string the `X-Liro-Timestamp` header takes. */
function unixSeconds(now = new Date()) {
    return Math.floor(now.getTime() / 1000).toString(10);
}
