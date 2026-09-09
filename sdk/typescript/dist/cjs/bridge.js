"use strict";
/**
 * `LiroBridge` — the whole of what an integrator touches.
 *
 * ```ts
 * const bridge = await LiroBridge.connect({ applicationName: 'Moj ERP', secretStore });
 * const [signed] = await bridge.signPdf(pdfBytes);
 * ```
 *
 * Finding the agent, pairing, storing the secret, the canonical string,
 * nonces, the job, the event stream, the timeouts and turning error
 * codes into typed exceptions are this class's problem. None of them is
 * the integrator's.
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.LiroBridge = exports.DEFAULT_ORIGIN = void 0;
const environment_js_1 = require("./environment.js");
const errors_js_1 = require("./errors.js");
const errors_js_2 = require("./errors.js");
const discovery_js_1 = require("./discovery.js");
const events_js_1 = require("./events.js");
const pairing_js_1 = require("./pairing.js");
const secrets_js_1 = require("./secrets.js");
const transport_js_1 = require("./transport.js");
/** The default origin, for an application that has no web origin of its own. */
exports.DEFAULT_ORIGIN = 'local';
const DEFAULT_REQUEST_TIMEOUT_MS = 30_000;
const DEFAULT_SIGN_TIMEOUT_MS = 600_000;
/** How often the result endpoint is asked while a job is still running. */
const RESULT_POLL_INTERVAL_MS = 500;
/**
 * What `console.log(bridge)` and `JSON.stringify(bridge)` are allowed to
 * reach.
 *
 * A `LiroBridge` holds the caller's own `SecretStore`, and a store holds
 * device secrets — that is its whole job. Node's inspector walks object
 * graphs, so without this, printing the bridge prints whatever the
 * store happens to be keeping in memory: measured directly, a store
 * backed by a `Map` of serialised pairings put a base64 device secret on
 * screen from one `console.log(bridge)`.
 *
 * Nothing about that is the store's fault and nothing about it is
 * avoidable from the store's side. What is avoidable is this object
 * being the path to it.
 */
const inspectCustom = Symbol.for('nodejs.util.inspect.custom');
/** SHA-256 digests are 32 bytes; anything else is not one. */
const SHA256_DIGEST_BYTES = 32;
class LiroBridge {
    transport;
    options;
    progressHandlers = [];
    pairing;
    agent;
    constructor(transport, pairing, agent, options) {
        this.transport = transport;
        this.pairing = pairing;
        this.agent = agent;
        this.options = options;
    }
    /**
     * Finds the agent, pairs if this is the first run, and returns a
     * client ready to sign.
     *
     * Pairing is skipped entirely when {@link ConnectOptions.secretStore}
     * already holds one, so {@link ConnectOptions.onPairingCode} is never
     * called on any run but the first.
     */
    static async connect(options) {
        (0, environment_js_1.assertUsableEnvironment)();
        if (typeof options?.applicationName !== 'string' || options.applicationName.trim() === '') {
            throw new errors_js_1.LiroError('CONFIGURATION_INVALID', {
                detail: 'connect() needs an applicationName: it is what the person sees above every signature',
            });
        }
        if (options.secretStore === undefined || options.secretStore === null) {
            throw new errors_js_1.LiroError('CONFIGURATION_INVALID', {
                detail: 'connect() needs a secretStore, and there is deliberately no default. The device secret is this ' +
                    'application’s authority to ask for a signature, so where it lives is your decision to make — see ' +
                    'FileSecretStore for the one implementation shipped with this SDK, and prefer your own secrets ' +
                    'manager for a server',
            });
        }
        const resolved = {
            applicationName: options.applicationName,
            secretStore: options.secretStore,
            origin: options.origin ?? exports.DEFAULT_ORIGIN,
            requestTimeoutMs: options.requestTimeoutMs ?? DEFAULT_REQUEST_TIMEOUT_MS,
            signTimeoutMs: options.signTimeoutMs ?? DEFAULT_SIGN_TIMEOUT_MS,
        };
        const bridgePath = options.bridgeFilePath ?? (0, discovery_js_1.defaultBridgeFilePath)();
        const info = await (0, discovery_js_1.readBridgeFile)(bridgePath);
        const transport = new transport_js_1.Transport((0, discovery_js_1.baseUrlFor)(info.port), resolved.requestTimeoutMs);
        const agent = await fetchHealth(transport);
        if (info.protocolVersion !== 0 && info.protocolVersion !== discovery_js_1.PROTOCOL_VERSION) {
            throw new errors_js_1.LiroError('VERSION_TOO_OLD', {
                detail: `the agent speaks protocol version ${info.protocolVersion} and this SDK speaks ${discovery_js_1.PROTOCOL_VERSION}. ` +
                    'Update whichever is older',
            });
        }
        let pairing = await loadPairing(resolved.secretStore, resolved.applicationName);
        if (pairing === null) {
            if (typeof options.onPairingCode !== 'function') {
                throw new errors_js_1.LiroError('CONFIGURATION_INVALID', {
                    detail: `no pairing is stored for ${JSON.stringify(resolved.applicationName)} and connect() was given no ` +
                        'onPairingCode. The agent shows a six-digit code on the person’s screen and the code is in no ' +
                        'response — it has to travel through them, so this SDK needs a way to ask',
                });
            }
            pairing = await (0, pairing_js_1.pair)({
                transport,
                applicationName: resolved.applicationName,
                origin: resolved.origin,
                askForCode: options.onPairingCode,
            });
            await resolved.secretStore.set(resolved.applicationName, pairing);
        }
        transport.setPairing(pairing);
        return new LiroBridge(transport, pairing, agent, resolved);
    }
    /** What `console.log(bridge)` shows: no store, no transport, no secret. */
    [inspectCustom]() {
        return (`LiroBridge { applicationName: ${JSON.stringify(this.options.applicationName)}, ` +
            `origin: ${JSON.stringify(this.options.origin)}, appId: '${this.pairing.appId}', ` +
            `agentVersion: ${JSON.stringify(this.agent.agentVersion)}, ` +
            `protocolVersion: ${this.agent.protocolVersion} }`);
    }
    /** What `JSON.stringify(bridge)` sees. Same rule, same reason. */
    toJSON() {
        return {
            applicationName: this.options.applicationName,
            origin: this.options.origin,
            appId: this.pairing.appId,
            agentVersion: this.agent.agentVersion,
            protocolVersion: this.agent.protocolVersion,
        };
    }
    toString() {
        return this[inspectCustom]();
    }
    /** The agent's version, protocol version and minimum client version, as read at connect. */
    get agentInfo() {
        return this.agent;
    }
    /** The pairing in use. The device secret is not on it in any form a log can reach. */
    get pairingInfo() {
        return this.pairing;
    }
    /** Asks the agent again, live. */
    async health() {
        return fetchHealth(this.transport);
    }
    /**
     * Which certificates this machine can sign with —
     * `GET /v2/certificates`.
     *
     * This is where a thumbprint for {@link signDigests} comes from, and
     * what an SDK offers the person a choice from.
     *
     * May throw `CERTIFICATE_LISTING_DISABLED`. That is **a setting on the
     * machine, not a fault in the request**: on a machine holding several
     * clients' cards this listing would tell an application paired by one
     * client the names on the other clients' certificates, so the person
     * at the machine can switch it off. Signing is unaffected.
     */
    async certificates() {
        const response = await this.transport.send({
            method: 'GET',
            path: '/v2/certificates',
            // Safe to repeat: it reads and changes nothing.
            retryable: true,
        });
        const body = (0, transport_js_1.parseJSON)(response, 'GET /v2/certificates');
        if (!Array.isArray(body.certificates)) {
            throw new errors_js_1.LiroError('PROTOCOL_VIOLATION', { detail: 'GET /v2/certificates answered with no certificate list' });
        }
        return body.certificates.map((raw) => toCertificate(raw));
    }
    /**
     * Signs whole documents — `POST /v2/sign/pdf`.
     *
     * Takes bytes and returns bytes. One document needs no wrapping:
     *
     * ```ts
     * const [signed] = await bridge.signPdf(pdfBytes);
     * ```
     *
     * A document that failed has `content: null` and a `failure`; the
     * array never closes up over one, because that is how a signature ends
     * up attached to the wrong document. When *every* document fails the
     * job itself failed and this throws instead.
     */
    async signPdf(documents, options = {}) {
        const normalised = normaliseDocuments(documents);
        const body = {
            documents: normalised.map((d) => ({ name: d.name, content: base64(d.content) })),
        };
        if (options.certificateThumbprint !== undefined) {
            body['certificateThumbprint'] = options.certificateThumbprint;
        }
        if (options.level !== undefined) {
            body['level'] = options.level;
        }
        if (options.stamp !== undefined) {
            body['stamp'] = options.stamp.visible
                ? { visible: true, position: options.stamp.position ?? 'bottom-right' }
                : { visible: false };
        }
        const job = await this.submit('/v2/sign/pdf', body, normalised.length, options.signal, options.timeoutMs);
        const result = await this.awaitResult(job, options.signal, options.timeoutMs);
        return this.toSignedDocuments((0, transport_js_1.parseJSON)(result, job.resultUrl), normalised, job);
    }
    /**
     * Signs pre-computed digests — `POST /v2/sign`, the path on which the
     * agent never possesses the document.
     *
     * **`certificateThumbprint` is required, and it is not a hint.** You
     * have already built a CMS around one signer certificate, so a
     * signature made with any other key produces a document that verifies
     * against nothing. The agent offers that certificate to the person and
     * no other; they still choose it and still press Approve.
     *
     * **The hole this leaves, stated rather than glossed.** To build a CMS
     * you need the signer certificate itself — for `issuerAndSerialNumber`,
     * for `signingCertificateV2`, and for the certificates set. This
     * protocol will not give it to you: {@link certificates} returns names
     * and thumbprints and deliberately no DER and no PEM, because a
     * Serbian qualified certificate carries the holder's national identity
     * number and email address inside it and that listing is answered
     * without any window, at any moment a paired application chooses
     * (`PROTOCOL.md` §5.5).
     *
     * What you can do today: call {@link certificates}, let the person
     * choose one, and build the CMS around the copy of that certificate
     * your integration already has — one they exported, or one taken from
     * a document they have already signed. If you have no such copy, use
     * {@link signPdf} instead and let the agent build the CMS. Making the
     * certificate arrive with something the person approved is work this
     * protocol has not done yet, and nothing here pretends otherwise.
     */
    async signDigests(digests, options) {
        if (!Array.isArray(digests) || digests.length === 0) {
            throw new errors_js_1.LiroError('CONFIGURATION_INVALID', { detail: 'signDigests needs at least one digest' });
        }
        if (typeof options?.certificateThumbprint !== 'string' || options.certificateThumbprint.trim() === '') {
            throw new errors_js_1.LiroError('CONFIGURATION_INVALID', {
                detail: 'signDigests needs a certificateThumbprint. It is not a hint: you built a CMS around one signer ' +
                    'certificate, and a signature made with any other key produces a document that verifies against ' +
                    'nothing. Get one from certificates()',
            });
        }
        digests.forEach((digest, index) => {
            if (!(digest instanceof Uint8Array) || digest.length !== SHA256_DIGEST_BYTES) {
                throw new errors_js_1.LiroError('CONFIGURATION_INVALID', {
                    detail: `digest ${index} is not ${SHA256_DIGEST_BYTES} bytes; this protocol signs SHA-256 digests and nothing else`,
                });
            }
        });
        if (options.labels !== undefined && options.labels.length !== digests.length) {
            throw new errors_js_1.LiroError('CONFIGURATION_INVALID', {
                detail: `${options.labels.length} labels for ${digests.length} digests. The agent needs exactly one per ` +
                    'digest or none at all: a shorter list leaves the person’s window showing some documents with a ' +
                    'name and some without',
            });
        }
        const body = {
            certificateThumbprint: options.certificateThumbprint,
            digestAlgorithm: 'SHA256',
            digests: digests.map((d) => base64(d)),
        };
        if (options.labels !== undefined) {
            body['labels'] = [...options.labels];
        }
        const job = await this.submit('/v2/sign', body, digests.length, options.signal, options.timeoutMs);
        const result = await this.awaitResult(job, options.signal, options.timeoutMs);
        return this.toSignatures((0, transport_js_1.parseJSON)(result, job.resultUrl), digests.length, job);
    }
    /**
     * Registers a progress handler. Optional: everything works without
     * one, and somebody signing a single document should not have to think
     * about a stream.
     *
     * Every event carries the whole picture rather than a change to it, so
     * a handler that misses one has lost nothing. Anything the handler
     * throws is swallowed: a batch is not failed by a logging call.
     */
    onProgress(handler) {
        if (typeof handler !== 'function') {
            throw new errors_js_1.LiroError('CONFIGURATION_INVALID', { detail: 'onProgress needs a function' });
        }
        this.progressHandlers.push(handler);
    }
    /** Removes a handler {@link onProgress} added. */
    offProgress(handler) {
        const at = this.progressHandlers.indexOf(handler);
        if (at !== -1) {
            this.progressHandlers.splice(at, 1);
        }
    }
    /**
     * Forgets the stored pairing, so the next {@link connect} pairs again.
     *
     * **This is local only.** The protocol has no way to unpair, by
     * design: a pairing is revoked by the person, in the agent's own
     * Settings window, where they can see every application that is paired
     * and disconnect any of them. Until they do, the agent still knows
     * this `appId` — it is this side that has forgotten the secret.
     */
    async disconnect() {
        await this.options.secretStore.clear(this.options.applicationName);
        this.transport.setPairing(null);
    }
    /**
     * `POST /v2/echo` — the canonical string the agent built from a
     * request, to compare against your own.
     *
     * A diagnostic, and only that. This SDK builds the canonical string
     * itself and does not need it; it is here because every authentication
     * failure answers `AUTH_FAILED` with no detail, deliberately and
     * permanently, and this is one of the two things that says anything
     * (the other is the agent's own log). It is what the `AUTH_FAILED`
     * message points at, and an SDK that names an endpoint its user then
     * has to hand-roll has not helped.
     *
     * It is authenticated like every other call, so it answers only for a
     * request that already authenticated: it diagnoses a body hash or a
     * path, not a wrong secret.
     */
    async echo(body = new Uint8Array(0)) {
        const bytes = typeof body === 'string' ? new TextEncoder().encode(body) : body;
        const response = await this.transport.send({
            method: 'POST',
            path: '/v2/echo',
            body: bytes,
            retryable: false,
        });
        const parsed = (0, transport_js_1.parseJSON)(response, 'POST /v2/echo');
        if (typeof parsed.canonicalString !== 'string' || typeof parsed.bodySha256 !== 'string') {
            throw new errors_js_1.LiroError('PROTOCOL_VIOLATION', { detail: 'POST /v2/echo answered with no canonical string' });
        }
        return { canonicalString: parsed.canonicalString, bodySha256: parsed.bodySha256 };
    }
    // ---- internals ----------------------------------------------------
    /**
     * Submits a batch and returns the job.
     *
     * **Never retried.** A submission that timed out may already have
     * created a job, and sending it again is a hundred documents signed
     * twice — with a second window in front of the person for a batch they
     * have already approved.
     */
    async submit(path, body, expected, signal, timeoutMs) {
        const response = await this.transport.send({
            method: 'POST',
            path,
            body: (0, transport_js_1.encodeJSON)(body),
            retryable: false,
            ...(signal === undefined ? {} : { signal }),
            ...(timeoutMs === undefined ? {} : { timeoutMs: Math.min(timeoutMs, this.options.requestTimeoutMs) }),
        });
        const parsed = (0, transport_js_1.parseJSON)(response, `POST ${path}`);
        const jobId = parsed.jobId;
        const eventsUrl = parsed.eventsUrl;
        const resultUrl = parsed.resultUrl;
        if (typeof jobId !== 'string' || jobId === '') {
            throw new errors_js_1.LiroError('PROTOCOL_VIOLATION', { detail: `POST ${path} answered with no jobId` });
        }
        if (!isJobPath(eventsUrl) || !isJobPath(resultUrl)) {
            throw new errors_js_1.LiroError('PROTOCOL_VIOLATION', {
                detail: `POST ${path} answered with job URLs this SDK will not follow`,
            });
        }
        const total = typeof parsed.total === 'number' ? parsed.total : expected;
        if (total !== expected) {
            // The count the person is about to approve is not the count that
            // was sent. Nothing good follows from continuing.
            throw new errors_js_1.LiroError('PROTOCOL_VIOLATION', {
                detail: `${expected} documents were sent and the agent accepted ${total}`,
            });
        }
        return {
            jobId,
            batchFingerprint: typeof parsed.batchFingerprint === 'string' ? parsed.batchFingerprint : '',
            total,
            eventsUrl,
            resultUrl,
        };
    }
    /**
     * Follows the job to a terminal state, reporting progress, and
     * collects the result.
     *
     * The event stream is the primary source and polling the result
     * endpoint is the fallback, because a stream that drops is a normal
     * thing on a machine that goes to sleep and a batch that has been
     * approved should not be lost to it. Polling never consumes anything:
     * the result endpoint answers `202` while the job runs, and the one
     * `200` that hands the signatures over is the last call made.
     */
    async awaitResult(job, signal, timeoutMs) {
        const budget = timeoutMs ?? this.options.signTimeoutMs;
        const deadline = Date.now() + budget;
        const stop = new AbortController();
        const unlink = linkSignals(stop, signal);
        try {
            await this.followEvents(job, stop.signal, deadline);
            return await this.collectResult(job, stop.signal, deadline);
        }
        finally {
            unlink();
            stop.abort();
        }
    }
    /** Reads the event stream until the job ends, or gives up quietly. */
    async followEvents(job, signal, deadline) {
        try {
            const response = await this.transport.open({
                method: 'GET',
                path: job.eventsUrl,
                retryable: false,
                signal,
            });
            for await (const event of (0, events_js_1.readEventStream)(response)) {
                this.emit(job.jobId, event);
                if (event.state === 'completed' || event.state === 'failed') {
                    return;
                }
                if (Date.now() > deadline) {
                    throw new errors_js_1.LiroError('TIMEOUT', {
                        detail: `the batch did not finish within ${deadline}ms of its budget`,
                    });
                }
            }
        }
        catch (err) {
            if (err instanceof errors_js_1.LiroError && (err.code === 'CANCELLED' || err.code === 'TIMEOUT')) {
                throw err;
            }
            // Anything else — a dropped stream, a proxy that buffered it, an
            // agent restarted mid-batch — falls through to polling, which is
            // the authority on how the job ended anyway. Failing here would
            // lose a batch the person has already approved.
        }
    }
    /** Asks the result endpoint until it has something other than `202`. */
    async collectResult(job, signal, deadline) {
        for (;;) {
            if (signal.aborted) {
                throw new errors_js_1.LiroError('CANCELLED', { detail: `watching job ${job.jobId} was cancelled` });
            }
            if (Date.now() > deadline) {
                throw new errors_js_1.LiroError('TIMEOUT', {
                    detail: `job ${job.jobId} did not finish in time. It may still be running in the agent`,
                });
            }
            const response = await this.transport.send({
                method: 'GET',
                path: job.resultUrl,
                // Never retried, and never repeated after a 200: the result is
                // delivered once and the job is then forgotten. A 202 changes
                // nothing, which is why the loop below is safe.
                retryable: false,
                signal,
            });
            if (response.status !== 202) {
                return response;
            }
            this.emit(job.jobId, (0, transport_js_1.parseJSON)(response, job.resultUrl));
            await sleep(RESULT_POLL_INTERVAL_MS, signal);
        }
    }
    /** Hands one update to every registered handler, swallowing whatever they throw. */
    emit(jobId, event) {
        if (this.progressHandlers.length === 0) {
            return;
        }
        const progress = {
            state: (event.state ?? 'queued'),
            completed: numberOr(event.completed, 0),
            total: numberOr(event.total, 0),
            failed: numberOr(event.failed, 0),
            etaMs: typeof event.etaMs === 'number' ? event.etaMs : null,
            consentRemainingMs: typeof event.consentRemainingMs === 'number' ? event.consentRemainingMs : null,
            code: typeof event.code === 'string' && event.code !== '' ? event.code : null,
            jobId,
        };
        for (const handler of this.progressHandlers) {
            try {
                handler(progress);
            }
            catch {
                // A progress handler is a logging call. It does not get to fail
                // a batch somebody has already approved and paid a PIN for.
            }
        }
    }
    /** Maps a `/v2/sign/pdf` result, checking what §4.5 asks to be checked. */
    toSignedDocuments(body, sent, job) {
        if (!Array.isArray(body.documents)) {
            throw new errors_js_1.LiroError('PROTOCOL_VIOLATION', { detail: `${job.resultUrl} answered with no documents` });
        }
        if (body.documents.length !== sent.length) {
            throw new errors_js_1.LiroError('PROTOCOL_VIOLATION', {
                detail: `${sent.length} documents were sent and ${body.documents.length} came back`,
            });
        }
        const failures = failuresByIndex(body.failures);
        return body.documents.map((raw, index) => {
            const entry = (typeof raw === 'object' && raw !== null ? raw : {});
            const failure = failures.get(index) ?? null;
            const encoded = entry['content'];
            if (failure !== null || typeof encoded !== 'string' || encoded === '') {
                return {
                    name: typeof entry['name'] === 'string' ? entry['name'] : (sent[index]?.name ?? ''),
                    content: null,
                    achievedLevel: null,
                    failure: failure ?? {
                        index,
                        code: 'SIGN_FAILED',
                        agentCode: 'SIGN_FAILED',
                    },
                };
            }
            const content = decodeBase64(encoded);
            if (content.length === 0) {
                // A signed document of no bytes is not a signed document. A
                // mismatch here is a bug somewhere and silence is the wrong
                // response to it.
                throw new errors_js_1.LiroError('PROTOCOL_VIOLATION', {
                    detail: `document ${index} came back signed and empty`,
                });
            }
            return {
                name: typeof entry['name'] === 'string' ? entry['name'] : (sent[index]?.name ?? ''),
                content,
                achievedLevel: typeof entry['achievedLevel'] === 'string' ? entry['achievedLevel'] : null,
                failure: null,
            };
        });
    }
    /** Maps a `/v2/sign` result, checking the same things. */
    toSignatures(body, sent, job) {
        if (!Array.isArray(body.signatures)) {
            throw new errors_js_1.LiroError('PROTOCOL_VIOLATION', { detail: `${job.resultUrl} answered with no signatures` });
        }
        if (body.signatures.length !== sent) {
            throw new errors_js_1.LiroError('PROTOCOL_VIOLATION', {
                detail: `${sent} digests were sent and ${body.signatures.length} signatures came back`,
            });
        }
        const failures = failuresByIndex(body.failures);
        return body.signatures.map((raw, index) => {
            const failure = failures.get(index) ?? null;
            if (typeof raw !== 'string' || raw === '') {
                return {
                    index,
                    value: null,
                    failure: failure ?? { index, code: 'SIGN_FAILED', agentCode: 'SIGN_FAILED' },
                };
            }
            const value = decodeBase64(raw);
            if (value.length === 0) {
                throw new errors_js_1.LiroError('PROTOCOL_VIOLATION', { detail: `signature ${index} came back empty` });
            }
            return { index, value, failure: null };
        });
    }
}
exports.LiroBridge = LiroBridge;
// ---- helpers --------------------------------------------------------
async function fetchHealth(transport) {
    const response = await transport.send({
        method: 'GET',
        path: '/v2/health',
        // No authentication: an SDK has to be able to ask whether the agent
        // is there before it has paired with it. And no Content-Type, because
        // there is no content — see Transport.buildRequest.
        unauthenticated: true,
        retryable: true,
    });
    const body = (0, transport_js_1.parseJSON)(response, 'GET /v2/health');
    return {
        agentVersion: typeof body.agentVersion === 'string' ? body.agentVersion : '',
        protocolVersion: typeof body.protocolVersion === 'number' ? body.protocolVersion : 0,
        minimumClientVersion: typeof body.minimumClientVersion === 'string' ? body.minimumClientVersion : '0.0.0',
    };
}
async function loadPairing(store, appName) {
    const stored = await store.get(appName);
    if (stored === null || stored === undefined) {
        return null;
    }
    if (stored instanceof secrets_js_1.StoredPairing) {
        // Reading the bytes here rather than at the first request: a store
        // that gave back something unusable should say so at connect, not
        // as AUTH_FAILED on a signature somebody is waiting for.
        stored.deviceSecretBytes();
        return stored;
    }
    // A store that persisted the serialised form and handed the plain
    // object straight back is doing the reasonable thing; rebuild it.
    return secrets_js_1.StoredPairing.deserialise(stored);
}
function toCertificate(raw) {
    const entry = (typeof raw === 'object' && raw !== null ? raw : {});
    const purpose = entry['purpose'];
    const reason = entry['notUsableReason'];
    return {
        thumbprint: typeof entry['thumbprint'] === 'string' ? entry['thumbprint'] : '',
        displayName: typeof entry['displayName'] === 'string' ? entry['displayName'] : '',
        issuer: typeof entry['issuer'] === 'string' ? entry['issuer'] : '',
        purpose: purpose === 'signing' || purpose === 'authentication' || purpose === 'unknown'
            ? purpose
            : 'unknown',
        qualified: entry['qualified'] === true,
        usable: entry['usable'] === true,
        notUsableReason: typeof reason === 'string' && reason !== '' ? reason : null,
        isTestKey: entry['isTestKey'] === true,
    };
}
function failuresByIndex(raw) {
    const out = new Map();
    if (!Array.isArray(raw)) {
        return out;
    }
    for (const item of raw) {
        const entry = (typeof item === 'object' && item !== null ? item : {});
        if (typeof entry.index !== 'number' || typeof entry.code !== 'string') {
            continue;
        }
        const agentCode = entry.code;
        out.set(entry.index, {
            index: entry.index,
            code: (0, errors_js_2.isKnownAgentCode)(agentCode) ? agentCode : 'UNKNOWN',
            agentCode,
        });
    }
    return out;
}
function normaliseDocuments(input) {
    if (input instanceof Uint8Array) {
        return [{ name: 'document.pdf', content: input }];
    }
    // One named document needs no wrapping either, for the same reason one
    // buffer does not: the common case is a single document, and an array
    // literal around it is ceremony.
    if (isDocument(input)) {
        return normaliseDocuments([input]);
    }
    if (!Array.isArray(input) || input.length === 0) {
        throw new errors_js_1.LiroError('CONFIGURATION_INVALID', { detail: 'signPdf needs at least one document' });
    }
    return input.map((item, index) => {
        if (item instanceof Uint8Array) {
            return { name: `document-${index + 1}.pdf`, content: item };
        }
        if (typeof item !== 'object' || item === null || !(item.content instanceof Uint8Array)) {
            throw new errors_js_1.LiroError('CONFIGURATION_INVALID', {
                detail: `document ${index} is neither a Uint8Array nor { name, content }`,
            });
        }
        if (item.content.length === 0) {
            throw new errors_js_1.LiroError('CONFIGURATION_INVALID', { detail: `document ${index} is empty` });
        }
        return { name: typeof item.name === 'string' && item.name !== '' ? item.name : `document-${index + 1}.pdf`, content: item.content };
    });
}
function isDocument(value) {
    return (typeof value === 'object' &&
        value !== null &&
        !Array.isArray(value) &&
        value.content instanceof Uint8Array);
}
function isJobPath(value) {
    return typeof value === 'string' && /^\/v2\/jobs\/[A-Za-z0-9_-]+\/(events|result)$/.test(value);
}
function base64(bytes) {
    return Buffer.from(bytes.buffer, bytes.byteOffset, bytes.byteLength).toString('base64');
}
function decodeBase64(text) {
    return new Uint8Array(Buffer.from(text, 'base64'));
}
function numberOr(value, fallback) {
    return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}
/** Aborts `target` when `source` aborts. Returns the undo. */
function linkSignals(target, source) {
    if (source === undefined) {
        return () => undefined;
    }
    if (source.aborted) {
        target.abort(source.reason);
        return () => undefined;
    }
    const onAbort = () => target.abort(source.reason);
    source.addEventListener('abort', onAbort, { once: true });
    return () => source.removeEventListener('abort', onAbort);
}
function sleep(ms, signal) {
    return new Promise((resolve, reject) => {
        const timer = setTimeout(() => {
            signal.removeEventListener('abort', onAbort);
            resolve();
        }, ms);
        function onAbort() {
            clearTimeout(timer);
            reject(new errors_js_1.LiroError('CANCELLED', { detail: 'cancelled while waiting for the job' }));
        }
        signal.addEventListener('abort', onAbort, { once: true });
    });
}
