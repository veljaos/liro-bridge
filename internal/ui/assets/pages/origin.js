(function () {
  "use strict";

  window.__liroOnMessage = function (payload) {
    if (payload.type !== "init") return;
    window.liroApplyStaticStrings();
    window.liroSetText(document.getElementById("origin"), payload.model.origin);
    // Nothing affirmative is ever the initially focused control in this
    // program (F5 §5.6): Enter on a window that just appeared refuses.
    document.getElementById("deny-btn").focus();
  };

  document.getElementById("deny-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });
  document.getElementById("allow-btn").addEventListener("click", function () {
    window.liroSend("approve");
  });
  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape") window.liroSend("cancel");
  });
})();
