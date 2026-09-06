// Shared Go<->page bridge (F5 §2.4). Loaded by every window's page.
// Go pushes JSON via ExecuteScript calling window.__liroReceive; the
// page sends exactly one of three messages back via
// window.chrome.webview.postMessage — approve, cancel, selectCertificate.
(function () {
  "use strict";

  var strings = {};

  // What the click that sent "approve" meant. The page->Go message
  // surface stays at exactly three types (D-083), so a button reports
  // what it was here and Go reads the record through ExecuteScript's
  // own return value. Shared by every page, because every page in the
  // signing flow now has more than one thing a click can mean — Back,
  // at the very least.
  var pendingAction = null;

  window.__liroAction = function () {
    var a = pendingAction;
    pendingAction = null;
    return JSON.stringify(a || {});
  };

  window.liroAct = function (action, extra) {
    pendingAction = Object.assign({ action: action }, extra || {});
    window.liroSend("approve");
  };

  window.__liroReceive = function (payload) {
    if (!payload || typeof payload !== "object") return;
    // Whichever payload carries them, whatever its type. A navigation
    // is a fresh document with an empty table, and the payload that
    // refills it is not always the one that renders the first screen:
    // the signing window navigates to its main page on the way to the
    // progress screen, and that page's document list must not be shown
    // on the way past (signflow_windows.go's gotoPage).
    if (payload.strings) {
      strings = payload.strings;
    }
    if (typeof window.__liroOnMessage === "function") {
      window.__liroOnMessage(payload);
    }
  };

  // t looks up a pre-localised string by key. Never used to render
  // untrusted data — untrusted text always goes through setText below.
  window.liroT = function (key) {
    return Object.prototype.hasOwnProperty.call(strings, key) ? strings[key] : key;
  };

  window.liroSend = function (type, extra) {
    var msg = Object.assign({ type: type }, extra || {});
    // Pass the object itself, not JSON.stringify(msg): WebView2's
    // postMessage already serialises an object argument, and the native
    // side's WebMessageAsJson returns that serialisation directly. A
    // pre-stringified argument is instead treated as a *string* message,
    // so WebMessageAsJson returns the JSON encoding of that string —
    // i.e. the whole payload re-quoted and escaped one level deeper than
    // internal/ui.ParseMessage (messages.go) expects, which silently
    // dropped every approve/cancel/selectCertificate click as a result
    // (logged as "not one of approve/cancel/selectCertificate", never
    // surfaced to the user — verified directly against a real WebView2
    // window, see docs/decisions.md).
    window.chrome.webview.postMessage(msg);
  };

  // setText inserts untrusted or trusted text via textContent, never
  // innerHTML (SPEC §6.6 / F5 §5.3).
  window.liroSetText = function (el, text) {
    el.textContent = text == null ? "" : String(text);
  };

  // renderStatusFiles fills a status line's file list: one row per file,
  // its name beside what it is. Both windows with an Export button
  // render the same payload in the same place, so the rendering lives
  // here rather than twice.
  //
  // Every value goes in through setText: a file name is a name, never
  // markup (SPEC §6.6).
  window.liroRenderStatusFiles = function (el, files) {
    el.innerHTML = "";
    if (!files || !files.length) {
      el.hidden = true;
      return;
    }
    files.forEach(function (f) {
      var name = document.createElement("span");
      name.className = "liro-status-file-name";
      window.liroSetText(name, f.name);
      el.appendChild(name);
      var detail = document.createElement("span");
      detail.className = "liro-status-file-detail";
      window.liroSetText(detail, f.detail);
      el.appendChild(detail);
    });
    el.hidden = false;
  };

  // renderStep draws the step header every step of the signing flow
  // carries: the way back on the left, and where you are on the right.
  // A missing or absent step object hides the header outright, which is
  // what a window that is not a step of anything gets — Settings, and a
  // request that carries its own answers and shows the approval alone.
  //
  // step: {index, total, back, backText, label}. index is 1-based.
  window.liroRenderStep = function (step) {
    var host = document.getElementById("step-header");
    if (!host) return;
    if (!step || !step.total) {
      host.hidden = true;
      return;
    }
    host.hidden = false;
    host.setAttribute("aria-label", step.label || "");

    var back = document.getElementById("step-back-btn");
    back.hidden = step.back !== true;
    window.liroSetText(back, step.backText || "");

    var dots = document.getElementById("step-dots");
    dots.innerHTML = "";
    for (var i = 1; i <= step.total; i++) {
      var dot = document.createElement("span");
      dot.className = "liro-steps-dot" +
        (i === step.index ? " liro-steps-dot-current" : (i < step.index ? " liro-steps-dot-done" : ""));
      dots.appendChild(dot);
    }
  };

  // applyStaticStrings resolves every element with data-i18n to
  // liroT(key) via textContent — used for the page's own fixed labels,
  // never for batch data.
  window.liroApplyStaticStrings = function () {
    document.querySelectorAll("[data-i18n]").forEach(function (el) {
      window.liroSetText(el, window.liroT(el.getAttribute("data-i18n")));
    });
    document.querySelectorAll("[data-i18n-placeholder]").forEach(function (el) {
      el.setAttribute("placeholder", window.liroT(el.getAttribute("data-i18n-placeholder")));
    });
  };
})();
