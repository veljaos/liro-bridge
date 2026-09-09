/**
 * The public shapes: what goes in, what comes back, and what a progress
 * handler sees.
 *
 * Nothing here uses a Node-specific type. Documents are `Uint8Array`
 * rather than `Buffer` so that a consumer needs no `@types/node` to
 * compile against this SDK, and a `Buffer` is a `Uint8Array` anyway, so
 * passing one costs nothing.
 */
export {};
