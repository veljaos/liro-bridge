/**
 * `@liro/bridge` — the TypeScript SDK for Liro Bridge, the local
 * qualified-signature agent.
 *
 * ```ts
 * import { FileSecretStore, LiroBridge } from '@liro/bridge';
 *
 * const bridge = await LiroBridge.connect({
 *   applicationName: 'Moj ERP',
 *   secretStore: new FileSecretStore('./liro-secrets.json'),
 *   onPairingCode: () => askThePersonForTheSixDigits(),
 * });
 * const [signed] = await bridge.signPdf(pdfBytes);
 * ```
 *
 * **The device secret belongs on your server and nowhere else.** It is
 * this application's whole authority to ask for a signature; in a page
 * it belongs to everyone who visits. This SDK refuses to construct in a
 * browser, and the agent refuses browser requests outright — it sends no
 * CORS headers at all and answers a preflight with 403.
 *
 * **Nothing is signed without a person.** There is no flag, header or
 * option anywhere in this SDK or in the protocol that skips the agent's
 * own consent window.
 */
export { LiroBridge, DEFAULT_ORIGIN } from './bridge.js';
export { SDK_VERSION } from './version.js';
export { LiroError, isKnownAgentCode, allKnownAgentCodes, messageForCode } from './errors.js';
export { FileSecretStore, StoredPairing, REDACTED_SECRET } from './secrets.js';
export { defaultBridgeFilePath, readBridgeFile, PROTOCOL_VERSION } from './discovery.js';
export { EMPTY_BODY_SHA256, MAX_NONCE_LENGTH, HEADER_APP_ID, HEADER_NONCE, HEADER_SIGNATURE, HEADER_TIMESTAMP, canonicalString, signCanonical, sha256Hex, newNonce, } from './canonical.js';
export { MINIMUM_NODE_MAJOR } from './environment.js';
