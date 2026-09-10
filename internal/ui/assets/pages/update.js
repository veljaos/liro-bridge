(function () {
  "use strict";

  // Which screen is up is whatever Go last said. The page remembers
  // nothing between payloads — the trap D-120 and D-121 each had to
  // remove once.
  function show(which) {
    ["available", "working", "failed"].forEach(function (name) {
      document.getElementById("state-" + name).hidden = name !== which;
    });
  }

  window.__liroOnMessage = function (payload) {
    if (payload.type === "init") {
      window.liroApplyStaticStrings();
      window.liroSetText(document.getElementById("version-line"), payload.model.versionLine);
      window.liroSetText(document.getElementById("released-line"), payload.model.releasedLine);
      show("available");
      // Nothing that changes the machine is the initially focused
      // control (F5 §5.6, applied here for the same reason it applies
      // to Approve): the way out is what the keyboard lands on.
      document.getElementById("later-btn").focus();
      return;
    }
    if (payload.type === "working") {
      show("working");
      return;
    }
    if (payload.type === "failed") {
      window.liroSetText(document.getElementById("failed-body"), payload.model.body);
      show("failed");
      document.getElementById("close-btn").focus();
    }
  };

  document.getElementById("install-btn").addEventListener("click", function () {
    window.liroSend("approve");
  });
  document.getElementById("later-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });
  document.getElementById("close-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });
  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape") window.liroSend("cancel");
  });
})();
