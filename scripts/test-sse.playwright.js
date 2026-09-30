// Execute with Playwright MCP browser_run_code_unsafe using this file as filename.
// Set SSE_TEST_URL (or ROOMS_TEST_URL) when server is not at localhost:9779.
async (page) => {
  const configuredURL =
    typeof process !== "undefined" && process.env
      ? process.env.SSE_TEST_URL || process.env.ROOMS_TEST_URL
      : "";
  const baseURL = (configuredURL || "http://localhost:9779").replace(/\/$/, "");
  const timeout = 25000;
  const checks = [];
  const guestContexts = [];
  const ownedRoomCodes = [];
  const streamRequests = [];
  const streamResponses = [];
  const streamFailures = [];
  const stateRequests = [];
  let roomCode = "";
  let secondaryHostPage = null;

  function assert(condition, step, detail) {
    if (!condition) throw new Error(`${step}: ${detail}`);
  }

  function mark(step) {
    checks.push(step);
  }

  async function submitAndWaitForNavigation(targetPage, button, step) {
    const navigation = targetPage.waitForNavigation({
      waitUntil: "domcontentloaded",
      timeout,
    });
    const [response] = await Promise.all([navigation, button.click()]);
    assert(Boolean(response), step, "form submission did not produce a document response");
    assert(response.status() < 400, step, `form returned HTTP ${response.status()}`);
    return response;
  }

  async function waitForRoom(targetPage, { code, status, playerCount }, step) {
    await targetPage.waitForFunction(
      ({ code, status, playerCount }) => {
        const state = document.querySelector("#room-state");
        if (!state || state.dataset.roomCode !== code) return false;
        if (status && state.dataset.status !== status) return false;
        if (
          playerCount !== undefined &&
          state.querySelectorAll("[data-player-id][data-x][data-y][data-health]").length !== playerCount
        ) {
          return false;
        }
        return true;
      },
      { code, status, playerCount },
      { timeout },
    ).catch((error) => {
      throw new Error(`${step}: room state did not reach requested code/status/roster (${error.message})`);
    });
  }

  async function newIndependentPage() {
    const browser = page.context().browser();
    assert(Boolean(browser), "Create independent guest context", "page has no browser instance");
    const context = await browser.newContext();
    guestContexts.push(context);
    const guestPage = await context.newPage();
    return { context, page: guestPage };
  }

  async function createRoom(username) {
    await page.goto(baseURL, { waitUntil: "domcontentloaded", timeout });
    const form = page.locator("form[action='/rooms']");
    await form.locator("input[name='username']").fill(username);
    await submitAndWaitForNavigation(
      page,
      form.getByRole("button", { name: "Create room", exact: true }),
      "Create owned SSE test room",
    );
    await page.locator("#room-state").waitFor({ state: "visible", timeout });
    roomCode = await page.locator("#room-state").getAttribute("data-room-code") || "";
    assert(/^[A-Z0-9]{6}$/.test(roomCode), "Create owned SSE test room", `invalid room code ${JSON.stringify(roomCode)}`);
    ownedRoomCodes.push(roomCode);
    await waitForRoom(page, { code: roomCode, status: "lobby", playerCount: 1 }, "Create owned SSE test room");
  }

  async function fillHomeJoin(targetPage, code, username) {
    await targetPage.goto(baseURL, { waitUntil: "domcontentloaded", timeout });
    const form = targetPage.locator("form[action='/rooms/join']");
    await form.locator("input[name='code']").fill(code);
    await form.locator("input[name='username']").fill(username);
    return form.getByRole("button", { name: "Join room", exact: true });
  }

  async function assertIdleActionOpacities(step, targetPage = page) {
    const buttons = await targetPage.locator("#room-state .direction-pad button, #room-state .game-action-button, #room-state .leave-button").evaluateAll((nodes) =>
      nodes.map((button) => ({
        label: button.getAttribute("aria-label") || button.textContent.trim(),
        opacity: getComputedStyle(button).opacity,
      })),
    );
    const faded = buttons.filter((button) => Math.abs(Number(button.opacity) - 1) > 0.001);
    assert(faded.length === 0, step, `idle action buttons faded: ${JSON.stringify(faded)}`);
    return buttons;
  }

  async function assertNonGameActionOpacities(step, targetPage = page) {
    const buttons = await targetPage.locator("#room-state .direction-pad button, #room-state .leave-button").evaluateAll((nodes) =>
      nodes.map((button) => ({
        label: button.getAttribute("aria-label") || button.textContent.trim(),
        opacity: getComputedStyle(button).opacity,
      })),
    );
    const faded = buttons.filter((button) => Math.abs(Number(button.opacity) - 1) > 0.001);
    assert(faded.length === 0, step, `movement or leave action buttons faded: ${JSON.stringify(faded)}`);
    return buttons;
  }

  async function assertPersistentNodes(step) {
    const identity = await page.evaluate(() => ({
      state: document.querySelector("#room-state") === window.__sseTestInitialState,
      stream: document.querySelector("#room-events") === window.__sseTestInitialStream,
      inspector: document.querySelector("[data-inspect-dialog]") === window.__sseTestInitialInspector,
      inspectorOutsideState: !document.querySelector("#room-state")?.contains(document.querySelector("[data-inspect-dialog]")),
    }));
    assert(
      identity.state && identity.stream && identity.inspector && identity.inspectorOutsideState,
      step,
      `morph replaced persistent room/stream/inspector identity: ${JSON.stringify(identity)}`,
    );
    return identity;
  }

  async function assertNoFrontendStateGets(step) {
    assert(stateRequests.length === 0, step, `browser issued GET /rooms/:code/state: ${JSON.stringify(stateRequests)}`);
  }

  async function holdGameAction(action, expectedStatus, targetPage = page) {
    const path = `/rooms/${roomCode}/${action.toLowerCase()}`;
    const routePattern = `**${path}`;
    let releaseAction;
    const actionGate = new Promise((resolve) => {
      releaseAction = resolve;
    });
    const handler = async (route) => {
      if (route.request().method() !== "POST") {
        await route.continue();
        return;
      }
      await actionGate;
      await route.continue();
    };
    await targetPage.route(routePattern, handler);

    const button = targetPage.getByRole("button", { name: `${action} game`, exact: true });
    const requestPromise = targetPage.waitForRequest(
      (request) => new URL(request.url()).pathname === path && request.method() === "POST",
      { timeout },
    );
    let clickPromise;
    try {
      clickPromise = button.click();
      await requestPromise;
      await targetPage.waitForFunction(
        () => {
          const form = document.querySelector("#game-action");
          const button = form?.querySelector("button[type='submit']");
          return form?.classList.contains("htmx-request") && button && Math.abs(Number(getComputedStyle(button).opacity) - 0.65) < 0.001;
        },
        null,
        { timeout },
      );
      const heldButtons = await targetPage.locator("#room-state .direction-pad button, #room-state .game-action-button, #room-state .leave-button").evaluateAll((nodes) =>
        nodes.map((candidate) => ({
          label: candidate.getAttribute("aria-label") || candidate.textContent.trim(),
          opacity: getComputedStyle(candidate).opacity,
          selected: candidate.closest("#game-action") !== null,
        })),
      );
      const selected = heldButtons.filter((candidate) => candidate.selected);
      const otherFaded = heldButtons.filter((candidate) => !candidate.selected && Math.abs(Number(candidate.opacity) - 1) > 0.001);
      assert(
        selected.length === 1 && Math.abs(Number(selected[0].opacity) - 0.65) < 0.001 && otherFaded.length === 0,
        `${action} loading feedback`,
        `expected only selected action at 0.65 opacity: ${JSON.stringify(heldButtons)}`,
      );
      mark(`${action} button retained 0.65 loading opacity while held POST was pending`);
    } finally {
      releaseAction();
      await clickPromise?.catch(() => {});
      await targetPage.unroute(routePattern, handler).catch(() => {});
    }

    await targetPage.waitForFunction(
      (status) => document.querySelector("#room-state")?.dataset.status === status,
      expectedStatus,
      { timeout },
    );
    await assertIdleActionOpacities(`${action} completes`, targetPage);
  }

  async function startWithStaleResponse(actionPage, continueWhileHeld) {
    const path = `/rooms/${roomCode}/start`;
    const routePattern = `**${path}`;
    let releaseBackend;
    let releaseResponse;
    let resolveResponse;
    const backendGate = new Promise((resolve) => {
      releaseBackend = resolve;
    });
    const responseGate = new Promise((resolve) => {
      releaseResponse = resolve;
    });
    const responseReady = new Promise((resolve) => {
      resolveResponse = resolve;
    });
    const handler = async (route) => {
      await backendGate;
      const response = await route.fetch();
      resolveResponse(response);
      await responseGate;
      await route.fulfill({ response });
    };
    await page.route(routePattern, handler);

    const startButton = page.getByRole("button", { name: "Start game", exact: true });
    const requestPromise = page.waitForRequest(
      (request) => new URL(request.url()).pathname === path && request.method() === "POST",
      { timeout },
    );
    let clickPromise;
    try {
      clickPromise = startButton.click();
      await requestPromise;
      await page.waitForFunction(() => {
        const form = document.querySelector("#game-action");
        const button = form?.querySelector("button[type='submit']");
        return form?.classList.contains("htmx-request") && button && Math.abs(Number(getComputedStyle(button).opacity) - 0.65) < 0.001;
      }, null, { timeout });
      const pendingOpacity = await page.locator("#room-state .direction-pad button, #room-state .game-action-button, #room-state .leave-button").evaluateAll((nodes) =>
        nodes.map((button) => ({
          label: button.getAttribute("aria-label") || button.textContent.trim(),
          opacity: getComputedStyle(button).opacity,
          selected: button.closest("#game-action") !== null,
        })),
      );
      const selected = pendingOpacity.filter((button) => button.selected);
      const fadedOthers = pendingOpacity.filter((button) => !button.selected && Math.abs(Number(button.opacity) - 1) > 0.001);
      assert(
        selected.length === 1 && Math.abs(Number(selected[0].opacity) - 0.65) < 0.001 && fadedOthers.length === 0,
        "Start loading feedback",
        `expected only Start button at 0.65 opacity: ${JSON.stringify(pendingOpacity)}`,
      );
      mark("Start button retained 0.65 loading opacity while its POST waited for backend release");

      releaseBackend();
      const response = await responseReady;
      assert(response.status() === 200, "Start backend response", `expected HTTP 200, got ${response.status()}`);
      await waitForRoom(page, { code: roomCode, status: "active", playerCount: 3 }, "Primary receives start SSE push while POST response is held");
      await waitForRoom(actionPage, { code: roomCode, status: "active", playerCount: 3 }, "Secondary host receives start SSE push");
      await assertPersistentNodes("Start SSE push before stale action response");
      await assertNonGameActionOpacities("Start push while action response is held");
      await assertIdleActionOpacities("Start push on second host tab", actionPage);
      await continueWhileHeld(response);
    } finally {
      releaseBackend();
      releaseResponse();
      await clickPromise?.catch(() => {});
      await page.unroute(routePattern, handler).catch(() => {});
    }
  }

  page.on("request", (request) => {
    try {
      const url = new URL(request.url());
      if (/^\/rooms\/[A-Z0-9]{6}\/events$/.test(url.pathname)) {
        streamRequests.push({ method: request.method(), url: url.pathname, headers: request.headers() });
      }
      if (/^\/rooms\/[A-Z0-9]{6}\/state$/.test(url.pathname) && request.method() === "GET") {
        stateRequests.push({ url: url.pathname, method: request.method() });
      }
    } catch {
      // Ignore non-HTTP requests from the page.
    }
  });
  page.on("response", (response) => {
    try {
      const url = new URL(response.url());
      if (/^\/rooms\/[A-Z0-9]{6}\/events$/.test(url.pathname)) {
        const headers = response.headers();
        streamResponses.push({ status: response.status(), contentType: headers["content-type"] || "" });
      }
    } catch {
      // Ignore non-HTTP responses from the page.
    }
  });
  page.on("requestfailed", (request) => {
    try {
      const url = new URL(request.url());
      if (/^\/rooms\/[A-Z0-9]{6}\/events$/.test(url.pathname)) {
        streamFailures.push({ url: url.pathname, failure: request.failure()?.errorText || "" });
      }
    } catch {
      // Ignore non-HTTP request failures.
    }
  });

  await page.addInitScript(() => {
    window.__sseTestAfterSwaps = 0;
    window.__sseTestAfterConnections = [];
    window.__sseTestAfterMessages = 0;
    window.__sseTestBeforeMessages = [];
    window.__sseTestBeforeSwaps = [];
    document.addEventListener("htmx:after:swap", () => {
      window.__sseTestAfterSwaps += 1;
    }, true);
    document.addEventListener("htmx:sse:after:connection", (event) => {
      const connection = event.detail?.connection;
      window.__sseTestAfterConnections.push({
        status: connection?.status || 0,
        attempt: connection?.attempt ?? -1,
        url: connection?.ctx?.request?.action || "",
      });
    }, true);
    document.addEventListener("htmx:sse:after:message", () => {
      window.__sseTestAfterMessages += 1;
    }, true);
    document.addEventListener("htmx:sse:before:message", (event) => {
      const message = event.detail?.message;
      window.__sseTestBeforeMessages.push({ id: message?.id || "", event: message?.event || "" });
    }, true);
    document.addEventListener("htmx:before:swap", (event) => {
      const context = event.detail?.ctx;
      if (context?.target?.id !== "room-state") return;
      const fragment = new DOMParser().parseFromString(context.text || "", "text/html");
      const incoming = fragment.querySelector("#room-state");
      window.__sseTestBeforeSwaps.push({
        incomingRevision: Number(incoming?.dataset.revision),
        currentRevision: Number(context.target.dataset.revision),
      });
    }, true);
  });

  try {
    const hostName = `SSE Host ${Date.now().toString(36).slice(-6)}`;
    await createRoom(hostName);
    await page.waitForFunction(
      () => window.htmx?.version === "4.0.0",
      null,
      { timeout },
    );
    await page.waitForFunction(
      () => window.__sseTestAfterConnections?.length >= 1 && window.__sseTestBeforeMessages.length >= 1,
      null,
      { timeout },
    );
    const initialContract = await page.evaluate(() => {
      const state = document.querySelector("#room-state");
      const stream = document.querySelector("#room-events");
      window.__sseTestInitialState = state;
      window.__sseTestInitialStream = stream;
      window.__sseTestInitialInspector = document.querySelector("[data-inspect-dialog]");
      return {
        version: window.htmx?.version || "",
        stateTrigger: state?.getAttribute("hx-trigger") || "",
        stateGet: state?.getAttribute("hx-get") || "",
        pollTriggers: Array.from(document.querySelectorAll("[hx-trigger]"))
          .filter((element) => /\bevery\b/i.test(element.getAttribute("hx-trigger") || ""))
          .map((element) => ({ id: element.id, trigger: element.getAttribute("hx-trigger") })),
        streamURL: stream?.getAttribute("hx-sse:connect") || "",
        stateSwap: state?.getAttribute("hx-swap") || "",
        eventTarget: stream?.getAttribute("hx-target") || "",
        eventSwap: stream?.getAttribute("hx-swap") || "",
        inspectorOutsideState: Boolean(state && !state.contains(document.querySelector("[data-inspect-dialog]"))),
      };
    });
    assert(initialContract.version === "4.0.0", "HTMX core version", `expected 4.0.0, got ${initialContract.version}`);
    assert(
      !initialContract.stateTrigger && !initialContract.stateGet && initialContract.pollTriggers.length === 0,
      "No state polling contract",
      `room-state has polling attributes: ${JSON.stringify(initialContract)}`,
    );
    assert(
      initialContract.streamURL === `/rooms/${roomCode}/events` &&
        initialContract.stateSwap === "outerMorph" &&
        initialContract.eventTarget === "#room-state" &&
        initialContract.eventSwap === "outerMorph" &&
        initialContract.inspectorOutsideState,
      "Persistent SSE room contract",
      `stream or morph contract mismatch: ${JSON.stringify(initialContract)}`,
    );
    await page.waitForFunction(() => window.__sseTestAfterConnections.length === 1, null, { timeout });
    assert(streamRequests.length === 1, "Initial stream count", `expected one room event request, got ${streamRequests.length}`);
    assert(
      streamResponses.length === 1 &&
        streamResponses[0].status === 200 &&
        streamResponses[0].contentType.toLowerCase().includes("text/event-stream"),
      "Initial SSE response headers",
      `expected HTTP 200 text/event-stream, got ${JSON.stringify(streamResponses)}`,
    );
    await assertPersistentNodes("Initial stream identity");
    await assertIdleActionOpacities("Initial healthy SSE connection");
    mark("HTMX 4.0.0 room uses persistent outerMorph SSE stream with HTTP 200 text/event-stream response");

    const initialSwapCount = await page.evaluate(() => window.__sseTestAfterSwaps);
    const initialStateRevision = Number(await page.locator("#room-state").getAttribute("data-revision"));
    const initialMessage = await page.evaluate(() => window.__sseTestBeforeMessages[0]);
    assert(
      initialMessage.id === String(initialStateRevision) && initialMessage.event === "" &&
        (await page.evaluate(() => window.__sseTestAfterMessages)) === 0,
      "Initial duplicate snapshot discard",
      `initial SSE snapshot should match rendered revision and be ignored: ${JSON.stringify(initialMessage)}`,
    );
    await page.waitForTimeout(3200);
    const quietPeriod = await page.evaluate(() => ({
      swaps: window.__sseTestAfterSwaps,
      connections: window.__sseTestAfterConnections.length,
      opacity: Array.from(document.querySelectorAll("#room-state .direction-pad button, #room-state .game-action-button, #room-state .leave-button"), (button) => getComputedStyle(button).opacity),
    }));
    assert(
      quietPeriod.swaps === initialSwapCount && quietPeriod.connections === 1 && streamRequests.length === 1,
      "SSE idle quiet period",
      `3.2s idle produced swaps or extra streams: ${JSON.stringify({ quietPeriod, streamRequests })}`,
    );
    assert(
      quietPeriod.opacity.every((value) => Math.abs(Number(value) - 1) < 0.001),
      "SSE idle button opacity",
      `buttons faded during idle period: ${JSON.stringify(quietPeriod.opacity)}`,
    );
    await assertNoFrontendStateGets("SSE idle quiet period");
    assert(
      Number(await page.locator("#room-state").getAttribute("data-revision")) === initialStateRevision,
      "SSE idle revision",
      "room revision changed without a room mutation",
    );
    mark("3.2s unchanged lobby produced no state GET, no extra stream, no HTMX swap, and no faded action controls");

    const firstGuestName = "SSE Scout";
    const firstGuest = await newIndependentPage();
    const firstJoinButton = await fillHomeJoin(firstGuest.page, roomCode, firstGuestName);
    await submitAndWaitForNavigation(firstGuest.page, firstJoinButton, "Join owned room as first SSE guest");
    await waitForRoom(firstGuest.page, { code: roomCode, status: "lobby", playerCount: 2 }, "First SSE guest room");
    await waitForRoom(page, { code: roomCode, status: "lobby", playerCount: 2 }, "Host sees first guest over SSE");
    const firstGuestRevision = Number(await page.locator("#room-state").getAttribute("data-revision"));
    assert(firstGuestRevision > initialStateRevision, "First guest SSE update", `revision did not advance: ${initialStateRevision} -> ${firstGuestRevision}`);
    await assertPersistentNodes("First guest SSE push");
    await assertIdleActionOpacities("After guest-join push");
    assert(streamRequests.length === 1, "Persistent SSE after guest join", `room stream reconnected unexpectedly: ${streamRequests.length}`);
    mark("Guest joined through room form; host received real SSE revision without replacing persistent nodes");

    // Cancel stream, take page context offline, and mutate room while reconnect cannot reach server.
    const lastEventID = await page.evaluate(() => document.getElementById("room-events")?._htmx?.sse?.lastEventId || "");
    assert(lastEventID === String(firstGuestRevision), "SSE Last-Event-ID setup", `expected ${firstGuestRevision}, got ${JSON.stringify(lastEventID)}`);
    const reconnectRequestPromise = page.waitForRequest(
      (request) => new URL(request.url()).pathname === `/rooms/${roomCode}/events`,
      { timeout },
    );
    const reconnectFailurePromise = page.waitForEvent(
      "requestfailed",
      (request) => new URL(request.url()).pathname === `/rooms/${roomCode}/events` &&
        (request.headers()["last-event-id"] || request.headers()["Last-Event-ID"]) === lastEventID,
      { timeout },
    );
    await page.evaluate(async () => {
      const connection = document.getElementById("room-events")?._htmx?.sse;
      if (!connection?.reader) throw new Error("active SSE reader missing before reconnect test");
      connection.reader.cancel("acceptance reconnect test").catch(() => {});
    });
    await page.context().setOffline(true);
    const reconnectRequest = await reconnectRequestPromise;
    const reconnectHeaders = reconnectRequest.headers();
    assert(
      (reconnectHeaders["last-event-id"] || reconnectHeaders["Last-Event-ID"]) === lastEventID,
      "SSE reconnect Last-Event-ID",
      `expected Last-Event-ID ${lastEventID}, got ${JSON.stringify(reconnectHeaders)}`,
    );
    const failedReconnect = await reconnectFailurePromise;
    assert(
      failedReconnect.failure()?.errorText,
      "SSE offline reconnect failure",
      `expected browser to drop reconnect while offline: ${JSON.stringify(streamFailures)}`,
    );

    const secondGuestName = "SSE Relay";
    const secondGuest = await newIndependentPage();
    const secondJoinButton = await fillHomeJoin(secondGuest.page, roomCode, secondGuestName);
    await submitAndWaitForNavigation(secondGuest.page, secondJoinButton, "Join owned room while host SSE is disconnected");
    await waitForRoom(secondGuest.page, { code: roomCode, status: "lobby", playerCount: 3 }, "Second SSE guest room");
    const disconnectedRevision = Number(await secondGuest.page.locator("#room-state").getAttribute("data-revision"));
    assert(disconnectedRevision > firstGuestRevision, "Mutation during SSE disconnect", `revision did not advance while stream was held: ${firstGuestRevision} -> ${disconnectedRevision}`);
    await page.context().setOffline(false);
    await page.waitForFunction(
      ({ code, revision }) => {
        const state = document.querySelector("#room-state");
        return state?.dataset.roomCode === code &&
          state.querySelectorAll("[data-player-id][data-x][data-y][data-health]").length === 3 &&
          Number(state.dataset.revision) >= revision &&
          window.__sseTestAfterConnections.length >= 2;
      },
      { code: roomCode, revision: disconnectedRevision },
      { timeout },
    );
    assert(
      streamRequests.length >= 3 && streamFailures.length >= 1,
      "SSE reconnect stream count",
      `expected failed offline and successful online reconnect requests, got requests=${streamRequests.length}, failures=${JSON.stringify(streamFailures)}`,
    );
    await assertPersistentNodes("SSE reconnection reconciles latest snapshot");
    await assertIdleActionOpacities("After reconnect snapshot");
    await assertNoFrontendStateGets("SSE reconnect");
    mark(`Cancelled stream reconnected with Last-Event-ID ${lastEventID}; latest room revision ${disconnectedRevision} included peer join`);

    // Reconnect once without a mutation: server's initial duplicate ID must not morph current DOM.
    const duplicateRevision = Number(await page.locator("#room-state").getAttribute("data-revision"));
    const duplicateSwapBaseline = await page.evaluate(() => window.__sseTestAfterSwaps);
    const duplicateMessageBaseline = await page.evaluate(() => window.__sseTestBeforeMessages.length);
    const duplicateRequestPromise = page.waitForRequest(
      (request) => new URL(request.url()).pathname === `/rooms/${roomCode}/events`,
      { timeout },
    );
    await page.evaluate(async () => {
      const connection = document.getElementById("room-events")?._htmx?.sse;
      if (!connection?.reader) throw new Error("active SSE reader missing before duplicate-ID check");
      await connection.reader.cancel("acceptance duplicate-ID test");
    });
    const duplicateRequest = await duplicateRequestPromise;
    assert(
      (duplicateRequest.headers()["last-event-id"] || duplicateRequest.headers()["Last-Event-ID"]) === String(duplicateRevision),
      "Duplicate SSE Last-Event-ID",
      `expected ${duplicateRevision}, got ${JSON.stringify(duplicateRequest.headers())}`,
    );
    await page.waitForFunction(
      ({ connections, messages }) =>
        window.__sseTestAfterConnections.length >= connections &&
        window.__sseTestBeforeMessages.length >= messages + 1,
      { connections: 3, messages: duplicateMessageBaseline },
      { timeout },
    );
    const duplicateMessage = await page.evaluate(() => window.__sseTestBeforeMessages.at(-1));
    const duplicateState = await page.locator("#room-state").evaluate((state) => ({
      revision: Number(state.dataset.revision),
      playerCount: state.querySelectorAll("[data-player-id][data-x][data-y][data-health]").length,
    }));
    assert(
      duplicateMessage.id === String(duplicateRevision) && duplicateMessage.event === "",
      "Duplicate SSE message setup",
      `expected unnamed duplicate revision ${duplicateRevision}, got ${JSON.stringify(duplicateMessage)}`,
    );
    assert(
      duplicateState.revision === duplicateRevision && duplicateState.playerCount === 3 &&
        (await page.evaluate(() => window.__sseTestAfterSwaps)) === duplicateSwapBaseline,
      "Duplicate SSE ID discard",
      `duplicate revision changed DOM or caused swap: ${JSON.stringify({ duplicateState, duplicateSwapBaseline })}`,
    );
    assert(streamRequests.length >= 4, "Duplicate-ID reconnect stream count", `expected initial, offline, latest-snapshot, and duplicate requests, got ${streamRequests.length}`);
    await assertPersistentNodes("Duplicate SSE ID discard");
    await assertNoFrontendStateGets("Duplicate SSE ID discard");
    mark(`Discarded duplicate SSE id ${duplicateMessage.id} without DOM morph`);

    const contractAfterJoin = await page.evaluate(() => ({
      rootRevision: Number(document.querySelector("#room-state")?.dataset.revision),
      connections: window.__sseTestAfterConnections.length,
      swaps: window.__sseTestAfterSwaps,
    }));
    assert(contractAfterJoin.rootRevision >= disconnectedRevision, "SSE reconnect snapshot", `stale revision rendered: ${JSON.stringify(contractAfterJoin)}`);

    const lobbyMovementButtons = await page.locator("#room-state .direction-pad button").count();
    assert(lobbyMovementButtons === 4, "Movement control baseline", `expected four movement buttons, got ${lobbyMovementButtons}`);

    secondaryHostPage = await page.context().newPage();
    await secondaryHostPage.goto(`${baseURL}/rooms/${roomCode}`, { waitUntil: "domcontentloaded", timeout });
    await waitForRoom(secondaryHostPage, { code: roomCode, status: "lobby", playerCount: 3 }, "Open second host tab");
    await secondaryHostPage.evaluate(() => {
      window.__sseTestSecondaryInitialState = document.querySelector("#room-state");
    });

    const actionResults = {};
    await startWithStaleResponse(secondaryHostPage, async (startResponse) => {
      actionResults.startResponseStatus = startResponse.status();
      const activeRevision = Number(await page.locator("#room-state").getAttribute("data-revision"));
      actionResults.activeRevision = activeRevision;
      await assertNonGameActionOpacities("Active primary after start push");
      await assertIdleActionOpacities("Active second host tab", secondaryHostPage);
      await assertPersistentNodes("Start game push");
      await waitForRoom(firstGuest.page, { code: roomCode, status: "active", playerCount: 3 }, "First guest observes game start");
      await waitForRoom(secondGuest.page, { code: roomCode, status: "active", playerCount: 3 }, "Second guest observes game start");

      // Send a real invalid movement form; HTMX 4 must swap the 422 HTML alert.
      const invalidMoveRevision = Number(await secondaryHostPage.locator("#room-state").getAttribute("data-revision"));
      const invalidMoveResponse = secondaryHostPage.waitForResponse(
        (response) => new URL(response.url()).pathname === `/rooms/${roomCode}/actions` && response.status() === 422,
        { timeout },
      );
      await secondaryHostPage.locator("#room-state .direction-pad form").first().evaluate((form) => {
        const direction = form.querySelector("input[name='direction']");
        const button = form.querySelector("button[type='submit']");
        if (!direction || !button) throw new Error("movement form is missing direction or button");
        direction.value = "invalid-direction";
        button.disabled = false;
        form.requestSubmit(button);
      });
      const rejectedResponse = await invalidMoveResponse;
      await secondaryHostPage.locator("#room-state [role='alert']").waitFor({ state: "visible", timeout });
      const invalidMoveResult = await secondaryHostPage.locator("#room-state").evaluate((state) => ({
        revision: Number(state.dataset.revision),
        alert: state.querySelector("[role='alert']")?.textContent?.trim() || "",
        sameRoot: state === window.__sseTestSecondaryInitialState,
      }));
      assert(rejectedResponse.status() === 422, "Invalid move HTTP status", `expected 422, got ${rejectedResponse.status()}`);
      assert(
        invalidMoveResult.revision === invalidMoveRevision && invalidMoveResult.alert.length > 0 && invalidMoveResult.sameRoot,
        "Invalid move HTML error swap",
        `expected visible 422 message, unchanged state revision, and preserved root: ${JSON.stringify(invalidMoveResult)}`,
      );
      actionResults.invalidMove = { status: rejectedResponse.status(), ...invalidMoveResult };
      await assertIdleActionOpacities("After invalid movement response", secondaryHostPage);
      await assertNoFrontendStateGets("Invalid movement response");
      mark("Invalid movement form received 422 HTML; HTMX 4 showed error without mutation or root replacement");

      const hostBeforeMove = await secondaryHostPage.locator("#room-state [data-player-id][data-x][data-y][data-health]").evaluateAll((rows) => {
        const host = rows.find((row) => row.querySelector(".host-badge"));
        return host ? { id: host.dataset.playerId, x: Number(host.dataset.x), y: Number(host.dataset.y) } : null;
      });
      assert(Boolean(hostBeforeMove), "Find host movement row", "host row missing");
      const availableDirection = await secondaryHostPage.evaluate(() => {
        for (const direction of ["north", "south", "east", "west"]) {
          const button = document.querySelector(`#room-state button[aria-label='Move ${direction}']`);
          if (button && !button.disabled) return direction;
        }
        return "";
      });
      let movement = { result: "blocked-start", direction: null, from: hostBeforeMove, to: hostBeforeMove };
      if (availableDirection) {
        const delta = {
          north: { x: 0, y: 1 },
          south: { x: 0, y: -1 },
          east: { x: 1, y: 0 },
          west: { x: -1, y: 0 },
        }[availableDirection];
        const expected = { x: hostBeforeMove.x + delta.x, y: hostBeforeMove.y + delta.y };
        const previousRevision = Number(await secondaryHostPage.locator("#room-state").getAttribute("data-revision"));
        await secondaryHostPage.getByRole("button", { name: `Move ${availableDirection}`, exact: true }).click();
        await secondaryHostPage.waitForFunction(
          ({ id, x, y, revision }) => {
            const state = document.querySelector("#room-state");
            const host = Array.from(state?.querySelectorAll("[data-player-id][data-x][data-y][data-health]") || [])
              .find((row) => row.dataset.playerId === id);
            return Number(state?.dataset.revision) > revision && host && Number(host.dataset.x) === x && Number(host.dataset.y) === y;
          },
          { id: hostBeforeMove.id, ...expected, revision: previousRevision },
          { timeout },
        );
        const movedRevision = Number(await secondaryHostPage.locator("#room-state").getAttribute("data-revision"));
        await page.waitForFunction(
          (revision) => Number(document.querySelector("#room-state")?.dataset.revision) >= revision,
          movedRevision,
          { timeout },
        );
        const movedHost = await secondaryHostPage.locator("#room-state [data-player-id][data-x][data-y][data-health]").evaluateAll((rows) => {
          const host = rows.find((row) => row.querySelector(".host-badge"));
          return host ? { id: host.dataset.playerId, x: Number(host.dataset.x), y: Number(host.dataset.y) } : null;
        });
        movement = { result: "moved", direction: availableDirection, from: hostBeforeMove, to: movedHost, revision: movedRevision };
        mark(`Host moved ${availableDirection}; both host tabs rendered movement revision ${movedRevision}`);
      } else {
        assert(
          (await secondaryHostPage.locator("#room-state .direction-pad button:disabled").count()) === 4,
          "Blocked starting room",
          "movement buttons were not disabled despite no available direction",
        );
        mark("Starting room had no legal move; invalid direction was rejected through live 422 HTML response");
      }
      actionResults.movement = movement;
      await assertPersistentNodes("Movement result");
      await assertNonGameActionOpacities("After movement push while Start response remains held");
      await assertIdleActionOpacities("After movement response", secondaryHostPage);
      await assertNoFrontendStateGets("After movement response");

      const beforeFinishRevision = Number(await secondaryHostPage.locator("#room-state").getAttribute("data-revision"));
      await holdGameAction("Finish", "finished", secondaryHostPage);
      await waitForRoom(page, { code: roomCode, status: "finished", playerCount: 3 }, "Primary sees finish SSE push");
      const finishedRevision = Number(await page.locator("#room-state").getAttribute("data-revision"));
      assert(finishedRevision > beforeFinishRevision, "Finish SSE revision", `revision did not advance: ${beforeFinishRevision} -> ${finishedRevision}`);
      actionResults.finishedRevision = finishedRevision;
      await assertPersistentNodes("Finish game push");
      await assertNonGameActionOpacities("After finish push while stale Start response remains held");
      await assertIdleActionOpacities("After finish response", secondaryHostPage);
      await waitForRoom(firstGuest.page, { code: roomCode, status: "finished", playerCount: 3 }, "First guest observes finish");
      await waitForRoom(secondGuest.page, { code: roomCode, status: "finished", playerCount: 3 }, "Second guest observes finish");
      await assertNoFrontendStateGets("Finish game");
      actionResults.staleSwapBaseline = await page.evaluate(() => window.__sseTestBeforeSwaps.length);
      mark("Finish form produced real finished revision; peers and persistent room nodes received SSE update");
    });

    await page.waitForFunction(
      ({ baseline, revision }) => window.__sseTestBeforeSwaps
        .slice(baseline)
        .some((swap) => swap.incomingRevision < revision),
      { baseline: actionResults.staleSwapBaseline, revision: actionResults.finishedRevision },
      { timeout },
    );
    const staleActionState = await page.locator("#room-state").evaluate((state) => ({
      revision: Number(state.dataset.revision),
      status: state.dataset.status,
      players: state.querySelectorAll("[data-player-id][data-x][data-y][data-health]").length,
    }));
    assert(
      staleActionState.revision === actionResults.finishedRevision &&
        staleActionState.status === "finished" && staleActionState.players === 3,
      "Discard stale action fragment",
      `old Start response replaced newer Finish snapshot: ${JSON.stringify(staleActionState)}`,
    );
    await assertPersistentNodes("Stale action response discard");
    await assertIdleActionOpacities("After stale action response discard");
    await assertNoFrontendStateGets("After stale action response discard");
    mark(`Discarded stale Start action fragment below DOM revision ${actionResults.finishedRevision}`);

    const finalConnectionState = await page.evaluate(() => ({
      connectionCount: window.__sseTestAfterConnections.length,
      connectionStatuses: window.__sseTestAfterConnections.map((connection) => connection.status),
      swaps: window.__sseTestAfterSwaps,
    }));
    assert(
      finalConnectionState.connectionCount === 3 && finalConnectionState.connectionStatuses.every((status) => status === 200),
      "Healthy SSE stream through room actions",
      `unexpected stream reconnect/status after successful reconnect: ${JSON.stringify(finalConnectionState)}`,
    );

    const leaveButton = page.getByRole("button", { name: "Leave game", exact: true });
    assert(
      Math.abs(Number(await leaveButton.evaluate((button) => getComputedStyle(button).opacity)) - 1) < 0.001,
      "Leave button after SSE push",
      "leave action faded after finished SSE update",
    );
    await submitAndWaitForNavigation(page, leaveButton, "Host leaves owned SSE test room");
    mark("Leave form remained fully opaque after finish push and submitted through browser navigation");

    await assertNoFrontendStateGets("Final SSE acceptance summary");
    return {
      checks,
      ownedRoomCodes,
      initialRevision: initialStateRevision,
      guestJoinRevision: firstGuestRevision,
      reconnect: {
        lastEventID,
        revisionWhileDisconnected: disconnectedRevision,
        requestHeaders: reconnectHeaders,
        streamRequests: streamRequests.length,
      },
      duplicateMessage,
      actionResults,
      staleActionState,
      streamResponses,
      finalConnectionState,
      frontendStateGetCount: stateRequests.length,
      quietPeriodMs: 3200,
    };
  } finally {
    await page.context().setOffline(false).catch(() => {});
    if (roomCode) {
      const cleanupContexts = [page.context(), ...guestContexts];
      await Promise.all(
        cleanupContexts.map((context) =>
          context.request
            .post(`${baseURL}/rooms/${roomCode}/leave`, { maxRedirects: 0 })
            .catch(() => null),
        ),
      );
    }
    await secondaryHostPage?.close().catch(() => {});
    await Promise.all(guestContexts.map((context) => context.close().catch(() => {})));
  }
}
