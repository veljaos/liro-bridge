// Shared Go<->page bridge (F5 §2.4). Loaded by every window's page.
// Go pushes JSON via ExecuteScript calling window.__liroReceive; the
// page sends exactly one of three messages back via
// window.chrome.webview.postMessage — approve, cancel, selectCertificate.
(function () {
  "use strict";

  var strings = {};

  window.__liroReceive = function (payload) {
    if (!payload || typeof payload !== "object") return;
    if (payload.type === "init" && payload.strings) {
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
