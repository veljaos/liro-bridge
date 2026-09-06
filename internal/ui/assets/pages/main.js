(function () {
  "use strict";

  // The page holds no state of its own beyond what Go has told it, and
  // decides nothing. Every string is already localised; every file name
  // goes in through textContent (SPEC §6.6 / F5 §5.3), never innerHTML.
  //
  // Actions travel back to Go the way D-083 established: the message
  // surface stays at exactly three types, so a button records what it
  // was and sends "approve"; Go reads the record through
  // ExecuteScript's own return value. The record itself lives in
  // bridge.js now (liroAct / __liroAction), because every page of the
  // flow needs one. Nothing here ever sends a path — a row is
  // identified by its index, which Go validates against the list it
  // already holds.
  var act = window.liroAct;

  window.__liroOnMessage = function (payload) {
    switch (payload.type) {
      case "init":
        window.liroApplyStaticStrings();
        renderFiles(payload);
        break;
      // The page's own fixed labels and nothing else: no screen is
      // chosen, because the step that navigated here says which one it
      // wants next. Every screen starts hidden for the same reason —
      // whatever a fresh document happened to show first would be
      // visible for the length of the navigation, and on the way to
      // the progress screen that was the document list, which reads as
      // the flow jumping back to where it started.
      case "strings":
        window.liroApplyStaticStrings();
        break;
      case "files":
        renderFiles(payload);
        break;
      case "queue":
        renderQueue(payload);
        break;
      case "report":
        renderReport(payload);
        break;
      case "status":
        renderStatus(payload);
        break;
      case "ask":
        renderAsk(payload);
        break;
    }
  };

  // Every screen this page can show. The three at the end are the
  // questions that come between the approval and the first signature;
  // they were on the consent window while that was a window of its own.
  var screens = [
    "state-files", "state-queue", "state-report",
    "state-tsachoice", "state-outputexists", "state-alreadysigned",
    "state-failed"
  ];

  function show(id) {
    screens.forEach(function (s) {
      document.getElementById(s).hidden = s !== id;
    });
  }

  // ---- files -------------------------------------------------------

  function renderFiles(payload) {
    show("state-files");
    window.liroRenderStep(payload.step);
    var files = payload.files || [];

    document.getElementById("empty-state").hidden = files.length !== 0;
    document.getElementById("file-list").hidden = files.length === 0;
    window.liroSetText(document.getElementById("files-count"), payload.countText || "");

    var list = document.getElementById("file-list");
    list.innerHTML = "";
    files.forEach(function (f, i) {
      var row = document.createElement("div");
      row.className = "file-row";

      var name = document.createElement("span");
      name.className = "file-name";
      window.liroSetText(name, f.name);
      row.appendChild(name);

      var size = document.createElement("span");
      size.className = "file-size";
      window.liroSetText(size, f.sizeText);
      row.appendChild(size);

      var remove = document.createElement("button");
      remove.className = "liro-btn liro-btn-quiet liro-btn-compact file-remove";
      window.liroSetText(remove, window.liroT("main.remove_file"));
      remove.addEventListener("click", function () {
        act("remove", { index: i });
      });
      row.appendChild(remove);

      // Only for a name that appears more than once in this list: the
      // folder is what makes the two rows tell themselves apart. It
      // goes on its own line, which is what flex-wrap on .file-row is
      // for.
      if (f.folder) {
        var folder = document.createElement("span");
        folder.className = "file-folder";
        window.liroSetText(folder, f.folder);
        row.appendChild(folder);
      }

      list.appendChild(row);
    });

    renderNotices(payload.notices || []);

    // The primary action goes back to its own label on every re-render,
    // which is what brings it out of the busy state below when the flow
    // came back to this step. Its words are Go's: this step ends in
    // Next when a certificate and a method are still to be chosen, and
    // in Sign only when nothing else is being asked.
    var sign = document.getElementById("sign-btn");
    window.liroSetText(sign, payload.primaryLabel || "");
    sign.disabled = files.length === 0;
    document.getElementById("clear-btn").disabled = files.length === 0;
    window.liroSetText(document.getElementById("output-folder"), payload.outputFolderText || "");
    document.getElementById("output-beside-btn").hidden = payload.outputFolderChosen !== true;
  }

  function renderNotices(notices) {
    var box = document.getElementById("notices");
    box.innerHTML = "";
    notices.forEach(function (n) {
      var p = document.createElement("p");
      p.className = "notice" + (n.problem ? " notice-problem" : "");
      window.liroSetText(p, n.text);
      box.appendChild(p);
    });
  }

  // ---- queue -------------------------------------------------------

  function renderQueue(payload) {
    show("state-queue");
    // The batch is running: it is not a step of anything, and a header
    // saying which step it is would be left over from the one before.
    window.liroRenderStep(null);
    window.liroSetText(document.getElementById("queue-label"), payload.label || "");
    window.liroSetText(document.getElementById("queue-eta"), payload.etaText || "");
    document.getElementById("queue-per-signature").hidden = !payload.perSignaturePIN;

    // "Preparing card..." is indeterminate on purpose: a bar sitting
    // still at 0% for the measured ~4.9s of card initialisation reads
    // as a freeze (F6 §3, SPEC §12.9).
    var indeterminate = payload.indeterminate === true;
    document.getElementById("queue-progress").hidden = indeterminate;
    document.getElementById("queue-progress-indeterminate").hidden = !indeterminate;
    if (!indeterminate) {
      document.getElementById("queue-bar").style.width = (payload.percent || 0) + "%";
    }

    var stop = document.getElementById("stop-btn");
    stop.disabled = payload.stopping === true;
    if (payload.stopping) {
      window.liroSetText(stop, window.liroT("main.stopping"));
    }

    renderQueueRows(payload.files || []);
  }

  function renderQueueRows(files) {
    var list = document.getElementById("queue-list");
    list.innerHTML = "";
    files.forEach(function (f) {
      var row = document.createElement("div");
      row.className = "file-row file-row-" + f.state;

      var name = document.createElement("span");
      name.className = "file-name";
      window.liroSetText(name, f.name);
      row.appendChild(name);

      var state = document.createElement("span");
      state.className = "file-state";
      window.liroSetText(state, f.stateText);
      row.appendChild(state);

      // Same rule as the document list: only where the name is not
      // enough on its own.
      if (f.folder) {
        var qFolder = document.createElement("span");
        qFolder.className = "file-folder";
        window.liroSetText(qFolder, f.folder);
        row.appendChild(qFolder);
      }

      if (f.reason) {
        var reason = document.createElement("span");
        reason.className = "file-reason";
        window.liroSetText(reason, f.reason);
        row.appendChild(reason);
      }

      list.appendChild(row);
    });
  }

  // ---- report ------------------------------------------------------

  function renderReport(payload) {
    show("state-report");
    window.liroRenderStep(null);
    window.liroSetText(document.getElementById("report-title"), payload.title || "");
    window.liroSetText(document.getElementById("report-counts"), payload.counts || "");

    var abort = document.getElementById("report-abort");
    abort.hidden = !payload.abortMessage;
    if (payload.abortMessage) {
      window.liroSetText(abort, payload.abortMessage);
    }

    var adjusted = document.getElementById("report-stamp-adjusted");
    adjusted.hidden = !payload.stampAdjusted;
    window.liroSetText(adjusted, payload.stampAdjusted || "");

    var already = document.getElementById("report-already-signed");
    already.hidden = !payload.alreadySigned;
    window.liroSetText(already, payload.alreadySigned || "");

    var auditNotice = document.getElementById("report-audit-notice");
    auditNotice.hidden = !payload.auditNotice;
    window.liroSetText(auditNotice, payload.auditNotice || "");

    var failures = payload.failures || [];
    document.getElementById("report-failures").hidden = failures.length === 0;
    var ul = document.getElementById("report-failure-list");
    ul.innerHTML = "";
    failures.forEach(function (f) {
      var li = document.createElement("li");
      // Name and reason in words, never a code (SPEC §7, F6 §5).
      window.liroSetText(li, f.name + " — " + f.reason);
      ul.appendChild(li);
    });

    window.liroSetText(document.getElementById("report-output"), payload.outputText || "");
    var level = document.getElementById("report-level");
    window.liroSetText(level, payload.levelText || "");
    level.className = "liro-text-small" + (payload.levelIntent ? " liro-outcome liro-outcome-" + payload.levelIntent : "");
    var levelNote = document.getElementById("report-level-note");
    window.liroSetText(levelNote, payload.levelNote || "");
    levelNote.hidden = !payload.levelNote;
    document.getElementById("report-open-btn").disabled = !payload.canOpenOutput;
    // Signing more means going back to a document list, which a run
    // that brought its own documents does not have.
    document.getElementById("report-again-btn").hidden = payload.canSignMore === false;
    document.getElementById("report-status").hidden = true;
  }

  // ---- the questions between the approval and the first signature ---

  function renderAsk(payload) {
    var ask = payload.ask || {};
    // Asked after the approval, so the flow is past its steps.
    window.liroRenderStep(null);
    if (ask.state === "tsaChoice") {
      window.liroSetText(document.getElementById("tsa-reason"), ask.tsaReasonText || "");
      show("state-tsachoice");
      // Nothing that proceeds is focused first: Cancel gets it, the
      // same rule F5 §5.6 applies to Approve on the certificate step.
      document.getElementById("tsa-cancel-btn").focus();
      return;
    }
    if (ask.state === "outputExists") {
      window.liroSetText(document.getElementById("output-exists-path"), ask.outputExistsPath || "");
      window.liroSetText(document.getElementById("output-rename-btn"), ask.outputRenameText || "");
      show("state-outputexists");
      // Nothing that writes a file is focused first either.
      document.getElementById("output-cancel-btn").focus();
      return;
    }
    if (ask.state === "alreadySigned") {
      window.liroSetText(document.getElementById("already-signed-explain"), ask.alreadySignedText || "");
      window.liroSetText(document.getElementById("already-skip-btn"), ask.alreadySkipText || "");
      window.liroSetText(document.getElementById("already-sign-btn"), ask.alreadySignText || "");
      show("state-alreadysigned");
      // Nothing that signs anything is focused first.
      document.getElementById("already-cancel-btn").focus();
      return;
    }
    if (ask.state === "failed") {
      window.liroSetText(document.getElementById("failed-message"), ask.failedMessageText || "");
      show("state-failed");
      document.getElementById("copy-details-btn").onclick = function () {
        if (navigator.clipboard) navigator.clipboard.writeText(ask.failedDetails || "");
      };
      document.getElementById("close-failed-btn").focus();
    }
  }

  function renderStatus(payload) {
    var el = document.getElementById("report-status");
    window.liroSetText(el, payload.text || "");
    el.className = "liro-text-small" + (payload.intent ? " liro-outcome-" + payload.intent : "");
    el.hidden = !payload.text;
  }

  // ---- wiring ------------------------------------------------------

  function on(id, action) {
    document.getElementById(id).addEventListener("click", function () {
      act(action);
    });
  }

  on("browse-btn", "browse");
  on("clear-btn", "clear");

  // This is the one action with a wait behind it. Not the window any
  // more — a step is a navigation now, measured in tens of milliseconds
  // (D-148, D-150) — but the certificates: enumerating them touches the
  // card and the Trusted List, and it happens on the way out of this
  // step. A button that looks untouched is a button somebody presses
  // again, so the first press takes it out of service and says what is
  // happening; renderFiles above puts it back.
  document.getElementById("sign-btn").addEventListener("click", function () {
    var sign = document.getElementById("sign-btn");
    if (sign.disabled) return;
    sign.disabled = true;
    window.liroSetText(sign, window.liroT("main.sign_opening"));
    act("next");
  });

  on("step-back-btn", "back");
  on("tsa-without-btn", "tsaWithoutTimestamp");
  on("tsa-configure-btn", "tsaConfigure");
  on("already-skip-btn", "alreadySkip");
  on("already-sign-btn", "alreadySign");
  on("output-rename-btn", "outputRename");
  on("output-overwrite-btn", "outputOverwrite");
  on("output-change-btn", "chooseOutputFolder");
  on("output-beside-btn", "clearOutputFolder");
  on("stop-btn", "stop");
  on("report-open-btn", "openOutput");
  on("report-export-btn", "exportReport");
  on("report-again-btn", "newBatch");
  on("report-finish-btn", "finish");

  // The three ways out of a question asked after the approval. Each is
  // a refusal of this batch, which is what cancel has always meant.
  ["tsa-cancel-btn", "output-cancel-btn", "already-cancel-btn", "close-failed-btn"].forEach(function (id) {
    document.getElementById(id).addEventListener("click", function () {
      window.liroSend("cancel");
    });
  });

  // WebView2's own external-drop handling is off for this window
  // (D-114), so the shell delivers the drop to the native frame and Go
  // reads the real paths. The page still tracks the drag so the window
  // can say it is a target — dragover must be cancelled or the visual
  // feedback never arrives.
  document.addEventListener("dragover", function (ev) {
    ev.preventDefault();
    document.body.classList.add("drag-over");
  });
  document.addEventListener("dragleave", function (ev) {
    if (ev.relatedTarget === null) {
      document.body.classList.remove("drag-over");
    }
  });
  document.addEventListener("drop", function (ev) {
    ev.preventDefault();
    document.body.classList.remove("drag-over");
  });

  // Escape closes, matching every other window (F5 §5.6).
  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape") {
      window.liroSend("cancel");
    }
  });
})();
