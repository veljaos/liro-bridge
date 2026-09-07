(function () {
  "use strict";

  // The page holds no state of its own: which screen is up is whatever
  // Go last said, every time. A page that remembers is a page that
  // shows the previous request's answer to the next one.
  function show(which) {
    document.getElementById("state-code").hidden = which !== "code";
    document.getElementById("state-connected").hidden = which !== "connected";
  }

  window.__liroOnMessage = function (payload) {
    if (payload.type === "init") {
      window.liroApplyStaticStrings();
      window.liroSetText(document.getElementById("app-name"), payload.model.applicationName);
      window.liroSetText(document.getElementById("app-origin"), payload.model.origin);
      window.liroSetText(document.getElementById("pairing-code"), payload.model.code);
      window.liroSetText(document.getElementById("connected-name"), payload.model.applicationName);
      show("code");
      // Nothing affirmative is ever the initially focused control in
      // this program (F5 §5.6); here the only button is the refusal, so
      // focusing it costs nothing and keeps the window keyboard-usable
      // from the moment it opens.
      document.getElementById("deny-btn").focus();
      return;
    }
    if (payload.type === "connected") {
      // The code has been spent. Clearing it means it is gone from the
      // window rather than merely hidden behind the screen in front.
      window.liroSetText(document.getElementById("pairing-code"), "");
      show("connected");
      document.getElementById("close-btn").focus();
    }
  };

  document.getElementById("deny-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });
  document.getElementById("close-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });
  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape") window.liroSend("cancel");
  });
})();
