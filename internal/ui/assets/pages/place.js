(function () {
  "use strict";

  // The placement window's page.
  //
  // The rule this file is built around: **the position is held in
  // points, never in pixels.** Pixels are derived from it to draw and
  // read back from a drag; they are never what is kept. That is what
  // makes zoom incapable of moving the stamp — at any zoom, the same
  // two numbers go back to Go.
  //
  // Go supplies the page's box, its rotation, the stamp's size, the
  // bounds the position may take and the four corner positions, all in
  // points. This file converts between points and screen pixels, and
  // does nothing else with geometry.

  var state = {
    page: 1,
    pageCount: 1,
    scale: 1,
    fit: true,
    // The page as Go described it, in points.
    box: [0, 0, 595, 842],
    rotate: 0,
    // The stamp, in points.
    stampW: 190,
    stampH: 48,
    // The position, in points: the stamp's lower-left corner in the
    // page's own coordinates.
    x: 0,
    y: 0,
    bounds: { minX: 0, minY: 0, maxX: 0, maxY: 0 },
    corners: [],
    snapDistance: 10,
    zoomSteps: [0.5, 0.75, 1, 1.25, 1.5, 2, 3, 4],
    pending: null,
    ready: false,
    snapped: "",
    saved: null
  };

  var el = {};
  ["viewport", "canvas", "page-image", "margin-guide", "stamp", "stamp-image",
    "preview-note", "position-readout", "snap-readout", "page-input", "page-of",
    "zoom-btn", "first-btn", "prev-btn", "next-btn", "last-btn",
    "zoom-in-btn", "zoom-out-btn", "cancel-btn", "use-btn",
    "saved-readout", "saved-reset-btn"].forEach(function (id) {
    el[id] = document.getElementById(id);
  });

  // --- points <-> pixels -------------------------------------------

  function displaySizePt() {
    var w = state.box[2] - state.box[0];
    var h = state.box[3] - state.box[1];
    if (state.rotate === 90 || state.rotate === 270) return { w: h, h: w };
    return { w: w, h: h };
  }

  // toDisplay mirrors placement.View.ToDisplay in Go. The four cases
  // are the page's rotation; there is no fifth.
  function toDisplay(x, y) {
    var b = state.box, s = state.scale;
    switch (state.rotate) {
      case 90: return { x: (y - b[1]) * s, y: (x - b[0]) * s };
      case 180: return { x: (b[2] - x) * s, y: (y - b[1]) * s };
      case 270: return { x: (b[3] - y) * s, y: (b[2] - x) * s };
      default: return { x: (x - b[0]) * s, y: (b[3] - y) * s };
    }
  }

  function toPDF(dx, dy) {
    var b = state.box, s = state.scale;
    if (!s) return { x: b[0], y: b[1] };
    switch (state.rotate) {
      case 90: return { x: b[0] + dy / s, y: b[1] + dx / s };
      case 180: return { x: b[2] - dx / s, y: b[1] + dy / s };
      case 270: return { x: b[2] - dy / s, y: b[3] - dx / s };
      default: return { x: b[0] + dx / s, y: b[3] - dy / s };
    }
  }

  // toPDFDelta converts a movement on screen, in *points*, into a
  // movement in the page's coordinates. Doing it on a direction rather
  // than a point is what drops the translation and keeps only the
  // rotation, so a drag reads the same however the page is turned.
  function toPDFDelta(dxPt, dyPt) {
    switch (state.rotate) {
      case 90: return { x: dyPt, y: dxPt };
      case 180: return { x: -dxPt, y: dyPt };
      case 270: return { x: -dyPt, y: -dxPt };
      default: return { x: dxPt, y: -dyPt };
    }
  }

  // footprint is the stamp's size in the page's own coordinates, which
  // swaps on a quarter-turned page because the stamp is drawn upright
  // as displayed.
  function footprint() {
    if (state.rotate === 90 || state.rotate === 270) {
      return { w: state.stampH, h: state.stampW };
    }
    return { w: state.stampW, h: state.stampH };
  }

  function clampPosition(x, y) {
    var b = state.bounds;
    return {
      x: Math.min(Math.max(x, b.minX), Math.max(b.minX, b.maxX)),
      y: Math.min(Math.max(y, b.minY), Math.max(b.minY, b.maxY))
    };
  }

  // snapPosition pulls the stamp to a corner when it is close enough,
  // and says which. The corners come from Go, computed by the same code
  // that places a stamp when a corner is chosen by name — so a stamp
  // snapped to the bottom right lands exactly where choosing
  // "bottom-right" would have put it.
  function snapPosition(x, y) {
    var best = null, bestD = state.snapDistance;
    state.corners.forEach(function (c) {
      var d = Math.hypot(c.x - x, c.y - y);
      if (d <= bestD) { bestD = d; best = c; }
    });
    if (best) return { x: best.x, y: best.y, name: best.name };
    return { x: x, y: y, name: "" };
  }

  // --- drawing ------------------------------------------------------

  function layout() {
    var size = displaySizePt();
    var w = Math.round(size.w * state.scale);
    var h = Math.round(size.h * state.scale);
    el.canvas.style.width = w + "px";
    el.canvas.style.height = h + "px";

    var m = state.margin * state.scale;
    el["margin-guide"].style.left = m + "px";
    el["margin-guide"].style.top = m + "px";
    el["margin-guide"].style.width = Math.max(0, w - 2 * m) + "px";
    el["margin-guide"].style.height = Math.max(0, h - 2 * m) + "px";

    drawStamp();
  }

  function drawStamp() {
    var f = footprint();
    var a = toDisplay(state.x, state.y);
    var b = toDisplay(state.x + f.w, state.y + f.h);
    var left = Math.min(a.x, b.x), top = Math.min(a.y, b.y);
    el.stamp.style.left = left + "px";
    el.stamp.style.top = top + "px";
    el.stamp.style.width = (state.stampW * state.scale) + "px";
    el.stamp.style.height = (state.stampH * state.scale) + "px";
    el.stamp.hidden = false;

    window.liroSetText(el["position-readout"],
      "x: " + Math.round(state.x) + ", y: " + Math.round(state.y));
    if (state.snapped) {
      window.liroSetText(el["snap-readout"], window.liroT("place.snapped_to") + " " + snapLabel(state.snapped));
      el.stamp.classList.add("is-snapped");
    } else {
      window.liroSetText(el["snap-readout"], "");
      el.stamp.classList.remove("is-snapped");
    }
  }

  function snapLabel(name) {
    return window.liroT("place.corner_" + name.replace(/-/g, "_"));
  }

  // scrollStampIntoView is what a page change needs: the stamp sits near
  // the foot of a page that may be taller than the window, and arriving
  // on a new page looking at its head means looking at empty paper.
  function scrollStampIntoView() {
    var vp = el.viewport;
    var left = parseFloat(el.stamp.style.left) || 0;
    var top = parseFloat(el.stamp.style.top) || 0;
    var w = parseFloat(el.stamp.style.width) || 0;
    var h = parseFloat(el.stamp.style.height) || 0;
    var pad = parseFloat(getComputedStyle(vp).paddingTop) || 0;
    var padL = parseFloat(getComputedStyle(vp).paddingLeft) || 0;
    var canvasLeft = el.canvas.offsetLeft;
    var x0 = canvasLeft + left, x1 = x0 + w;
    var y0 = pad + top, y1 = y0 + h;
    if (y1 > vp.scrollTop + vp.clientHeight) {
      vp.scrollTop = y1 - vp.clientHeight + pad;
    } else if (y0 < vp.scrollTop) {
      vp.scrollTop = Math.max(0, y0 - pad);
    }
    if (x1 > vp.scrollLeft + vp.clientWidth) {
      vp.scrollLeft = x1 - vp.clientWidth + padL;
    } else if (x0 < vp.scrollLeft) {
      vp.scrollLeft = Math.max(0, x0 - padL);
    }
  }

  function drawChrome() {
    el["page-input"].value = String(state.page);
    el["page-input"].max = String(state.pageCount);
    window.liroSetText(el["page-of"], window.liroT("place.of") + " " + state.pageCount);
    window.liroSetText(el["zoom-btn"],
      state.fit ? window.liroT("place.fit") : Math.round(state.scale * 100) + "%");
    el["first-btn"].disabled = state.page <= 1;
    el["prev-btn"].disabled = state.page <= 1;
    el["next-btn"].disabled = state.page >= state.pageCount;
    el["last-btn"].disabled = state.page >= state.pageCount;
  }

  // --- asking Go for a page ----------------------------------------

  // Every page change and every zoom change needs a new image, which
  // only Go can produce. The request travels the way the timestamp
  // choice and the settings form already do: the page sends one of the
  // three messages and Go reads what it means back out of the page
  // (D-095). The message surface is still exactly three types.
  var requestTimer = null;
  function requestPage() {
    if (requestTimer) clearTimeout(requestTimer);
    requestTimer = setTimeout(function () {
      requestTimer = null;
      state.pending = { action: "render", page: state.page, scale: state.scale };
      window.liroSend("approve");
    }, 90);
  }

  window.__liroPlacementRequest = function () {
    var req = state.pending || { action: "done", page: state.page, scale: state.scale };
    state.pending = null;
    req.x = state.x;
    req.y = state.y;
    return JSON.stringify(req);
  };

  // --- zoom ---------------------------------------------------------

  function fitScale() {
    var size = displaySizePt();
    var vw = el.viewport.clientWidth - 32;
    var vh = el.viewport.clientHeight - 32;
    if (size.w <= 0 || size.h <= 0 || vw <= 0 || vh <= 0) return 1;
    return Math.min(vw / size.w, vh / size.h);
  }

  // setScale keeps the point the person is looking at under the place
  // they are looking at it (F6b §2.3): the anchor is in *content*
  // coordinates, computed before the scale changes and restored after,
  // so zooming in on a signature line keeps that line where it was.
  function setScale(next, anchorClientX, anchorClientY, isFit) {
    next = Math.min(Math.max(next, 0.05), 8);
    var rect = el.canvas.getBoundingClientRect();
    var ax, ay, hadAnchor = false;
    if (typeof anchorClientX === "number") {
      var px = anchorClientX - rect.left;
      var py = anchorClientY - rect.top;
      var p = toPDF(px, py);
      ax = p.x; ay = p.y; hadAnchor = true;
      var offX = anchorClientX - el.viewport.getBoundingClientRect().left;
      var offY = anchorClientY - el.viewport.getBoundingClientRect().top;
      state.anchorOffset = { x: offX, y: offY };
    }
    state.scale = next;
    state.fit = !!isFit;
    layout();
    drawChrome();
    if (hadAnchor) {
      var d = toDisplay(ax, ay);
      el.viewport.scrollLeft = d.x + parseFloat(getComputedStyle(el.viewport).paddingLeft) - state.anchorOffset.x;
      el.viewport.scrollTop = d.y + parseFloat(getComputedStyle(el.viewport).paddingTop) - state.anchorOffset.y;
    }
    requestPage();
  }

  function viewportCentreClient() {
    var r = el.viewport.getBoundingClientRect();
    return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
  }

  function stepZoom(direction) {
    var current = state.scale;
    var steps = state.zoomSteps;
    var next = current;
    if (direction > 0) {
      for (var i = 0; i < steps.length; i++) {
        if (steps[i] > current + 1e-6) { next = steps[i]; break; }
      }
    } else {
      for (var j = steps.length - 1; j >= 0; j--) {
        if (steps[j] < current - 1e-6) { next = steps[j]; break; }
      }
    }
    var c = viewportCentreClient();
    setScale(next, c.x, c.y, false);
  }

  // --- dragging -----------------------------------------------------

  var drag = null;
  el.stamp.addEventListener("pointerdown", function (ev) {
    if (!state.ready) return;
    el.stamp.setPointerCapture(ev.pointerId);
    var rect = el.stamp.getBoundingClientRect();
    drag = { grabX: ev.clientX - rect.left, grabY: ev.clientY - rect.top };
    el.stamp.classList.add("is-dragging");
    ev.preventDefault();
  });

  el.stamp.addEventListener("pointermove", function (ev) {
    if (!drag) return;
    var canvasRect = el.canvas.getBoundingClientRect();
    var left = ev.clientX - canvasRect.left - drag.grabX;
    var top = ev.clientY - canvasRect.top - drag.grabY;
    moveToDisplayCorner(left, top);
    ev.preventDefault();
  });

  function endDrag(ev) {
    if (!drag) return;
    drag = null;
    el.stamp.classList.remove("is-dragging");
    if (ev && ev.pointerId !== undefined && el.stamp.hasPointerCapture(ev.pointerId)) {
      el.stamp.releasePointerCapture(ev.pointerId);
    }
  }
  el.stamp.addEventListener("pointerup", endDrag);
  el.stamp.addEventListener("pointercancel", endDrag);

  // moveToDisplayCorner takes the stamp's top-left corner on screen and
  // works out where its lower-left corner is in the page — by mapping
  // all four corners back and taking the smallest of each, which is
  // exactly what Go does and needs no per-rotation case of its own.
  function moveToDisplayCorner(left, top) {
    var w = state.stampW * state.scale;
    var h = state.stampH * state.scale;
    var minX = Infinity, minY = Infinity;
    [[left, top], [left + w, top], [left + w, top + h], [left, top + h]].forEach(function (c) {
      var p = toPDF(c[0], c[1]);
      minX = Math.min(minX, p.x);
      minY = Math.min(minY, p.y);
    });
    setPosition(minX, minY);
  }

  function setPosition(x, y) {
    var c = clampPosition(x, y);
    var s = snapPosition(c.x, c.y);
    var clampedSnap = clampPosition(s.x, s.y);
    state.x = clampedSnap.x;
    state.y = clampedSnap.y;
    state.snapped = s.name;
    drawStamp();
  }

  // --- keyboard -----------------------------------------------------

  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape") { window.liroSend("cancel"); return; }
    if (ev.target === el["page-input"]) {
      if (ev.key === "Enter") {
        goToPage(parseInt(el["page-input"].value, 10));
        ev.preventDefault();
      }
      return;
    }
    var step = ev.shiftKey ? 10 : 1;
    var d = null;
    switch (ev.key) {
      case "ArrowLeft": d = toPDFDelta(-step, 0); break;
      case "ArrowRight": d = toPDFDelta(step, 0); break;
      case "ArrowUp": d = toPDFDelta(0, -step); break;
      case "ArrowDown": d = toPDFDelta(0, step); break;
      case "PageUp": goToPage(state.page - 1); ev.preventDefault(); return;
      case "PageDown": goToPage(state.page + 1); ev.preventDefault(); return;
      default: return;
    }
    // A nudge is deliberate, so it is never snapped away from where it
    // was aimed: snapping exists to make a corner effortless with a
    // mouse, and an arrow key is the opposite of that.
    var c = clampPosition(state.x + d.x, state.y + d.y);
    state.x = c.x;
    state.y = c.y;
    state.snapped = "";
    drawStamp();
    ev.preventDefault();
  });

  // Ctrl and the wheel zooms, anchored where the pointer is; the wheel
  // alone scrolls, which is the browser's own behaviour and is left
  // alone (F6b §2.3).
  el.viewport.addEventListener("wheel", function (ev) {
    if (!ev.ctrlKey) return;
    ev.preventDefault();
    var factor = ev.deltaY < 0 ? 1.15 : 1 / 1.15;
    setScale(state.scale * factor, ev.clientX, ev.clientY, false);
  }, { passive: false });

  // --- paging -------------------------------------------------------

  function goToPage(n) {
    if (!(n >= 1)) n = 1;
    if (n > state.pageCount) n = state.pageCount;
    if (n === state.page) return;
    state.page = n;
    drawChrome();
    requestPage();
  }

  el["first-btn"].addEventListener("click", function () { goToPage(1); });
  el["prev-btn"].addEventListener("click", function () { goToPage(state.page - 1); });
  el["next-btn"].addEventListener("click", function () { goToPage(state.page + 1); });
  el["last-btn"].addEventListener("click", function () { goToPage(state.pageCount); });
  el["page-input"].addEventListener("change", function () {
    goToPage(parseInt(el["page-input"].value, 10));
  });
  el["zoom-in-btn"].addEventListener("click", function () { stepZoom(1); });
  el["zoom-out-btn"].addEventListener("click", function () { stepZoom(-1); });
  el["zoom-btn"].addEventListener("click", function () {
    setScale(fitScale(), undefined, undefined, true);
  });

  // Back to the position this document was last signed at: the same
  // page, the same point. A different page needs a new image, so it
  // takes the ordinary render path; the same page only moves the
  // rectangle.
  el["saved-reset-btn"].addEventListener("click", function () {
    if (!state.saved) return;
    state.x = state.saved.x;
    state.y = state.saved.y;
    state.snapped = "";
    if (state.saved.page !== state.page) {
      state.page = state.saved.page;
      requestPage();
      return;
    }
    drawStamp();
  });

  el["cancel-btn"].addEventListener("click", function () { window.liroSend("cancel"); });
  el["use-btn"].addEventListener("click", function () {
    state.pending = { action: "done", page: state.page, scale: state.scale };
    window.liroSend("approve");
  });

  // --- messages from Go ---------------------------------------------

  window.__liroOnMessage = function (payload) {
    if (payload.type === "init") {
      window.liroApplyStaticStrings();
      window.liroSetText(el["cancel-btn"], payload.cancelLabel || "");
      window.liroSetText(el["use-btn"], payload.useLabel || "");
      window.liroSetText(el["first-btn"], payload.firstLabel || "");
      window.liroSetText(el["prev-btn"], payload.prevLabel || "");
      window.liroSetText(el["next-btn"], payload.nextLabel || "");
      window.liroSetText(el["last-btn"], payload.lastLabel || "");
      state.pageCount = payload.pageCount || 1;
      state.margin = payload.margin || 12;
      state.snapDistance = payload.snapDistance || 10;
      if (payload.zoomSteps && payload.zoomSteps.length) state.zoomSteps = payload.zoomSteps;
      if (payload.stampImage) el["stamp-image"].src = payload.stampImage;
      state.stampW = payload.stampWidth || 190;
      state.stampH = payload.stampHeight || 48;
      // What this document was last signed at, if anything: a line
      // saying where, and a button that puts the stamp back there.
      state.saved = payload.saved || null;
      window.liroSetText(el["saved-readout"], payload.savedText || "");
      window.liroSetText(el["saved-reset-btn"], payload.savedResetLabel || "");
      el["saved-readout"].hidden = !state.saved;
      el["saved-reset-btn"].hidden = !state.saved;
      drawChrome();
      return;
    }
    if (payload.type === "page") {
      state.page = payload.page;
      state.box = payload.box;
      state.rotate = payload.rotate || 0;
      state.bounds = payload.bounds;
      state.corners = payload.corners || [];
      if (payload.pageCount) state.pageCount = payload.pageCount;
      if (typeof payload.x === "number") state.x = payload.x;
      if (typeof payload.y === "number") state.y = payload.y;
      if (payload.image) el["page-image"].src = payload.image;
      if (state.fit || !state.ready) {
        state.scale = payload.scale || fitScale();
      }
      var pageChanged = state.shownPage !== payload.page;
      state.shownPage = payload.page;
      state.ready = true;
      // A position carried over from another page may be off this one.
      setPosition(state.x, state.y);
      layout();
      drawChrome();
      if (pageChanged) scrollStampIntoView();
      if (state.fit) {
        // Fit is recomputed for this page's own shape: a landscape page
        // in a portrait document has a different one.
        var f = fitScale();
        if (Math.abs(f - state.scale) > 0.001) {
          state.scale = f;
          layout();
          drawChrome();
          requestPage();
        }
      }
      return;
    }
    if (payload.type === "note") {
      window.liroSetText(el["preview-note"], payload.text || "");
      el["preview-note"].hidden = !payload.text;
      return;
    }
  };
})();
