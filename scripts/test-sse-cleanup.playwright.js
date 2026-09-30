async (page) => {
  const baseURL = "http://localhost:9779";
  const maxAttempts = 12;
  const timeout = 20000;
  const browser = page.context().browser();
  const checks = [];
  const attempts = [];
  const pageErrors = [];
  const consoleErrors = [];
  let room = null;

  function assert(condition, message, detail = "") {
    if (!condition) throw new Error(`${message}${detail ? `: ${detail}` : ""}`);
  }

  async function within(promise, label, limit = timeout) {
    let timer;
    try {
      return await Promise.race([
        promise,
        new Promise((_, reject) => {
          timer = setTimeout(() => reject(new Error(`Timed out waiting for ${label}`)), limit);
        }),
      ]);
    } finally {
      clearTimeout(timer);
    }
  }

  function monitorPage(targetPage, label) {
    const metrics = {
      streamRequests: 0,
      stateGETs: 0,
      actionRequests: 0,
      pageErrors: [],
      consoleErrors: [],
    };
    targetPage.on("request", (request) => {
      let url;
      try {
        url = new URL(request.url());
      } catch {
        return;
      }
      if (request.method() === "GET" && /\/rooms\/[^/]+\/events$/.test(url.pathname)) {
        metrics.streamRequests++;
      }
      if (request.method() === "GET" && /\/rooms\/[^/]+\/state$/.test(url.pathname)) {
        metrics.stateGETs++;
      }
      if (request.method() === "POST" && /\/rooms\/[^/]+\/actions$/.test(url.pathname)) {
        metrics.actionRequests++;
      }
    });
    targetPage.on("pageerror", (error) => {
      const entry = { page: label, message: error.message, stack: error.stack };
      metrics.pageErrors.push(entry);
      pageErrors.push(entry);
    });
    targetPage.on("console", (message) => {
      if (message.type() === "error") {
        const entry = { page: label, message: message.text() };
        metrics.consoleErrors.push(entry);
        consoleErrors.push(entry);
      }
    });
    return metrics;
  }

  async function cleanupOwnedRoom(ownedRoom) {
    if (!ownedRoom) return "no owned room";
    try {
      if (!ownedRoom.page.isClosed()) {
        await ownedRoom.page.close({ runBeforeUnload: false });
      }
      const response = await ownedRoom.context.request.post(
        `${baseURL}/rooms/${ownedRoom.code}/leave`,
        { maxRedirects: 0, timeout: 5000 },
      );
      return response.status() === 303
        ? "authorized Leave returned 303"
        : `authorized Leave returned ${response.status()}`;
    } catch (error) {
      return `cleanup failed: ${error.message}`;
    } finally {
      await ownedRoom.context.close().catch(() => {});
    }
  }

  async function createOwnedRoom(attempt) {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
    context.setDefaultTimeout(timeout);
    const testPage = await context.newPage();
    const username = `StaleCleanup-${Date.now().toString(36)}-${attempt}`.slice(0, 32);
    const metrics = monitorPage(testPage, `attempt-${attempt}`);
    let code = "";
    const candidate = { context, page: testPage, username, code, metrics, attempt };
    try {
      await within(testPage.goto(baseURL, { waitUntil: "domcontentloaded" }), "entry page");
      const form = testPage.locator("form[action='/rooms']");
      await form.locator('input[name="username"]').fill(username);
      const navigation = testPage.waitForNavigation({ waitUntil: "domcontentloaded", timeout });
      await Promise.all([
        navigation,
        form.getByRole("button", { name: "Create room", exact: true }).click(),
      ]);
      await within(testPage.locator("#room-state").waitFor({ state: "visible" }), "created room state");
      code = await testPage.locator("#room-state").getAttribute("data-room-code");
      assert(code, "Created room has no code");
      candidate.code = code;
      await within(testPage.waitForFunction(() => {
        const connection = document.querySelector("#room-events")?._htmx?.sse;
        return Boolean(connection?.reader && connection.status === 200 && connection.attempt === 0);
      }), "real SSE reader");
      return candidate;
    } catch (error) {
      await cleanupOwnedRoom(candidate);
      throw error;
    }
  }

  assert(browser, "Runner page has no browser instance");

  let selectedDirection = "";
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    const candidate = await createOwnedRoom(attempt);
    try {
      await candidate.page.getByRole("button", { name: "Start game", exact: true }).click();
      await within(candidate.page.waitForFunction(() => {
        return document.querySelector("#room-state")?.dataset.status === "active";
      }), "active game state");
      const legalDirections = await candidate.page.locator(".direction-pad button:not([disabled])").evaluateAll((buttons) => {
        return buttons.map((button) => button.getAttribute("aria-label")?.replace(/^Move /, ""));
      });
      if (legalDirections.length > 0) {
        room = candidate;
        selectedDirection = legalDirections[0];
        attempts.push({
          attempt,
          roomCode: candidate.code,
          result: "legal movement found",
          legalDirections,
        });
        break;
      }
      const cleanup = await cleanupOwnedRoom(candidate);
      attempts.push({ attempt, roomCode: candidate.code, result: "no legal movement direction", cleanup });
    } catch (error) {
      await cleanupOwnedRoom(candidate);
      attempts.push({ attempt, roomCode: candidate.code, result: "start failed", error: error.message });
      throw error;
    }
  }

  assert(room, `No legal movement direction found after ${maxAttempts} owned rooms`);

  let moveGate = null;
  let moveFulfilled = null;
  let routeHeld = false;
  let backendMoveStatus = null;
  let moveResponseRevision = null;
  let backendMoveError = null;
  let scenarioError = null;
  let details = {};

  try {
    const testPage = room.page;
    const roomCode = room.code;
    const movePath = `/rooms/${roomCode}/actions`;
    const beforeMove = await testPage.locator("#room-state").evaluate((state) => ({
      revision: Number(state.dataset.revision),
      coordinate: state.dataset.currentRoom,
      status: state.dataset.status,
    }));
    assert(beforeMove.status === "active", "Game was not active before movement", beforeMove.status);

    await testPage.evaluate(() => {
      window.__cleanupRegressionFinallyRequests = [];
      document.addEventListener("htmx:finally:request", (event) => {
        const context = event.detail?.ctx;
        window.__cleanupRegressionFinallyRequests.push({
          action: context?.request?.action || context?.request?.url || "",
          sourceID: context?.sourceElement?.id || "",
        });
      });
    });

    moveGate = new Promise((resolve) => { room.releaseMove = resolve; });
    const moveBackendReady = new Promise((resolve) => { room.resolveMoveReady = resolve; });
    moveFulfilled = new Promise((resolve) => { room.resolveMoveFulfilled = resolve; });
    room.context.route(`**${movePath}`, async (route) => {
      if (route.request().method() !== "POST") {
        await route.continue();
        return;
      }
      try {
        const response = await route.fetch({ maxRedirects: 0 });
        const body = await response.text();
        backendMoveStatus = response.status();
        const revisionMatch = body.match(/data-revision="(\d+)"/);
        moveResponseRevision = revisionMatch ? Number(revisionMatch[1]) : null;
        routeHeld = true;
        room.resolveMoveReady({ status: backendMoveStatus, revision: moveResponseRevision });
        await moveGate;
        await route.fulfill({ response, body });
        routeHeld = false;
        room.resolveMoveFulfilled();
      } catch (error) {
        backendMoveError = error.message;
        routeHeld = false;
        room.resolveMoveReady({ error: error.message });
        room.resolveMoveFulfilled();
        try { await route.abort(); } catch {}
      }
    });

    const moveButton = testPage.getByRole("button", { name: `Move ${selectedDirection}`, exact: true });
    assert(await moveButton.isEnabled(), "Selected movement button was not enabled", selectedDirection);
    await moveButton.click();
    const moveResult = await within(moveBackendReady, "real backend Move 200 response held");
    assert(!moveResult.error, "Move backend request failed", moveResult.error || backendMoveError || "");
    assert(backendMoveStatus === 200, "Move backend did not return HTTP 200", String(backendMoveStatus));
    assert(moveResponseRevision === beforeMove.revision + 1, "Move response revision was unexpected", String(moveResponseRevision));

    await within(testPage.waitForFunction(({ revision, coordinate }) => {
      const state = document.querySelector("#room-state");
      return state && Number(state.dataset.revision) > revision && state.dataset.currentRoom !== coordinate;
    }, { revision: beforeMove.revision, coordinate: beforeMove.coordinate }), "SSE move revision and coordinate update");
    const afterMove = await testPage.locator("#room-state").evaluate((state) => ({
      revision: Number(state.dataset.revision),
      coordinate: state.dataset.currentRoom,
      status: state.dataset.status,
    }));
    assert(afterMove.revision === moveResponseRevision, "SSE Move update revision differed from held response", `${afterMove.revision} vs ${moveResponseRevision}`);
    assert(afterMove.coordinate !== beforeMove.coordinate, "Legal movement did not change coordinate");

    const finishResponse = await within(room.context.request.post(`${baseURL}/rooms/${roomCode}/finish`, {
      headers: { "HX-Request": "true" },
      maxRedirects: 0,
      timeout,
    }), "same-context authorized Finish POST");
    const finishHTML = await finishResponse.text();
    const finishRevisionMatch = finishHTML.match(/data-revision="(\d+)"/);
    const finishRevision = finishRevisionMatch ? Number(finishRevisionMatch[1]) : null;
    assert(finishResponse.status() === 200, "Finish endpoint did not return HTTP 200", String(finishResponse.status()));
    assert(finishRevision === 4, "Finish response did not have expected revision 4", String(finishRevision));

    await within(testPage.waitForFunction((revision) => {
      const state = document.querySelector("#room-state");
      const buttons = Array.from(document.querySelectorAll(".direction-pad button"));
      return state?.dataset.status === "finished" &&
        Number(state.dataset.revision) === revision &&
        buttons.length === 4 && buttons.every((button) => button.disabled);
    }, finishRevision), "finished revision 4 and all movement buttons disabled");
    const finishedSSE = await testPage.locator("#room-state").evaluate((state) => ({
      revision: Number(state.dataset.revision),
      coordinate: state.dataset.currentRoom,
      status: state.dataset.status,
    }));
    const buttonsBeforeRelease = await testPage.locator(".direction-pad button").evaluateAll((buttons) => buttons.map((button) => ({
      direction: button.getAttribute("aria-label")?.replace(/^Move /, ""),
      disabled: button.disabled,
    })));

    room.releaseMove();
    await within(moveFulfilled, "release and fulfill stale Move response");
    await within(testPage.waitForFunction((path) => {
      return (window.__cleanupRegressionFinallyRequests || []).some((entry) => {
        try { return new URL(entry.action, location.href).pathname === path; } catch { return false; }
      });
    }, movePath), "htmx:finally:request for Move action");
    await within(testPage.evaluate(() => new Promise((resolve) => {
      requestAnimationFrame(() => requestAnimationFrame(resolve));
    })), "two animation frames after HTMX cleanup");

    const finalState = await testPage.locator("#room-state").evaluate((state) => ({
      revision: Number(state.dataset.revision),
      coordinate: state.dataset.currentRoom,
      status: state.dataset.status,
    }));
    const finalButtons = await testPage.locator(".direction-pad button").evaluateAll((buttons) => buttons.map((button) => ({
      direction: button.getAttribute("aria-label")?.replace(/^Move /, ""),
      disabled: button.disabled,
    })));
    const finallyRequests = await testPage.evaluate(() => window.__cleanupRegressionFinallyRequests);
    details = {
      roomCode,
      username: room.username,
      selectedDirection,
      beforeMove,
      moveStatus: backendMoveStatus,
      moveResponseRevision,
      afterMoveSSE: afterMove,
      finishStatus: finishResponse.status(),
      finishRevision,
      finishedSSE,
      buttonsBeforeStaleResponseRelease: buttonsBeforeRelease,
      finallyRequests,
      finalStateAfterTwoFrames: finalState,
      finalButtonsAfterTwoFrames: finalButtons,
      getStateRequests: room.metrics.stateGETs,
      streamRequests: room.metrics.streamRequests,
      moveActionRequests: room.metrics.actionRequests,
    };

    assert(finalState.revision === 4 && finalState.status === "finished", "Final room state changed from finished revision 4", JSON.stringify(finalState));
    assert(finalButtons.length === 4 && finalButtons.every((button) => button.disabled), "Stale Move cleanup re-enabled a finished movement button", JSON.stringify(finalButtons));
    assert(room.metrics.stateGETs === 0, "Browser made unexpected GET /state request", String(room.metrics.stateGETs));
    assert(room.metrics.actionRequests === 1, "Browser issued unexpected Move request count", String(room.metrics.actionRequests));
    assert(finallyRequests.some((entry) => new URL(entry.action, baseURL).pathname === movePath), "Move htmx:finally:request was not observed");
  } catch (error) {
    scenarioError = error;
  } finally {
    if (room?.releaseMove) room.releaseMove();
    if (routeHeld && moveFulfilled) {
      await within(moveFulfilled, "held Move route during cleanup", 5000).catch(() => {});
    }
    if (room) {
      room.cleanup = await cleanupOwnedRoom(room);
      attempts.push({
        attempt: room.attempt,
        roomCode: room.code,
        result: "regression scenario completed",
        cleanup: room.cleanup,
      });
    }
  }

  assert(pageErrors.length === 0, "Browser page errors were recorded", JSON.stringify(pageErrors));
  assert(!scenarioError, "SSE action-cleanup regression failed", scenarioError?.message || "");
  checks.push(`Created owned room ${details.roomCode}; started game and moved ${details.selectedDirection} through real UI.`);
  checks.push(`Held backend Move ${details.moveStatus} at revision ${details.moveResponseRevision}; SSE advanced to ${details.afterMoveSSE.coordinate}.`);
  checks.push(`Same-context authorized Finish returned revision ${details.finishRevision}; SSE showed finished state with all four buttons disabled.`);
  checks.push("Released stale Move response; observed htmx:finally:request and waited two animation frames; all four movement buttons stayed disabled.");
  checks.push("GET /state count was zero; browser pageErrors count was zero.");
  return {
    checks,
    attempts,
    regression: details,
    pageErrors,
    consoleErrors,
  };
}
