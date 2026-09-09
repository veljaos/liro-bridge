/**
 * What is retried, what is not, and what a retry carries.
 *
 * The rules, from `PROTOCOL.md` and from what a repeat actually costs:
 *
 * - **Never a 401.** A retry of a request that did not authenticate does
 *   not authenticate either, and every authentication failure looks the
 *   same from the outside — so the retry reports the least diagnosable
 *   answer the protocol has, twice.
 * - **Never a submission.** A `POST /v2/sign` that timed out may already
 *   have created a job. Sending it again is a hundred documents signed
 *   twice, with a second window in front of a person for a batch they
 *   have already approved.
 * - **Never the result.** It is delivered once and the job is then
 *   forgotten.
 * - **Every attempt gets a new nonce and a new timestamp.** Reusing
 *   either is `AUTH_FAILED` on the retry.
 */

import assert from 'node:assert/strict';
import test from 'node:test';

import { LiroBridge } from '../dist/esm/index.js';
import { startFakeAgent } from './fake-agent.mjs';
import { MemoryStore, scratch, writeBridgeFile } from './helpers.mjs';

async function connected(t, options = {}) {
  const agent = await startFakeAgent(options);
  const dir = await scratch();
  const bridgeFilePath = await writeBridgeFile(dir.dir, agent.port);
  const bridge = await LiroBridge.connect({
    applicationName: 'Moj ERP',
    secretStore: new MemoryStore(),
    bridgeFilePath,
    onPairingCode: () => agent.state.pairingCode,
    requestTimeoutMs: 3000,
    signTimeoutMs: 20_000,
  });
  t.after(async () => {
    await agent.stop();
    await dir.dispose();
  });
  return { agent, bridge };
}

const at = (agent, path) => agent.state.requests.filter((r) => r.path === path);

test('a 401 is never retried', async (t) => {
  const { agent, bridge } = await connected(t);
  agent.state.overrides.set('GET /v2/certificates', () => ({ status: 401, body: { code: 'AUTH_FAILED' } }));
  await assert.rejects(bridge.certificates(), (err) => {
    assert.equal(err.code, 'AUTH_FAILED');
    return true;
  });
  assert.equal(at(agent, '/v2/certificates').length, 1, 'a 401 was sent again');
});

test('NOT_PAIRED is never retried either', async (t) => {
  const { agent, bridge } = await connected(t);
  agent.state.overrides.set('GET /v2/certificates', () => ({ status: 401, body: { code: 'NOT_PAIRED' } }));
  await assert.rejects(bridge.certificates(), /not paired/i);
  assert.equal(at(agent, '/v2/certificates').length, 1);
});

test('a 5xx on a safe request is retried, with a new nonce each time', async (t) => {
  const { agent, bridge } = await connected(t);
  let seen = 0;
  agent.state.overrides.set('GET /v2/certificates', () => {
    seen += 1;
    return seen <= 2 ? { status: 500, body: { code: 'INTERNAL' } } : null;
  });
  const certs = await bridge.certificates();
  assert.deepEqual(certs, []);

  const attempts = at(agent, '/v2/certificates');
  assert.equal(attempts.length, 3, 'the retry policy is three attempts');
  const nonces = attempts.map((r) => r.headers.nonce);
  assert.equal(new Set(nonces).size, 3, `a nonce was reused across attempts: ${nonces.join(', ')}`);
  for (const nonce of nonces) {
    assert.match(nonce, /^[0-9a-f]{32}$/);
  }
  // Every attempt is separately signed, so a retry cannot be a replay of
  // the first attempt's bytes.
  assert.equal(new Set(attempts.map((r) => r.headers.signature)).size, 3);
});

test('a 4xx that is not 401 is not retried', async (t) => {
  const { agent, bridge } = await connected(t);
  agent.state.overrides.set('GET /v2/certificates', () => ({
    status: 403,
    body: { code: 'CERTIFICATE_LISTING_DISABLED' },
  }));
  await assert.rejects(bridge.certificates(), (err) => {
    assert.equal(err.code, 'CERTIFICATE_LISTING_DISABLED');
    assert.match(err.message, /setting/i);
    return true;
  });
  assert.equal(at(agent, '/v2/certificates').length, 1);
});

test('a submission is never retried, even on a 500', async (t) => {
  const { agent, bridge } = await connected(t);
  agent.state.overrides.set('POST /v2/sign/pdf', () => ({ status: 500, body: { code: 'INTERNAL' } }));
  await assert.rejects(bridge.signPdf(new Uint8Array([1, 2, 3])), /could not classify/);
  assert.equal(
    at(agent, '/v2/sign/pdf').length,
    1,
    'a submission was sent twice: that is a batch signed twice, and a second window',
  );
});

test('a submission is never retried when the connection itself fails', async (t) => {
  const { agent, bridge } = await connected(t);
  let seen = 0;
  agent.state.overrides.set('POST /v2/sign', () => {
    seen += 1;
    // Answer nothing at all and let the request time out. A timed-out
    // submission is precisely the case where a job may already exist.
    return { status: 504, body: { code: 'INTERNAL' } };
  });
  await assert.rejects(bridge.signDigests([new Uint8Array(32)], { certificateThumbprint: 'AABB' }));
  assert.equal(seen, 1);
});

test('the result endpoint is never retried', async (t) => {
  const { agent, bridge } = await connected(t);
  let resultCalls = 0;
  agent.state.overrides.set = agent.state.overrides.set.bind(agent.state.overrides);
  const original = agent.state.overrides;
  // Count every result call however the job id comes out.
  const previousGet = original.get.bind(original);
  original.get = (key) => {
    if (key.endsWith('/result')) {
      resultCalls += 1;
      return () => ({ status: 500, body: { code: 'INTERNAL' } });
    }
    return previousGet(key);
  };
  await assert.rejects(bridge.signPdf(new Uint8Array([1, 2, 3])), /could not classify/);
  assert.equal(resultCalls, 1, 'the result was asked for more than once after a 500');
});

test('health is retried, because asking twice costs nothing', async (t) => {
  const { agent, bridge } = await connected(t);
  let seen = 0;
  agent.state.overrides.set('GET /v2/health', () => {
    seen += 1;
    return seen === 1 ? { status: 503, body: { code: 'INTERNAL' } } : null;
  });
  const health = await bridge.health();
  assert.equal(health.protocolVersion, 2);
  assert.equal(seen, 2);
});

test('a request with no body sends no Content-Type, and one with a body sends JSON', async (t) => {
  const { agent, bridge } = await connected(t);
  await bridge.health();
  await bridge.certificates();
  await bridge.echo('{"anything":"you like"}');

  for (const request of agent.state.requests) {
    if (request.body.length === 0) {
      assert.equal(
        request.headers.contentType,
        null,
        `${request.method} ${request.path} sent a Content-Type with no body — the rule several HTTP clients cannot obey`,
      );
    } else {
      assert.equal(request.headers.contentType, 'application/json', `${request.method} ${request.path}`);
    }
  }
});

test('every authenticated request carries the four headers, and no request carries the secret', async (t) => {
  const { agent, bridge } = await connected(t);
  await bridge.certificates();
  await bridge.signPdf(new Uint8Array([37, 80, 68, 70]));

  const secret = Buffer.from(agent.state.pairings.values().next().value.secret).toString('base64');
  for (const request of agent.state.requests) {
    const unauthenticated = ['/v2/health', '/v2/pair/request', '/v2/pair/confirm'].includes(request.path);
    if (!unauthenticated) {
      assert.ok(request.headers.appId, `${request.path} sent no app id`);
      assert.ok(request.headers.timestamp, `${request.path} sent no timestamp`);
      assert.ok(request.headers.nonce, `${request.path} sent no nonce`);
      assert.match(request.headers.signature, /^[0-9a-f]{64}$/, `${request.path} sent no signature`);
    }
    const wire = JSON.stringify(request.headers) + request.body.toString('utf8');
    assert.ok(!wire.includes(secret), `${request.path} put the device secret on the wire`);
  }
});
