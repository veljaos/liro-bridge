# Phase F6b — Visual stamp placement

**Prerequisite reading:** `docs/SPEC.md` in full, then `docs/decisions.md` in full.

**Prerequisite phase:** F6 complete, including its fix rounds. Drag and drop works, the three-step flow works, the four-line stamp renders correctly with real certificates from two different issuers, and settings persist.

**Goal of this phase:** the person sees the page and puts the stamp where they want it.

**This phase is small in scope and high in expectation.** It is one window doing one thing, and it is the part of the product the owner will touch most often. The instruction from the owner was: *take it slowly, do it properly.* A rushed placement picker is worse than the corner selector that already works.

**Explicitly not in this phase:** no HTTP protocol, no SDK. No automatic detection of whether a corner is already occupied — the owner ruled that out deliberately: the user can see the page and will move the stamp themselves, and a guess would be wrong half the time while getting in the way of a deliberate choice.

---

## 1. What exists already

Do not rebuild these. Wire the new window to them.

- `appearance.Options` accepts explicit coordinates in PDF points, and F6 clamps them into the page box less the margin.
- The stamp's geometry is fixed: **190 pt wide**, height by line count, currently **48 pt** for four lines.
- `pdf.FindPage`, `pdf.ResolveMediaBox` and `pdf.ResolveRotate` already resolve a page's box through the `/Parent` chain and respect `/Rotate`.
- The stamp window from F6 already holds on/off, corner, page, reference line and the document-number toggle, and persists them.
- The consent flow's step 3 already exists and can be skipped when a caller supplies the position.

---

## 2. The window

A single window: the page on the left or filling most of it, a draggable stamp rectangle on top, and the controls it needs and nothing more.

### 2.1 Rendering the page

WebView2 has a built-in PDF viewer, which is the obvious route and the wrong one here: it owns its own scrolling, zoom and page navigation, and it will not let a rectangle be dragged over it in page coordinates.

**Render each page to an image in Go and display that image.** The project already renders pages during testing; use the same approach in production. The image is a picture of one page at a chosen scale, and the stamp rectangle is an ordinary element positioned over it — which makes the coordinate arithmetic simple and entirely ours.

Render lazily: the page currently shown, and at most one either side. A two-hundred-page document must not cost two hundred renders.

### 2.2 Multiple pages

Documents have many pages, and the signature does not always belong on the first.

- **Previous** and **Next**, with `Strana N od M` between them.
- A page number that can be typed, and Enter jumps to it.
- **First** and **Last** shortcuts — the last page is where signatures usually go.
- The chosen page is part of the saved position.
- Page size and rotation can differ from page to page in one document. Re-read the box for each page; do not assume the first page's dimensions.

### 2.3 Zoom

**Zoom changes the page image, never the window.** The window keeps its size; only what is drawn inside it changes.

- Buttons **−** and **+**, and a percentage that can be clicked to reset to Fit.
- Steps: 50, 75, 100, 125, 150, 200, 300, 400 percent. **Fit** is an additional mode, the default, which scales the page to the available area.
- `Ctrl` plus mouse wheel zooms; the wheel alone scrolls.
- **Zoom is anchored on the cursor**, not the page centre. Zooming in to read fine print near a signature line should keep that spot under the pointer.
- When the page is larger than the visible area, it scrolls, and the scroll position survives a zoom change — the point under the cursor stays under the cursor.
- **The stamp's size in page points never changes.** At 200 percent it looks twice as large on screen because everything does. Its position in points is what is saved, and zoom must not alter it by so much as a point.

### 2.4 Dragging

- The stamp rectangle is dragged with the mouse. The cursor becomes a move cursor over it.
- The whole rectangle drags; there is no resizing. The stamp's size is determined by its content.
- **Arrow keys nudge by 1 pt; Shift with an arrow by 10 pt.** Someone aligning a stamp under a printed initial needs precision a mouse cannot give.
- Show the current position in points as it moves — `x: 381, y: 24` — so a position can be noted and reproduced.
- Snapping to the four corners when the stamp comes within about 10 pt of one, with the snap visible. Corners remain the common case and should stay effortless.
- The stamp is drawn as it will actually appear — logo, four lines, real text from the certificate — not as an empty box. What the person places is what they will get.

### 2.5 Margins

The original Bridge did not let the stamp sit flush against the page edge, and neither does this one.

**The margin is 12 pt.** The owner's instruction: not too large — someone may genuinely want the stamp close to the edge, but not on it. Twelve points is about four millimetres, enough that no printer or binding will clip it.

- Show the margin as a faint guide, so the limit is visible rather than merely felt.
- When a drag reaches the margin, the stamp stops. It does not bounce and it does not disappear under the edge.
- Explicit coordinates from an API caller are clamped, not refused — the F6 rule stands, with 12 pt as the value.

Note that this changes F6's 24 pt. Record the change and the reason.

### 2.6 Rotated pages

`/Rotate` is common in scanned documents. A page rotated 90 degrees must display rotated — as the reader shows it — and the stamp must land where the person put it as they see it.

The conversion between screen coordinates and PDF coordinates has to account for rotation. Test all four values: 0, 90, 180, 270.

---

## 3. Remembered positions

This is the requirement behind the phase, and it matters more than the dragging.

The owner has documents signed a thousand times where the stamp must sit in a specific place — under printed initials, for instance. Dragging it a thousand times is worse than the corner selector it replaces.

- The chosen position — page, x, y — is saved and offered as the default next time.
- When a batch is signed, the position applies to **every document in it**. Ask once, not once per file.
- A saved position that falls outside a particular document's page box is clamped into it, and the person is told which documents were adjusted. A one-page contract and a fifty-page report do not have the same last page, and a position saved on page 12 means nothing in a four-page document — in that case fall back to the same relative place on the last page and say so.
- The stamp window shows what is currently saved and offers to reset it.

Whether more than one named position is worth storing is a decision for you: if the owner's documents are always the same shape, a single remembered position is enough and simpler. Record which you chose and why.

---

## 4. How it is reached

- From step 3 of the signing flow: alongside the four corners, an option to place it visually. Choosing it opens this window on the first document of the batch.
- From Settings, to set a default without signing anything.
- When a caller has already supplied a position, this window does not appear at all — the F6 rule that step 3 is skippable stands.

---

## 5. Encrypted and unreadable documents

A document that cannot be parsed cannot be previewed. `CodePDFEncrypted` already exists.

When the preview cannot be produced, say so plainly and fall back to the corner selector. Signing an encrypted PDF is out of scope; a document that fails to render for another reason should still be signable at a corner.

---

## 6. Performance

- The window opens in under a second on a typical document. If the first render is slower, show the frame and the controls immediately and fill the page image when it arrives — never a blank window.
- Zoom and page changes feel immediate. Cache rendered pages at the current zoom; discard them when zoom changes.
- A two-hundred-page document must not be rendered up front. Measure with one and report the numbers.
- Dragging is smooth. Do not re-render the page while dragging; only the rectangle moves.

---

## 7. Tests

Coordinate arithmetic is pure and must be tested exhaustively — it is where this will go wrong.

- Screen to PDF coordinates and back, at every zoom step, round-tripping to the same point.
- The same, for all four rotations.
- The same, for A4 portrait, A4 landscape, and a non-standard page size.
- Clamping at all four edges, at every zoom step, with the result always inside the box less 12 pt.
- Corner snapping: within 10 pt snaps, beyond it does not.
- Arrow-key nudging by exactly 1 pt and 10 pt regardless of zoom.
- A saved position applied to a document with a smaller page is clamped, and the adjustment is reported.
- A saved page number beyond a document's page count falls back to the last page.
- Page boxes differing between pages of one document.
- The stamp's position in points is unchanged by any sequence of zoom operations.

And, in the running binary: a real multi-page document, dragged, zoomed, paged through, signed, and the result opened to confirm the stamp landed where it was put — to the point.

---

## Rules

- This phase only. No HTTP, no SDK.
- **No synthetic mouse or keyboard input, ever** — D-094. Screenshots; ask the owner to click when a screenshot genuinely requires it. This phase is about dragging, so expect to ask.
- **Never kill processes by image name.** Match on the command line, kill by exact PID, only processes this project started.
- **Verify by running the built binary and looking at the window.** Every visible defect in this project has passed its tests while being broken on screen.
- Rebuild before reporting anything claimed as observed.
- Every colour from a token; `checkcss` green. Every new string in all three catalogues; help and usage English.
- Every existing test passes. No `//nolint`.
- Record decisions for: how the page is rendered and why not WebView2's viewer; the 12 pt margin superseding 24; zoom anchoring; and whether one remembered position or several.

Run the full check suite with `-count=1`, on Windows, with and without the `softtoken` tag.

**Report:** how the page is rendered and what it costs, the numbers for a two-hundred-page document, how screen and PDF coordinates are converted, and what remains for the owner to verify by hand.

---

## Exit checklist

### Rendering

- [ ] Pages rendered in Go and displayed as images, not through WebView2's PDF viewer
- [ ] Lazy: current page and at most one either side
- [ ] Two hundred pages does not render two hundred pages; the number is measured and reported
- [ ] Window shows frame and controls immediately, fills the image when ready

### Pages

- [ ] Previous, Next, page number entry, First, Last
- [ ] `Strana N od M` visible
- [ ] Chosen page saved with the position
- [ ] Page box and rotation re-read per page

### Zoom

- [ ] Steps 50 to 400 plus Fit; Fit is the default
- [ ] The window never resizes
- [ ] `Ctrl` and wheel zooms; wheel alone scrolls
- [ ] **Zoom anchored on the cursor**
- [ ] Scroll position preserved across a zoom change
- [ ] **The stamp's position in points is unchanged by zoom, proven by test**

### Dragging

- [ ] Whole rectangle drags; no resizing
- [ ] Move cursor over the stamp
- [ ] Arrow keys nudge 1 pt, Shift-arrow 10 pt, at any zoom
- [ ] Position shown in points, live
- [ ] Corner snapping within 10 pt, visibly
- [ ] The stamp is drawn as it will appear, with real content

### Margins

- [ ] 12 pt on every edge, superseding F6's 24
- [ ] Margin shown as a faint guide
- [ ] The drag stops at the margin; nothing bounces or disappears
- [ ] Explicit API coordinates clamped, not refused

### Rotation

- [ ] 0, 90, 180 and 270 all display as a reader shows them
- [ ] The stamp lands where it was placed, for each

### Remembered position

- [ ] Page, x and y saved and offered next time
- [ ] Applied to every document in a batch, asked once
- [ ] Out-of-box positions clamped, adjustments reported
- [ ] Page beyond the document's count falls back to the last page
- [ ] Current saved position shown, with a reset

### Reachability

- [ ] Offered in step 3 beside the four corners
- [ ] Reachable from Settings
- [ ] Not shown at all when a caller supplied the position

### Failure

- [ ] Preview failure says so plainly and falls back to corners
- [ ] Encrypted documents handled through the existing code

### Tests

- [ ] Every case in §7 passes
- [ ] Coordinate round-trips exhaustive across zoom, rotation and page size

### Manual acceptance (pending)

- [ ] A real multi-page document: drag, zoom, page through, sign
- [ ] The stamp lands to the point where it was placed
- [ ] A saved position applied to a batch of differently shaped documents
- [ ] Dragging feels smooth at 400 percent on a large page
