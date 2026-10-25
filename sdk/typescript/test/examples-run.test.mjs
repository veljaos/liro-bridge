/**
 * Every example, run — against the stand-in agent, over a real loopback
 * socket, finding it the way it finds the real one: through the
 * discovery file on this platform ([[D-459]]).
 *
 * What this is and is not. It is the first time `Sign.java` and
 * `sign.php` were ever executed (C13): they compile or parse, they find
 * the file where SPEC §14 puts it on this platform, they pair with the
 * code typed on standard input, they sign their requests in a way an
 * agent written from PROTOCOL.md accepts, they submit a PDF, poll, and
 * write what comes back. It is **not** a run against the real agent:
 * there is no consent window here, no card and no signature — the fake
 * agent answers with the document it was sent. The README's table of
 * what ran against the real agent is a different and stronger claim,
 * and this test does not make it.
 *
 * An example whose toolchain is not on this machine is skipped, and the
 * skip says which toolchain, so a green run never hides an example that
 * did not run.
 */

import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { mkdtemp, mkdir, readFile, rm, writeFile, copyFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

import { startFakeAgent } from './fake-agent.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const examplesDir = join(here, '..', '..', 'examples');

/** A tiny PDF is enough: the stand-in reads only its length. */
const PDF = Buffer.from('%PDF-1.4\n1 0 obj <<>> endobj\ntrailer <<>>\n%%EOF\n', 'latin1');

function have(cmd, args = ['--version']) {
  const r = spawnSync(cmd, args, { stdio: 'ignore' });
  return r.status === 0 || (r.error === undefined && r.status !== null);
}

/** Runs cmd with the discovery file in place and the code on stdin. */
function runExample(cmd, args, { cwd, env, stdin, timeoutMs = 60_000 }) {
  return new Promise((resolve) => {
    const child = spawn(cmd, args, { cwd, env, stdio: ['pipe', 'pipe', 'pipe'] });
    let out = '';
    let err = '';
    child.stdout.on('data', (d) => (out += d));
    child.stderr.on('data', (d) => (err += d));
    const timer = setTimeout(() => child.kill('SIGKILL'), timeoutMs);
    child.on('close', (code) => {
      clearTimeout(timer);
      resolve({ code, out, err });
    });
    child.stdin.end(stdin);
  });
}

/**
 * The platform's discovery file, written for the stand-in. Linux and
 * macOS only — on Windows the examples read LOCALAPPDATA, which this
 * test would have to redirect for the whole process.
 */
async function placeDiscovery(root, port) {
  if (process.platform === 'linux') {
    const runtime = join(root, 'run');
    await mkdir(join(runtime, 'liro'), { recursive: true });
    await writeFile(join(runtime, 'liro', 'bridge.json'), JSON.stringify({ port, agentVersion: 'fake', protocolVersion: 2 }));
    return { XDG_RUNTIME_DIR: runtime, HOME: join(root, 'home') };
  }
  if (process.platform === 'darwin') {
    const home = join(root, 'home');
    const dir = join(home, 'Library', 'Application Support', 'Liro');
    await mkdir(dir, { recursive: true });
    await writeFile(join(dir, 'bridge.json'), JSON.stringify({ port, agentVersion: 'fake', protocolVersion: 2 }));
    return { HOME: home };
  }
  return null;
}

const EXAMPLES = [
  {
    file: 'sign.php',
    needs: () => (have('php', ['-v']) ? null : 'php'),
    run: (dir, pdf, env) => runExample('php', [join(dir, 'sign.php'), pdf], { cwd: dir, env, stdin: '481516\n' }),
  },
  {
    file: 'Sign.java',
    needs: () => (have('java', ['-version']) && have('javac', ['-version']) ? null : 'a JDK (java, javac)'),
    run: async (dir, pdf, env) => {
      const c = spawnSync('javac', ['-d', dir, join(dir, 'Sign.java')], { encoding: 'utf8' });
      if (c.status !== 0) {
        return { code: c.status, out: c.stdout, err: `javac: ${c.stderr}` };
      }
      return runExample('java', ['-cp', dir, 'Sign', pdf], { cwd: dir, env, stdin: '481516\n' });
    },
  },
  {
    file: 'sign.py',
    needs: () => (have('python3') ? null : 'python3'),
    run: (dir, pdf, env) => runExample('python3', [join(dir, 'sign.py'), pdf], { cwd: dir, env, stdin: '481516\n' }),
  },
];

for (const ex of EXAMPLES) {
  test(`${ex.file} runs end to end against the stand-in agent, found through this platform's discovery file`, async (t) => {
    const missing = ex.needs();
    if (missing !== null) {
      t.skip(`${missing} is not on this machine, so ${ex.file} did not run`);
      return;
    }
    const agent = await startFakeAgent({
      certificates: [{
        thumbprint: 'AB'.repeat(20), displayName: 'Test Key', isTestKey: true, usable: true,
        source: 'softtoken', notBefore: '2026-01-01T00:00:00Z', notAfter: '2027-01-01T00:00:00Z',
      }],
    });
    const root = await mkdtemp(join(tmpdir(), 'liro-example-'));
    try {
      const discovery = await placeDiscovery(root, agent.port);
      if (discovery === null) {
        t.skip(`this test places the discovery file on Linux and macOS only, not ${process.platform}`);
        return;
      }
      await mkdir(join(root, 'home'), { recursive: true });
      const dir = join(root, 'ex');
      await mkdir(dir);
      await copyFile(join(examplesDir, ex.file), join(dir, ex.file));
      const pdf = join(dir, 'ugovor.pdf');
      await writeFile(pdf, PDF);

      // The stand-in finishes a job when its event stream is read; these
      // examples poll the result instead, as a person's approval would
      // let them. A job is marked done half a second after it arrives —
      // the person pressing Approve, in this test's terms.
      const approve = setInterval(() => {
        for (const job of agent.state.jobs.values()) {
          if (!job.finished && Date.now() - job.startedAt > 500) {
            job.finished = true;
          }
        }
      }, 100);
      const env = { PATH: process.env.PATH, LANG: 'C.UTF-8', ...discovery };
      let r;
      try {
        r = await ex.run(dir, pdf, env);
      } finally {
        clearInterval(approve);
      }
      const said = `exit ${r.code}\n--- stdout\n${r.out}\n--- stderr\n${r.err}`;
      assert.equal(r.code, 0, said);

      const paths = agent.state.requests.map((q) => `${q.method} ${q.path.replace(/[0-9a-f]{16,}/g, ':id')}`);
      for (const want of ['GET /v2/health', 'POST /v2/pair/request', 'POST /v2/pair/confirm', 'POST /v2/sign/pdf']) {
        assert.ok(paths.includes(want), `${ex.file} never sent ${want}; it sent ${paths.join(', ')}`);
      }
      assert.ok(paths.some((p) => /^GET \/v2\/jobs\/.+\/result$/.test(p)), `${ex.file} never asked for its result: ${paths.join(', ')}`);
      const signed = await readFile(join(dir, 'ugovor-signed.pdf'));
      assert.ok(signed.length > 0, `${ex.file} wrote an empty result`);
    } finally {
      await agent.stop();
      await rm(root, { recursive: true, force: true });
    }
  });
}
