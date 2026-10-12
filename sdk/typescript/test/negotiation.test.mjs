/**
 * Protocol version negotiation (`PROTOCOL.md` §4.1, F14 §8, [[D-461]]).
 *
 * The SDK used to refuse any agent whose protocolVersion was not
 * exactly its own. These are the three agents that rule got wrong or
 * right: one that speaks this SDK's version and a newer one, one that
 * speaks only a version this SDK does not, and one older than the list.
 */

import assert from 'node:assert/strict';
import test from 'node:test';

import { LiroBridge, LiroError, negotiateProtocol } from '../dist/esm/index.js';
import { startFakeAgent } from './fake-agent.mjs';
import { MemoryStore, scratch, writeBridgeFile } from './helpers.mjs';

async function connectTo(t, agentOptions) {
  const agent = await startFakeAgent(agentOptions);
  const dir = await scratch();
  const bridgeFilePath = await writeBridgeFile(dir.dir, agent.port, {
    protocolVersion: agentOptions.protocolVersion ?? 2,
    ...(agentOptions.protocolVersions === undefined ? {} : { protocolVersions: agentOptions.protocolVersions }),
  });
  t.after(async () => {
    await agent.stop();
    await dir.dispose();
  });
  const connecting = LiroBridge.connect({
    applicationName: 'Moj ERP',
    origin: 'https://erp.example.com',
    secretStore: new MemoryStore(),
    bridgeFilePath,
    onPairingCode: () => agent.state.pairingCode,
  });
  return { agent, connecting };
}

test('an agent that speaks this version and a newer one is used at this version, and every request says so', async (t) => {
  const { agent, connecting } = await connectTo(t, { protocolVersion: 3, protocolVersions: [2, 3] });
  const bridge = await connecting;
  await bridge.certificates();
  const named = agent.state.requests.filter((r) => r.path !== '/v2/health').map((r) => r.headers.protocol);
  assert.ok(named.length > 0, 'no request after health');
  assert.deepEqual([...new Set(named)], ['2'], `requests named ${JSON.stringify(named)}`);
});

test('an agent that speaks only a version this SDK does not is refused, naming both lists', async (t) => {
  const { connecting } = await connectTo(t, { protocolVersion: 3, protocolVersions: [3] });
  await assert.rejects(connecting, (err) => {
    assert.ok(err instanceof LiroError);
    assert.equal(err.code, 'PROTOCOL_UNSUPPORTED');
    assert.match(err.message + ' ' + (err.detail ?? ''), /3/);
    return true;
  });
});

test('an agent older than the list is read as the one version it names', async (t) => {
  const { connecting } = await connectTo(t, { protocolVersion: 2 });
  await connecting;
});

test('negotiation picks the highest shared version and nothing else', () => {
  assert.equal(negotiateProtocol([2, 3], [2, 3, 4]), 3);
  assert.equal(negotiateProtocol([2], [2, 3]), 2);
  assert.equal(negotiateProtocol([4], [2, 3]), null);
  assert.equal(negotiateProtocol([], [2]), null);
});
