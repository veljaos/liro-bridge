(function () {
  "use strict";

  // The stamp window (F6 §6). Like the settings window, it reports its
  // whole form back through one read (D-083) rather than adding a
  // fourth page->Go message type.
  var saved = false;

  window.__liroStampSettings = function () {
    return JSON.stringify({
      saved: saved,
      visible: document.getElementById("stamp-visible").checked,
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
    fillOptions("stamp-position", payload.positions || []);
    fillOptions("stamp-page", payload.pages || []);

    var m = payload.model || {};
    document.getElementById("stamp-visible").checked = m.visible === true;
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

    syncVisibility();
    // Nothing that writes is focused first, matching every other window
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

  // A control for something nobody is drawing is noise, so the details
  // collapse with the checkbox — and the page-number box only appears
  // when a specific page is what was asked for.
  function syncVisibility() {
    document.getElementById("stamp-details").hidden =
      !document.getElementById("stamp-visible").checked;
    document.getElementById("stamp-page-number-field").hidden =
      document.getElementById("stamp-page").value !== "number";
  }

  document.getElementById("stamp-visible").addEventListener("change", syncVisibility);
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
