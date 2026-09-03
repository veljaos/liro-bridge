(function () {
  "use strict";

  window.__liroOnMessage = function (payload) {
    if (payload.type !== "init") return;
    window.liroApplyStaticStrings();
    renderCertList(payload.model.certificates || []);
  };

  function renderCertList(certs) {
    var list = document.getElementById("cert-list");
    list.innerHTML = "";
    document.getElementById("empty").hidden = certs.length !== 0;

    certs.forEach(function (cert) {
      var row = document.createElement("div");
      row.className = "liro-cert-row" + (cert.usable ? "" : " liro-cert-row-disabled");
      row.setAttribute("role", "listitem");

      var nameLine = document.createElement("div");
      window.liroSetText(nameLine, cert.displayName);
      row.appendChild(nameLine);

      var metaLine = document.createElement("div");
      metaLine.className = "cert-role";
      window.liroSetText(metaLine, cert.roleText + " — " + cert.issuerText + " — …" + cert.thumbprintTail);
      row.appendChild(metaLine);

      if (cert.isTestKey) {
        var badge = document.createElement("span");
        badge.className = "liro-badge liro-badge-test-key";
        window.liroSetText(badge, cert.testKeyLabel);
        row.appendChild(badge);
      }
      if (cert.qualified) {
        var qbadge = document.createElement("span");
        qbadge.className = "liro-badge liro-badge-qualified";
        window.liroSetText(qbadge, window.liroT("certswindow.qualified_badge"));
        row.appendChild(qbadge);
      }

      if (!cert.usable) {
        var reason = document.createElement("div");
        reason.className = "cert-reason";
        window.liroSetText(reason, cert.disabledReasonText);
        row.appendChild(reason);
      }

      list.appendChild(row);
    });
  }

  document.getElementById("close-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });

  // Matches every other window's Escape-cancels convention (F5 §5.6).
  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape") {
      window.liroSend("cancel");
    }
  });
})();
