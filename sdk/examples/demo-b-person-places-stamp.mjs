// Demo B — the application supplies the documents, the person answers
// the stamp question.
//
//   node sdk/examples/demo-b-person-places-stamp.mjs
//   node sdk/examples/demo-b-person-places-stamp.mjs --fresh
//   node sdk/examples/demo-b-person-places-stamp.mjs ugovor.pdf
//
// This is demo-a-application-decides.mjs with ONE change: `stamp` is not
// in the request. Same documents, same certificate, same level, same
// origin. Everything different about the run comes from that one
// omission.
//
// What it produces: a second step. The person gets the signing-method
// screen and chooses there — no visible mark, a corner, or choosing
// where the signature goes by looking at the page.
//
// READ THIS BEFORE YOU RUN IT. The third option — "Potpiši birajući
// poziciju potpisa" — cannot complete on this path, and that is
// deliberate rather than broken. The placement picker draws the page it
// is placing a stamp on, and a protocol batch's documents arrived over a
// socket and are nowhere on disk (signflow_windows.go's
// signAtAChosenPosition). Choosing it and pressing Potpiši falls back to
// the corners with a warning line. So: the person chooses the method
// here, but placing a stamp by eye is something only a local batch can
// do. Watch it refuse — that is the most useful thing in this demo.

import {
  LEVEL,
  chooseCertificate,
  connectAsDemo,
  heading,
  loadDocuments,
  narrateProgress,
  note,
  parseArgs,
  reportFailure,
  saveResults,
} from './demo-protocol-common.mjs';

const { fresh, paths } = parseArgs(process.argv);

try {
  heading('Demo B — the person answers the stamp question');
  note('One document, one named certificate, one named level — and no');
  note('stamp in the request, so the method screen appears and you choose.');

  const bridge = await connectAsDemo({
    applicationName: 'Liro Business (demo B)',
    storeFile: 'pairing-b.json',
    fresh,
  });
  note(`\n  agent ${bridge.agentInfo.agentVersion}, protocol ${bridge.agentInfo.protocolVersion}`);

  const certificate = await chooseCertificate(bridge);
  const documents = await loadDocuments(paths, 1);

  heading('Submitting.');
  note(`${documents.length} document: ${documents.map((d) => d.name).join(', ')}`);
  note('Two steps this time, so the window carries a step header with two');
  note('dots and a way back. Step 1 is the approval, step 2 is the method.');

  narrateProgress(bridge);

  const signed = await bridge.signPdf(documents, {
    certificateThumbprint: certificate.thumbprint,
    level: LEVEL,
    // No `stamp`. That is the whole difference from demo A: the method
    // question is left for the person, which is what adds the step.
  });

  await saveResults(signed, 'demo-b');
} catch (error) {
  reportFailure(error);
}
