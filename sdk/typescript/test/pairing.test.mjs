/**
 * Pairing, against an agent that speaks the protocol over a real socket.
 */

import assert from 'node:assert/strict';
import test from 'node:test';

import { LiroBridge, LiroError } from '../dist/esm/index.js';
import { startFakeAgent } from './fake-agent.mjs';
import { MemoryStore, NaiveJsonStore, scratch, writeBridgeFile } from './helpers.mjs';

/** Starts an agent and a scratch directory, and cleans both up. */
async function withAgent(t, options = {}) {
  const agent = await startFakeAgent(options);
  const dir = await scratch();
  const bridgeFilePath = await writeBridgeFile(dir.dir, agent.port);
  t.after(async () => {
    await agent.stop();
    await dir.dispose();
  });
  return { agent, dir, bridgeFilePath };
}

test('the first connect pairs, and the second does not ask again', async (t) => {
  const { agent, bridgeFilePath } = await withAgent(t);
  const store = new MemoryStore();
  let asked = 0;

  const bridge = await LiroBridge.connect({
    applicationName: 'Moj ERP',
    origin: 'https://erp.example.com',
    secretStore: store,
    bridgeFilePath,
    onPairingCode: () => {
      asked += 1;
      return agent.state.pairingCode;
    },
  });

  assert.equal(asked, 1);
  assert.equal(store.setCalls, 1);
  assert.equal(bridge.pairingInfo.applicationName, 'Moj ERP');
  assert.equal(bridge.pairingInfo.origin, 'https://erp.example.com');

  const again = await LiroBridge.connect({
    applicationName: 'Moj ERP',
    origin: 'https://erp.example.com',
    secretStore: store,
    bridgeFilePath,
    onPairingCode: () => {
      asked += 1;
      return agent.state.pairingCode;
    },
  });

  assert.equal(asked, 1, 'onPairingCode was called on a run that already had a secret');
  assert.equal(again.pairingInfo.appId, bridge.pairingInfo.appId);
});

test('the same origin is sent in both calls', async (t) => {
  const { agent, bridgeFilePath } = await withAgent(t);
  await LiroBridge.connect({
    applicationName: 'Moj ERP',
    origin: 'https://erp.example.com',
    secretStore: new MemoryStore(),
    bridgeFilePath,
    onPairingCode: () => agent.state.pairingCode,
  });

  const request = agent.state.requests.find((r) => r.path === '/v2/pair/request');
  const confirm = agent.state.requests.find((r) => r.path === '/v2/pair/confirm');
  assert.ok(request && confirm);
  const sentOrigin = JSON.parse(request.body.toString('utf8')).origin;
  const confirmedOrigin = JSON.parse(confirm.body.toString('utf8')).origin;
  assert.equal(sentOrigin, 'https://erp.example.com');
  assert.equal(
    confirmedOrigin,
    sentOrigin,
    'pair/confirm must carry the same origin, byte for byte — the mistake this SDK exists to remove',
  );
});

test('the default origin is "local", and it is what the agent binds', async (t) => {
  const { agent, bridgeFilePath } = await withAgent(t);
  const bridge = await LiroBridge.connect({
    applicationName: 'Moj ERP',
    secretStore: new MemoryStore(),
    bridgeFilePath,
    onPairingCode: () => agent.state.pairingCode,
  });
  assert.equal(bridge.pairingInfo.origin, 'local');
  const confirm = agent.state.requests.find((r) => r.path === '/v2/pair/confirm');
  assert.equal(JSON.parse(confirm.body.toString('utf8')).origin, 'local');
});

test('a wrong code reports how many attempts are left and asks again', async (t) => {
  const { agent, bridgeFilePath } = await withAgent(t);
  const prompts = [];

  await LiroBridge.connect({
    applicationName: 'Moj ERP',
    secretStore: new MemoryStore(),
    bridgeFilePath,
    onPairingCode: (prompt) => {
      prompts.push(prompt);
      // Two wrong, then the right one.
      return prompts.length <= 2 ? '000000' : agent.state.pairingCode;
    },
  });

  assert.equal(prompts.length, 3);
  assert.deepEqual(
    prompts.map((p) => [p.attempt, p.attemptsRemaining]),
    [
      [1, null],
      [2, 4],
      [3, 3],
    ],
  );
  assert.equal(prompts[0].applicationName, 'Moj ERP');
  assert.equal(prompts[0].expiresInSeconds, 300);
  assert.equal(
    agent.state.requests.filter((r) => r.path === '/v2/pair/request').length,
    1,
    'a wrong code must not start a new pairing request',
  );
});

test('five wrong codes void the request, and that is its own error', async (t) => {
  const { bridgeFilePath } = await withAgent(t);
  await assert.rejects(
    LiroBridge.connect({
      applicationName: 'Moj ERP',
      secretStore: new MemoryStore(),
      bridgeFilePath,
      onPairingCode: () => '000000',
    }),
    (err) => {
      assert.ok(err instanceof LiroError);
      assert.equal(err.code, 'PAIRING_EXPIRED');
      assert.match(err.message, /five wrong codes/i);
      assert.match(err.message, /Start a new one/i);
      return true;
    },
  );
});

test('a busy pairing window, a refusal and an origin mismatch are three different errors', async (t) => {
  for (const [code, status, expected] of [
    ['PAIRING_IN_PROGRESS', 409, /Another application/],
    ['PAIRING_DENIED', 403, /refused the pairing/],
    ['PAIRING_ORIGIN_MISMATCH', 403, /byte for byte/],
    ['RATE_LIMITED', 429, /Too many pairing requests/],
  ]) {
    const { agent, bridgeFilePath } = await withAgent(t);
    agent.state.overrides.set('POST /v2/pair/request', () => ({ status, body: { code } }));
    await assert.rejects(
      LiroBridge.connect({
        applicationName: 'Moj ERP',
        secretStore: new MemoryStore(),
        bridgeFilePath,
        onPairingCode: () => '123456',
      }),
      (err) => {
        assert.equal(err.code, code);
        assert.match(err.message, expected);
        return true;
      },
    );
  }
});

test('a code read out with spaces or dashes still works', async (t) => {
  for (const spelling of ['481 516', '481-516', ' 481516 ', '481–516']) {
    const { agent, bridgeFilePath } = await withAgent(t);
    const bridge = await LiroBridge.connect({
      applicationName: 'Moj ERP',
      secretStore: new MemoryStore(),
      bridgeFilePath,
      onPairingCode: () => spelling,
    });
    assert.ok(bridge.pairingInfo.appId.length > 0, `${JSON.stringify(spelling)} was not accepted`);
  }
});

test('something that is not six digits is refused before it reaches the agent', async (t) => {
  const { agent, bridgeFilePath } = await withAgent(t);
  await assert.rejects(
    LiroBridge.connect({
      applicationName: 'Moj ERP',
      secretStore: new MemoryStore(),
      bridgeFilePath,
      onPairingCode: () => 'four hundred',
    }),
    (err) => {
      assert.equal(err.code, 'CONFIGURATION_INVALID');
      assert.match(err.message, /six digits/);
      return true;
    },
  );
  assert.equal(
    agent.state.requests.filter((r) => r.path === '/v2/pair/confirm').length,
    0,
    'a code that cannot be right must not spend one of the five attempts',
  );
});

test('connect says what to do when there is no stored pairing and no way to ask', async (t) => {
  const { bridgeFilePath } = await withAgent(t);
  await assert.rejects(
    LiroBridge.connect({ applicationName: 'Moj ERP', secretStore: new MemoryStore(), bridgeFilePath }),
    (err) => {
      assert.equal(err.code, 'CONFIGURATION_INVALID');
      assert.match(err.message, /onPairingCode/);
      assert.match(err.message, /travel through them/);
      return true;
    },
  );
});

test('a store that persisted JSON.stringify fails at connect, naming the fix', async (t) => {
  const { agent, bridgeFilePath } = await withAgent(t);
  const store = new NaiveJsonStore();
  await LiroBridge.connect({
    applicationName: 'Moj ERP',
    secretStore: store,
    bridgeFilePath,
    onPairingCode: () => agent.state.pairingCode,
  });
  assert.ok(store.text !== null, 'the naive store kept nothing at all');

  // The next run reads it back, and this is where the mistake surfaces —
  // at connect, with a sentence, rather than as AUTH_FAILED on a
  // signature somebody is waiting for.
  await assert.rejects(
    LiroBridge.connect({
      applicationName: 'Moj ERP',
      secretStore: store,
      bridgeFilePath,
      onPairingCode: () => agent.state.pairingCode,
    }),
    (err) => {
      assert.equal(err.code, 'SECRET_STORE_INVALID');
      assert.match(err.message, /serialise\(\)/);
      return true;
    },
  );
});

test('pairing is never retried, however the agent misbehaves', async (t) => {
  const { agent, bridgeFilePath } = await withAgent(t);
  // Only pair/request fails — a 500, which is the one status a retryable
  // request would be sent again for.
  agent.state.overrides.set('POST /v2/pair/request', () => ({ status: 500, body: { code: 'INTERNAL' } }));
  await assert.rejects(
    LiroBridge.connect({
      applicationName: 'Moj ERP',
      secretStore: new MemoryStore(),
      bridgeFilePath,
      onPairingCode: () => agent.state.pairingCode,
    }),
    /INTERNAL|could not classify/,
  );
  assert.equal(
    agent.state.requests.filter((r) => r.path === '/v2/pair/request').length,
    1,
    'pair/request was sent more than once: a repeat opens a second window on somebody’s screen',
  );
});

test('disconnect forgets the secret locally and says nothing to the agent', async (t) => {
  const { agent, bridgeFilePath } = await withAgent(t);
  const store = new MemoryStore();
  const bridge = await LiroBridge.connect({
    applicationName: 'Moj ERP',
    secretStore: store,
    bridgeFilePath,
    onPairingCode: () => agent.state.pairingCode,
  });
  const before = agent.state.requests.length;
  await bridge.disconnect();
  assert.equal(store.clearCalls, 1);
  assert.equal(await store.get('Moj ERP'), null);
  assert.equal(agent.state.requests.length, before, 'disconnect talked to the agent; the protocol has no unpair call');
});
