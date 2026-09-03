(function () {
  "use strict";

  window.__liroOnMessage = function (payload) {
    if (payload.type !== "init") return;
    window.liroApplyStaticStrings();
    renderEntries(payload.model.entries || []);
  };

  function renderEntries(entries) {
    var list = document.getElementById("entry-list");
    list.innerHTML = "";
    document.getElementById("empty").hidden = entries.length !== 0;

    entries.forEach(function (entry) {
      var row = document.createElement("div");
      row.className = "audit-entry";
      row.setAttribute("role", "listitem");

      var top = document.createElement("div");
      var outcomeText = entry.outcomeText;
      if (entry.isTestKey) outcomeText += " — " + entry.testKeyLabel;
      window.liroSetText(top, entry.timestampText + " — " + outcomeText);
      if (entry.outcome !== "approved") {
        top.className = "liro-badge liro-badge-warning";
      }
      row.appendChild(top);

      var meta = document.createElement("div");
      meta.className = "audit-meta";
      window.liroSetText(meta, entry.applicationText + " — " + entry.documentCountText + " — …" + entry.thumbprintTail);
      row.appendChild(meta);

      list.appendChild(row);
    });
  }

  document.getElementById("close-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });

  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape") {
      window.liroSend("cancel");
    }
  });
})();
