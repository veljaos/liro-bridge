(function () {
  "use strict";

  window.__liroOnMessage = function (payload) {
    if (payload.type !== "init") return;
    window.liroApplyStaticStrings();
    window.liroSetText(document.getElementById("app-name"), payload.model.applicationName);
    window.liroSetText(document.getElementById("app-origin"), payload.model.origin);

    // Same principle as the consent window (F5 §5.6): the affirmative
    // action is never the initially focused control.
    document.getElementById("deny-btn").focus();
  };

  document.getElementById("allow-btn").addEventListener("click", function () {
    window.liroSend("approve");
  });
  document.getElementById("deny-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });
  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape") window.liroSend("cancel");
  });
})();
