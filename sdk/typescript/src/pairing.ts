/**
 * Pairing — `PROTOCOL.md` §2.
 *
 * Two calls with a person in between. The agent opens its own window
 * showing six digits; **the code is not in any response and never will
 * be**, because if it were, an application could pair itself with nobody
 * watching and the window would prove nothing. The code travels through
 * a person, and that is the whole mechanism.
 *
 * The awkward part of this protocol, made bearable, is one thing in
 * particular: **both calls carry `origin`, and the two must be
 * byte-for-byte equal.** It is a required field of the confirm body, not
 * only of the request body. Getting that wrong is what cost the owner
 * time writing the first client by hand, and it is the one mistake this
 * module makes structurally impossible — the same string is used twice,
 * from one variable, and there is no way to pass a different one.
 */

import { LiroError } from './errors.js';
import { StoredPairing } from './secrets.js';
import { Transport, encodeJSON, parseJSON } from './transport.js';
import type { PairingPrompt } from './types.js';

interface PairRequestResponse {
  requestId?: unknown;
  expiresInSeconds?: unknown;
}

interface PairConfirmResponse {
  appId?: unknown;
  deviceSecret?: unknown;
  applicationName?: unknown;
  origin?: unknown;
}

/** What {@link pair} needs. */
export interface PairArgs {
  readonly transport: Transport;
  readonly applicationName: string;
  readonly origin: string;
  readonly askForCode: (prompt: PairingPrompt) => string | Promise<string>;
  readonly signal?: AbortSignal;
}

/**
 * Runs a whole pairing and returns the result.
 *
 * A wrong code does **not** start over: the pairing request stays live,
 * the agent says how many attempts remain, and this asks again. Five
 * wrong codes void the request, which the agent reports as
 * `PAIRING_EXPIRED` — a distinct answer from the person having refused
 * (`PAIRING_DENIED`) or from another application's window being open
 * (`PAIRING_IN_PROGRESS`), because each of those needs something
 * different from whoever receives it.
 */
export async function pair(args: PairArgs): Promise<StoredPairing> {
  const { transport, applicationName, origin, askForCode, signal } = args;

  // The one origin, read once and used for both calls. Nothing in this
  // function can send a second value, which is the point.
  const body = { applicationName, origin };
  const requested = await transport.send({
    method: 'POST',
    path: '/v2/pair/request',
    body: encodeJSON(body),
    unauthenticated: true,
    // Not retried: a repeat either opens a second window on somebody's
    // screen or is answered PAIRING_IN_PROGRESS by the first one, and
    // the pairing rate limit counts refused requests too.
    retryable: false,
    ...(signal === undefined ? {} : { signal }),
  });

  const requestBody = parseJSON<PairRequestResponse>(requested, 'POST /v2/pair/request');
  const requestId = requestBody.requestId;
  if (typeof requestId !== 'string' || requestId === '') {
    throw new LiroError('PROTOCOL_VIOLATION', { detail: 'pair/request answered with no requestId' });
  }
  const expiresInSeconds = typeof requestBody.expiresInSeconds === 'number' ? requestBody.expiresInSeconds : 300;

  let attempt = 1;
  let attemptsRemaining: number | null = null;

  for (;;) {
    const prompt: PairingPrompt = { applicationName, origin, expiresInSeconds, attempt, attemptsRemaining };
    const code = normaliseCode(await askForCode(prompt));

    let confirmed;
    try {
      confirmed = await transport.send({
        method: 'POST',
        path: '/v2/pair/confirm',
        // The same `origin`, from the same variable as the request
        // above. The agent compares the two strings and answers
        // PAIRING_ORIGIN_MISMATCH when they differ or when this one is
        // missing.
        body: encodeJSON({ requestId, code, origin }),
        unauthenticated: true,
        // Never retried: every confirm spends one of five attempts, and
        // a repeat sent after a response that never arrived would spend
        // a second one for the same code.
        retryable: false,
        ...(signal === undefined ? {} : { signal }),
      });
    } catch (err) {
      if (err instanceof LiroError && err.code === 'PAIRING_CODE_INCORRECT') {
        attemptsRemaining = err.attemptsRemaining;
        attempt += 1;
        continue;
      }
      throw err;
    }

    return toPairing(parseJSON<PairConfirmResponse>(confirmed, 'POST /v2/pair/confirm'), applicationName, origin);
  }
}

/**
 * Accepts the six digits however a person read them out — with spaces,
 * with the dashes some interfaces show, or bare.
 *
 * The agent compares the code as a string, so `"037 868"` and
 * `"037868"` are different codes to it. Cleaning this up here rather
 * than telling every integrator to is the difference between a working
 * first integration and a support question.
 */
function normaliseCode(raw: unknown): string {
  if (typeof raw !== 'string') {
    throw new LiroError('CONFIGURATION_INVALID', {
      detail: 'onPairingCode must return the six digits from the agent’s window as a string',
    });
  }
  // Whitespace, the hyphen-minus, the Unicode dashes U+2010..U+2015 and
  // the minus sign U+2212 — every separator a person plausibly types or
  // pastes between groups of digits.
  const digits = raw.replace(/[\s‐-―−-]/gu, '');
  if (!/^[0-9]{6}$/.test(digits)) {
    throw new LiroError('CONFIGURATION_INVALID', {
      detail: `onPairingCode returned ${JSON.stringify(raw)}, and the agent’s window shows six digits`,
    });
  }
  return digits;
}

/** Turns the confirm response into a pairing, refusing anything unusable. */
function toPairing(body: PairConfirmResponse, applicationName: string, origin: string): StoredPairing {
  const appId = body.appId;
  const secret = body.deviceSecret;
  if (typeof appId !== 'string' || appId === '') {
    throw new LiroError('PROTOCOL_VIOLATION', { detail: 'pair/confirm answered with no appId' });
  }
  if (typeof secret !== 'string' || secret === '') {
    throw new LiroError('PROTOCOL_VIOLATION', { detail: 'pair/confirm answered with no device secret' });
  }
  const bytes = Buffer.from(secret, 'base64');
  if (bytes.length !== 32) {
    throw new LiroError('PROTOCOL_VIOLATION', {
      detail: `pair/confirm answered with a device secret of ${bytes.length} bytes, and a device secret is 32`,
    });
  }
  // The agent echoes back the name it actually bound, which is the
  // sanitised form of what was asked for — control characters and
  // direction overrides stripped, truncated at 120 characters with the
  // middle elided. That is what the person approved and what they will
  // see above every later signature, so it is what gets stored: a
  // pairing that remembered the unsanitised name would disagree with
  // the window on the one value SPEC §6.6 says must not vary.
  const boundName = typeof body.applicationName === 'string' && body.applicationName !== '' ? body.applicationName : applicationName;
  const boundOrigin = typeof body.origin === 'string' && body.origin !== '' ? body.origin : origin;
  return new StoredPairing(appId, boundName, boundOrigin, new Uint8Array(bytes));
}
