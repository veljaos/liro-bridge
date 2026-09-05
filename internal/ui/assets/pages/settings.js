(function () {
  "use strict";

  var pendingAction = "";
  var presetURLs = {};

  // syncPresetSelection checks the preset whose URL is exactly what the
  // URL field holds, and none when it matches neither (Task 1c: neither
  // preset is preselected, and a hand-typed URL never silently claims
  // to be one of them).
  // It registers no event handlers. It ran with a copy of the stamp
  // button's click handler pasted into it, so every keystroke in the URL
  // field added another: measured at seven messages for one press of
  // the stamp button after five keystrokes, which is seven stamp
  // windows opened one after another.
  function syncPresetSelection() {
    var url = document.getElementById("tsa-url").value;
    document.querySelectorAll("input[name=tsa-preset]").forEach(function (el) {
      el.checked = presetURLs[el.value] === url && url !== "";
    });
  }

  function showStatus(status) {
    var el = document.getElementById("action-status");
    window.liroSetText(document.getElementById("action-status-text"), status.text);
    window.liroRenderStatusFiles(document.getElementById("action-status-files"), status.files);
    el.className = "liro-fixed-region" + (status.intent ? " liro-outcome-" + status.intent : "");
    el.hidden = !status.text;
  }

  window.__liroOnMessage = function (payload) {
    if (payload.type === "status") {
      showStatus(payload.status);
      return;
    }
    if (payload.type !== "init") return;
    window.liroApplyStaticStrings();
    // A freshly opened window says nothing about what an action did:
    // the status line starts empty, so reopening Settings never shows a
    // stale "exported to ..." from a previous session.
    showStatus({ text: "", intent: "" });
    var m = payload.model;
    (m.tsaPresets || []).forEach(function (p) { presetURLs[p.id] = p.url; });
    document.getElementById("locale").value = m.locale;
    document.getElementById("start-with-windows").checked = !!m.startWithWindows;
    document.getElementById("tsa-url").value = m.tsaURL || "";
    document.getElementById("tsa-user").value = m.tsaUser || "";
    document.getElementById("tsa-password").value = m.tsaPassword || "";
    document.getElementById("tsa-client-cert-path").value = m.tsaClientCertPath || "";
    document.getElementById("tsa-client-cert-password").value = m.tsaClientCertPassword || "";
    document.getElementById("output-suffix").value = m.outputSuffix || "";
    document.getElementById("output-folder").value = m.outputFolder || "";
    document.getElementById("explorer-menu").checked = !!m.explorerMenu;
    document.getElementById("check-updates-daily").checked = !!m.checkUpdatesDaily;
    window.liroSetText(document.getElementById("version"), m.version);
    // Task 3 (F5 fourth-real-run review): three levels, B-B included.
    // An unrecognised saved value falls back to the project's default
    // rather than leaving every radio unchecked — Go validates the
    // same way (config.validate), so this only ever fires for a value
    // that was already replaced there.
    var levelIDs = { "b-b": "level-bb", "b-t": "level-bt", "b-lt": "level-blt" };
    document.getElementById(levelIDs[m.signatureLevel] || "level-blt").checked = true;
    syncPresetSelection();
  };

  // __liroCollectState is read back by Go via Window.Eval (never a
  // page->Go message — see internal/ui's Window.Eval doc comment /
  // D-08x) after the page sends "approve" with a pendingAction set.
  window.__liroCollectState = function () {
    var checkedLevel = document.querySelector("input[name=level]:checked");
    var level = checkedLevel ? checkedLevel.value : "b-lt";
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
      outputFolder: document.getElementById("output-folder").value,
      explorerMenu: document.getElementById("explorer-menu").checked,
      signatureLevel: level,
      checkUpdatesDaily: document.getElementById("check-updates-daily").checked,
    });
  };

  function act(action) {
    pendingAction = action;
    window.liroSend("approve");
  }

  document.getElementById("stamp-settings-btn").addEventListener("click", function () {
    act("stampSettings");
  });

  document.querySelectorAll("input[name=tsa-preset]").forEach(function (el) {
    el.addEventListener("change", function () {
      if (!el.checked) return;
      document.getElementById("tsa-url").value = presetURLs[el.value] || "";
    });
  });
  document.getElementById("tsa-url").addEventListener("input", syncPresetSelection);

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
