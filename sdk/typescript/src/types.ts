/**
 * The public shapes: what goes in, what comes back, and what a progress
 * handler sees.
 *
 * Nothing here uses a Node-specific type. Documents are `Uint8Array`
 * rather than `Buffer` so that a consumer needs no `@types/node` to
 * compile against this SDK, and a `Buffer` is a `Uint8Array` anyway, so
 * passing one costs nothing.
 */

import type { LiroErrorCode } from './errors.js';
import type { SecretStore } from './secrets.js';

/** What `GET /v2/health` answers. */
export interface Health {
  /** The agent's own version. */
  readonly agentVersion: string;
  /** The protocol version it speaks. */
  readonly protocolVersion: number;
  /**
   * The oldest SDK this agent will serve. If this SDK's version is
   * below it, tell the person to update the agent rather than failing
   * at them later.
   */
  readonly minimumClientVersion: string;
}

/** The role a certificate's KeyUsage gives it. */
export type CertificatePurpose = 'signing' | 'authentication' | 'unknown';

/**
 * One certificate the machine can sign with, as
 * `GET /v2/certificates` reports it.
 *
 * **No certificate is returned — not the DER, not a PEM, not the public
 * key.** A Serbian qualified certificate carries the holder's national
 * identity number and email address inside it, and this listing is
 * answered without any window, at any moment a paired application
 * chooses. See {@link LiroBridge.signDigests} for the gap that leaves.
 */
export interface Certificate {
  /**
   * The SHA-1 thumbprint, uppercase hex — exactly what
   * {@link SignOptions.certificateThumbprint} takes.
   */
  readonly thumbprint: string;
  /** The signer's name, built from `givenName` + `surname`, never parsed out of the common name. */
  readonly displayName: string;
  /** The issuing CA's common name. */
  readonly issuer: string;
  /** The role this certificate's KeyUsage gives it. */
  readonly purpose: CertificatePurpose;
  /** Whether its issuer is a Trusted List service that was granted at the time of asking. */
  readonly qualified: boolean;
  /** Whether it can sign **right now**. */
  readonly usable: boolean;
  /**
   * Why not, when `usable` is false: `CARD_NOT_PRESENT`,
   * `CERT_EXPIRED` or `CERT_NOT_USABLE`. `null` when it can sign.
   *
   * A certificate that cannot sign right now is listed rather than
   * hidden: an absent card and an expired certificate are real choices
   * temporarily unavailable, and leaving them out is what makes a card
   * look broken.
   */
  readonly notUsableReason: string | null;
  /**
   * A test certificate rather than a real one. **If you show a
   * certificate to a person, show this too** — SPEC §16.6 requires a
   * test signature to be visibly marked everywhere it appears.
   */
  readonly isTestKey: boolean;
}

/** The PAdES levels this protocol names. */
export type SignatureLevel = 'b-b' | 'b-t' | 'b-lt';

/** The four corners a visible stamp can go in. */
export type StampPosition = 'bottom-right' | 'bottom-left' | 'top-right' | 'top-left';

/**
 * How the signature should look.
 *
 * Supplying it in full means the person is **not asked** — they see the
 * approval and nothing else, which is the one-window, one-click case.
 * Leaving it out means they choose, exactly as they do when they sign
 * something themselves.
 */
export interface StampChoice {
  /** Whether to draw a visible stamp on the page at all. */
  readonly visible: boolean;
  /** Which corner, when `visible` is true. */
  readonly position?: StampPosition;
}

/** One document to sign, with the name the person will see. */
export interface DocumentToSign {
  /**
   * The name shown on the agent's window. Treated as untrusted display
   * text: control characters and Unicode direction overrides are
   * stripped, long names are truncated with the middle elided, and it
   * is never treated as a path.
   */
  readonly name: string;
  /** The PDF itself. */
  readonly content: Uint8Array;
}

/** Options for {@link LiroBridge.signPdf}. */
export interface SignOptions {
  /**
   * Which certificate to sign with. **Optional here**: the agent builds
   * the CMS, so any usable certificate produces a valid document, and
   * leaving it out means the person chooses.
   */
  readonly certificateThumbprint?: string;
  /**
   * The PAdES level to aim for. Leaving it out means "whatever this
   * agent is configured to produce", which is the person's own standing
   * answer to the same question.
   *
   * What actually comes back is {@link SignedDocument.achievedLevel},
   * which is never the level you asked for — a timestamp authority that
   * did not answer produces `B-B` and says so.
   */
  readonly level?: SignatureLevel;
  /** How the signature should look. See {@link StampChoice}. */
  readonly stamp?: StampChoice;
  /** Cancels the wait. The job keeps running in the agent; only this SDK stops watching. */
  readonly signal?: AbortSignal;
  /** How long to wait for the whole batch. Defaults to {@link ConnectOptions.signTimeoutMs}. */
  readonly timeoutMs?: number;
}

/** Options for {@link LiroBridge.signDigests}. */
export interface SignDigestsOptions {
  /**
   * Which certificate to sign with. **Required here, and not a hint.**
   * You have already built a CMS around one signer certificate, so a
   * signature made with any other key produces a document that verifies
   * against nothing.
   */
  readonly certificateThumbprint: string;
  /** One display name per digest, or none at all. The agent refuses a partial list. */
  readonly labels?: readonly string[];
  /** Cancels the wait. The job keeps running in the agent; only this SDK stops watching. */
  readonly signal?: AbortSignal;
  /** How long to wait for the whole batch. Defaults to {@link ConnectOptions.signTimeoutMs}. */
  readonly timeoutMs?: number;
}

/** Why one document in a batch produced nothing. */
export interface SignFailure {
  /** Its position in what you sent. */
  readonly index: number;
  /** The code, as a value a `switch` can be exhaustive over. */
  readonly code: LiroErrorCode;
  /** The exact code string the agent sent, whether or not this SDK knows it. */
  readonly agentCode: string;
}

/**
 * One signed document.
 *
 * `content` is `null`, and `failure` is set, for a document that did
 * not sign. The array never closes up over a failure: if you sent
 * document 47 you can find entry 47, because that is how a signature
 * ends up attached to the wrong document.
 */
export interface SignedDocument {
  /** The name you sent. */
  readonly name: string;
  /** The signed PDF, or `null` when this one failed. */
  readonly content: Uint8Array | null;
  /**
   * The level this document **actually reached**, never the one you
   * asked for. `null` when it failed.
   */
  readonly achievedLevel: string | null;
  /** Why it failed, or `null` when it did not. */
  readonly failure: SignFailure | null;
}

/**
 * One signature over one digest.
 *
 * `value` is `null`, and `failure` is set, for a digest that did not
 * sign. Same ordering rule as {@link SignedDocument}.
 */
export interface Signature {
  /** Its position in what you sent. */
  readonly index: number;
  /** The raw signature bytes, or `null` when this one failed. */
  readonly value: Uint8Array | null;
  /** Why it failed, or `null` when it did not. */
  readonly failure: SignFailure | null;
}

/** Where a job has got to — `PROTOCOL.md` §6.2's seven states. */
export type JobState =
  /** Accepted. Nothing is on screen yet: another job's window may be open. */
  | 'queued'
  /** The agent's window is up and the person has not answered. Carries `consentRemainingMs`. */
  | 'awaiting_consent'
  /** They approved. The agent is opening the card, where the operating system may ask for a PIN. */
  | 'awaiting_pin'
  /**
   * The first signature is in flight.
   *
   * **This is expected, not a stall.** Measured on real hardware: about
   * 4.0 s on a MUP card and 12.7 s on a Pošta one. It is card
   * initialisation and there is no way to avoid it, which is why it has
   * a state of its own — a progress bar that does not move for twelve
   * seconds reads as a hang, and this is what to show instead.
   */
  | 'preparing_card'
  /** Every signature after the first — about 0.41 s each. */
  | 'signing'
  /** Finished, with a result waiting to be collected. */
  | 'completed'
  /** Nothing was signed. Carries `code`. */
  | 'failed';

/** One progress update. Every event carries the whole picture, not a change to it. */
export interface Progress {
  /** Where the job has got to. */
  readonly state: JobState;
  /** How many documents are finished. */
  readonly completed: number;
  /** How many there are in all. */
  readonly total: number;
  /** How many failed. */
  readonly failed: number;
  /**
   * Estimated milliseconds remaining, or `null`.
   *
   * `null` until a first signature has actually been measured. There is
   * nothing honest to put there before then, and it is never computed
   * from a constant — SPEC §12.9, because a hard-coded estimate becomes
   * a lie the day a slower card ships.
   */
  readonly etaMs: number | null;
  /**
   * Milliseconds left for the person to answer, while `state` is
   * `awaiting_consent`; `null` otherwise. Show your own countdown from
   * this rather than guessing when the window will expire.
   */
  readonly consentRemainingMs: number | null;
  /** The failure code, while `state` is `failed`; `null` otherwise. */
  readonly code: string | null;
  /** The job this update is about. */
  readonly jobId: string;
}

/** A progress handler. Anything it throws is swallowed, so it cannot fail a batch. */
export type ProgressHandler = (progress: Progress) => void;

/** What the agent answered a submission with. */
export interface JobHandle {
  /** The job's identifier. */
  readonly jobId: string;
  /**
   * SHA-256, hex, over the concatenated digests, in order — the same
   * value the person sees on the consent window. Compute it yourself
   * and compare, so that what was approved and what you sent are known
   * to be the same thing.
   */
  readonly batchFingerprint: string;
  /** How many documents the agent accepted. */
  readonly total: number;
}

/** What {@link LiroBridge.connect} needs. */
export interface ConnectOptions {
  /**
   * The name the person sees above every signature this application
   * asks for. It is bound at pairing and cannot be changed in a signing
   * request — otherwise an application could pair as "Test" and present
   * itself as "Liro".
   *
   * It is also the key the {@link secretStore} is asked under, so two
   * different applications on one machine must not share a name.
   */
  readonly applicationName: string;

  /**
   * Where the device secret is kept. **Required, with no default.**
   *
   * See {@link SecretStore}, and {@link FileSecretStore} for the one
   * implementation shipped with this SDK.
   */
  readonly secretStore: SecretStore;

  /**
   * The origin bound at pairing, shown **verbatim** on the agent's own
   * window so that a person who sees `http://` where they expected
   * `https://` can notice.
   *
   * Defaults to `"local"`, which is the truthful answer for a program
   * on the machine that has no web origin of its own. If your
   * application does have one — an ERP behind `https://erp.example.com`
   * — pass it: it is what the person judges the request by.
   *
   * The agent refuses an origin containing whitespace, a control
   * character or a Unicode direction override, and one longer than 255
   * bytes. It is refused rather than cleaned up, because a value
   * altered on its way to the screen is not verbatim.
   */
  readonly origin?: string;

  /**
   * Asked for the six digits on the agent's screen, when there is no
   * stored pairing.
   *
   * The agent opens a window showing a six-digit code. **The code is not
   * in any response and never will be**: if it were, an application
   * could pair itself with nobody watching. It travels through a person,
   * which is the whole mechanism.
   *
   * Called again, with `attemptsRemaining` filled in, after a wrong
   * code — up to five, after which the request is void and a new one is
   * needed.
   */
  readonly onPairingCode?: (prompt: PairingPrompt) => string | Promise<string>;

  /**
   * The discovery file to read. Defaults to the per-user path for this
   * platform. There is no port option and no port scanning.
   */
  readonly bridgeFilePath?: string;

  /** How long one ordinary request may take. Default 30 000 ms. */
  readonly requestTimeoutMs?: number;

  /**
   * How long to wait for a whole batch, from submission to result.
   * Default 600 000 ms — ten minutes, which covers 120 s of consent,
   * a slow card's first signature, and several hundred documents after
   * it.
   */
  readonly signTimeoutMs?: number;
}

/** What {@link ConnectOptions.onPairingCode} is told. */
export interface PairingPrompt {
  /** The application name the agent's window is showing. */
  readonly applicationName: string;
  /** The origin the agent's window is showing, verbatim. */
  readonly origin: string;
  /** How long the code is good for, from the agent. */
  readonly expiresInSeconds: number;
  /** 1 the first time, 2 after one wrong code, and so on. */
  readonly attempt: number;
  /** How many tries are left after a wrong code; `null` on the first attempt. */
  readonly attemptsRemaining: number | null;
}
