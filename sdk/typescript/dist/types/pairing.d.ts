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
import { StoredPairing } from './secrets.js';
import { Transport } from './transport.js';
import type { PairingPrompt } from './types.js';
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
export declare function pair(args: PairArgs): Promise<StoredPairing>;
