(function () {
  "use strict";

  var model = null;
  var selectedThumbprint = null;

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

      // A row that cannot be chosen is a signing certificate whose card
      // is out or whose validity has run out — a real choice
      // temporarily unavailable, which is why it is here at all with
      // its reason on it. A certificate that is not for signing never
      // reaches this list any more; Go leaves it out.
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
    // The line that stands where "choose a certificate" would. Go
    // decides the sentence, because only Go knows whether the listing is
    // still running, whether there is a reader, whether there is a card
    // in it, and whether the Windows service that answers any of those
    // questions is running at all.
    var notice = model.certNoticeText || "";
    var noticeEl = document.getElementById("no-usable-cert");
    window.liroSetText(noticeEl, notice);
    noticeEl.hidden = notice === "";
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
    // A new batch is a new decision. SPEC §18.15 forbids a remembered
    // certificate across sessions, and a page that keeps the previous
    // batch's selection through a fresh init is that rule failing in the
    // one place it is implemented — Approve would already be pressable,
    // for a certificate nobody chose for these documents.
    //
    // This now happens on every arrival at the step, including one
    // reached by pressing Back from the step after it: coming back is
    // how a wrong certificate is corrected, so coming back must not
    // leave the wrong one still chosen.
    selectedThumbprint = null;
    document.getElementById("approve-btn").disabled = true;
    // A new batch starts with no clock on it. One left over from the
    // last one would be counting down to something that has already
    // happened.
    document.getElementById("countdown").hidden = true;

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

    // F5 §5.6: Approve is never the initially focused control — Cancel
    // gets it instead, so the user must move to Approve deliberately.
    // Cancel, not Back: the way out of a decision is refusing it, and a
    // step header that happens to be there must not become the thing
    // the keyboard lands on.
    document.getElementById("cancel-btn").focus();
  }

  // F7 §7.4's countdown. The page owns no clock: Go says how many
  // seconds are left and in what words, once a second, and this draws
  // it — the same rule every other screen here follows, that a page
  // holds no state beyond what Go last told it (D-120, D-121).
  function renderCountdown(payload) {
    var el = document.getElementById("countdown");
    window.liroSetText(el, payload.text);
    el.hidden = !payload.text;
  }

  window.__liroOnMessage = function (payload) {
    if (payload.type === "countdown") {
      renderCountdown(payload);
      return;
    }
    if (payload.type !== "init") return;
    model = payload.model;
    window.liroApplyStaticStrings();
    window.liroRenderStep(payload.step);
    renderWaiting();
  };

  document.getElementById("approve-btn").addEventListener("click", function () {
    if (!selectedThumbprint) return;
    window.liroAct("approve");
  });
  document.getElementById("cancel-btn").addEventListener("click", function () {
    window.liroSend("cancel");
  });
  document.getElementById("step-back-btn").addEventListener("click", function () {
    window.liroAct("back");
  });

  // F5 §5.6: Escape cancels.
  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape") {
      window.liroSend("cancel");
    }
  });
})();
