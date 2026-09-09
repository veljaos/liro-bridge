/**
 * A stand-in agent that speaks the protocol from `PROTOCOL.md`, over a
 * real loopback socket, with real HMAC verification.
 *
 * It is deliberately written from the document rather than from the
 * SDK: it computes its own canonical string, its own body hash and its
 * own signature with Node's `crypto` and compares against what arrived.
 * A test whose fake agent shared the SDK's own `canonicalString` would
 * be green for the reason decision D-044 names for verifiers — a bug in
 * a shared helper passes in both directions and the test proves nothing.
 *
 * It is not a substitute for running against the real binary. It cannot
 * be: it has no consent window, no card and no opinion about anything.
 * What it is for is the cases a real agent cannot be made to produce on
 * demand — a 500, a reused nonce, five wrong pairing codes, every error
 * code in §7.1 — and for making every one of them a repeatable test
 * rather than a story about one afternoon.
 */

import { createHash, createHmac, randomBytes, randomUUID, timingSafeEqual } from 'node:crypto';
import { createServer } from 'node:http';

const EMPTY_BODY_SHA256 = 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855';

/** Starts a fake agent. Returns `{ port, url, stop, state }`. */
export async function startFakeAgent(options = {}) {
  const state = {
    /** Every request that arrived, in order, with its four auth headers. */
    requests: [],
    /** The pairings it has issued, by appId. */
    pairings: new Map(),
    /** Pairing requests in flight, by requestId. */
    pairingRequests: new Map(),
    /** Jobs, by id. */
    jobs: new Map(),
    /** Nonces already spent, as `appId nonce`. */
    nonces: new Set(),
    /** Set to a number to answer the next N requests with that status. */
    failNext: null,
    /** The six digits the next pairing will accept. */
    pairingCode: options.pairingCode ?? '481516',
    /** How many wrong codes are left before the request is void. */
    codeAttempts: 5,
    /** What the agent's version reports. */
    agentVersion: options.agentVersion ?? '1.4.0-fake',
    protocolVersion: options.protocolVersion ?? 2,
    minimumClientVersion: options.minimumClientVersion ?? '0.0.0',
    certificates: options.certificates ?? [],
    /** The 32 bytes the next pairing issues. */
    deviceSecret: options.deviceSecret ?? null,
    /** Overrides, keyed by `METHOD path`, returning `{status, body}` or null. */
    overrides: new Map(),
    /** How each submitted job behaves. */
    jobBehaviour: options.jobBehaviour ?? (() => ({ kind: 'complete' })),
  };

  const server = createServer((req, res) => {
    void handle(req, res, state).catch((err) => {
      res.writeHead(500, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ code: 'INTERNAL', details: { fake: String(err) } }));
    });
  });

  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const port = server.address().port;

  return {
    port,
    url: `http://127.0.0.1:${port}`,
    state,
    async stop() {
      for (const job of state.jobs.values()) {
        job.finished = true;
      }
      await new Promise((resolve) => server.close(resolve));
    },
  };
}

async function handle(req, res, state) {
  const body = await readBody(req);
  const path = req.url.split('?')[0];
  const record = {
    method: req.method,
    path,
    body,
    headers: {
      appId: req.headers['x-liro-app-id'] ?? null,
      timestamp: req.headers['x-liro-timestamp'] ?? null,
      nonce: req.headers['x-liro-nonce'] ?? null,
      signature: req.headers['x-liro-signature'] ?? null,
      contentType: req.headers['content-type'] ?? null,
    },
  };
  state.requests.push(record);

  if (req.method === 'OPTIONS') {
    return json(res, 403, { code: 'REQUEST_INVALID', details: { field: 'preflight' } });
  }
  if (state.failNext !== null && state.failNext.remaining > 0) {
    state.failNext.remaining -= 1;
    return json(res, state.failNext.status, { code: state.failNext.code ?? 'INTERNAL' });
  }
  const override = state.overrides.get(`${req.method} ${path}`);
  if (override !== undefined) {
    const answer = override(record, state);
    if (answer !== null && answer !== undefined) {
      return json(res, answer.status, answer.body);
    }
  }

  // A request with a body must declare JSON; a request without one must
  // not send a Content-Type at all — PROTOCOL.md §2.5. Both halves are
  // checked, because the second is what a real client got wrong.
  if (body.length > 0 && record.headers.contentType !== 'application/json') {
    return json(res, 400, { code: 'REQUEST_INVALID', details: { expectedContentType: 'application/json' } });
  }

  switch (`${req.method} ${path}`) {
    case 'GET /v2/health':
      return json(res, 200, {
        agentVersion: state.agentVersion,
        protocolVersion: state.protocolVersion,
        minimumClientVersion: state.minimumClientVersion,
      });
    case 'POST /v2/pair/request':
      return pairRequest(res, body, state);
    case 'POST /v2/pair/confirm':
      return pairConfirm(res, body, state);
    default:
      break;
  }

  const pairing = authenticate(record, state);
  if (pairing.error !== null) {
    return json(res, pairing.status, pairing.error);
  }

  switch (`${req.method} ${path}`) {
    case 'GET /v2/certificates':
      return json(res, 200, { certificates: state.certificates });
    case 'POST /v2/echo':
      return json(res, 200, {
        canonicalString: canonical(req.method, path, record.headers.timestamp, record.headers.nonce, body),
        bodySha256: sha256Hex(body),
      });
    case 'POST /v2/sign':
    case 'POST /v2/sign/pdf':
      return submit(res, path, body, pairing.appId, state);
    default:
      break;
  }

  const events = /^\/v2\/jobs\/([A-Za-z0-9_-]+)\/events$/.exec(path);
  if (events !== null && req.method === 'GET') {
    return streamEvents(res, events[1], pairing.appId, state);
  }
  const result = /^\/v2\/jobs\/([A-Za-z0-9_-]+)\/result$/.exec(path);
  if (result !== null && req.method === 'GET') {
    return jobResult(res, result[1], pairing.appId, state);
  }

  return json(res, 404, { code: 'JOB_NOT_FOUND' });
}

function pairRequest(res, body, state) {
  let parsed;
  try {
    parsed = JSON.parse(body.toString('utf8'));
  } catch {
    return json(res, 400, { code: 'REQUEST_INVALID' });
  }
  if (typeof parsed.origin !== 'string' || parsed.origin === '') {
    return json(res, 400, { code: 'REQUEST_INVALID', details: { field: 'origin' } });
  }
  if (/[\s\u0000-\u001F\u007F\u202A-\u202E\u2066-\u2069]/u.test(parsed.origin)) {
    return json(res, 400, { code: 'REQUEST_INVALID', details: { field: 'origin' } });
  }
  const requestId = randomUUID().replace(/-/g, '');
  state.codeAttempts = 5;
  state.pairingRequests.set(requestId, {
    applicationName: String(parsed.applicationName ?? '').slice(0, 120),
    origin: parsed.origin,
  });
  return json(res, 200, { requestId, expiresInSeconds: 300 });
}

function pairConfirm(res, body, state) {
  let parsed;
  try {
    parsed = JSON.parse(body.toString('utf8'));
  } catch {
    return json(res, 400, { code: 'REQUEST_INVALID' });
  }
  const pending = state.pairingRequests.get(parsed.requestId);
  if (pending === undefined) {
    return json(res, 410, { code: 'PAIRING_EXPIRED' });
  }
  // The origin is compared, and a confirm that leaves it out is refused
  // — PROTOCOL.md §2.1. This is the mistake the SDK is meant to make
  // impossible, so the fake agent is strict about it.
  if (typeof parsed.origin !== 'string' || parsed.origin !== pending.origin) {
    return json(res, 403, { code: 'PAIRING_ORIGIN_MISMATCH' });
  }
  if (parsed.code !== state.pairingCode) {
    state.codeAttempts -= 1;
    if (state.codeAttempts <= 0) {
      state.pairingRequests.delete(parsed.requestId);
      return json(res, 410, { code: 'PAIRING_EXPIRED' });
    }
    return json(res, 401, {
      code: 'PAIRING_CODE_INCORRECT',
      details: { attemptsRemaining: state.codeAttempts },
    });
  }
  state.pairingRequests.delete(parsed.requestId);
  const appId = randomUUID().replace(/-/g, '');
  const secret = state.deviceSecret ?? randomBytes(32);
  state.pairings.set(appId, { secret, origin: pending.origin, applicationName: pending.applicationName });
  return json(res, 200, {
    appId,
    deviceSecret: secret.toString('base64'),
    applicationName: pending.applicationName,
    origin: pending.origin,
  });
}

/** Verifies the four headers, exactly as PROTOCOL.md §3 describes. */
function authenticate(record, state) {
  const { appId, timestamp, nonce, signature } = record.headers;
  if (appId === null || timestamp === null || nonce === null || signature === null) {
    return { error: { code: 'AUTH_FAILED' }, status: 401, appId: null };
  }
  const pairing = state.pairings.get(appId);
  if (pairing === undefined) {
    return { error: { code: 'NOT_PAIRED' }, status: 401, appId: null };
  }
  if (nonce.length > 128) {
    return { error: { code: 'AUTH_FAILED' }, status: 401, appId: null };
  }
  const seconds = Number.parseInt(timestamp, 10);
  if (!Number.isFinite(seconds) || Math.abs(Date.now() / 1000 - seconds) > 60) {
    return { error: { code: 'AUTH_FAILED' }, status: 401, appId: null };
  }
  const want = createHmac('sha256', pairing.secret)
    .update(canonical(record.method, record.path, timestamp, nonce, record.body), 'utf8')
    .digest();
  let presented;
  try {
    presented = Buffer.from(signature, 'hex');
  } catch {
    presented = Buffer.alloc(0);
  }
  if (presented.length !== want.length || !timingSafeEqual(presented, want)) {
    return { error: { code: 'AUTH_FAILED' }, status: 401, appId: null };
  }
  // Spent last, after the signature verified — so a forged request
  // cannot consume a nonce a real client is about to use.
  const key = `${appId} ${nonce}`;
  if (state.nonces.has(key)) {
    return { error: { code: 'AUTH_FAILED' }, status: 401, appId: null };
  }
  state.nonces.add(key);
  return { error: null, status: 200, appId };
}

function submit(res, path, body, appId, state) {
  let parsed;
  try {
    parsed = JSON.parse(body.toString('utf8'));
  } catch {
    return json(res, 400, { code: 'REQUEST_INVALID' });
  }
  const kind = path === '/v2/sign' ? 'digests' : 'documents';
  const items = kind === 'digests' ? (parsed.digests ?? []) : (parsed.documents ?? []);
  if (!Array.isArray(items) || items.length === 0) {
    return json(res, 400, { code: 'REQUEST_INVALID', details: { field: kind } });
  }
  for (const job of state.jobs.values()) {
    if (job.owner === appId && !job.collected && !job.finished) {
      return json(res, 409, { code: 'JOB_IN_PROGRESS' });
    }
  }
  const jobId = randomUUID().replace(/-/g, '');
  const digests =
    kind === 'digests'
      ? items.map((d) => Buffer.from(d, 'base64'))
      : items.map((d) => createHash('sha256').update(Buffer.from(d.content, 'base64')).digest());
  const behaviour = state.jobBehaviour({ kind, parsed, items, jobId });
  const job = {
    id: jobId,
    owner: appId,
    kind,
    total: items.length,
    request: parsed,
    behaviour,
    finished: false,
    collected: false,
    startedAt: Date.now(),
  };
  state.jobs.set(jobId, job);
  return json(res, 202, {
    jobId,
    batchFingerprint: createHash('sha256').update(Buffer.concat(digests)).digest('hex'),
    total: items.length,
    eventsUrl: `/v2/jobs/${jobId}/events`,
    resultUrl: `/v2/jobs/${jobId}/result`,
  });
}

async function streamEvents(res, jobId, appId, state) {
  const job = state.jobs.get(jobId);
  if (job === undefined || job.owner !== appId) {
    return json(res, 404, { code: 'JOB_NOT_FOUND' });
  }
  res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-store' });
  const send = (event) => res.write(`data: ${JSON.stringify(event)}\n\n`);

  send({ state: 'queued', completed: 0, total: job.total, failed: 0 });
  send({
    state: 'awaiting_consent',
    completed: 0,
    total: job.total,
    failed: 0,
    consentRemainingMs: 119_000,
  });
  send({ state: 'awaiting_pin', completed: 0, total: job.total, failed: 0 });
  send({ state: 'preparing_card', completed: 0, total: job.total, failed: 0 });
  for (let i = 1; i <= job.total; i++) {
    send({ state: 'signing', completed: i, total: job.total, failed: 0, etaMs: (job.total - i) * 410 });
  }
  if (job.behaviour.kind === 'fail') {
    send({ state: 'failed', completed: 0, total: job.total, failed: job.total, code: job.behaviour.code });
  } else {
    send({ state: 'completed', completed: job.total, total: job.total, failed: job.behaviour.failed?.length ?? 0 });
  }
  job.finished = true;
  res.end();
}

function jobResult(res, jobId, appId, state) {
  const job = state.jobs.get(jobId);
  if (job === undefined || job.owner !== appId) {
    return json(res, 404, { code: 'JOB_NOT_FOUND' });
  }
  if (!job.finished) {
    return json(res, 202, { state: 'signing', completed: 0, total: job.total, failed: 0 });
  }
  if (job.collected) {
    return json(res, 404, { code: 'JOB_NOT_FOUND' });
  }
  job.collected = true;
  state.jobs.delete(jobId);

  if (job.behaviour.kind === 'fail') {
    return json(res, job.behaviour.status ?? 422, { code: job.behaviour.code });
  }
  const failedIndexes = new Set(job.behaviour.failed ?? []);
  const failures = [...failedIndexes].map((index) => ({ index, code: job.behaviour.failureCode ?? 'SIGN_FAILED' }));
  const counts = {
    total: job.total,
    succeeded: job.total - failedIndexes.size,
    failed: failedIndexes.size,
  };
  if (job.kind === 'digests') {
    const signatures = Array.from({ length: job.total }, (_, i) =>
      failedIndexes.has(i) ? null : Buffer.from(`signature-${i}`).toString('base64'),
    );
    return json(res, 200, { signatures, failures, counts });
  }
  const documents = job.request.documents.map((doc, i) =>
    failedIndexes.has(i)
      ? { name: doc.name }
      : {
          name: doc.name,
          content: Buffer.concat([Buffer.from(doc.content, 'base64'), Buffer.from('-SIGNED')]).toString('base64'),
          achievedLevel: job.behaviour.achievedLevel ?? 'B-T',
        },
  );
  return json(res, 200, { documents, failures, counts });
}

function canonical(method, path, timestamp, nonce, body) {
  return [method.toUpperCase(), path, timestamp, nonce, sha256Hex(body)].join('\n');
}

function sha256Hex(body) {
  if (body.length === 0) {
    // Written out rather than computed, so that a mistake in the SDK's
    // own empty-body handling cannot be matched by the same mistake here.
    return EMPTY_BODY_SHA256;
  }
  return createHash('sha256').update(body).digest('hex');
}

function json(res, status, body) {
  const text = JSON.stringify(body);
  res.writeHead(status, { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(text) });
  res.end(text);
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    req.on('data', (chunk) => chunks.push(chunk));
    req.on('end', () => resolve(Buffer.concat(chunks)));
    req.on('error', reject);
  });
}
