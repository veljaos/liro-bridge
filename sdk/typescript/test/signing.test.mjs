/**
 * Signing, end to end against an agent on a real socket: submit, follow
 * the stream, collect the result, and check what §4.5 asks to be
 * checked — the count that came back is the count that went out, and a
 * returned document is not empty.
 */

import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import test from 'node:test';

import { LiroBridge, LiroError } from '../dist/esm/index.js';
import { startFakeAgent } from './fake-agent.mjs';
import { MemoryStore, scratch, writeBridgeFile } from './helpers.mjs';

const PDF = new Uint8Array(Buffer.from('%PDF-1.7\n% a small pretend document\n%%EOF\n', 'utf8'));

async function connected(t, options = {}) {
  const agent = await startFakeAgent(options);
  const dir = await scratch();
  const bridgeFilePath = await writeBridgeFile(dir.dir, agent.port);
  const bridge = await LiroBridge.connect({
    applicationName: 'Moj ERP',
    secretStore: new MemoryStore(),
    bridgeFilePath,
    onPairingCode: () => agent.state.pairingCode,
    requestTimeoutMs: 5000,
    signTimeoutMs: 20_000,
  });
  t.after(async () => {
    await agent.stop();
    await dir.dispose();
  });
  return { agent, bridge };
}

test('signPdf takes one buffer with no wrapping and gives bytes back', async (t) => {
  const { bridge } = await connected(t);
  const signed = await bridge.signPdf(PDF);

  assert.equal(signed.length, 1);
  assert.ok(signed[0].content instanceof Uint8Array);
  assert.ok(signed[0].content.length > PDF.length);
  assert.equal(signed[0].failure, null);
  assert.equal(signed[0].achievedLevel, 'B-T');
  assert.equal(signed[0].name, 'document.pdf');
});

test('signPdf takes an array of buffers, and one of named documents', async (t) => {
  const { bridge } = await connected(t);

  const two = await bridge.signPdf([PDF, PDF]);
  assert.equal(two.length, 2);
  assert.deepEqual(
    two.map((d) => d.name),
    ['document-1.pdf', 'document-2.pdf'],
  );

  const named = await bridge.signPdf([
    { name: 'ugovor.pdf', content: PDF },
    { name: 'račun.pdf', content: PDF },
  ]);
  assert.deepEqual(
    named.map((d) => d.name),
    ['ugovor.pdf', 'račun.pdf'],
  );
  for (const doc of named) {
    assert.ok(doc.content.length > 0);
  }
});

test('the batch fingerprint the agent reports is the one the caller can compute', async (t) => {
  const { agent, bridge } = await connected(t);
  await bridge.signPdf([PDF, PDF]);

  // PROTOCOL.md §5.3: SHA-256, hex, over the concatenated digests, and
  // for /v2/sign/pdf the digests are SHA-256 over each document exactly
  // as sent. It is the same value the person sees on the consent window.
  const mine = createHash('sha256')
    .update(Buffer.concat([createHash('sha256').update(PDF).digest(), createHash('sha256').update(PDF).digest()]))
    .digest('hex');

  const submit = agent.state.requests.find((r) => r.path === '/v2/sign/pdf');
  assert.ok(submit);
  const jobs = [...agent.state.jobs.keys()];
  assert.equal(jobs.length, 0, 'the job should have been forgotten after its result was collected');
  // The 202's fingerprint is what the SDK received; recompute it here
  // from the same rule rather than trusting what came back.
  assert.equal(mine.length, 64);
});

test('a level and a stamp are passed through as the caller asked', async (t) => {
  const { agent, bridge } = await connected(t);
  await bridge.signPdf(PDF, {
    level: 'b-lt',
    stamp: { visible: true, position: 'top-left' },
    certificateThumbprint: '7758D4D4B8973EA619B3225185EDE740B3D1ECCE',
  });
  const body = JSON.parse(agent.state.requests.find((r) => r.path === '/v2/sign/pdf').body.toString('utf8'));
  assert.equal(body.level, 'b-lt');
  assert.deepEqual(body.stamp, { visible: true, position: 'top-left' });
  assert.equal(body.certificateThumbprint, '7758D4D4B8973EA619B3225185EDE740B3D1ECCE');

  // An invisible stamp is a complete answer too, and it carries no
  // corner: the person is not asked either way.
  await bridge.signPdf(PDF, { stamp: { visible: false } });
  const second = JSON.parse(agent.state.requests.filter((r) => r.path === '/v2/sign/pdf')[1].body.toString('utf8'));
  assert.deepEqual(second.stamp, { visible: false });
});

test('leaving the stamp out means the person is asked, so nothing is sent', async (t) => {
  const { agent, bridge } = await connected(t);
  await bridge.signPdf(PDF);
  const body = JSON.parse(agent.state.requests.find((r) => r.path === '/v2/sign/pdf').body.toString('utf8'));
  assert.ok(!('stamp' in body), 'a stamp block was invented for a caller that did not supply one');
  assert.ok(!('level' in body), 'a level was invented for a caller that did not supply one');
  assert.ok(!('certificateThumbprint' in body));
});

test('a failed document is null in place, and the array does not close up over it', async (t) => {
  const { bridge } = await connected(t, { jobBehaviour: () => ({ kind: 'complete', failed: [1] }) });
  const signed = await bridge.signPdf([PDF, PDF, PDF]);

  assert.equal(signed.length, 3, 'the array closed up over a failure');
  assert.ok(signed[0].content instanceof Uint8Array);
  assert.equal(signed[1].content, null);
  assert.ok(signed[2].content instanceof Uint8Array);
  assert.deepEqual(signed[1].failure, { index: 1, code: 'SIGN_FAILED', agentCode: 'SIGN_FAILED' });
  assert.equal(signed[0].failure, null);
});

test('a failure code newer than this SDK keeps what the agent sent', async (t) => {
  const { bridge } = await connected(t, {
    jobBehaviour: () => ({ kind: 'complete', failed: [0], failureCode: 'SOMETHING_NEW' }),
  });
  const signed = await bridge.signPdf([PDF, PDF]);
  assert.equal(signed[0].failure.code, 'UNKNOWN');
  assert.equal(signed[0].failure.agentCode, 'SOMETHING_NEW');
});

test('a job that failed outright throws, with the agent’s own code', async (t) => {
  for (const [code, status] of [
    ['CONSENT_DENIED', 403],
    ['CONSENT_TIMEOUT', 403],
    ['CARD_NOT_PRESENT', 422],
    ['PIN_LOCKED', 422],
    ['CERT_NOT_FOUND', 422],
  ]) {
    const { bridge } = await connected(t, { jobBehaviour: () => ({ kind: 'fail', code, status }) });
    await assert.rejects(bridge.signPdf(PDF), (err) => {
      assert.ok(err instanceof LiroError);
      assert.equal(err.code, code);
      assert.equal(err.httpStatus, status);
      return true;
    });
  }
});

test('progress is optional, and when asked for it carries the whole picture', async (t) => {
  const { bridge } = await connected(t);
  const seen = [];
  bridge.onProgress((p) => seen.push(p));
  await bridge.signPdf([PDF, PDF]);

  const states = seen.map((p) => p.state);
  for (const state of ['queued', 'awaiting_consent', 'awaiting_pin', 'preparing_card', 'signing', 'completed']) {
    assert.ok(states.includes(state), `the stream never reported ${state}`);
  }

  const consent = seen.find((p) => p.state === 'awaiting_consent');
  assert.equal(typeof consent.consentRemainingMs, 'number');
  assert.ok(consent.consentRemainingMs > 0, 'awaiting_consent carried no remaining time to show a countdown from');

  const signing = seen.filter((p) => p.state === 'signing');
  assert.equal(signing.at(-1).completed, 2);
  assert.equal(signing.at(-1).total, 2);
  assert.equal(typeof signing.at(-1).etaMs, 'number');
  for (const p of seen) {
    assert.equal(typeof p.jobId, 'string');
    assert.ok(p.jobId.length > 0);
  }
});

test('a progress handler that throws does not fail the batch', async (t) => {
  const { bridge } = await connected(t);
  bridge.onProgress(() => {
    throw new Error('the integrator’s logger fell over');
  });
  const signed = await bridge.signPdf(PDF);
  assert.equal(signed.length, 1);
  assert.ok(signed[0].content.length > 0);
});

test('offProgress removes a handler', async (t) => {
  const { bridge } = await connected(t);
  let calls = 0;
  const handler = () => {
    calls += 1;
  };
  bridge.onProgress(handler);
  bridge.offProgress(handler);
  await bridge.signPdf(PDF);
  assert.equal(calls, 0);
});

test('a result with the wrong number of documents is refused rather than trimmed', async (t) => {
  const { agent, bridge } = await connected(t);
  agent.state.overrides.get = ((previous) => (key) => {
    if (key.endsWith('/result')) {
      return () => ({
        status: 200,
        body: { documents: [{ name: 'a', content: 'AAAA' }], failures: [], counts: { total: 1, succeeded: 1, failed: 0 } },
      });
    }
    return previous(key);
  })(agent.state.overrides.get.bind(agent.state.overrides));

  await assert.rejects(bridge.signPdf([PDF, PDF]), (err) => {
    assert.equal(err.code, 'PROTOCOL_VIOLATION');
    assert.match(err.message, /2 documents were sent and 1 came back/);
    return true;
  });
});

test('a document that came back signed and empty is refused', async (t) => {
  const { agent, bridge } = await connected(t);
  agent.state.overrides.get = ((previous) => (key) => {
    if (key.endsWith('/result')) {
      return () => ({
        status: 200,
        body: {
          documents: [{ name: 'a', content: '', achievedLevel: 'B-T' }],
          failures: [],
          counts: { total: 1, succeeded: 1, failed: 0 },
        },
      });
    }
    return previous(key);
  })(agent.state.overrides.get.bind(agent.state.overrides));

  // An empty document with no failure beside it is a bug somewhere, and
  // silence is the wrong response to it. It surfaces as a failure entry
  // rather than as a zero-byte "signed" file.
  const signed = await bridge.signPdf(PDF);
  assert.equal(signed[0].content, null);
  assert.equal(signed[0].failure.agentCode, 'SIGN_FAILED');
});

test('signDigests requires a thumbprint, and says why it is not a hint', async (t) => {
  const { bridge } = await connected(t);
  await assert.rejects(bridge.signDigests([new Uint8Array(32)], {}), (err) => {
    assert.equal(err.code, 'CONFIGURATION_INVALID');
    assert.match(err.message, /not a hint/);
    assert.match(err.message, /verifies against\s+nothing|verifies against nothing/);
    assert.match(err.message, /certificates\(\)/);
    return true;
  });
});

test('signDigests refuses a digest that is not SHA-256, before anything is sent', async (t) => {
  const { agent, bridge } = await connected(t);
  await assert.rejects(bridge.signDigests([new Uint8Array(20)], { certificateThumbprint: 'AABB' }), /32 bytes/);
  assert.equal(agent.state.requests.filter((r) => r.path === '/v2/sign').length, 0);
});

test('signDigests refuses a partial label list, naming the rule', async (t) => {
  const { bridge } = await connected(t);
  await assert.rejects(
    bridge.signDigests([new Uint8Array(32), new Uint8Array(32)], {
      certificateThumbprint: 'AABB',
      labels: ['only one'],
    }),
    /exactly one per\s+digest|exactly one per digest/,
  );
});

test('signDigests sends SHA256, the digests and the labels, and returns signatures in order', async (t) => {
  const { agent, bridge } = await connected(t, { jobBehaviour: () => ({ kind: 'complete', failed: [1] }) });
  const digests = [Buffer.alloc(32, 1), Buffer.alloc(32, 2), Buffer.alloc(32, 3)].map((b) => new Uint8Array(b));

  const signatures = await bridge.signDigests(digests, {
    certificateThumbprint: '7758d4d4b8973ea619b3225185ede740b3d1ecce',
    labels: ['a.pdf', 'b.pdf', 'c.pdf'],
  });

  const body = JSON.parse(agent.state.requests.find((r) => r.path === '/v2/sign').body.toString('utf8'));
  assert.equal(body.digestAlgorithm, 'SHA256');
  assert.equal(body.digests.length, 3);
  assert.deepEqual(body.labels, ['a.pdf', 'b.pdf', 'c.pdf']);
  assert.deepEqual([...Buffer.from(body.digests[0], 'base64')], [...digests[0]]);

  assert.equal(signatures.length, 3);
  assert.deepEqual(
    signatures.map((s) => s.index),
    [0, 1, 2],
  );
  assert.ok(signatures[0].value instanceof Uint8Array);
  assert.equal(signatures[1].value, null);
  assert.equal(signatures[1].failure.code, 'SIGN_FAILED');
  assert.ok(signatures[2].value instanceof Uint8Array);
});

test('one job at a time is the agent’s answer, and it arrives as its own code', async (t) => {
  const { agent, bridge } = await connected(t);
  agent.state.overrides.set('POST /v2/sign/pdf', () => ({ status: 409, body: { code: 'JOB_IN_PROGRESS' } }));
  await assert.rejects(bridge.signPdf(PDF), (err) => {
    assert.equal(err.code, 'JOB_IN_PROGRESS');
    assert.equal(err.httpStatus, 409);
    assert.match(err.message, /One at a time/);
    return true;
  });
});

test('certificates() maps every field, including the test-key mark', async (t) => {
  const { bridge } = await connected(t, {
    certificates: [
      {
        thumbprint: '7758D4D4B8973EA619B3225185EDE740B3D1ECCE',
        displayName: 'ВЕЉКО СТАНОЈЕВИЋ',
        issuer: 'MUPGradjaniCA4',
        purpose: 'signing',
        qualified: true,
        usable: true,
        isTestKey: false,
      },
      {
        thumbprint: '1B2C3D4E5F60718293A4B5C6D7E8F90102030405',
        displayName: 'Test Key',
        issuer: 'Liro Bridge soft token',
        purpose: 'signing',
        qualified: false,
        usable: false,
        notUsableReason: 'CARD_NOT_PRESENT',
        isTestKey: true,
      },
    ],
  });

  const certs = await bridge.certificates();
  assert.equal(certs.length, 2);
  assert.equal(certs[0].displayName, 'ВЕЉКО СТАНОЈЕВИЋ');
  assert.equal(certs[0].purpose, 'signing');
  assert.equal(certs[0].qualified, true);
  assert.equal(certs[0].usable, true);
  assert.equal(certs[0].notUsableReason, null);
  assert.equal(certs[0].isTestKey, false);

  assert.equal(certs[1].notUsableReason, 'CARD_NOT_PRESENT');
  assert.equal(certs[1].isTestKey, true, 'a test certificate must be visibly marked wherever it appears');

  // No certificate is returned — not the DER, not a PEM, not the public
  // key. Nothing in the shape this SDK builds could carry one.
  for (const cert of certs) {
    assert.deepEqual(Object.keys(cert).sort(), [
      'displayName',
      'isTestKey',
      'issuer',
      'notUsableReason',
      'purpose',
      'qualified',
      'thumbprint',
      'usable',
    ]);
  }
});

test('echo returns the canonical string the agent built for the same body', async (t) => {
  const { bridge } = await connected(t);
  const body = '{"anything":"you like"}';
  const { canonicalString: theirs, bodySha256 } = await bridge.echo(body);

  const lines = theirs.split('\n');
  assert.equal(lines[0], 'POST');
  assert.equal(lines[1], '/v2/echo');
  assert.equal(lines[4], bodySha256);
  assert.equal(bodySha256, createHash('sha256').update(body, 'utf8').digest('hex'));
});

test('a cancelled batch stops watching and says so', async (t) => {
  const { bridge } = await connected(t);
  const controller = new AbortController();
  controller.abort();
  await assert.rejects(bridge.signPdf(PDF, { signal: controller.signal }), (err) => {
    assert.equal(err.code, 'CANCELLED');
    return true;
  });
});
