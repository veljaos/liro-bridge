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

      // Task 6: timestamp on the left, the outcome as a single word,
      // right-aligned, coloured by intent — never a sentence mixing
      // both together.
      var top = document.createElement("div");
      top.className = "liro-row";
      var timestamp = document.createElement("span");
      window.liroSetText(timestamp, entry.timestampText);
      top.appendChild(timestamp);
      var outcome = document.createElement("span");
      outcome.className = "liro-outcome liro-outcome-" + entry.outcomeIntent;
      window.liroSetText(outcome, entry.outcomeText);
      top.appendChild(outcome);
      row.appendChild(top);

      var meta = document.createElement("div");
      meta.className = "audit-meta";
      var metaText = entry.applicationText + " — " + entry.documentCountText;
      if (entry.isTestKey) metaText += " — " + entry.testKeyLabel;
      window.liroSetText(meta, metaText);
      meta.appendChild(document.createTextNode(" · "));
      var thumb = document.createElement("span");
      thumb.className = "liro-cert-thumb";
      window.liroSetText(thumb, entry.thumbprintTail);
      meta.appendChild(thumb);
      row.appendChild(meta);

      // Task 1: the level the batch actually reached, marked when it is
      // B-B. Absent entirely for an entry that produced no signature.
      if (entry.levelText) {
        var level = document.createElement("div");
        level.className = "audit-level" + (entry.levelIntent ? " liro-outcome-" + entry.levelIntent : "");
        window.liroSetText(level, entry.levelText);
        row.appendChild(level);
      }

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
