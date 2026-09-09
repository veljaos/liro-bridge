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
import { StoredPairing } from './secrets.js';
import type { Certificate, ConnectOptions, DocumentToSign, Health, ProgressHandler, SignDigestsOptions, SignOptions, SignedDocument, Signature } from './types.js';
/** The default origin, for an application that has no web origin of its own. */
export declare const DEFAULT_ORIGIN = "local";
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
declare const inspectCustom: unique symbol;
export declare class LiroBridge {
    private readonly transport;
    private readonly options;
    private readonly progressHandlers;
    private pairing;
    private readonly agent;
    private constructor();
    /**
     * Finds the agent, pairs if this is the first run, and returns a
     * client ready to sign.
     *
     * Pairing is skipped entirely when {@link ConnectOptions.secretStore}
     * already holds one, so {@link ConnectOptions.onPairingCode} is never
     * called on any run but the first.
     */
    static connect(options: ConnectOptions): Promise<LiroBridge>;
    /** What `console.log(bridge)` shows: no store, no transport, no secret. */
    [inspectCustom](): string;
    /** What `JSON.stringify(bridge)` sees. Same rule, same reason. */
    toJSON(): Record<string, string | number>;
    toString(): string;
    /** The agent's version, protocol version and minimum client version, as read at connect. */
    get agentInfo(): Health;
    /** The pairing in use. The device secret is not on it in any form a log can reach. */
    get pairingInfo(): StoredPairing;
    /** Asks the agent again, live. */
    health(): Promise<Health>;
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
    certificates(): Promise<Certificate[]>;
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
    signPdf(documents: Uint8Array | DocumentToSign | readonly Uint8Array[] | readonly DocumentToSign[], options?: SignOptions): Promise<SignedDocument[]>;
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
    signDigests(digests: readonly Uint8Array[], options: SignDigestsOptions): Promise<Signature[]>;
    /**
     * Registers a progress handler. Optional: everything works without
     * one, and somebody signing a single document should not have to think
     * about a stream.
     *
     * Every event carries the whole picture rather than a change to it, so
     * a handler that misses one has lost nothing. Anything the handler
     * throws is swallowed: a batch is not failed by a logging call.
     */
    onProgress(handler: ProgressHandler): void;
    /** Removes a handler {@link onProgress} added. */
    offProgress(handler: ProgressHandler): void;
    /**
     * Forgets the stored pairing, so the next {@link connect} pairs again.
     *
     * **This is local only.** The protocol has no way to unpair, by
     * design: a pairing is revoked by the person, in the agent's own
     * Settings window, where they can see every application that is paired
     * and disconnect any of them. Until they do, the agent still knows
     * this `appId` — it is this side that has forgotten the secret.
     */
    disconnect(): Promise<void>;
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
    echo(body?: Uint8Array | string): Promise<{
        canonicalString: string;
        bodySha256: string;
    }>;
    /**
     * Submits a batch and returns the job.
     *
     * **Never retried.** A submission that timed out may already have
     * created a job, and sending it again is a hundred documents signed
     * twice — with a second window in front of the person for a batch they
     * have already approved.
     */
    private submit;
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
    private awaitResult;
    /** Reads the event stream until the job ends, or gives up quietly. */
    private followEvents;
    /** Asks the result endpoint until it has something other than `202`. */
    private collectResult;
    /** Hands one update to every registered handler, swallowing whatever they throw. */
    private emit;
    /** Maps a `/v2/sign/pdf` result, checking what §4.5 asks to be checked. */
    private toSignedDocuments;
    /** Maps a `/v2/sign` result, checking the same things. */
    private toSignatures;
}
export {};
