(function () {
  "use strict";

  var pendingAction = "";

  window.__liroOnMessage = function (payload) {
    if (payload.type !== "init") return;
    window.liroApplyStaticStrings();
    var m = payload.model;
    document.getElementById("locale").value = m.locale;
    document.getElementById("start-with-windows").checked = !!m.startWithWindows;
    document.getElementById("tsa-url").value = m.tsaURL || "";
    document.getElementById("tsa-user").value = m.tsaUser || "";
    document.getElementById("tsa-password").value = m.tsaPassword || "";
    document.getElementById("tsa-client-cert-path").value = m.tsaClientCertPath || "";
    document.getElementById("tsa-client-cert-password").value = m.tsaClientCertPassword || "";
    document.getElementById("output-suffix").value = m.outputSuffix || "";
    document.getElementById("check-updates-daily").checked = !!m.checkUpdatesDaily;
    window.liroSetText(document.getElementById("version"), m.version);
    if (m.signatureLevel === "b-t") {
      document.getElementById("level-bt").checked = true;
    } else {
      document.getElementById("level-blt").checked = true;
    }
  };

  // __liroCollectState is read back by Go via Window.Eval (never a
  // page->Go message — see internal/ui's Window.Eval doc comment /
  // D-08x) after the page sends "approve" with a pendingAction set.
  window.__liroCollectState = function () {
    var level = document.getElementById("level-bt").checked ? "b-t" : "b-lt";
    return JSON.stringify({
      action: pendingAction,
      locale: document.getElementById("locale").value,
      startWithWindows: document.getElementById("start-with-windows").checked,
      tsaURL: document.getElementById("tsa-url").value,
      tsaUser: document.getElementById("tsa-user").value,
      tsaPassword: document.getElementById("tsa-password").value,
      tsaClientCertPath: document.getElementById("tsa-client-cert-path").value,
      tsaClientCertPassword: document.getElementById("tsa-client-cert-password").value,
      outputSuffix: document.getElementById("output-suffix").value,
      signatureLevel: level,
      checkUpdatesDaily: document.getElementById("check-updates-daily").checked,
    });
  };

  function act(action) {
    pendingAction = action;
    window.liroSend("approve");
  }

  document.getElementById("save-btn").addEventListener("click", function () { act("save"); });
  document.getElementById("export-audit-btn").addEventListener("click", function () { act("exportAuditLog"); });
  document.getElementById("check-updates-btn").addEventListener("click", function () { act("checkUpdatesNow"); });
  document.getElementById("close-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });
  document.getElementById("copy-version-btn").addEventListener("click", function () {
    var text = document.getElementById("version").textContent;
    if (navigator.clipboard) navigator.clipboard.writeText(text);
  });
})();
