(function () {
  "use strict";

  // The page holds no state of its own beyond what Go has told it, and
  // decides nothing. Every string is already localised; every file name
  // goes in through textContent (SPEC §6.6 / F5 §5.3), never innerHTML.
  //
  // Actions travel back to Go the way D-083 established: the message
  // surface stays at exactly three types, so a button records what it
  // was into __liroMainAction and sends "approve"; Go reads the record
  // through ExecuteScript's own return value. Nothing here ever sends a
  // path — a row is identified by its index, which Go validates against
  // the list it already holds.
  var pendingAction = null;

  window.__liroMainAction = function () {
    var a = pendingAction;
    pendingAction = null;
    return JSON.stringify(a || {});
  };

  function act(action, extra) {
    pendingAction = Object.assign({ action: action }, extra || {});
    window.liroSend("approve");
  }

  window.__liroOnMessage = function (payload) {
    switch (payload.type) {
      case "init":
        window.liroApplyStaticStrings();
        renderFiles(payload);
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
    }
  };

  function show(id) {
    ["state-files", "state-queue", "state-report"].forEach(function (s) {
      document.getElementById(s).hidden = s !== id;
    });
  }

  // ---- files -------------------------------------------------------

  function renderFiles(payload) {
    show("state-files");
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

      list.appendChild(row);
    });

    renderNotices(payload.notices || []);

    document.getElementById("sign-btn").disabled = files.length === 0;
    document.getElementById("clear-btn").disabled = files.length === 0;
    window.liroSetText(document.getElementById("output-folder"), payload.outputFolderText || "");
    window.liroSetText(document.getElementById("stamp-summary"), payload.stampSummary || "");
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
    window.liroSetText(document.getElementById("report-title"), payload.title || "");
    window.liroSetText(document.getElementById("report-counts"), payload.counts || "");

    var abort = document.getElementById("report-abort");
    abort.hidden = !payload.abortMessage;
    if (payload.abortMessage) {
      window.liroSetText(abort, payload.abortMessage);
    }

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
    window.liroSetText(document.getElementById("report-level"), payload.levelText || "");
    document.getElementById("report-open-btn").disabled = !payload.canOpenOutput;
    document.getElementById("report-status").hidden = true;
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
  on("sign-btn", "sign");
  on("output-change-btn", "chooseOutputFolder");
  on("stamp-change-btn", "stampSettings");
  on("stop-btn", "stop");
  on("report-open-btn", "openOutput");
  on("report-export-btn", "exportReport");
  on("report-again-btn", "newBatch");

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
