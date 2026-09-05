(function () {
  "use strict";

  // Step 3's page. Like the settings window, it reports its whole form
  // back through one read (D-083) rather than adding a fourth page->Go
  // message type.
  var saved = false;
  var showMore = false;

  window.__liroStampSettings = function () {
    return JSON.stringify({
      saved: saved,
      visible: document.getElementById("stamp-mode").value === "visible",
      position: document.getElementById("stamp-position").value,
      page: pageValue(),
      reference: document.getElementById("stamp-reference").value,
      showDocumentID: document.getElementById("stamp-document-id").checked
    });
  };

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

  window.__liroOnMessage = function (payload) {
    if (payload.type !== "init") return;
    // A fresh init is a fresh form: the page holds no state of its own
    // beyond what Go has told it, and a Save from a previous posting
    // must not be reported for this one.
    saved = false;
    window.liroApplyStaticStrings();
    fillOptions("stamp-mode", payload.modes || []);
    fillOptions("stamp-position", payload.positions || []);
    fillOptions("stamp-page", payload.pages || []);

    // The two action labels come from Go because they differ by where
    // this window was opened from: signing a batch ends in "Sign",
    // editing a standing preference ends in "Save".
    window.liroSetText(document.getElementById("save-btn"), payload.primaryLabel || "");
    window.liroSetText(document.getElementById("cancel-btn"), payload.secondaryLabel || "");

    var m = payload.model || {};
    document.getElementById("stamp-mode").value = m.visible === true ? "visible" : "invisible";
    document.getElementById("stamp-position").value = m.position || "bottom-right";
    document.getElementById("stamp-reference").value = m.reference || "";
    document.getElementById("stamp-document-id").checked = m.showDocumentID === true;

    if (m.page === "first" || m.page === "last") {
      document.getElementById("stamp-page").value = m.page;
      document.getElementById("stamp-page-number").value = "1";
    } else {
      document.getElementById("stamp-page").value = "number";
      document.getElementById("stamp-page-number").value = m.page || "1";
    }

    // Step 3 asks one thing. The standing preferences below belong to
    // Settings and are not on screen at all when this window is the
    // last step before a signature.
    showMore = payload.showMore === true;
    // Hidden, not merely empty: an empty paragraph still takes its
    // line box and its gap, which on a 440-point window is a visible
    // band of nothing under the title.
    var subtitle = document.getElementById("stamp-subtitle");
    window.liroSetText(subtitle, payload.subtitle || "");
    subtitle.hidden = !payload.subtitle;

    syncVisibility();
    // Nothing that signs is focused first, matching every other window
    // (F5 §5.6 / D-085).
    document.getElementById("cancel-btn").focus();
  };

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

  // A control for something nobody is drawing is noise, so the corner
  // and the rest disappear for an invisible signature — and the
  // page-number box only appears when a specific page is what was
  // asked for.
  function syncVisibility() {
    var visible = document.getElementById("stamp-mode").value === "visible";
    document.getElementById("stamp-position-field").hidden = !visible;
    document.getElementById("stamp-more").hidden = !(visible && showMore);
    document.getElementById("stamp-page-number-field").hidden =
      document.getElementById("stamp-page").value !== "number";
  }

  document.getElementById("stamp-mode").addEventListener("change", syncVisibility);
  document.getElementById("stamp-page").addEventListener("change", syncVisibility);

  document.getElementById("save-btn").addEventListener("click", function () {
    saved = true;
    window.liroSend("approve");
  });
  document.getElementById("cancel-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });
  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape") {
      window.liroSend("cancel");
    }
  });
})();
