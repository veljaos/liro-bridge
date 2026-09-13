// Shared plumbing for the two protocol demonstrations beside this file.
//
//   demo-a-application-decides.mjs   — the application answers everything
//   demo-b-person-places-stamp.mjs   — the person answers the stamp question
//
// These two are not examples to copy into an integration; the seven
// files in this directory already are that. These exist to be *watched*:
// they drive the agent down the path an ERP actually uses — the
// protocol — so the windows a person would see can be seen. Everything
// that is not the difference between the two demos lives here, so that
// the difference is one object literal in each of them and nothing else.
//
// Both demos are deliberately identical in every other respect: same
// documents, same certificate, same level, same origin. If the two runs
// look different, the stamp question is why.

import { createInterface } from 'node:readline/promises';
import { mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { basename, join, resolve } from 'node:path';
import { stdin, stdout } from 'node:process';

import { FileSecretStore, LiroBridge, LiroError } from '@liro/bridge';

/** Where the demos keep their pairings and write their output. */
export const DEMO_DIR = resolve(process.env.LIRO_DEMO_DIR ?? join(process.cwd(), 'liro-demo'));

/**
 * The document used when none is named on the command line: this
 * project's own blank A4 page. A blank page is the right fixture for
 * watching a stamp — the stamp is the only thing on it, so where it
 * lands is unmistakable.
 */
const DEFAULT_PDF = resolve(join(process.cwd(), 'testdata', 'pdfs', 'blank.pdf'));

/**
 * The level both demos ask for.
 *
 * B-B on purpose. The agent's own default is B-LT, which needs a
 * timestamp authority, and with none configured the agent stops to ask
 * the person whether to sign without one (SPEC §12.8) — a screen that
 * has nothing to do with what either demo is about. Asking for B-B
 * means that screen never appears and each demo shows exactly the
 * question it is meant to show.
 */
const LEVEL = process.env.LIRO_LEVEL ?? 'b-b';

export function parseArgs(argv) {
  const args = argv.slice(2);
  const fresh = args.includes('--fresh');
  const paths = args.filter((a) => !a.startsWith('--'));
  return { fresh, paths };
}

export function heading(text) {
  console.log(`\n[1m${text}[0m`);
}

export function note(text) {
  console.log(`  ${text}`);
}

/** Asks the person for the six digits the agent's pairing window is showing. */
async function askForTheCode({ applicationName, origin, expiresInSeconds, attemptsRemaining }) {
  heading('The pairing window is on screen now.');
  note(`It is showing "${applicationName}" and the origin ${origin}, verbatim.`);
  note(`There is no Allow button. The six digits are the approval, and they`);
  note(`travel through you — they are in no response and never will be.`);
  note(`The code is good for ${expiresInSeconds} s.`);
  const rl = createInterface({ input: stdin, output: stdout });
  try {
    const suffix = attemptsRemaining === null ? '' : ` (${attemptsRemaining} attempts left)`;
    return (await rl.question(`\n  Six digits${suffix}: `)).trim();
  } finally {
    rl.close();
  }
}

async function confirm(question) {
  const rl = createInterface({ input: stdin, output: stdout });
  try {
    const answer = (await rl.question(`  ${question} `)).trim().toLowerCase();
    return answer === 'yes' || answer === 'da';
  } finally {
    rl.close();
  }
}

/**
 * Connects, pairing if there is no stored pairing for this demo.
 *
 * Each demo keeps its own store file, so each pairs separately and each
 * shows its own pairing window — and both then appear as separate rows
 * under "Povezane aplikacije" in Settings. `--fresh` deletes the stored
 * pairing so the window comes back on a later run; the row left behind
 * in Settings can be removed there with "Prekini vezu".
 */
export async function connectAsDemo({ applicationName, storeFile, fresh }) {
  const store = join(DEMO_DIR, storeFile);
  await mkdir(DEMO_DIR, { recursive: true });
  if (fresh) {
    await rm(store, { force: true });
    note('--fresh: the stored pairing was deleted, so this run pairs again.');
  }

  try {
    return await LiroBridge.connect({
      applicationName,
      origin: process.env.LIRO_ORIGIN ?? 'https://erp.example.rs',
      secretStore: new FileSecretStore(store),
      onPairingCode: askForTheCode,
    });
  } catch (error) {
    if (error instanceof LiroError && (error.code === 'AGENT_NOT_RUNNING' || error.code === 'NETWORK')) {
      console.error('\nThe agent is not running. Start it first:\n');
      console.error('  go run -tags softtoken ./cmd/liro-bridge tray\n');
      console.error('with LIRO_SOFTTOKEN_P12 and LIRO_SOFTTOKEN_PASSWORD set — see the walkthrough.');
      process.exit(1);
    }
    throw error;
  }
}

/**
 * Chooses the certificate the application will name in the request.
 *
 * The soft token is preferred whenever one is present, so that a demo
 * run makes a test signature and asks for no PIN. A real certificate is
 * used only when there is no test key, and only after the person says
 * so out loud: this machine may well have a real card in the reader,
 * and a demonstration must not quietly spend a signature on it.
 */
export async function chooseCertificate(bridge) {
  let certificates;
  try {
    certificates = await bridge.certificates();
  } catch (error) {
    if (error instanceof LiroError && error.code === 'CERTIFICATE_LISTING_DISABLED') {
      console.error('\nThe agent will not list certificates: the switch is off in Settings.');
      console.error('Turn on "Dozvoli programima da pitaju koji su sertifikati na ovom računaru".');
      process.exit(1);
    }
    throw error;
  }

  heading('What this machine can sign with');
  for (const c of certificates) {
    const mark = c.isTestKey ? ' [TEST KEY]' : '';
    const state = c.usable ? 'usable' : (c.notUsableReason ?? 'not usable');
    note(`${c.thumbprint.slice(-8)}  ${c.displayName}${mark}  — ${state}`);
  }

  const named = process.env.LIRO_THUMBPRINT;
  const usable = certificates.filter((c) => c.usable);
  const chosen = named
    ? certificates.find((c) => c.thumbprint.toUpperCase() === named.toUpperCase())
    : (usable.find((c) => c.isTestKey) ?? usable[0]);

  if (!chosen) {
    console.error('\nNo usable certificate. Put a card in, or run the agent with the soft token.');
    process.exit(1);
  }

  if (!chosen.isTestKey && process.env.LIRO_REAL_CARD !== '1') {
    heading('This is a real certificate, not the soft token.');
    note(`${chosen.displayName} — ${chosen.thumbprint.slice(-8)}`);
    note('Continuing makes a REAL signature and the card will ask for your PIN.');
    if (!(await confirm('Type yes to go ahead:'))) {
      console.error('  Stopped. Run the agent with the soft token to demo without a card.');
      process.exit(1);
    }
  }

  note(`\n  Chosen: ${chosen.displayName} (…${chosen.thumbprint.slice(-8)})${chosen.isTestKey ? ' [TEST KEY]' : ''}`);
  return chosen;
}

/** Loads the documents, defaulting to this project's blank A4 page. */
export async function loadDocuments(paths, count) {
  if (paths.length > 0) {
    return Promise.all(
      paths.map(async (p) => ({ name: basename(p), content: await readFile(p) })),
    );
  }
  const content = await readFile(DEFAULT_PDF);
  return Array.from({ length: count }, (_, i) => ({
    name: count === 1 ? 'Ugovor.pdf' : `Ugovor-${String(i + 1).padStart(3, '0')}.pdf`,
    // The same blank page under different names. The names are what the
    // consent window shows; the bytes only have to be a real PDF.
    content,
  }));
}

/**
 * Narrates the job on the console beside the windows, so that what is
 * happening on screen and what the application is being told can be
 * read together. Every event carries the whole picture, not a change
 * to it, so nothing here accumulates state.
 */
export function narrateProgress(bridge) {
  let lastConsentSecond = null;
  let said = new Set();

  bridge.onProgress((p) => {
    switch (p.state) {
      case 'queued':
        if (!said.has('queued')) {
          said.add('queued');
          note('queued — accepted, nothing on screen yet.');
        }
        break;
      case 'awaiting_consent': {
        const left = Math.round((p.consentRemainingMs ?? 0) / 1000);
        // Once, then every tenth second: this event republishes every
        // second and a line per second is not information.
        if (lastConsentSecond === null || (left % 10 === 0 && left !== lastConsentSecond)) {
          lastConsentSecond = left;
          note(`awaiting_consent — the window is up, ${left} s left to answer.`);
        }
        break;
      }
      case 'awaiting_pin':
        note('awaiting_pin — approved. The agent is opening the card.');
        break;
      case 'preparing_card':
        if (!said.has('preparing_card')) {
          said.add('preparing_card');
          note('preparing_card — the first signature is in flight. Expected, not a stall:');
          note('  about 4 s on a MUP card, 12.7 s on a Pošta one. A soft token is instant.');
        }
        break;
      case 'signing':
        note(`signing — ${p.completed}/${p.total}${p.etaMs === null ? '' : ` (~${Math.round(p.etaMs / 1000)} s left)`}`);
        break;
      case 'completed':
        note(`completed — ${p.completed}/${p.total} signed, ${p.failed} failed.`);
        break;
      case 'failed':
        note(`failed — ${p.code}`);
        break;
      default:
        note(String(p.state));
    }
  });
}

/** Writes what came back, and says where, so the stamp can be looked at. */
export async function saveResults(signed, subdirectory) {
  const outDir = join(DEMO_DIR, 'out', subdirectory);
  await mkdir(outDir, { recursive: true });

  heading('What came back');
  let written = 0;
  for (const doc of signed) {
    if (doc.content === null) {
      note(`${doc.name} — NOT signed: ${doc.failure?.agentCode}`);
      continue;
    }
    const target = join(outDir, doc.name.replace(/\.pdf$/i, '') + '-signed.pdf');
    await writeFile(target, doc.content);
    written += 1;
    note(`${doc.name} → ${target}`);
    note(`   ${doc.content.length} bytes, level actually reached: ${doc.achievedLevel}`);
  }
  if (written > 0) {
    heading('Open one and look at the stamp.');
    note(outDir);
  }
  return written;
}

/** The level both demos ask for, and why — see LEVEL above. */
export { LEVEL };

/** Turns a LiroError into one line and a non-zero exit. */
export function reportFailure(error) {
  if (error instanceof LiroError) {
    console.error(`\n${error.code}: ${error.message}`);
    process.exit(1);
  }
  throw error;
}
