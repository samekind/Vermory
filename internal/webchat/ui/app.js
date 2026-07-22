(() => {
  "use strict";

  const localStorageKey = "vermory.webchat.v1";
  const channel = "web_chat";
  const elements = {
    appShell: document.querySelector("#app-shell"),
    authGate: document.querySelector("#auth-gate"),
    authForm: document.querySelector("#auth-form"),
    apiToken: document.querySelector("#api-token"),
    authError: document.querySelector("#auth-error"),
    sessionControls: document.querySelector("#session-controls"),
    sessionRole: document.querySelector("#session-role"),
    signOut: document.querySelector("#sign-out"),
    workspace: document.querySelector(".workspace"),
    mobileTabs: [...document.querySelectorAll("[data-view]")],
    threadList: document.querySelector("#thread-list"),
    activeThreadTitle: document.querySelector("#active-thread-title"),
    transcript: document.querySelector("#transcript"),
    composer: document.querySelector("#composer"),
    messageInput: document.querySelector("#message-input"),
    sendMessage: document.querySelector("#send-message"),
    composerNote: document.querySelector("#composer-note"),
    syncState: document.querySelector("#sync-state"),
    connectionStatus: document.querySelector("#connection-status"),
    connectionLabel: document.querySelector("#connection-label"),
    memoryContent: document.querySelector("#memory-content"),
    memoryCount: document.querySelector("#memory-count"),
    mobileMemoryCount: document.querySelector("#mobile-memory-count"),
    reviewCount: document.querySelector("#review-count"),
    memoryTabs: [...document.querySelectorAll("[data-memory-view]")],
    memoryTabsContainer: document.querySelector("#memory-tabs"),
    toast: document.querySelector("#toast"),
  };

  let activeStorageKey = "";
  let apiCredential = "";
  let runtimeMode = "probing";
  let authenticatedRole = "";
  let state = freshState();
  let inspection = emptyInspection();
  let candidates = [];
  let memoryView = "remembered";
  let editingMemoryID = "";
  let confirmingForgetID = "";
  let toastTimer = 0;
  let refreshSequence = 0;

  function emptyInspection() {
    return { observations: [], memories: [] };
  }

  function freshState() {
    const first = newThread();
    return { version: 1, threads: [first], activeThreadID: first.id };
  }

  function loadState() {
    if (!activeStorageKey) return freshState();
    try {
      const stored = JSON.parse(localStorage.getItem(activeStorageKey));
      if (stored && stored.version === 1 && Array.isArray(stored.threads)) {
        const threads = stored.threads.filter(validThread).map((thread) => ({
          id: thread.id,
          title: thread.title || "Untitled conversation",
          preview: thread.preview || "No messages yet",
          initialized: thread.initialized === true,
          pending: validPending(thread.pending) ? thread.pending : null,
        }));
        if (threads.length > 0) {
          return {
            version: 1,
            threads,
            activeThreadID: threads.some((thread) => thread.id === stored.activeThreadID) ? stored.activeThreadID : threads[0].id,
          };
        }
      }
    } catch (_) {
      localStorage.removeItem(activeStorageKey);
    }
    return freshState();
  }

  function validThread(thread) {
    return thread && typeof thread.id === "string" && thread.id.length > 0 && thread.id.length <= 512;
  }

  function validPending(pending) {
    return pending && typeof pending.operationID === "string" && typeof pending.message === "string";
  }

  function newThread() {
    return {
      id: `browser-${crypto.randomUUID()}`,
      title: "Untitled conversation",
      preview: "No messages yet",
      initialized: false,
      pending: null,
    };
  }

  function activeThread() {
    return state.threads.find((thread) => thread.id === state.activeThreadID) || state.threads[0];
  }

  function persistState() {
    if (activeStorageKey) localStorage.setItem(activeStorageKey, JSON.stringify(state));
  }

  function setConnection(online, label) {
    elements.connectionStatus.classList.toggle("is-online", online);
    elements.connectionStatus.classList.toggle("is-offline", !online);
    elements.connectionLabel.textContent = label || (online ? "Connected" : "Offline");
  }

  function setSync(label, busy = false) {
    elements.syncState.textContent = label;
    elements.transcript.setAttribute("aria-busy", busy ? "true" : "false");
  }

  function canGovern() {
    return runtimeMode === "local" || authenticatedRole === "operator" || authenticatedRole === "owner";
  }

  function activateStorage(storageScope) {
    activeStorageKey = storageScope === "local" ? localStorageKey : `${localStorageKey}.${storageScope}`;
    state = loadState();
    inspection = emptyInspection();
    candidates = [];
    memoryView = "remembered";
    editingMemoryID = "";
    confirmingForgetID = "";
  }

  function applyAuthorityUI() {
    const governed = canGovern();
    elements.memoryTabsContainer.hidden = !governed;
    if (!governed) memoryView = "remembered";
    elements.sessionControls.hidden = runtimeMode !== "authenticated";
    elements.sessionRole.textContent = authenticatedRole === "client" ? "Chat access" : "Memory operator";
  }

  function renderApplication() {
    applyAuthorityUI();
    renderThreads();
    renderTranscript();
    renderMemory();
    syncComposerToThread();
  }

  function showApplication() {
    elements.authGate.hidden = true;
    elements.appShell.hidden = false;
    elements.authError.hidden = true;
    elements.authError.textContent = "";
    renderApplication();
  }

  function showAuthentication(message = "") {
    for (const thread of state.threads) {
      if (thread.pending?.phase === "sending") thread.pending.phase = "failed";
    }
    persistState();
    apiCredential = "";
    authenticatedRole = "";
    activeStorageKey = "";
    state = freshState();
    inspection = emptyInspection();
    candidates = [];
    elements.appShell.hidden = true;
    elements.authGate.hidden = false;
    elements.authError.textContent = message;
    elements.authError.hidden = !message;
    elements.apiToken.value = "";
    requestAnimationFrame(() => elements.apiToken.focus());
  }

  async function enterAuthenticatedMode(session) {
    if (!session || !["client", "operator", "owner"].includes(session.role) || !/^[a-f0-9]{32}$/.test(session.storage_scope || "")) {
      throw new Error("The server returned an invalid access session.");
    }
    runtimeMode = "authenticated";
    authenticatedRole = session.role;
    activateStorage(session.storage_scope);
    showApplication();
    setConnection(navigator.onLine, navigator.onLine ? "Connected" : "Offline");
    await refreshCurrent({ quiet: true });
  }

  function enterLocalMode() {
    runtimeMode = "local";
    authenticatedRole = "operator";
    activateStorage("local");
    showApplication();
    setConnection(navigator.onLine, navigator.onLine ? "Connected" : "Offline");
    void refreshCurrent();
  }

  function setMobileView(view) {
    elements.workspace.dataset.mobileView = view;
    for (const tab of elements.mobileTabs) {
      tab.classList.toggle("is-active", tab.dataset.view === view);
    }
  }

  function renderThreads() {
    elements.threadList.replaceChildren();
    for (const thread of state.threads) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = `thread-item${thread.id === state.activeThreadID ? " is-active" : ""}`;
      button.dataset.threadId = thread.id;

      const title = document.createElement("span");
      title.className = "thread-title";
      title.textContent = thread.title;
      const preview = document.createElement("span");
      preview.className = "thread-preview";
      preview.textContent = thread.pending?.phase === "failed" ? "Send failed - retry available" : thread.preview;
      button.append(title, preview);
      elements.threadList.append(button);
    }
    elements.activeThreadTitle.textContent = activeThread().title;
  }

  function observationRole(kind) {
    if (kind === "user_message") return "user";
    if (kind === "assistant_message") return "assistant";
    return "";
  }

  function pendingHasServerReply(pending, observations) {
    if (!pending) return false;
    const newer = observations.filter((item) => Number(item.sequence) > Number(pending.baselineSequence || 0));
    const userIndex = newer.findIndex((item) => item.kind === "user_message" && item.content === pending.message);
    return userIndex >= 0 && newer.slice(userIndex + 1).some((item) => item.kind === "assistant_message");
  }

  function pendingAlreadyRecorded(pending, observations) {
    if (!pending) return false;
    return observations.some((item) =>
      Number(item.sequence) > Number(pending.baselineSequence || 0) &&
      item.kind === "user_message" &&
      item.content === pending.message
    );
  }

  function reconcilePending() {
    const thread = activeThread();
    if (pendingHasServerReply(thread.pending, inspection.observations || [])) {
      thread.pending = null;
      persistState();
    }
  }

  function renderTranscript() {
    reconcilePending();
    elements.transcript.replaceChildren();
    const observations = (inspection.observations || []).filter((item) => observationRole(item.kind));
    const thread = activeThread();

    if (observations.length === 0 && !thread.pending) {
      const empty = document.createElement("div");
      empty.className = "empty-state";
      empty.innerHTML = '<div class="empty-glyph" aria-hidden="true">V</div><h3>No messages yet</h3>';
      elements.transcript.append(empty);
      return;
    }

    for (const item of observations) {
      elements.transcript.append(renderMessage({
        id: item.id,
        role: observationRole(item.kind),
        content: item.content,
        pending: false,
      }));
    }

    if (thread.pending && !pendingAlreadyRecorded(thread.pending, observations)) {
      elements.transcript.append(renderMessage({
        id: thread.pending.operationID,
        role: "user",
        content: thread.pending.message,
        pending: true,
        phase: thread.pending.phase,
      }));
    } else if (thread.pending && thread.pending.phase === "failed") {
      const status = document.createElement("div");
      status.className = "message-row is-user is-failed";
      status.innerHTML = '<p class="message-label">Not delivered</p><div class="message-bubble">The reply was not received. Retry the same request to reconcile it safely.</div>';
      const meta = document.createElement("div");
      meta.className = "message-meta";
      const retry = document.createElement("button");
      retry.type = "button";
      retry.className = "message-action";
      retry.dataset.action = "retry";
      retry.textContent = "Retry";
      meta.append(retry);
      status.append(meta);
      elements.transcript.append(status);
    }
    requestAnimationFrame(() => {
      elements.transcript.scrollTop = elements.transcript.scrollHeight;
    });
  }

  function renderMessage(message) {
    const row = document.createElement("article");
    row.className = `message-row is-${message.role}${message.phase === "failed" ? " is-failed" : ""}`;
    row.dataset.messageId = message.id;

    const label = document.createElement("p");
    label.className = "message-label";
    label.textContent = message.role === "user" ? "You" : "Vermory";
    const bubble = document.createElement("div");
    bubble.className = "message-bubble";
    bubble.textContent = message.content;
    row.append(label, bubble);

    const meta = document.createElement("div");
    meta.className = "message-meta";
    if (message.pending) {
      const status = document.createElement("span");
      status.textContent = message.phase === "failed" ? "Not delivered" : "Sending";
      meta.append(status);
      if (message.phase === "failed") {
        const retry = document.createElement("button");
        retry.type = "button";
        retry.className = "message-action";
        retry.dataset.action = "retry";
        retry.textContent = "Retry";
        meta.append(retry);
      }
    } else if (canGovern()) {
      const remember = document.createElement("button");
      remember.type = "button";
      remember.className = "message-action";
      remember.dataset.action = "remember";
      remember.dataset.observationId = message.id;
      remember.textContent = "Remember";
      meta.append(remember);
    }
    row.append(meta);
    return row;
  }

  function renderMemory() {
    const memories = (inspection.memories || []).filter((item) => item.lifecycle_status === "active" && item.effective_state === "current");
    elements.memoryCount.textContent = String(memories.length);
    elements.mobileMemoryCount.textContent = memories.length > 0 ? `(${memories.length})` : "";
    elements.reviewCount.textContent = candidates.length > 0 ? `(${candidates.length})` : "";
    elements.memoryContent.replaceChildren();

    const items = memoryView === "remembered" ? memories : candidates;
    if (items.length === 0) {
      const empty = document.createElement("p");
      empty.className = "memory-empty";
      empty.textContent = memoryView === "remembered" ? "Nothing is remembered for this conversation." : "Nothing is waiting for review.";
      elements.memoryContent.append(empty);
      return;
    }

    for (const item of items) {
      elements.memoryContent.append(memoryView === "remembered" ? renderRemembered(item) : renderCandidate(item));
    }
  }

  function renderRemembered(memory) {
    const article = document.createElement("article");
    article.className = "memory-item";
    article.dataset.memoryId = memory.id;

    if (editingMemoryID === memory.id) {
      const editor = document.createElement("form");
      editor.className = "memory-editor";
      editor.dataset.action = "save-correction";
      const textarea = document.createElement("textarea");
      textarea.name = "content";
      textarea.required = true;
      textarea.value = memory.content;
      const actions = document.createElement("div");
      actions.className = "memory-actions";
      actions.append(actionButton("Save", "submit"), actionButton("Cancel", "cancel-edit"));
      editor.append(textarea, actions);
      article.append(editor);
      return article;
    }

    const copy = document.createElement("p");
    copy.className = "memory-copy";
    copy.textContent = memory.content;
    article.append(copy);
    if (canGovern()) {
      const actions = document.createElement("div");
      actions.className = "memory-actions";
      actions.append(actionButton("Correct", "edit-memory"));
      const forget = actionButton(confirmingForgetID === memory.id ? "Confirm forget" : "Forget", "forget-memory");
      forget.classList.add("danger");
      actions.append(forget);
      if (confirmingForgetID === memory.id) actions.append(actionButton("Cancel", "cancel-forget"));
      article.append(actions);
    }
    return article;
  }

  function renderCandidate(candidate) {
    const article = document.createElement("article");
    article.className = "memory-item";
    article.dataset.candidateId = candidate.candidate_memory_id;
    const source = document.createElement("p");
    source.className = "memory-source";
    source.textContent = "Suggested from this conversation";
    const copy = document.createElement("p");
    copy.className = "memory-copy";
    copy.textContent = candidate.content;
    article.append(source, copy);
    if (candidate.source_quote && candidate.source_quote !== candidate.content) {
      const quote = document.createElement("p");
      quote.className = "candidate-quote";
      quote.textContent = candidate.source_quote;
      article.append(quote);
    }
    const actions = document.createElement("div");
    actions.className = "memory-actions";
    actions.append(actionButton("Remember", "accept-candidate"), actionButton("Skip", "reject-candidate"));
    article.append(actions);
    return article;
  }

  function actionButton(label, action) {
    const button = document.createElement("button");
    button.type = action === "submit" ? "submit" : "button";
    button.className = "text-command";
    if (action !== "submit") button.dataset.action = action;
    button.textContent = label;
    return button;
  }

  async function requestJSON(path, options = {}) {
    const response = await fetch(path, {
      ...options,
      headers: {
        ...(options.body ? { "Content-Type": "application/json" } : {}),
        ...(apiCredential ? { Authorization: `Bearer ${apiCredential}` } : {}),
        ...(options.headers || {}),
      },
    });
    let body = null;
    try {
      body = await response.json();
    } catch (_) {
      body = null;
    }
    if (!response.ok) {
      const error = new Error(body?.error?.message || body?.message || "The request could not be completed.");
      error.status = response.status;
      if (response.status === 401 && runtimeMode === "authenticated") {
        showAuthentication("Your access is no longer valid. Enter an active token to continue.");
      }
      throw error;
    }
    return body;
  }

  function conversationQuery(threadID) {
    return new URLSearchParams({ channel, thread_id: threadID });
  }

  function conversationBody(operationID, extra = {}) {
    return { operation_id: operationID, channel, thread_id: activeThread().id, ...extra };
  }

  async function refreshCurrent({ quiet = false } = {}) {
    const thread = activeThread();
    const threadID = thread.id;
    const refreshID = ++refreshSequence;
    const refreshStillCurrent = () => state.activeThreadID === threadID && refreshID === refreshSequence;
    if (!thread.initialized) {
      inspection = emptyInspection();
      candidates = [];
      setConnection(navigator.onLine, navigator.onLine ? "Connected" : "Offline");
      setSync("Ready");
      renderThreads();
      renderTranscript();
      renderMemory();
      return;
    }
    if (!quiet) setSync("Syncing", true);
    try {
      const query = conversationQuery(threadID);
      const [nextInspection, inbox] = await Promise.all([
        requestJSON(`/v1/conversations/inspect?${query}`),
        canGovern() ? requestJSON(`/v1/memories/candidates?${query}`) : Promise.resolve({ candidates: [] }),
      ]);
      if (!refreshStillCurrent()) return;
      inspection = nextInspection || emptyInspection();
      candidates = inbox?.candidates || [];
      updateThreadSummary();
      setConnection(true, "Connected");
      setSync("Up to date");
    } catch (error) {
      if (!refreshStillCurrent()) return;
      if (!navigator.onLine) {
        setConnection(false, "Offline");
      } else if (error instanceof TypeError) {
        setConnection(false, "Connection issue");
      }
      setSync("Unavailable");
      if (!quiet) showToast(error.message);
    }
    renderThreads();
    renderTranscript();
    renderMemory();
  }

  function updateThreadSummary() {
    const thread = activeThread();
    const conversational = (inspection.observations || []).filter((item) => observationRole(item.kind));
    const firstUser = conversational.find((item) => item.kind === "user_message");
    const last = conversational[conversational.length - 1];
    if (firstUser) thread.title = summarize(firstUser.content, 38);
    if (last) thread.preview = summarize(last.content, 58);
    persistState();
  }

  function summarize(value, limit) {
    const normalized = String(value || "").replace(/\s+/g, " ").trim();
    return normalized.length > limit ? `${normalized.slice(0, limit - 1)}...` : normalized || "Untitled conversation";
  }

  async function sendPending() {
    const thread = activeThread();
    if (!thread.pending || thread.pending.phase === "sending") return;
    const threadID = thread.id;
    thread.pending.phase = "sending";
    persistState();
    elements.sendMessage.disabled = true;
    elements.composerNote.textContent = "Sending";
    renderTranscript();

    try {
      await requestJSON("/v1/chat/turn", {
        method: "POST",
        body: JSON.stringify(conversationBody(thread.pending.operationID, { message: thread.pending.message })),
      });
      thread.initialized = true;
      thread.pending = null;
      persistState();
      if (activeThread().id === threadID) {
        elements.messageInput.value = "";
        resizeComposer();
        await refreshCurrent({ quiet: true });
      } else {
        renderThreads();
      }
    } catch (error) {
      thread.pending.phase = "failed";
      persistState();
      if (activeThread().id === threadID) {
        if (!navigator.onLine) {
          setConnection(false, "Offline");
        } else if (error instanceof TypeError) {
          setConnection(false, "Connection issue");
        }
        setSync("Send failed");
        renderTranscript();
        showToast("Message not delivered. Retry when the connection is available.");
      }
      renderThreads();
    } finally {
      elements.sendMessage.disabled = false;
      elements.composerNote.textContent = "Current conversation only";
    }
  }

  function startSend(message) {
    const thread = activeThread();
    const observations = inspection.observations || [];
    const baselineSequence = observations.reduce((max, item) => Math.max(max, Number(item.sequence) || 0), 0);
    thread.pending = {
      operationID: `browser-turn-${crypto.randomUUID()}`,
      message,
      phase: "queued",
      baselineSequence,
    };
    if (thread.title === "Untitled conversation") thread.title = summarize(message, 38);
    thread.preview = summarize(message, 58);
    persistState();
    renderThreads();
    renderTranscript();
    void sendPending();
  }

  async function rememberObservation(observationID, button) {
    if (!canGovern()) return;
    button.disabled = true;
    try {
      await requestJSON("/v1/memories/confirm", {
        method: "POST",
        body: JSON.stringify(conversationBody(`browser-confirm-${crypto.randomUUID()}`, { observation_id: observationID })),
      });
      await refreshCurrent({ quiet: true });
      showToast("Remembered for this conversation.");
    } catch (error) {
      button.disabled = false;
      showToast(error.message);
    }
  }

  async function reviewCandidate(candidateID, accept, button) {
    if (!canGovern()) return;
    button.disabled = true;
    try {
      await requestJSON(`/v1/memories/candidates/${accept ? "accept" : "reject"}`, {
        method: "POST",
        body: JSON.stringify(conversationBody(`browser-review-${crypto.randomUUID()}`, { candidate_memory_id: candidateID })),
      });
      await refreshCurrent({ quiet: true });
      showToast(accept ? "Remembered for this conversation." : "Suggestion skipped.");
    } catch (error) {
      button.disabled = false;
      showToast(error.message);
    }
  }

  async function correctMemory(memoryID, content, form) {
    if (!canGovern()) return;
    const submit = form.querySelector('button[type="submit"]');
    submit.disabled = true;
    try {
      await requestJSON("/v1/memories/correct", {
        method: "POST",
        body: JSON.stringify(conversationBody(`browser-correct-${crypto.randomUUID()}`, { memory_id: memoryID, content })),
      });
      editingMemoryID = "";
      await refreshCurrent({ quiet: true });
      showToast("Memory corrected.");
    } catch (error) {
      submit.disabled = false;
      showToast(error.message);
    }
  }

  async function forgetMemory(memoryID, button) {
    if (!canGovern()) return;
    button.disabled = true;
    try {
      await requestJSON("/v1/memories/forget", {
        method: "POST",
        body: JSON.stringify(conversationBody(`browser-forget-${crypto.randomUUID()}`, { memory_id: memoryID })),
      });
      confirmingForgetID = "";
      await refreshCurrent({ quiet: true });
      showToast("Memory forgotten.");
    } catch (error) {
      button.disabled = false;
      showToast(error.message);
    }
  }

  function createConversation() {
    const thread = newThread();
    state.threads.unshift(thread);
    state.activeThreadID = thread.id;
    inspection = emptyInspection();
    candidates = [];
    persistState();
    renderThreads();
    renderTranscript();
    renderMemory();
    syncComposerToThread();
    setMobileView("chat");
    void refreshCurrent({ quiet: true });
    elements.messageInput.focus();
  }

  function selectThread(threadID) {
    if (threadID === state.activeThreadID) {
      setMobileView("chat");
      return;
    }
    state.activeThreadID = threadID;
    inspection = emptyInspection();
    candidates = [];
    editingMemoryID = "";
    confirmingForgetID = "";
    persistState();
    renderThreads();
    renderTranscript();
    renderMemory();
    syncComposerToThread();
    setMobileView("chat");
    void refreshCurrent();
  }

  function showToast(message) {
    window.clearTimeout(toastTimer);
    elements.toast.textContent = message;
    elements.toast.hidden = false;
    toastTimer = window.setTimeout(() => {
      elements.toast.hidden = true;
    }, 3200);
  }

  function resizeComposer() {
    elements.messageInput.style.height = "auto";
    elements.messageInput.style.height = `${Math.min(elements.messageInput.scrollHeight, 180)}px`;
  }

  function syncComposerToThread() {
    elements.messageInput.value = activeThread().pending?.message || "";
    resizeComposer();
  }

  async function probeRuntime() {
    try {
      const response = await fetch("/v1/browser/runtime", {
        cache: "no-store",
        headers: { Accept: "application/json" },
      });
      const runtime = response.ok ? await response.json() : null;
      if (runtime?.mode === "local") {
        enterLocalMode();
        return;
      }
      if (runtime?.mode === "authenticated") {
        runtimeMode = "authenticated";
        showAuthentication();
        return;
      }
      showAuthentication("The Vermory service returned an unexpected authentication response.");
    } catch (_) {
      runtimeMode = "authenticated";
      showAuthentication("Vermory is not reachable. Check the secure connection and try again.");
    }
  }

  elements.authForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const submit = elements.authForm.querySelector('button[type="submit"]');
    const credential = elements.apiToken.value.trim();
    elements.apiToken.value = "";
    if (!credential) return;
    runtimeMode = "authenticated";
    apiCredential = credential;
    submit.disabled = true;
    elements.authError.hidden = true;
    try {
      const session = await requestJSON("/v1/session", { method: "GET" });
      await enterAuthenticatedMode(session);
    } catch (error) {
      apiCredential = "";
      showAuthentication(error.status === 401 ? "That access token is not active." : error.message);
    } finally {
      submit.disabled = false;
    }
  });

  elements.signOut.addEventListener("click", () => showAuthentication());

  document.querySelector("#new-thread").addEventListener("click", createConversation);
  document.querySelector("#new-thread-compact").addEventListener("click", createConversation);

  elements.threadList.addEventListener("click", (event) => {
    const button = event.target.closest("[data-thread-id]");
    if (button) selectThread(button.dataset.threadId);
  });

  elements.mobileTabs.forEach((tab) => tab.addEventListener("click", () => setMobileView(tab.dataset.view)));

  elements.memoryTabs.forEach((tab) => tab.addEventListener("click", () => {
    memoryView = tab.dataset.memoryView;
    for (const item of elements.memoryTabs) {
      const active = item === tab;
      item.classList.toggle("is-active", active);
      item.setAttribute("aria-selected", active ? "true" : "false");
    }
    renderMemory();
  }));

  elements.composer.addEventListener("submit", (event) => {
    event.preventDefault();
    const message = elements.messageInput.value.trim();
    if (!message || activeThread().pending) return;
    startSend(message);
  });

  elements.messageInput.addEventListener("input", resizeComposer);
  elements.messageInput.addEventListener("keydown", (event) => {
    if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      elements.composer.requestSubmit();
    }
  });

  elements.transcript.addEventListener("click", (event) => {
    const button = event.target.closest("[data-action]");
    if (!button) return;
    if (button.dataset.action === "retry") void sendPending();
    if (button.dataset.action === "remember") void rememberObservation(button.dataset.observationId, button);
  });

  elements.memoryContent.addEventListener("click", (event) => {
    const button = event.target.closest("[data-action]");
    if (!button) return;
    const memoryItem = button.closest("[data-memory-id]");
    const candidateItem = button.closest("[data-candidate-id]");
    switch (button.dataset.action) {
      case "edit-memory":
        editingMemoryID = memoryItem.dataset.memoryId;
        confirmingForgetID = "";
        renderMemory();
        break;
      case "cancel-edit":
        editingMemoryID = "";
        renderMemory();
        break;
      case "forget-memory":
        if (confirmingForgetID !== memoryItem.dataset.memoryId) {
          confirmingForgetID = memoryItem.dataset.memoryId;
          renderMemory();
        } else {
          void forgetMemory(memoryItem.dataset.memoryId, button);
        }
        break;
      case "cancel-forget":
        confirmingForgetID = "";
        renderMemory();
        break;
      case "accept-candidate":
        void reviewCandidate(candidateItem.dataset.candidateId, true, button);
        break;
      case "reject-candidate":
        void reviewCandidate(candidateItem.dataset.candidateId, false, button);
        break;
    }
  });

  elements.memoryContent.addEventListener("submit", (event) => {
    const form = event.target.closest('[data-action="save-correction"]');
    if (!form) return;
    event.preventDefault();
    const memoryID = form.closest("[data-memory-id]").dataset.memoryId;
    const content = new FormData(form).get("content").trim();
    if (content) void correctMemory(memoryID, content, form);
  });

  window.addEventListener("online", () => {
    setConnection(true, "Connected");
    if (runtimeMode === "local" || apiCredential) void refreshCurrent({ quiet: true });
  });
  window.addEventListener("offline", () => setConnection(false, "Offline"));

  void probeRuntime();
})();
