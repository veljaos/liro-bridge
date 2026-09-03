(function () {
  "use strict";

  var states = ["waiting", "preparing", "signing", "done", "failed"];
  var model = null;
  var selectedThumbprint = null;

  function showState(name) {
    states.forEach(function (s) {
      document.getElementById("state-" + s).hidden = s !== name;
    });
  }

  function renderCertList() {
    var list = document.getElementById("cert-list");
    list.innerHTML = "";
    var anyUsable = false;
    model.certificates.forEach(function (cert) {
      if (cert.usable) anyUsable = true;

      var row = document.createElement("div");
      row.className = "liro-cert-row" + (cert.usable ? "" : " liro-cert-row-disabled");
      row.setAttribute("role", "option");
      row.setAttribute("tabindex", cert.usable ? "0" : "-1");
      row.setAttribute("aria-selected", "false");
      row.setAttribute("aria-disabled", cert.usable ? "false" : "true");

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

      if (!cert.usable) {
        var reason = document.createElement("div");
        reason.className = "cert-reason";
        window.liroSetText(reason, cert.disabledReasonText);
        row.appendChild(reason);
      } else {
        row.addEventListener("click", function () { selectCert(cert, row); });
        row.addEventListener("keydown", function (ev) {
          if (ev.key === "Enter" || ev.key === " ") {
            ev.preventDefault();
            selectCert(cert, row);
          }
        });
      }

      list.appendChild(row);
    });
    document.getElementById("no-usable-cert").hidden = anyUsable;
    document.getElementById("select-prompt").hidden = !anyUsable;
  }

  function selectCert(cert, row) {
    selectedThumbprint = cert.thumbprint;
    document.querySelectorAll(".liro-cert-row").forEach(function (el) {
      el.setAttribute("aria-selected", el === row ? "true" : "false");
    });
    window.liroSetText(document.getElementById("signer-name"), cert.displayName);
    document.getElementById("approve-btn").disabled = false;
    window.liroSend("selectCertificate", { thumbprint: cert.thumbprint });
  }

  function renderWaiting() {
    window.liroSetText(document.getElementById("document-count"), model.documentCountText);
    window.liroSetText(document.getElementById("application-name"), model.applicationName);
    window.liroSetText(document.getElementById("fingerprint"), model.fingerprint);

    var fileList = document.getElementById("file-list");
    fileList.innerHTML = "";
    model.files.forEach(function (f) {
      var li = document.createElement("li");
      window.liroSetText(li, f);
      fileList.appendChild(li);
    });
    window.liroSetText(document.getElementById("file-overflow"), model.filesOverflowText);

    renderCertList();
    showState("waiting");

    // F5 §5.6: Approve is never the initially focused control — Cancel
    // gets it instead, so the user must move to Approve deliberately.
    document.getElementById("cancel-btn").focus();
  }

  function renderProgress(p) {
    if (p.state === "preparingCard") {
      showState("preparing");
    } else if (p.state === "signing") {
      window.liroSetText(document.getElementById("signing-label"), p.signingLabelText);
      document.getElementById("signing-bar").style.width = p.percent + "%";
      window.liroSetText(document.getElementById("eta"), p.etaText);
      document.getElementById("per-signature-warning").hidden = !p.perSignaturePIN;
      showState("signing");
    } else if (p.state === "done") {
      window.liroSetText(document.getElementById("done-summary"), p.doneSummaryText);
      window.liroSetText(document.getElementById("done-output"), p.doneOutputText);
      showState("done");
      document.getElementById("close-btn").focus();
    } else if (p.state === "failed") {
      window.liroSetText(document.getElementById("failed-message"), p.failedMessageText);
      showState("failed");
      document.getElementById("copy-details-btn").onclick = function () {
        if (navigator.clipboard) navigator.clipboard.writeText(p.failedDetails || "");
      };
      document.getElementById("close-failed-btn").focus();
    }
  }

  window.__liroOnMessage = function (payload) {
    if (payload.type === "init") {
      model = payload.model;
      window.liroApplyStaticStrings();
      renderWaiting();
    } else if (payload.type === "progress") {
      renderProgress(payload.progress);
    }
  };

  document.getElementById("approve-btn").addEventListener("click", function () {
    if (!selectedThumbprint) return;
    window.liroSend("approve");
  });
  document.getElementById("cancel-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });
  document.getElementById("close-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });
  document.getElementById("close-failed-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });

  // F5 §5.6: Escape cancels.
  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape") {
      window.liroSend("cancel");
    }
  });
})();
