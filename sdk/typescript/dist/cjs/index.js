"use strict";
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
Object.defineProperty(exports, "__esModule", { value: true });
exports.MINIMUM_NODE_MAJOR = exports.newNonce = exports.sha256Hex = exports.signCanonical = exports.canonicalString = exports.HEADER_TIMESTAMP = exports.HEADER_SIGNATURE = exports.HEADER_NONCE = exports.HEADER_APP_ID = exports.MAX_NONCE_LENGTH = exports.EMPTY_BODY_SHA256 = exports.PROTOCOL_VERSION = exports.readBridgeFile = exports.defaultBridgeFilePath = exports.REDACTED_SECRET = exports.StoredPairing = exports.FileSecretStore = exports.messageForCode = exports.allKnownAgentCodes = exports.isKnownAgentCode = exports.LiroError = exports.SDK_VERSION = exports.DEFAULT_ORIGIN = exports.LiroBridge = void 0;
var bridge_js_1 = require("./bridge.js");
Object.defineProperty(exports, "LiroBridge", { enumerable: true, get: function () { return bridge_js_1.LiroBridge; } });
Object.defineProperty(exports, "DEFAULT_ORIGIN", { enumerable: true, get: function () { return bridge_js_1.DEFAULT_ORIGIN; } });
var version_js_1 = require("./version.js");
Object.defineProperty(exports, "SDK_VERSION", { enumerable: true, get: function () { return version_js_1.SDK_VERSION; } });
var errors_js_1 = require("./errors.js");
Object.defineProperty(exports, "LiroError", { enumerable: true, get: function () { return errors_js_1.LiroError; } });
Object.defineProperty(exports, "isKnownAgentCode", { enumerable: true, get: function () { return errors_js_1.isKnownAgentCode; } });
Object.defineProperty(exports, "allKnownAgentCodes", { enumerable: true, get: function () { return errors_js_1.allKnownAgentCodes; } });
Object.defineProperty(exports, "messageForCode", { enumerable: true, get: function () { return errors_js_1.messageForCode; } });
var secrets_js_1 = require("./secrets.js");
Object.defineProperty(exports, "FileSecretStore", { enumerable: true, get: function () { return secrets_js_1.FileSecretStore; } });
Object.defineProperty(exports, "StoredPairing", { enumerable: true, get: function () { return secrets_js_1.StoredPairing; } });
Object.defineProperty(exports, "REDACTED_SECRET", { enumerable: true, get: function () { return secrets_js_1.REDACTED_SECRET; } });
var discovery_js_1 = require("./discovery.js");
Object.defineProperty(exports, "defaultBridgeFilePath", { enumerable: true, get: function () { return discovery_js_1.defaultBridgeFilePath; } });
Object.defineProperty(exports, "readBridgeFile", { enumerable: true, get: function () { return discovery_js_1.readBridgeFile; } });
Object.defineProperty(exports, "PROTOCOL_VERSION", { enumerable: true, get: function () { return discovery_js_1.PROTOCOL_VERSION; } });
var canonical_js_1 = require("./canonical.js");
Object.defineProperty(exports, "EMPTY_BODY_SHA256", { enumerable: true, get: function () { return canonical_js_1.EMPTY_BODY_SHA256; } });
Object.defineProperty(exports, "MAX_NONCE_LENGTH", { enumerable: true, get: function () { return canonical_js_1.MAX_NONCE_LENGTH; } });
Object.defineProperty(exports, "HEADER_APP_ID", { enumerable: true, get: function () { return canonical_js_1.HEADER_APP_ID; } });
Object.defineProperty(exports, "HEADER_NONCE", { enumerable: true, get: function () { return canonical_js_1.HEADER_NONCE; } });
Object.defineProperty(exports, "HEADER_SIGNATURE", { enumerable: true, get: function () { return canonical_js_1.HEADER_SIGNATURE; } });
Object.defineProperty(exports, "HEADER_TIMESTAMP", { enumerable: true, get: function () { return canonical_js_1.HEADER_TIMESTAMP; } });
Object.defineProperty(exports, "canonicalString", { enumerable: true, get: function () { return canonical_js_1.canonicalString; } });
Object.defineProperty(exports, "signCanonical", { enumerable: true, get: function () { return canonical_js_1.signCanonical; } });
Object.defineProperty(exports, "sha256Hex", { enumerable: true, get: function () { return canonical_js_1.sha256Hex; } });
Object.defineProperty(exports, "newNonce", { enumerable: true, get: function () { return canonical_js_1.newNonce; } });
var environment_js_1 = require("./environment.js");
Object.defineProperty(exports, "MINIMUM_NODE_MAJOR", { enumerable: true, get: function () { return environment_js_1.MINIMUM_NODE_MAJOR; } });
