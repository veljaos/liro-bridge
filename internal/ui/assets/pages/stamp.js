(function () {
  "use strict";

  // The signing method: one choice with three outcomes. Like the
  // settings window, the form reports itself back through one read
  // (D-083) rather than adding a fourth page->Go message type.
  var saved = false;
  var showMore = false;
  // action says what the click that sent "approve" meant: saving or
  // advancing with the form, stepping back, opening the placement
  // window, or giving a placed position back. It is explicit data the
  // page reports rather than something Go infers from which control was
  // last touched (D-095's rule), and it is what keeps the page->Go
  // surface at three message types.
  var action = "save";

  window.__liroStampSettings = function () {
    return JSON.stringify({
      saved: saved,
      action: action,
      method: checked("stamp-method") || "corners",
      position: checked("stamp-corner") || "bottom-right",
      page: pageValue(),
      reference: document.getElementById("stamp-reference").value,
      showDocumentID: document.getElementById("stamp-document-id").checked
    });
  };

  function checked(name) {
    var el = document.querySelector('input[name="' + name + '"]:checked');
    return el ? el.value : "";
  }

  // page is "first", "last", or a decimal page number — the same three
  // shapes config.ValidStampPage accepts, so Go validates exactly what
  // the page can produce.
  function pageValue() {
    var mode = document.getElementById("stamp-page").value;
    if (mode !== "number") return mode;
    var n = parseInt(document.getElementById("stamp-page-number").value, 10);
    if (!(n >= 1)) return "first";
    return String(n);
  }

  function renderStatus(st) {
    var box = document.getElementById("stamp-status");
    window.liroSetText(document.getElementById("stamp-status-text"), st.text || "");
    box.className = "liro-fixed-region" + (st.intent ? " liro-outcome-" + st.intent : "");
    box.hidden = !st.text;
  }

  window.__liroOnMessage = function (payload) {
    if (payload.type === "status") {
      renderStatus(payload.status || {});
      return;
    }
    if (payload.type !== "init") return;
    // A fresh init is a fresh form: the page holds no state of its own
    // beyond what Go has told it, and a Save from a previous posting
    // must not be reported for this one.
    saved = false;
    action = "save";
    // A fresh form says nothing about what the last one did.
    document.getElementById("stamp-status").hidden = true;
    window.liroApplyStaticStrings();
    window.liroRenderStep(payload.step || null);

    fillCorners(payload.positions || []);
    fillOptions("stamp-page", payload.pages || []);

    // The two action labels come from Go because they differ by where
    // this window was opened from: signing a batch ends in "Sign", and
    // editing a standing preference ends in "Save". It is the same word
    // for all three methods — the picker the first one opens is the
    // consequence of pressing Sign, not a screen before it.
    window.liroSetText(document.getElementById("save-btn"), payload.primaryLabel || "");
    window.liroSetText(document.getElementById("cancel-btn"), payload.secondaryLabel || "");
    document.getElementById("cancel-btn").hidden = !payload.secondaryLabel;

    var m = payload.model || {};
    // The method Go says is chosen, never one this page works out for
    // itself: after a document that could not be previewed, the chosen
    // method is not the one the configuration still holds.
    select("stamp-method", m.method || "corners");
    select("stamp-corner", m.position || "bottom-right");
    document.getElementById("stamp-reference").value = m.reference || "";
    document.getElementById("stamp-document-id").checked = m.showDocumentID === true;

    window.liroSetText(document.getElementById("stamp-place-btn"),
      window.liroT("stampwindow.place_button"));
    window.liroSetText(document.getElementById("stamp-placed-reset-btn"),
      window.liroT("stampwindow.placed_reset"));

    if (m.page === "first" || m.page === "last") {
      document.getElementById("stamp-page").value = m.page;
      document.getElementById("stamp-page-number").value = "1";
    } else {
      document.getElementById("stamp-page").value = "number";
      document.getElementById("stamp-page-number").value = m.page || "1";
    }

    // The method screen asks one thing. The standing preferences below
    // belong to Settings and are not on screen at all when this is a
    // step of signing — and neither are the two buttons that change a
    // remembered position, which is a standing preference too.
    showMore = payload.showMore === true;
    // Hidden, not merely empty: an empty paragraph still takes its
    // line box and its gap, which on a 440-point window is a visible
    // band of nothing under the title.
    var subtitle = document.getElementById("stamp-subtitle");
    window.liroSetText(subtitle, payload.subtitle || "");
    subtitle.hidden = !payload.subtitle;

    syncVisibility();
    // Nothing that signs is focused first, matching every other screen
    // (F5 §5.6 / D-085).
    document.getElementById("cancel-btn").focus();
  };

  function select(name, value) {
    var wanted = document.querySelector('input[name="' + name + '"][value="' + value + '"]');
    if (wanted) wanted.checked = true;
    else {
      var first = document.querySelector('input[name="' + name + '"]');
      if (first) first.checked = true;
    }
    syncSelectedCards(name);
  }

  // The chosen card carries a class rather than only the :checked
  // pseudo-class, because what a person sees is the card and not the
  // radio hidden inside it.
  function syncSelectedCards(name) {
    var cls = name === "stamp-method" ? "stamp-method-selected" : "stamp-corner-selected";
    var inputs = document.querySelectorAll('input[name="' + name + '"]');
    Array.prototype.forEach.call(inputs, function (input) {
      var card = input.parentElement;
      if (input.checked) card.classList.add(cls);
      else card.classList.remove(cls);
    });
  }

  function fillOptions(id, options) {
    var select = document.getElementById(id);
    select.innerHTML = "";
    options.forEach(function (o) {
      var el = document.createElement("option");
      el.value = o.value;
      window.liroSetText(el, o.label);
      select.appendChild(el);
    });
  }

  // The corners arrive from Go already in the order they sit on a page
  // — top row first — with their labels already localised.
  function fillCorners(options) {
    var grid = document.getElementById("stamp-corner-grid");
    grid.innerHTML = "";
    grid.setAttribute("aria-label", window.liroT("stampwindow.position_label"));
    options.forEach(function (o) {
      var label = document.createElement("label");
      label.className = "stamp-corner";

      var input = document.createElement("input");
      input.className = "stamp-corner-input";
      input.type = "radio";
      input.name = "stamp-corner";
      input.id = "corner-" + o.value;
      input.value = o.value;
      input.addEventListener("change", function () {
        syncSelectedCards("stamp-corner");
      });
      label.appendChild(input);

      var text = document.createElement("span");
      window.liroSetText(text, o.label);
      label.appendChild(text);

      grid.appendChild(label);
    });
  }

  // Each option reveals the rest of its own answer, and only while it
  // is the chosen one: which corner, or — from Settings, where a
  // remembered position is a standing preference rather than the thing
  // about to be signed with — what can be done to that position. A
  // control for something nobody is drawing is noise.
  function syncVisibility() {
    var method = checked("stamp-method");
    document.getElementById("stamp-placed").hidden = !(method === "placed" && showMore);
    document.getElementById("stamp-placed-actions").hidden = !(method === "placed" && showMore);
    document.getElementById("stamp-corners").hidden = method !== "corners";
    document.getElementById("stamp-more").hidden = !(showMore && method !== "none");
    document.getElementById("stamp-page-number-field").hidden =
      document.getElementById("stamp-page").value !== "number" ||
      method === "placed";
  }

  Array.prototype.forEach.call(
    document.querySelectorAll('input[name="stamp-method"]'),
    function (input) {
      input.addEventListener("change", function () {
        syncSelectedCards("stamp-method");
        syncVisibility();
      });
    }
  );
  document.getElementById("stamp-page").addEventListener("change", syncVisibility);

  function send(a) {
    action = a;
    saved = a === "save";
    window.liroAct(a);
  }

  document.getElementById("stamp-place-btn").addEventListener("click", function () { send("place"); });
  document.getElementById("stamp-placed-reset-btn").addEventListener("click", function () { send("reset"); });
  document.getElementById("step-back-btn").addEventListener("click", function () { send("back"); });
  document.getElementById("save-btn").addEventListener("click", function () { send("save"); });
  document.getElementById("cancel-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });
  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape") {
      window.liroSend("cancel");
    }
  });
})();
