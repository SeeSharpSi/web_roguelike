// Execute with Playwright MCP browser_run_code_unsafe using this file as filename.
// Set ROOMS_TEST_URL when the server is not running at http://localhost:9779.
async (page) => {
  const baseURL =
    typeof process !== "undefined" && process.env && process.env.ROOMS_TEST_URL
      ? process.env.ROOMS_TEST_URL.replace(/\/$/, "")
      : "http://localhost:9779";
  const timeout = 20000;
  const checks = [];
  const extraContexts = [];

  function assert(condition, step, detail) {
    if (!condition) {
      throw new Error(`${step}: ${detail}`);
    }
  }

  function mark(step) {
    checks.push(step);
  }

  async function newIndependentPage() {
    const browser = page.context().browser();
    assert(Boolean(browser), "Create isolated browser context", "page has no browser instance");
    const context = await browser.newContext();
    extraContexts.push(context);
    return { context, page: await context.newPage() };
  }

  async function submitAndWaitForNavigation(targetPage, button, step) {
    const navigation = targetPage.waitForNavigation({
      waitUntil: "domcontentloaded",
      timeout,
    });
    const [response] = await Promise.all([navigation, button.click()]);
    assert(Boolean(response), step, "form submission did not produce a document response");
    return response;
  }

  async function waitForRoom(targetPage, { code, status, playerCount }, step) {
    await targetPage.waitForFunction(
      ({ code, status, playerCount }) => {
        const state = document.querySelector("#room-state");
        if (!state || state.dataset.roomCode !== code) return false;
        if (status && state.dataset.status !== status) return false;
        if (playerCount !== undefined) {
          const rows = state.querySelectorAll(
            "[data-player-id][data-x][data-y][data-health]",
          );
          if (rows.length !== playerCount) return false;
        }
        return true;
      },
      { code, status, playerCount },
      { timeout },
    ).catch((error) => {
      throw new Error(`${step}: room state did not reach requested code/status/roster (${error.message})`);
    });
  }

  async function visibleSnapshot(targetPage) {
    return targetPage.locator("#room-state").evaluate((state) => {
      const grid = state.querySelector(".map-grid");
      const players = Array.from(
        state.querySelectorAll("[data-player-id][data-x][data-y][data-health]"),
      )
        .map((row) => ({
          id: row.getAttribute("data-player-id"),
          x: row.getAttribute("data-x"),
          y: row.getAttribute("data-y"),
          health: row.getAttribute("data-health"),
          text: row.textContent || "",
        }))
        .sort((left, right) => left.id.localeCompare(right.id));
      return {
        code: state.getAttribute("data-room-code"),
        status: state.getAttribute("data-status"),
        revision: state.getAttribute("data-revision"),
        players,
        map: {
          gridStyle: grid ? grid.getAttribute("style") || "" : "",
          cells: Array.from(state.querySelectorAll(".room-cell"), (cell) =>
            cell.getAttribute("style") || "",
          ),
        },
      };
    });
  }

  async function apiSnapshot(targetPage, code, step) {
    const response = await targetPage.context().request.get(`${baseURL}/rooms/${code}/state`, {
      maxRedirects: 0,
    });
    assert(response.status() === 200, step, `GET state returned ${response.status()}, expected 200`);
    const markup = await response.text();
    return targetPage.evaluate((html) => {
      const documentCopy = new DOMParser().parseFromString(html, "text/html");
      const state = documentCopy.querySelector("#room-state");
      if (!state) return null;
      const grid = state.querySelector(".map-grid");
      const players = Array.from(
        state.querySelectorAll("[data-player-id][data-x][data-y][data-health]"),
      )
        .map((row) => ({
          id: row.getAttribute("data-player-id"),
          x: row.getAttribute("data-x"),
          y: row.getAttribute("data-y"),
          health: row.getAttribute("data-health"),
          text: row.textContent || "",
        }))
        .sort((left, right) => left.id.localeCompare(right.id));
      return {
        code: state.getAttribute("data-room-code"),
        status: state.getAttribute("data-status"),
        revision: state.getAttribute("data-revision"),
        players,
        map: {
          gridStyle: grid ? grid.getAttribute("style") || "" : "",
          cells: Array.from(state.querySelectorAll(".room-cell"), (cell) =>
            cell.getAttribute("style") || "",
          ),
        },
      };
    }, markup);
  }

  function playerByID(snapshot, id) {
    return snapshot.players.find((player) => player.id === id);
  }

  function playerStats(player) {
    return player && { x: player.x, y: player.y, health: player.health };
  }

  function assertSameStats(actual, expected, step) {
    assert(
      JSON.stringify(playerStats(actual)) === JSON.stringify(playerStats(expected)),
      step,
      `player stats changed: expected ${JSON.stringify(playerStats(expected))}, got ${JSON.stringify(playerStats(actual))}`,
    );
  }

  async function fillHomeJoin(targetPage, code, username) {
    await targetPage.goto(baseURL, { waitUntil: "domcontentloaded", timeout });
    const form = targetPage.locator("form[action='/rooms/join']");
    await form.locator("input[name='code']").fill(code);
    await form.locator("input[name='username']").fill(username);
    return form.getByRole("button", { name: "Join room", exact: true });
  }

  try {
    // 1. Create host room through the home form and preserve original session cookie.
    await page.goto(baseURL, { waitUntil: "domcontentloaded", timeout });
    const createForm = page.locator("form[action='/rooms']");
    await createForm.locator("input[name='username']").fill("Cassian");
    await submitAndWaitForNavigation(
      page,
      createForm.getByRole("button", { name: "Create room", exact: true }),
      "Create host room",
    );
    await page.locator("#room-state").waitFor({ state: "visible", timeout });
    const initialHost = await visibleSnapshot(page);
    const primaryCode = initialHost.code;
    assert(
      /^[A-Z0-9]{6}$/.test(primaryCode || ""),
      "Create host room",
      `expected canonical six-character room code, got ${JSON.stringify(primaryCode)}`,
    );
    await waitForRoom(page, { code: primaryCode, status: "lobby", playerCount: 1 }, "Create host room");
    const hostID = initialHost.players[0] && initialHost.players[0].id;
    assert(Boolean(hostID), "Create host room", "host roster row has no player ID");
    assert(initialHost.map.cells.length > 0, "Create host room", "map has no room cells");
    assert(
      /grid-template-columns/.test(initialHost.map.gridStyle) &&
        /grid-template-rows/.test(initialHost.map.gridStyle),
      "Create host room",
      "map grid dimensions missing from inline style",
    );
    const originalCookies = await page.context().cookies(baseURL);
    assert(originalCookies.length > 0, "Capture original session", "host session has no cookies");
    const originalCookieValues = originalCookies.map(({ name, value }) => ({ name, value }));

    const oldSession = await newIndependentPage();
    await oldSession.context.addCookies(originalCookies);
    mark("Host created canonical lobby with one player, map, stats, and original session cookie");

    // 2. Join same room from independent context and verify shared map and host SSE updates.
    const peer = await newIndependentPage();
    const peerJoinButton = await fillHomeJoin(peer.page, primaryCode, "Scout");
    await submitAndWaitForNavigation(peer.page, peerJoinButton, "Join primary room as Scout");
    await waitForRoom(
      peer.page,
      { code: primaryCode, status: "lobby", playerCount: 2 },
      "Join primary room as Scout",
    );
    await page
      .locator("#room-state [data-player-id][data-x][data-y][data-health]")
      .nth(1)
      .waitFor({ state: "attached", timeout });
    await waitForRoom(page, { code: primaryCode, status: "lobby", playerCount: 2 }, "Host roster SSE update");
    const peerSnapshot = await visibleSnapshot(peer.page);
    const joinedIDs = peerSnapshot.players.map((player) => player.id);
    assert(new Set(joinedIDs).size === 2, "Join primary room", "duplicate roster player IDs found");
    assert(joinedIDs.includes(hostID), "Join primary room", "host ID missing from peer roster");
    assert(
      peerSnapshot.players.some((player) => /Scout/.test(player.text)),
      "Join primary room",
      "Scout username missing from peer roster",
    );
    assert(
      JSON.stringify(peerSnapshot.map) === JSON.stringify(initialHost.map),
      "Join primary room",
      "peer sees a different map wall signature or grid dimensions",
    );
    mark("Independent Scout joined; host SSE update showed two players and identical map");

    // 3. Create another room with same username; room identity and state remain isolated.
    const isolated = await newIndependentPage();
    await isolated.page.goto(baseURL, { waitUntil: "domcontentloaded", timeout });
    const isolatedCreate = isolated.page.locator("form[action='/rooms']");
    await isolatedCreate.locator("input[name='username']").fill("Cassian");
    await submitAndWaitForNavigation(
      isolated.page,
      isolatedCreate.getByRole("button", { name: "Create room", exact: true }),
      "Create isolated room",
    );
    await isolated.page.locator("#room-state").waitFor({ state: "visible", timeout });
    const isolatedInitial = await visibleSnapshot(isolated.page);
    assert(
      /^[A-Z0-9]{6}$/.test(isolatedInitial.code || "") && isolatedInitial.code !== primaryCode,
      "Create isolated room",
      `second room code invalid or reused: ${JSON.stringify(isolatedInitial.code)}`,
    );
    await waitForRoom(
      isolated.page,
      { code: isolatedInitial.code, status: "lobby", playerCount: 1 },
      "Create isolated room",
    );
    const isolatedID = isolatedInitial.players[0] && isolatedInitial.players[0].id;
    assert(Boolean(isolatedID) && isolatedID !== hostID, "Create isolated room", "player ID was not isolated");
    mark("Same username created separate lobby with distinct code and player ID");

    // 4. Start room, observe peer SSE updates, and move or verify rejected invalid direction.
    await page.getByRole("button", { name: "Start game", exact: true }).click();
    await waitForRoom(page, { code: primaryCode, status: "active", playerCount: 2 }, "Start primary game");
    await waitForRoom(
      peer.page,
      { code: primaryCode, status: "active", playerCount: 2 },
      "Peer observes active game",
    );
    const activeBeforeMove = await visibleSnapshot(page);
    const hostBeforeMove = playerByID(activeBeforeMove, hostID);
    assert(Boolean(hostBeforeMove), "Start primary game", "host row missing after start");
    const directions = ["north", "south", "east", "west"];
    let availableDirection = null;
    for (const direction of directions) {
      const button = page.getByRole("button", { name: `Move ${direction}`, exact: true });
      if (await button.isEnabled()) {
        availableDirection = direction;
        break;
      }
    }

    let movement = { result: "blocked-start", direction: null, from: playerStats(hostBeforeMove), to: playerStats(hostBeforeMove) };
    if (availableDirection) {
      const deltas = {
        north: { x: 0, y: 1 },
        south: { x: 0, y: -1 },
        east: { x: 1, y: 0 },
        west: { x: -1, y: 0 },
      };
      const delta = deltas[availableDirection];
      const expectedX = Number(hostBeforeMove.x) + delta.x;
      const expectedY = Number(hostBeforeMove.y) + delta.y;
      await page.getByRole("button", { name: `Move ${availableDirection}`, exact: true }).click();
      await page.waitForFunction(
        ({ id, x, y }) => {
          const row = Array.from(
            document.querySelectorAll("#room-state [data-player-id][data-x][data-y][data-health]"),
          ).find((candidate) => candidate.getAttribute("data-player-id") === id);
          return row && Number(row.getAttribute("data-x")) === x && Number(row.getAttribute("data-y")) === y;
        },
        { id: hostID, x: expectedX, y: expectedY },
        { timeout },
      ).catch((error) => {
        throw new Error(`Move ${availableDirection}: host position did not update (${error.message})`);
      });
      await peer.page.waitForFunction(
        ({ id, x, y }) => {
          const row = Array.from(
            document.querySelectorAll("#room-state [data-player-id][data-x][data-y][data-health]"),
          ).find((candidate) => candidate.getAttribute("data-player-id") === id);
          return row && Number(row.getAttribute("data-x")) === x && Number(row.getAttribute("data-y")) === y;
        },
        { id: hostID, x: expectedX, y: expectedY },
        { timeout },
      ).catch((error) => {
        throw new Error(`Peer move SSE update: host position did not update (${error.message})`);
      });
      const afterMove = await visibleSnapshot(page);
      const movedHost = playerByID(afterMove, hostID);
      assert(
        Number(movedHost.x) === expectedX && Number(movedHost.y) === expectedY,
        "Move host player",
        `expected (${expectedX},${expectedY}), got (${movedHost.x},${movedHost.y})`,
      );
      const peerAfterMove = await visibleSnapshot(peer.page);
      assert(
        JSON.stringify(peerAfterMove.map) === JSON.stringify(initialHost.map),
        "Peer observes movement",
        "shared map wall signature changed during movement",
      );
      assert(
        JSON.stringify(afterMove.map) === JSON.stringify(initialHost.map),
        "Move host player",
        "map wall signature changed during movement",
      );
      movement = {
        result: "moved",
        direction: availableDirection,
        from: playerStats(hostBeforeMove),
        to: playerStats(movedHost),
      };
      mark(`Host moved ${availableDirection}; peer SSE update observed position and map stayed stable`);
    } else {
      const response = await page.context().request.post(`${baseURL}/rooms/${primaryCode}/actions`, {
        form: { direction: "invalid-direction" },
        maxRedirects: 0,
      });
      assert(
        response.status() >= 400 && response.status() < 500,
        "Blocked-start invalid move check",
        `invalid direction returned ${response.status()}, expected client error`,
      );
      const afterRejectedMove = await apiSnapshot(page, primaryCode, "Read state after invalid move");
      assert(Boolean(afterRejectedMove), "Blocked-start invalid move check", "state fragment missing");
      assert(
        JSON.stringify(afterRejectedMove.players) === JSON.stringify(activeBeforeMove.players) &&
          JSON.stringify(afterRejectedMove.map) === JSON.stringify(activeBeforeMove.map) &&
          afterRejectedMove.status === activeBeforeMove.status,
        "Blocked-start invalid move check",
        "invalid direction changed player state, status, or map",
      );
      mark("No legal start move; invalid direction rejected without changing state");
    }

    // 5. Verify non-host action denial and stranger state privacy.
    const peerStart = await peer.page.context().request.post(`${baseURL}/rooms/${primaryCode}/start`, {
      maxRedirects: 0,
    });
    assert(peerStart.status() === 403, "Peer start authorization", `expected 403, got ${peerStart.status()}`);
    const stranger = await newIndependentPage();
    const strangerState = await stranger.page.context().request.get(`${baseURL}/rooms/${primaryCode}/state`, {
      maxRedirects: 0,
    });
    assert(strangerState.status() === 401, "Stranger state authorization", `expected 401, got ${strangerState.status()}`);
    const strangerBody = await strangerState.text();
    assert(
      !/id=["']room-state|data-player-id|Cassian|Scout/.test(strangerBody),
      "Stranger state privacy",
      "unauthorized state response disclosed room or player data",
    );
    mark("Peer start denied with 403; stranger room-state denied with 401 and no game data");

    // 6. Prove original browser renders host first, then revoke it through a new host session.
    await oldSession.page.goto(`${baseURL}/rooms/${primaryCode}`, {
      waitUntil: "domcontentloaded",
      timeout,
    });
    await waitForRoom(
      oldSession.page,
      { code: primaryCode, status: "active", playerCount: 2 },
      "Load original host browser",
    );
    const originalBrowserSnapshot = await visibleSnapshot(oldSession.page);
    assert(
      Boolean(playerByID(originalBrowserSnapshot, hostID)),
      "Load original host browser",
      "saved original session did not render host player",
    );
    assertSameStats(
      playerByID(originalBrowserSnapshot, hostID),
      movement.to,
      "Load original host browser",
    );

    await page.context().clearCookies();
    const reconnectButton = await fillHomeJoin(page, ` ${primaryCode.toLowerCase()} `, " cAsSiAn ");
    await submitAndWaitForNavigation(page, reconnectButton, "Reconnect host in active room");
    await waitForRoom(page, { code: primaryCode, status: "active", playerCount: 2 }, "Reconnect host in active room");
    const reconnectedActive = await visibleSnapshot(page);
    const reconnectedHost = playerByID(reconnectedActive, hostID);
    assert(Boolean(reconnectedHost), "Reconnect active host", "original host player ID was not recovered");
    assertSameStats(reconnectedHost, movement.to, "Reconnect active host");
    assert(
      JSON.stringify(reconnectedActive.map) === JSON.stringify(initialHost.map),
      "Reconnect active host",
      "map changed across reconnect",
    );
    assert(reconnectedActive.players.length === 2, "Reconnect active host", "reconnect duplicated a participant");
    const replacementCookies = await page.context().cookies(baseURL);
    const changedSessionCookie = replacementCookies.some((current) =>
      originalCookieValues.some((original) => original.name === current.name && original.value !== current.value),
    );
    assert(changedSessionCookie, "Reconnect active host", "new session cookie ID did not replace original value");
    const oldState = await oldSession.context.request.get(`${baseURL}/rooms/${primaryCode}/state`, {
      maxRedirects: 0,
    });
    assert(oldState.status() === 401, "Revoke previous host session", `old GET state returned ${oldState.status()}, expected 401`);
    const oldAction = await oldSession.context.request.post(`${baseURL}/rooms/${primaryCode}/actions`, {
      form: { direction: "north" },
      maxRedirects: 0,
    });
    assert(oldAction.status() === 401, "Revoke previous host session", `old POST action returned ${oldAction.status()}, expected 401`);

    const recoveryPath = `/rooms/${primaryCode}`;
    const oldSessionRecoveryForm = oldSession.page.locator(`form[action='${recoveryPath}/join']`);
    await oldSessionRecoveryForm.waitFor({ state: "visible", timeout });
    await oldSession.page.locator("#room-state").waitFor({ state: "detached", timeout });
    assert(
      (await oldSession.page.evaluate(() => window.location.pathname)) === recoveryPath,
      "Old browser recovery redirect",
      `expected ${recoveryPath}, got ${oldSession.page.url()}`,
    );
    assert(
      (await oldSession.page.getByRole("button", { name: "Join room", exact: true }).count()) > 0,
      "Old browser recovery UI",
      "join action is not available to the revoked session",
    );
    await waitForRoom(peer.page, { code: primaryCode, status: "active", playerCount: 2 }, "Old browser does not reclaim player");
    mark("Active original browser initially rendered host, then redirected to join form after takeover; no auto-rejoin");

    // 7. Reject a new player in active room; independently created room remains untouched.
    const activeJoinButton = await fillHomeJoin(stranger.page, primaryCode, "Newcomer");
    await submitAndWaitForNavigation(
      stranger.page,
      activeJoinButton,
      "Reject new player after game start",
    );
    const activeJoinAlert = stranger.page.getByRole("alert");
    await activeJoinAlert.waitFor({ state: "visible", timeout });
    await waitForRoom(page, { code: primaryCode, status: "active", playerCount: 2 }, "Reject new active-room player");
    const isolatedAfterJoin = await visibleSnapshot(isolated.page);
    assert(
      isolatedAfterJoin.status === "lobby" && isolatedAfterJoin.players.length === 1 &&
        isolatedAfterJoin.players[0].id === isolatedID &&
        JSON.stringify(isolatedAfterJoin.map) === JSON.stringify(isolatedInitial.map) &&
        JSON.stringify(playerStats(isolatedAfterJoin.players[0])) ===
          JSON.stringify(playerStats(isolatedInitial.players[0])),
      "Room isolation after active join rejection",
      "second room state changed unexpectedly",
    );
    mark("Active room rejected new username with visible error; roster stayed two and second room stayed isolated");

    // 8. Finish from host UI, verify peer SSE update, then reconnect into finished room.
    const hostBeforeFinish = playerByID(await visibleSnapshot(page), hostID);
    await page.getByRole("button", { name: "Finish game", exact: true }).click();
    await waitForRoom(page, { code: primaryCode, status: "finished", playerCount: 2 }, "Finish primary game");
    await waitForRoom(
      peer.page,
      { code: primaryCode, status: "finished", playerCount: 2 },
      "Peer observes finished game",
    );
    await page.context().clearCookies();
    const finishedReconnectButton = await fillHomeJoin(page, primaryCode, "Cassian");
    await submitAndWaitForNavigation(page, finishedReconnectButton, "Reconnect host in finished room");
    await waitForRoom(
      page,
      { code: primaryCode, status: "finished", playerCount: 2 },
      "Reconnect host in finished room",
    );
    const finishedReconnect = await visibleSnapshot(page);
    const finishedHost = playerByID(finishedReconnect, hostID);
    assert(Boolean(finishedHost), "Reconnect finished host", "original host ID missing in finished room");
    assertSameStats(finishedHost, hostBeforeFinish, "Reconnect finished host");
    assert(
      JSON.stringify(finishedReconnect.map) === JSON.stringify(initialHost.map),
      "Reconnect finished host",
      "finished room map changed across reconnect",
    );
    mark("Peer observed finished status; host reconnected to finished room with same ID and stats");

    // 9. Explicitly abandon both players, verify host transfer, closed room, and unknown-code UI.
    const leaveHost = page.getByRole("button", { name: "Leave game", exact: true });
    const hostLeaveResponse = await submitAndWaitForNavigation(page, leaveHost, "Host explicitly leaves finished room");
    assert(hostLeaveResponse.status() < 400, "Host leaves room", `leave returned ${hostLeaveResponse.status()}`);
    await waitForRoom(peer.page, { code: primaryCode, status: "finished", playerCount: 1 }, "Peer observes host leave");
    const afterHostLeave = await visibleSnapshot(peer.page);
    assert(!afterHostLeave.players.some((player) => player.id === hostID), "Host leaves room", "host still present in roster");
    const remainingPeer = afterHostLeave.players[0];
    assert(Boolean(remainingPeer) && /Host/.test(remainingPeer.text), "Host transfer", "remaining peer lacks host indicator");
    assert(
      (await peer.page.getByRole("button", { name: "Start game", exact: true }).count()) === 0 &&
        (await peer.page.getByRole("button", { name: "Finish game", exact: true }).count()) === 0,
      "Finished host transfer",
      "finished room unexpectedly shows Start or Finish action",
    );

    const leavePeer = peer.page.getByRole("button", { name: "Leave game", exact: true });
    await submitAndWaitForNavigation(peer.page, leavePeer, "Peer explicitly leaves room");
    const closedRoomState = await peer.page.context().request.get(`${baseURL}/rooms/${primaryCode}/state`, {
      maxRedirects: 0,
    });
    assert(closedRoomState.status() === 404, "Close empty room", `GET state returned ${closedRoomState.status()}, expected 404`);

    const unknownCode = "ZZZZZZ";
    assert(
      unknownCode !== primaryCode && unknownCode !== isolatedInitial.code,
      "Unknown code setup",
      "chosen unknown code collides with a created room",
    );
    await stranger.page.goto(baseURL, { waitUntil: "domcontentloaded", timeout });
    const unknownJoinForm = stranger.page.locator("form[action='/rooms/join']");
    await unknownJoinForm.locator("input[name='code']").fill(unknownCode);
    await unknownJoinForm.locator("input[name='username']").fill("Visitor");
    const unknownResponse = await submitAndWaitForNavigation(
      stranger.page,
      unknownJoinForm.getByRole("button", { name: "Join room", exact: true }),
      "Join unknown room code",
    );
    assert(unknownResponse.status() === 404, "Unknown room code", `form returned ${unknownResponse.status()}, expected 404`);
    const unknownAlert = stranger.page.getByRole("alert");
    await unknownAlert.waitFor({ state: "visible", timeout });
    assert(
      ((await unknownAlert.textContent()) || "").trim().length > 0,
      "Unknown room code",
      "404 response has no visible error message",
    );
    mark("Host transfer verified; final leave closed room with 404; unknown code showed visible 404 error");

    return {
      checks,
      roomCodes: { primary: primaryCode, isolated: isolatedInitial.code },
      hostPlayerID: hostID,
      movement,
    };
  } finally {
    await Promise.all(extraContexts.map((context) => context.close().catch(() => {})));
  }
}
