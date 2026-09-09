/**
 * The canonical string, checked against `PROTOCOL.md` §3.2's worked
 * example — **read out of that document**, not copied into this file.
 *
 * The agent's own `TestTheProtocolDocumentsWorkedExampleIsTrue` does the
 * same thing from the Go side. So the document is the fixture for both
 * implementations and neither is the fixture for the other: if the
 * example is ever edited, both fail; if either implementation drifts,
 * only that one fails. A test that pinned the numbers here instead
 * would agree with a wrong document forever.
 */

import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import test from 'node:test';

import {
  EMPTY_BODY_SHA256,
  canonicalString,
  newNonce,
  sha256Hex,
  signCanonical,
  MAX_NONCE_LENGTH,
} from '../dist/esm/index.js';

const here = dirname(fileURLToPath(import.meta.url));
const protocolPath = join(here, '..', '..', '..', 'docs', 'PROTOCOL.md');

/** Pulls the fixed values out of §3.2's fenced blocks. */
async function workedExample() {
  const text = await readFile(protocolPath, 'utf8');
  const section = text.slice(text.indexOf('### 3.2 A worked example'), text.indexOf('### 3.3'));
  assert.notEqual(section, '', 'PROTOCOL.md has no §3.2');

  const field = (label) => {
    const match = new RegExp(`^${label}\\s+(\\S+)\\s*$`, 'm').exec(section);
    assert.ok(match, `§3.2 does not state ${label}`);
    return match[1];
  };

  // The body is the one line in §3.2 that is a JSON object on its own.
  const bodyMatch = /^\{".*\}$/m.exec(section);
  assert.ok(bodyMatch, '§3.2 does not show the request body');

  // The canonical string is the line containing four literal \n escapes.
  const canonicalMatch = /^POST\\n\/v2\/sign\\n.*$/m.exec(section);
  assert.ok(canonicalMatch, '§3.2 does not show the canonical string');

  const signatureMatch = /X-Liro-Signature\s*\n([0-9a-f]{64})/.exec(section);
  assert.ok(signatureMatch, '§3.2 does not show the signature');

  return {
    secretBase64: field('deviceSecret \\(base64\\)'),
    timestamp: field('X-Liro-Timestamp'),
    nonce: field('X-Liro-Nonce'),
    method: field('method'),
    path: field('path'),
    body: Buffer.from(bodyMatch[0], 'utf8'),
    bodyHash: /^SHA256HEX\(body\)\s*\n([0-9a-f]{64})/m.exec(section)[1],
    canonical: canonicalMatch[0],
    signature: signatureMatch[1],
  };
}

test('the canonical string matches PROTOCOL.md §3.2, read from the document', async () => {
  const example = await workedExample();

  assert.equal(example.body.length, 120, 'the documented body is 120 bytes');
  assert.equal(sha256Hex(example.body), example.bodyHash);

  const built = canonicalString(example.method, example.path, example.timestamp, example.nonce, example.body);

  // The document shows the string with its newlines escaped, which is
  // how an integrator is told to print their own for comparison.
  assert.equal(built.replaceAll('\n', '\\n'), example.canonical);

  const secret = Buffer.from(example.secretBase64, 'base64');
  assert.equal(secret.length, 32);
  assert.equal(secret.toString('utf8'), 'liro-bridge-example-secret-32byt');
  assert.equal(signCanonical(secret, built), example.signature);
});

test('an empty body hashes to the constant the document names', async () => {
  const text = await readFile(protocolPath, 'utf8');
  assert.ok(text.includes(EMPTY_BODY_SHA256), 'PROTOCOL.md no longer states the empty-body hash');
  // Recomputed rather than trusted: a constant that agrees with itself
  // proves nothing.
  assert.equal(sha256Hex(new Uint8Array(0)), EMPTY_BODY_SHA256);
});

test('only the method is upper-cased', () => {
  const body = new Uint8Array(0);
  const built = canonicalString('post', '/v2/Sign', '1757260800', 'AbCd', body);
  assert.equal(built.split('\n')[0], 'POST');
  assert.equal(built.split('\n')[1], '/v2/Sign', 'the path is used exactly as given');
  assert.equal(built.split('\n')[3], 'AbCd', 'the nonce is used exactly as given');
});

test('the timestamp is the string that was sent, not a re-formatting of it', () => {
  const body = new Uint8Array(0);
  const a = canonicalString('GET', '/v2/health', '1757260800', 'n', body);
  const b = canonicalString('GET', '/v2/health', '01757260800', 'n', body);
  assert.notEqual(a, b, 'the same instant spelled two ways must give two canonical strings');
});

test('nonces are unique, hexadecimal and inside the agent’s length limit', () => {
  const seen = new Set();
  for (let i = 0; i < 5000; i++) {
    const nonce = newNonce();
    assert.match(nonce, /^[0-9a-f]{32}$/);
    assert.ok(nonce.length <= MAX_NONCE_LENGTH);
    assert.ok(!seen.has(nonce), 'newNonce repeated a value');
    seen.add(nonce);
  }
});
