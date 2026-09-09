/** Scaffolding every test file shares. */

import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { StoredPairing } from '../dist/esm/index.js';

/** A temporary directory, removed when `dispose()` is called. */
export async function scratch() {
  const dir = await mkdtemp(join(tmpdir(), 'liro-sdk-'));
  return {
    dir,
    path: (name) => join(dir, name),
    dispose: () => rm(dir, { recursive: true, force: true }),
  };
}

/** Writes a discovery file for a fake agent and returns its path. */
export async function writeBridgeFile(dir, port, extra = {}) {
  const path = join(dir, 'bridge.json');
  await writeFile(
    path,
    `${JSON.stringify({ port, agentVersion: '1.4.0-fake', protocolVersion: 2, ...extra }, null, 2)}\n`,
    'utf8',
  );
  return path;
}

/**
 * A `SecretStore` that keeps pairings in memory, the way a naive
 * implementation would — through `serialise()` and `deserialise()`,
 * which is what the interface documentation asks for.
 */
export class MemoryStore {
  constructor() {
    this.records = new Map();
    this.setCalls = 0;
    this.clearCalls = 0;
  }

  async get(appName) {
    const record = this.records.get(appName);
    return record === undefined ? null : StoredPairing.deserialise(record);
  }

  async set(appName, pairing) {
    this.setCalls += 1;
    this.records.set(appName, pairing.serialise());
  }

  async clear(appName) {
    this.clearCalls += 1;
    this.records.delete(appName);
  }
}

/**
 * The mistake the interface documentation warns about: a store that
 * persists `JSON.stringify(pairing)` and hands the result back.
 */
export class NaiveJsonStore {
  constructor() {
    this.text = null;
  }

  async get() {
    return this.text === null ? null : StoredPairing.deserialise(JSON.parse(this.text));
  }

  async set(_appName, pairing) {
    this.text = JSON.stringify(pairing);
  }

  async clear() {
    this.text = null;
  }
}
