/**
 * The device secret appears in no message, stack, log or serialisation.
 *
 * This pairs against an agent that issues a **known** secret, drives
 * every error path the SDK has, and greps every string it produced —
 * messages, causes, stacks, `JSON.stringify`, `util.inspect` at full
 * depth with hidden properties shown — plus everything written to
 * stdout and stderr along the way.
 *
 * The secret is searched for in four spellings, because a leak does not
 * have to be a base64 one: the raw bytes as text, the base64 the
 * protocol carries, hex, and each half of the base64 on its own — the
 * last so that a truncation or a wrapped log line cannot hide one.
 */

import assert from 'node:assert/strict';
import { inspect } from 'node:util';
import test from 'node:test';

import { LiroBridge, LiroError, StoredPairing, allKnownAgentCodes } from '../dist/esm/index.js';
import { startFakeAgent } from './fake-agent.mjs';
import { MemoryStore, scratch, writeBridgeFile } from './helpers.mjs';

/** A secret whose bytes are printable, so a leak of the raw bytes is visible too. */
const SECRET = Buffer.from('liro-sdk-leak-test-secret-32byte', 'utf8');
const SPELLINGS = [
  SECRET.toString('utf8'),
  SECRET.toString('base64'),
  SECRET.toString('hex'),
  SECRET.toString('base64').slice(0, 20),
  SECRET.toString('base64').slice(-20),
];

/** Everything that can be got out of a value, as one string. */
function everyString(value, depth = 0) {
  if (depth > 4 || value === null || value === undefined) {
    return String(value);
  }
  const parts = [String(value)];
  try {
    parts.push(inspect(value, { depth: Infinity, showHidden: true, getters: true, breakLength: Infinity }));
  } catch (err) {
    parts.push(`inspect threw: ${err}`);
  }
  try {
    parts.push(JSON.stringify(value, Object.getOwnPropertyNames(Object(value))) ?? '');
  } catch (err) {
    parts.push(`stringify threw: ${err}`);
  }
  if (value instanceof Error) {
    parts.push(value.message, value.stack ?? '');
    if (value.cause !== undefined) {
      parts.push(everyString(value.cause, depth + 1));
    }
    if (value instanceof LiroError) {
      parts.push(value.agentCode, JSON.stringify(value.details ?? {}), String(value.httpStatus));
    }
  }
  return parts.join('\n');
}

function assertNoSecret(where, text) {
  for (const spelling of SPELLINGS) {
    assert.ok(
      !text.includes(spelling),
      `the device secret reached ${where} (as ${spelling === SECRET.toString('utf8') ? 'raw bytes' : 'an encoding'})`,
    );
  }
}

test('the secret reaches no string the SDK produces, along every path there is', async (t) => {
  const agent = await startFakeAgent({ deviceSecret: SECRET });
  const dir = await scratch();
  const bridgeFilePath = await writeBridgeFile(dir.dir, agent.port);
  t.after(async () => {
    await agent.stop();
    await dir.dispose();
  });

  // Everything written to the console during the whole run is captured,
  // not only what is thrown: a leak into a log is the failure this is
  // about, and a log is where an SDK most plausibly puts one.
  const written = [];
  const realOut = process.stdout.write.bind(process.stdout);
  const realErr = process.stderr.write.bind(process.stderr);
  process.stdout.write = (chunk, ...rest) => {
    written.push(String(chunk));
    return realOut(chunk, ...rest);
  };
  process.stderr.write = (chunk, ...rest) => {
    written.push(String(chunk));
    return realErr(chunk, ...rest);
  };

  const collected = [];
  const record = (where, value) => collected.push([where, everyString(value)]);

  try {
    const store = new MemoryStore();
    const bridge = await LiroBridge.connect({
      applicationName: 'Moj ERP',
      origin: 'https://erp.example.com',
      secretStore: store,
      bridgeFilePath,
      onPairingCode: () => agent.state.pairingCode,
      requestTimeoutMs: 2000,
      signTimeoutMs: 10_000,
    });

    // The pairing itself, every way it can be turned into text.
    record('the pairing object', bridge.pairingInfo);
    record('the bridge object', bridge);
    record('what the store kept, printed', store.records.get('Moj ERP') === undefined ? '' : 'kept');
    console.log('%s', bridge.pairingInfo);
    console.log(bridge.pairingInfo);
    console.error('a pairing in an error log:', bridge.pairingInfo);
    console.log(JSON.stringify({ pairing: bridge.pairingInfo }));
    console.log(inspect(bridge, { depth: Infinity, showHidden: true }));

    // A successful call, so the happy path's own strings are covered.
    record('a successful health', await bridge.health());
    record('a successful signature', await bridge.signPdf(new Uint8Array([37, 80, 68, 70])));

    // Every code the agent can answer with, on a real request.
    for (const code of allKnownAgentCodes()) {
      agent.state.overrides.set('GET /v2/certificates', () => ({
        status: code === 'INTERNAL' ? 500 : 422,
        body: { code, details: { field: 'something', attemptsRemaining: 3 } },
      }));
      try {
        await bridge.certificates();
        record(`certificates() answered ${code}`, 'no error');
      } catch (err) {
        record(`the error for ${code}`, err);
      }
    }
    agent.state.overrides.delete('GET /v2/certificates');

    // A body that is not JSON, an answer with no code, a job that
    // vanished, a stream that dies, a connection refused, a timeout, a
    // cancellation — every client-side path that builds a message.
    agent.state.overrides.set('GET /v2/certificates', () => ({ status: 200, body: 'not an object at all' }));
    await bridge.certificates().catch((err) => record('a body that is not the right shape', err));
    agent.state.overrides.set('GET /v2/certificates', () => ({ status: 418, body: { nope: true } }));
    await bridge.certificates().catch((err) => record('an error body with no code', err));
    agent.state.overrides.delete('GET /v2/certificates');

    const cancelled = new AbortController();
    cancelled.abort();
    await bridge
      .signPdf(new Uint8Array([1]), { signal: cancelled.signal })
      .catch((err) => record('a cancelled batch', err));

    await bridge.signPdf(new Uint8Array([1]), { timeoutMs: 1 }).catch((err) => record('a batch that timed out', err));

    // The store, and a pairing rebuilt from what it kept.
    const reloaded = await store.get('Moj ERP');
    record('a pairing read back out of the store', reloaded);
    record('a pairing that cannot be deserialised', (() => {
      try {
        StoredPairing.deserialise({ ...reloaded.serialise(), deviceSecret: 'not base64 of 32 bytes' });
      } catch (err) {
        return err;
      }
      return 'no error';
    })());

    // The agent gone, mid-life.
    await agent.stop();
    await bridge.certificates().catch((err) => record('the agent gone', err));
    await bridge.health().catch((err) => record('health with the agent gone', err));
    await bridge.signPdf(new Uint8Array([1])).catch((err) => record('a signature with the agent gone', err));
    await bridge.echo('{}').catch((err) => record('an echo with the agent gone', err));
  } finally {
    process.stdout.write = realOut;
    process.stderr.write = realErr;
  }

  assert.ok(collected.length > 40, `expected many paths exercised, got ${collected.length}`);
  for (const [where, text] of collected) {
    assertNoSecret(where, text);
  }
  assertNoSecret('stdout or stderr', written.join('\n'));

  // And the fixture is known to be capable of failing: the same search
  // over the secret itself finds it.
  assert.ok(SPELLINGS.every((s) => s.length > 0));
  assert.throws(() => assertNoSecret('a control', SECRET.toString('base64')), /device secret reached/);
});

test('nothing in the SDK’s own source interpolates the secret into a string', async () => {
  const { readFile, readdir } = await import('node:fs/promises');
  const { fileURLToPath } = await import('node:url');
  const { dirname, join } = await import('node:path');
  const src = join(dirname(fileURLToPath(import.meta.url)), '..', 'src');

  for (const name of await readdir(src)) {
    const text = await readFile(join(src, name), 'utf8');
    const code = text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '');
    for (const line of code.split('\n')) {
      const mentionsSecret = /deviceSecret|secretBytes|SECRET/.test(line);
      if (!mentionsSecret) {
        continue;
      }
      // The only places the secret's value is allowed to be turned into
      // text are `serialise()`, which exists for that purpose and is
      // named so, and the store that persists what it returns.
      const allowed =
        name === 'secrets.ts' || /pair\/confirm|deviceSecret\b.*(?:!==|===|typeof|:|\?)/.test(line);
      const interpolates = /\$\{[^}]*[sS]ecret[^}]*\}/.test(line);
      assert.ok(!interpolates || allowed, `${name} interpolates a secret into a string: ${line.trim()}`);
    }
  }
});
