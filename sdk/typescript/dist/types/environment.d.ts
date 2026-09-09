/**
 * Where this SDK is allowed to run, and where it refuses to.
 *
 * Both checks fail at construction rather than at the first request. A
 * developer who tries the wrong thing should be stopped in ten seconds,
 * not after building a feature on top of it.
 */
/** The oldest Node this SDK supports. */
export declare const MINIMUM_NODE_MAJOR = 18;
/**
 * Refuses to run in a browser.
 *
 * The device secret is the whole of an application's authority to ask
 * for a signature. In a page it belongs to everyone who visits — it is
 * in the bundle, in the network tab, and in every copy of the site
 * anybody has ever loaded. `PROTOCOL.md` §2.4 says so and the agent
 * enforces it: it sends no CORS headers at all and answers a preflight
 * with 403, so page JavaScript cannot make a single authenticated call.
 *
 * This check exists so that the refusal is legible. Without it a
 * developer meets an opaque `TypeError: Failed to fetch` from the
 * browser's own CORS machinery and has to work out why; with it they
 * get a sentence naming the reason before anything else happens.
 *
 * It is not a security boundary — nothing running inside a page could
 * be one. The boundary is the agent's, and it holds whether or not this
 * function is here.
 */
export declare function assertNotBrowser(): void;
/**
 * Refuses to run on a Node older than {@link MINIMUM_NODE_MAJOR}.
 *
 * Node 18 is where `fetch`, `ReadableStream` and `crypto.randomUUID`
 * became available without a flag, and this SDK uses all three. Failing
 * here with a sentence naming the version is the alternative to failing
 * at some random later point with `fetch is not defined`.
 */
export declare function assertSupportedNode(): void;
/** Both checks, in the order that produces the more useful message first. */
export declare function assertUsableEnvironment(): void;
