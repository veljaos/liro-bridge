(function () {
  "use strict";

  var states = ["waiting", "preparing", "signing", "done", "failed", "tsachoice", "outputexists"];
  var model = null;
  var selectedThumbprint = null;

  // tsaChoice is read back by Go via Window.Eval after the page sends
  // "approve" from the timestamp-choice screen — the same mechanism
  // settings.js's __liroCollectState uses, and for the same reason: the
  // page->Go message surface stays at exactly three types (D-083), so a
  // choice that is neither "the original consent decision" nor "cancel"
  // reports itself through ExecuteScript's own return value instead of a
  // fourth message type.
  var tsaChoice = "";
  window.__liroTSAChoice = function () {
    return JSON.stringify({ choice: tsaChoice });
  };

  // outputChoice is read back the same way (Task 4): "overwrite",
  // "rename", or "" when the screen has not been answered.
  var outputChoice = "";
  window.__liroOutputChoice = function () {
    return JSON.stringify({ choice: outputChoice });
  };

  // __liroStampChoice reports the visible-stamp decision (Task 1) —
  // again through Eval's return value rather than a fourth page->Go
  // message type (D-083). Go reads it at the moment Approve is pressed,
  // and saves it to the configuration so the next signature starts
  // from the same answer.
  window.__liroStampChoice = function () {
    return JSON.stringify({
      visible: document.getElementById("stamp-visible").checked,
      position: document.getElementById("stamp-position").value,
    });
  };

  // syncStampPosition hides the corner selector while no stamp is
  // being drawn: a position for a stamp that does not exist is noise,
  // and leaving it visible-but-inert is the kind of dead control this
  // review keeps finding.
  function syncStampPosition() {
    document.getElementById("stamp-position-field").hidden =
      !document.getElementById("stamp-visible").checked;
  }

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
      nameLine.className = "liro-cert-name";
      window.liroSetText(nameLine, cert.displayName);
      row.appendChild(nameLine);

      // Task 4: role/issuer sit quietly under the name; the thumbprint
      // tail is a quiet monospace suffix, not part of that sentence
      // (SPEC §11.5 — sometimes the only difference between two
      // certificates that otherwise look identical).
      var metaLine = document.createElement("div");
      metaLine.className = "liro-cert-meta";
      var metaText = document.createElement("span");
      window.liroSetText(metaText, cert.roleText + " — " + cert.issuerText);
      metaLine.appendChild(metaText);
      var thumb = document.createElement("span");
      thumb.className = "liro-cert-thumb";
      window.liroSetText(thumb, cert.thumbprintTail);
      metaLine.appendChild(thumb);
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

  function renderStampOptions() {
    var select = document.getElementById("stamp-position");
    select.innerHTML = "";
    (model.stampPositions || []).forEach(function (p) {
      var option = document.createElement("option");
      option.value = p.value;
      window.liroSetText(option, p.text);
      select.appendChild(option);
    });
    select.value = model.stampPosition;
    document.getElementById("stamp-visible").checked = !!model.stampVisible;
    syncStampPosition();
  }

  function renderWaiting() {
    window.liroSetText(document.getElementById("document-count"), model.documentCountText);
    window.liroSetText(document.getElementById("application-name"), model.applicationName);
    // Task 2 (F5 second-real-run review): the elided form is what is
    // rendered; the full 64 characters only ever leave through the Copy
    // button below, never onto the screen, where one unbroken token
    // pushed the whole Details card wider than the window.
    window.liroSetText(document.getElementById("fingerprint"), model.fingerprintShort);
    document.getElementById("copy-fingerprint-btn").onclick = function () {
      if (navigator.clipboard) navigator.clipboard.writeText(model.fingerprint);
    };

    var fileList = document.getElementById("file-list");
    fileList.innerHTML = "";
    model.files.forEach(function (f) {
      var li = document.createElement("li");
      window.liroSetText(li, f);
      fileList.appendChild(li);
    });
    window.liroSetText(document.getElementById("file-overflow"), model.filesOverflowText);

    renderCertList();
    renderStampOptions();
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
      var level = document.getElementById("done-level");
      window.liroSetText(level, p.doneLevelText);
      // Task 1: the achieved level is stated on every successful batch,
      // and marked in the warning intent when it is B-B — a signature
      // with no proof of when it was made (SPEC §12.8).
      level.className = p.doneLevelIntent ? "liro-outcome liro-outcome-" + p.doneLevelIntent : "";
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
    } else if (p.state === "outputExists") {
      window.liroSetText(document.getElementById("output-exists-path"), p.outputExistsPath);
      window.liroSetText(document.getElementById("output-rename-btn"), p.outputRenameText);
      outputChoice = "";
      showState("outputexists");
      // Nothing that writes a file is focused first, the same rule the
      // waiting screen applies to Approve (F5 §5.6).
      document.getElementById("output-cancel-btn").focus();
    } else if (p.state === "tsaChoice") {
      window.liroSetText(document.getElementById("tsa-reason"), p.tsaReasonText);
      tsaChoice = "";
      showState("tsachoice");
      // Nothing that proceeds is focused first: Cancel gets it, the
      // same rule F5 §5.6 applies to Approve on the waiting screen.
      document.getElementById("tsa-cancel-btn").focus();
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
  document.getElementById("tsa-without-btn").addEventListener("click", function () {
    tsaChoice = "withoutTimestamp";
    window.liroSend("approve");
  });
  document.getElementById("tsa-configure-btn").addEventListener("click", function () {
    tsaChoice = "configure";
    window.liroSend("approve");
  });
  document.getElementById("tsa-cancel-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });
  document.getElementById("stamp-visible").addEventListener("change", syncStampPosition);
  document.getElementById("output-rename-btn").addEventListener("click", function () {
    outputChoice = "rename";
    window.liroSend("approve");
  });
  document.getElementById("output-overwrite-btn").addEventListener("click", function () {
    outputChoice = "overwrite";
    window.liroSend("approve");
  });
  document.getElementById("output-cancel-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });

  // F5 §5.6: Escape cancels.
  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape") {
      window.liroSend("cancel");
    }
  });
})();
