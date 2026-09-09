/**
 * Finding the agent — `PROTOCOL.md` §4.
 *
 * The agent binds loopback only, on the first free port in 17580–17590,
 * and writes what it chose to a per-user file. **Read that file. Never
 * scan ports.** Several people can be signed in to one machine at once —
 * an accounting firm over RDP is the ordinary case, not an edge one —
 * and each of them has their own agent on its own port. Scanning finds
 * somebody else's, which is exactly what the per-user file prevents.
 *
 * There is no port-scanning code in this SDK, and there is no option
 * that would turn some on.
 */

import { readFile } from 'node:fs/promises';
import { homedir } from 'node:os';
import { join } from 'node:path';
import { LiroError } from './errors.js';

/** What the discovery file holds. */
export interface BridgeInfo {
  /** The loopback port the agent is listening on. */
  readonly port: number;
  /** The agent's own version. */
  readonly agentVersion: string;
  /** The protocol version it speaks. `2` for everything this SDK does. */
  readonly protocolVersion: number;
}

/** The protocol version this SDK is written against. */
export const PROTOCOL_VERSION = 2;

/**
 * The per-user discovery file for the current platform.
 *
 * The three paths are SPEC §14's, not this SDK's invention.
 * `PROTOCOL.md` documents the Windows one because Windows is where the
 * agent runs today; the other two are here so that an SDK reading a file
 * does not become the thing that has to change when the agent reaches
 * them.
 */
export function defaultBridgeFilePath(): string {
  switch (process.platform) {
    case 'win32': {
      const local = process.env['LOCALAPPDATA'];
      if (local) {
        return join(local, 'Liro', 'bridge.json');
      }
      return join(homedir(), 'AppData', 'Local', 'Liro', 'bridge.json');
    }
    case 'darwin':
      return join(homedir(), 'Library', 'Application Support', 'Liro', 'bridge.json');
    default: {
      const runtime = process.env['XDG_RUNTIME_DIR'];
      if (runtime) {
        return join(runtime, 'liro', 'bridge.json');
      }
      return join(homedir(), '.local', 'state', 'liro', 'bridge.json');
    }
  }
}

/**
 * Reads the discovery file.
 *
 * A missing file means the agent is not running, and so does a file
 * pointing at a port nothing answers on — a stale one left by a crash.
 * Both are the same fact to a caller, and both are `AGENT_NOT_RUNNING`.
 */
export async function readBridgeFile(path: string): Promise<BridgeInfo> {
  let text: string;
  try {
    text = await readFile(path, 'utf8');
  } catch (err) {
    throw new LiroError('AGENT_NOT_RUNNING', {
      detail: `no discovery file at ${path}. Start Liro Bridge; it writes the file when it starts and removes it when it stops`,
      cause: err,
    });
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(text.replace(/^﻿/, ''));
  } catch (err) {
    throw new LiroError('PROTOCOL_VIOLATION', {
      detail: `the discovery file at ${path} is not valid JSON`,
      cause: err,
    });
  }

  const info = parsed as Partial<BridgeInfo>;
  if (typeof info !== 'object' || info === null || typeof info.port !== 'number' || !Number.isInteger(info.port)) {
    throw new LiroError('PROTOCOL_VIOLATION', {
      detail: `the discovery file at ${path} names no port`,
    });
  }
  return {
    port: info.port,
    agentVersion: typeof info.agentVersion === 'string' ? info.agentVersion : '',
    protocolVersion: typeof info.protocolVersion === 'number' ? info.protocolVersion : 0,
  };
}

/** `http://127.0.0.1:<port>` — loopback, always, with no way to ask for anything else. */
export function baseUrlFor(port: number): string {
  return `http://127.0.0.1:${port}`;
}
