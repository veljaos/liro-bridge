/**
 * Where the SDK refuses to run.
 *
 * The browser refusal is not a security boundary — nothing running
 * inside a page could be one, and the agent's own refusal (no CORS
 * headers, preflight answered 403) is what actually holds. It is here so
 * that a developer who tries it is stopped in ten seconds by a sentence
 * naming the reason, rather than after building a feature on top of an
 * opaque `TypeError: Failed to fetch`.
 */

import assert from 'node:assert/strict';
import test from 'node:test';

import { LiroBridge, LiroError, MINIMUM_NODE_MAJOR } from '../dist/esm/index.js';

/** Runs `fn` with browser-shaped globals in place, then takes them away. */
async function asBrowser(globals, fn) {
  const previous = new Map();
  for (const [name, value] of Object.entries(globals)) {
    previous.set(name, Object.getOwnPropertyDescriptor(globalThis, name));
    Object.defineProperty(globalThis, name, { value, configurable: true, writable: true });
  }
  try {
    await fn();
  } finally {
    for (const [name, descriptor] of previous) {
      if (descriptor === undefined) {
        delete globalThis[name];
      } else {
        Object.defineProperty(globalThis, name, descriptor);
      }
    }
  }
}

test('connect refuses to construct when a document is in scope', async () => {
  await asBrowser({ document: { title: 'a page' } }, async () => {
    await assert.rejects(
      LiroBridge.connect({ applicationName: 'Moj ERP', secretStore: {} }),
      (err) => {
        assert.ok(err instanceof LiroError);
        assert.equal(err.code, 'UNSUPPORTED_ENVIRONMENT');
        assert.match(err.message, /browser/i);
        assert.match(err.message, /every visitor has/);
        assert.match(err.message, /backend/i);
        return true;
      },
    );
  });
});

test('connect refuses when window is the global object, as it is in a page', async () => {
  await asBrowser({ window: globalThis }, async () => {
    await assert.rejects(LiroBridge.connect({ applicationName: 'x', secretStore: {} }), /browser/i);
  });
});

test('connect refuses in a service worker and in Deno', async () => {
  await asBrowser({ WorkerGlobalScope: function WorkerGlobalScope() {} }, async () => {
    await assert.rejects(LiroBridge.connect({ applicationName: 'x', secretStore: {} }), /browser/i);
  });
  await asBrowser({ Deno: { version: {} } }, async () => {
    await assert.rejects(LiroBridge.connect({ applicationName: 'x', secretStore: {} }), /browser/i);
  });
});

test('the refusal comes before anything else is looked at', async () => {
  // No applicationName, no secretStore, no agent running: the browser
  // check still wins, because a developer in a page needs to be told
  // about the page and not about their arguments.
  await asBrowser({ document: {} }, async () => {
    await assert.rejects(LiroBridge.connect({}), /browser/i);
  });
});

test('the minimum Node version is 18 and is stated', () => {
  assert.equal(MINIMUM_NODE_MAJOR, 18);
});

test('connect says what it needs when the caller leaves it out', async () => {
  await assert.rejects(LiroBridge.connect({ secretStore: {} }), (err) => {
    assert.equal(err.code, 'CONFIGURATION_INVALID');
    assert.match(err.message, /applicationName/);
    return true;
  });

  await assert.rejects(LiroBridge.connect({ applicationName: 'Moj ERP' }), (err) => {
    assert.equal(err.code, 'CONFIGURATION_INVALID');
    assert.match(err.message, /secretStore/);
    assert.match(err.message, /deliberately no default/);
    assert.match(err.message, /your decision to make/);
    return true;
  });
});

test('a missing agent is AGENT_NOT_RUNNING, and says not to scan for it', async () => {
  await assert.rejects(
    LiroBridge.connect({
      applicationName: 'Moj ERP',
      secretStore: { get: async () => null, set: async () => {}, clear: async () => {} },
      bridgeFilePath: 'C:/nothing/is/here/bridge.json',
    }),
    (err) => {
      assert.equal(err.code, 'AGENT_NOT_RUNNING');
      assert.match(err.message, /no discovery file/);
      assert.match(err.message, /Start Liro Bridge/);
      return true;
    },
  );
});

test('nothing in this SDK scans ports', async () => {
  const { readFile, readdir } = await import('node:fs/promises');
  const { fileURLToPath } = await import('node:url');
  const { dirname, join } = await import('node:path');
  const here = dirname(fileURLToPath(import.meta.url));
  const src = join(here, '..', 'src');

  // PROTOCOL.md §4: read the discovery file, never scan. Several people
  // can be signed in to one machine at once — an accounting firm over
  // RDP is the ordinary case — and each has their own agent on its own
  // port. Scanning finds somebody else's.
  //
  // Three things say this SDK does not: the agent's port range is not
  // written down anywhere in it, exactly one file turns a number into an
  // address, and nothing opens a socket of its own.
  const withoutComments = (text) => text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '');

  for (const name of await readdir(src)) {
    const text = await readFile(join(src, name), 'utf8');
    const code = withoutComments(text);
    for (let port = 17580; port <= 17590; port++) {
      assert.ok(!code.includes(String(port)), `${name} names ${port}, a port from the agent's range`);
    }
    assert.ok(!/from ['"]node:net['"]/.test(code), `${name} imports node:net`);
    assert.ok(!/from ['"]node:dgram['"]/.test(code), `${name} imports node:dgram`);
    if (name !== 'discovery.ts') {
      assert.ok(!code.includes('127.0.0.1'), `${name} builds a loopback address of its own`);
    }
  }

  const discovery = await readFile(join(src, 'discovery.ts'), 'utf8');
  const addresses = [...withoutComments(discovery).matchAll(/127\.0\.0\.1/g)];
  assert.equal(addresses.length, 1, 'discovery.ts builds more than one address');
  assert.match(discovery, /http:\/\/127\.0\.0\.1:\$\{port\}/, 'the one address is not built from the port passed in');
});
