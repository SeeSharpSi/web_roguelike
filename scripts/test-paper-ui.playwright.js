// Execute with Playwright MCP browser_run_code_unsafe using this file as filename.
// Set PAPER_UI_TEST_URL (or ROOMS_TEST_URL) when server is not at localhost:9779.
async (page) => {
  const configuredURL =
    typeof process !== "undefined" && process.env
      ? process.env.PAPER_UI_TEST_URL || process.env.ROOMS_TEST_URL
      : "";
  const baseURL = (configuredURL || "http://localhost:9779").replace(/\/$/, "");
  const timeout = 25000;
  const checks = [];
  const guestContexts = [];
  const guestPages = [];
  const createdRoomCodes = [];

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
    guestPages.push(guestPage);
    return { context, page: guestPage };
  }

  async function createRoom(targetPage, username) {
    await targetPage.goto(baseURL, { waitUntil: "domcontentloaded", timeout });
    const form = targetPage.locator("form[action='/rooms']");
    await form.locator("input[name='username']").fill(username);
    await submitAndWaitForNavigation(
      targetPage,
      form.getByRole("button", { name: "Create room", exact: true }),
      `Create room as ${username}`,
    );
    await targetPage.locator("#room-state").waitFor({ state: "visible", timeout });
    const code = await targetPage.locator("#room-state").getAttribute("data-room-code");
    assert(/^[A-Z0-9]{6}$/.test(code || ""), "Create room", `invalid room code ${JSON.stringify(code)}`);
    createdRoomCodes.push(code);
    await waitForRoom(targetPage, { code, status: "lobby", playerCount: 1 }, "Create room");
    return code;
  }

  async function fillHomeJoin(targetPage, code, username) {
    await targetPage.goto(baseURL, { waitUntil: "domcontentloaded", timeout });
    const form = targetPage.locator("form[action='/rooms/join']");
    await form.locator("input[name='code']").fill(code);
    await form.locator("input[name='username']").fill(username);
    return form.getByRole("button", { name: "Join room", exact: true });
  }

  function assertNear(actual, expected, step, label, tolerance = 1) {
    assert(
      Number.isFinite(actual) && Math.abs(actual - expected) <= tolerance,
      step,
      `${label}: expected ${expected} ±${tolerance}px, got ${actual}`,
    );
  }

  function assertRect(rect, expected, step, label) {
    for (const [key, value] of Object.entries(expected)) {
      assertNear(rect[key], value, step, `${label}.${key}`);
    }
  }

  async function visibleRoster(targetPage) {
    return targetPage.locator("#room-state").evaluate((state) =>
      Array.from(state.querySelectorAll("[data-player-id][data-x][data-y][data-health]"), (row) => ({
        id: row.dataset.playerId,
        x: Number(row.dataset.x),
        y: Number(row.dataset.y),
        name: row.querySelector(".crew-name")?.textContent?.trim() || "",
      })),
    );
  }

  async function measureDesktopGeometry() {
    return page.evaluate(() => {
      const rect = (selector) => {
        const node = document.querySelector(selector);
        if (!node) return null;
        const box = node.getBoundingClientRect();
        return { x: box.x, y: box.y, width: box.width, height: box.height };
      };
      return {
        missionHeader: rect(".mission-header"),
        mapModule: rect(".map-module"),
        mapGrid: rect(".map-grid"),
        directiveCard: rect(".directive-card"),
        crewPanel: rect(".crew-panel"),
        vitalsPanel: rect(".vitals-panel"),
        dockPanel: rect(".dock-panel"),
        mapDimensions: {
          width: Number(document.querySelector(".sector-chart")?.dataset.mapWidth),
          length: Number(document.querySelector(".sector-chart")?.dataset.mapLength),
          cellSize: parseFloat(getComputedStyle(document.querySelector(".room-cell, .empty-cell")).width),
        },
      };
    });
  }

  async function checkWallPaletteAndRendering() {
    return page.locator("#room-state").evaluate((state) => {
      const colors = {
        transparent: "rgba(0, 0, 0, 0)",
        normal: "rgb(232, 222, 195)",
        door: "rgb(74, 166, 200)",
        destructible: "rgb(216, 91, 82)",
      };
      const allowed = new Set(Object.values(colors));
      const borderColors = [];
      for (const cell of state.querySelectorAll(".room-cell")) {
        const style = getComputedStyle(cell);
        borderColors.push(style.borderTopColor, style.borderRightColor, style.borderBottomColor, style.borderLeftColor);
      }
      const walls = Array.from(state.querySelectorAll(".map-wall"), (line) => {
        const box = {
          x1: Number(line.getAttribute("x1")),
          y1: Number(line.getAttribute("y1")),
          x2: Number(line.getAttribute("x2")),
          y2: Number(line.getAttribute("y2")),
        };
        const start = `${Math.min(box.x1, box.x2)},${Math.min(box.y1, box.y2)}`;
        const end = `${Math.max(box.x1, box.x2)},${Math.max(box.y1, box.y2)}`;
        const type = line.dataset.wallType;
        const expectedColor =
          type === "door" || type === "hidden_door"
            ? colors.door
            : type === "destructible"
              ? colors.destructible
              : colors.normal;
        const computed = getComputedStyle(line);
        return {
          ...box,
          edge: `${start}:${end}`,
          type,
          strokeAttribute: line.getAttribute("stroke"),
          stroke: computed.stroke,
          strokeWidth: computed.strokeWidth,
          expectedColor,
        };
      });
      const legend = {
        normal: getComputedStyle(state.querySelector(".legend-normal")).backgroundColor,
        door: getComputedStyle(state.querySelector(".legend-door")).backgroundColor,
        destructible: getComputedStyle(state.querySelector(".legend-damaged")).backgroundColor,
      };
      const floor = state.querySelector(".room-cell.uncharted-room, .empty-cell");
      const floorColor = floor ? getComputedStyle(floor).backgroundColor : "";
      const visibleWallPalette = new Set(walls.map((wall) => wall.stroke));
      return {
        bodyColor: getComputedStyle(document.body).backgroundColor,
        floorColor,
        borderColors,
        walls,
        legend,
        paletteCoverage: {
          blueInMatrix: borderColors.includes(colors.door),
          redInMatrix: borderColors.includes(colors.destructible),
          blueInVisibleWalls: visibleWallPalette.has(colors.door),
          redInVisibleWalls: visibleWallPalette.has(colors.destructible),
          blueInLegend: legend.door === colors.door,
          redInLegend: legend.destructible === colors.destructible,
        },
      };
    });
  }

  async function checkResponsiveViewport(width, height) {
    await page.setViewportSize({ width, height });
    const horizontal = await page.evaluate(() => ({
      innerWidth: window.innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      controls: Array.from(document.querySelectorAll("button"))
        .filter((node) => {
          const style = getComputedStyle(node);
          const box = node.getBoundingClientRect();
          return style.display !== "none" && style.visibility !== "hidden" && box.width > 0 && box.height > 0;
        })
        .map((node) => {
          const box = node.getBoundingClientRect();
          return { label: node.getAttribute("aria-label") || node.textContent.trim(), left: box.left, right: box.right };
        }),
    }));
    assert(
      horizontal.documentWidth <= horizontal.innerWidth + 1,
      `${width}px responsive width`,
      `document scrollWidth ${horizontal.documentWidth} exceeds viewport ${horizontal.innerWidth}`,
    );
    const outside = horizontal.controls.filter(
      (control) => control.left < -1 || control.right > horizontal.innerWidth + 1,
    );
    assert(
      outside.length === 0,
      `${width}px responsive controls`,
      `buttons outside horizontal viewport: ${JSON.stringify(outside)}`,
    );

    const panels = {};
    for (const [name, selector, topSelector, bottomSelector] of [
      ["roster", ".crew-panel", ".sidebar-panel-header h2", ".player-row:last-child"],
      ["readout", ".room-readout", ".readout-eyebrow", ".direction-pad button"],
      ["bottomDock", ".dock-panel", ".object-count", ".item-detail [data-inspect-trigger]"],
    ]) {
      const target = page.locator(selector);
      await target.scrollIntoViewIfNeeded({ timeout });
      panels[name] = await target.evaluate((node) => {
        const box = node.getBoundingClientRect();
        return { x: box.x, y: box.y, width: box.width, height: box.height };
      });
      const box = panels[name];
      assert(
        box.x >= -1 &&
          box.x + box.width <= width + 1 &&
          box.y < height &&
          box.y + box.height > 0,
        `${width}px responsive ${name}`,
        `panel has no visible intersection or exceeds horizontal bounds: ${JSON.stringify(box)}`,
      );
      const content = {};
      for (const [edge, childSelector] of [["top", topSelector], ["bottom", bottomSelector]]) {
        const matchingChildren = target.locator(childSelector);
        const child = edge === "bottom" ? matchingChildren.last() : matchingChildren.first();
        await child.waitFor({ state: "visible", timeout });
        await child.scrollIntoViewIfNeeded({ timeout });
        content[edge] = await child.evaluate((node) => {
          const rect = node.getBoundingClientRect();
          return { x: rect.x, y: rect.y, width: rect.width, height: rect.height };
        });
        const childBox = content[edge];
        assert(
          childBox.x >= -1 &&
            childBox.x + childBox.width <= width + 1 &&
            childBox.y < height &&
            childBox.y + childBox.height > 0,
          `${width}px responsive ${name} ${edge} content`,
          `content is not reachable after scrolling: ${JSON.stringify(childBox)}`,
        );
      }
      panels[name].content = content;
    }
    return { viewport: { width, height }, documentWidth: horizontal.documentWidth, buttonCount: horizontal.controls.length, panels };
  }

  try {
    await page.setViewportSize({ width: 1440, height: 900 });

    // Create disposable rooms through the real home form until generated starting room has an object.
    let roomCode = "";
    for (let attempt = 0; attempt < 16; attempt++) {
      roomCode = await createRoom(page, "Mira Voss");
      if ((await page.locator("#room-state button[data-quick-slot]").count()) > 0) break;
      assert(attempt < 15, "Find an inspectable starting-room object", "16 generated rooms had no quick-slot object");
      await submitAndWaitForNavigation(
        page,
        page.getByRole("button", { name: "Leave game", exact: true }),
        `Leave empty candidate room ${roomCode}`,
      );
      roomCode = "";
    }
    assert(Boolean(roomCode), "Find an inspectable starting-room object", "no candidate room retained");
    mark(`Created owned room ${roomCode} through home form with an inspectable starting-room object`);

    // Join three independent browser contexts, preserving this primary page for final review.
    for (const username of ["Eli Rane", "Sana Quill", "Orrin Vale"]) {
      const guest = await newIndependentPage();
      const joinButton = await fillHomeJoin(guest.page, roomCode, username);
      await submitAndWaitForNavigation(guest.page, joinButton, `Join ${roomCode} as ${username}`);
      await waitForRoom(guest.page, { code: roomCode, status: "lobby" }, `Guest observes ${username} join`);
    }
    await waitForRoom(page, { code: roomCode, status: "lobby", playerCount: 4 }, "Primary roster poll");
    assert(
      (await page.locator("#room-state .player-avatar").textContent())?.trim() === "P2",
      "Viewer roster number",
      "Mira Voss header avatar should be P2 after alphabetical four-player roster sort",
    );
    assert(await page.getByText("Mira Voss", { exact: false }).first().isVisible(), "Viewer username", "Mira Voss is not visible");
    const hostRow = await page.locator("#room-state [data-player-id][data-x][data-y][data-health]").evaluateAll((rows) =>
      rows.find((row) => row.textContent.includes("Mira Voss"))?.getAttribute("data-player-id") || "",
    );
    assert(Boolean(hostRow), "Find host player", "Mira Voss roster row has no data-player-id");
    mark("Three independent guests joined; host row, P2 avatar, and username rendered after polling");

    const geometries = await measureDesktopGeometry();
    assertRect(geometries.missionHeader, { height: 68 }, "Desktop Paper geometry", "missionHeader");
    assertRect(
      geometries.mapModule,
      { x: 44, y: 86, width: 938, height: 636 },
      "Desktop Paper geometry",
      "mapModule",
    );
    assertRect(
      geometries.mapGrid,
      { x: 88, y: 187 },
      "Desktop Paper geometry",
      "mapGrid",
    );
    assertRect(geometries.directiveCard, { height: 164 }, "Desktop Paper geometry", "directiveCard");
    assertRect(
      geometries.crewPanel,
      { y: 276, height: 270 },
      "Desktop Paper geometry",
      "crewPanel",
    );
    assertRect(
      geometries.vitalsPanel,
      { y: 572, height: 150 },
      "Desktop Paper geometry",
      "vitalsPanel",
    );
    assertRect(
      geometries.dockPanel,
      { x: 44, y: 736, width: 1364, height: 144 },
      "Desktop Paper geometry",
      "dockPanel",
    );
    assert(
      Number.isInteger(geometries.mapDimensions.width) &&
        geometries.mapDimensions.width > 0 &&
        Number.isInteger(geometries.mapDimensions.length) &&
        geometries.mapDimensions.length > 0,
      "Map chart dimensions",
      `map dimensions must be positive integers, got ${geometries.mapDimensions.width}×${geometries.mapDimensions.length}`,
    );
    assertNear(geometries.mapDimensions.cellSize, 38, "Map chart dimensions", "cellSize");
    assertNear(
      geometries.mapGrid.width,
      geometries.mapDimensions.width * geometries.mapDimensions.cellSize,
      "Map chart dimensions",
      "grid width from data-map-width × cell size",
    );
    assertNear(
      geometries.mapGrid.height,
      geometries.mapDimensions.length * geometries.mapDimensions.cellSize,
      "Map chart dimensions",
      "grid height from data-map-length × cell size",
    );
    const alignment = await page.locator("#room-state .player-row").evaluateAll((rows) =>
      rows.map((row) => {
        const rect = (selector) => {
          const box = row.querySelector(selector).getBoundingClientRect();
          return { x: box.x, width: box.width };
        };
        return {
          name: row.querySelector(".crew-name")?.textContent?.trim() || "",
          badge: rect(".crew-badge").x,
          identity: rect(".crew-identity").x,
          status: rect(".crew-status").x,
        };
      }),
    );
    assert(alignment.length === 4, "Crew column alignment", `expected four crew rows, got ${alignment.length}`);
    for (const column of ["badge", "identity", "status"]) {
      const positions = alignment.map((row) => row[column]);
      assert(
        Math.max(...positions) - Math.min(...positions) <= 1,
        "Crew column alignment",
        `${column} x positions differ: ${JSON.stringify(alignment)}`,
      );
    }
    mark("Desktop geometry matches Paper measurements; dynamic chart dimensions and crew columns align");

    const palette = await checkWallPaletteAndRendering();
    assert(palette.bodyColor === "rgb(0, 0, 0)", "Map palette", `body background is ${palette.bodyColor}`);
    const floorRGB = palette.floorColor.match(/\d+/g)?.slice(0, 3).map(Number) || [];
    assert(
      floorRGB.length === 3 && floorRGB.every((channel) => channel <= 32),
      "Map palette",
      `unexplored floor is not dark: ${palette.floorColor}`,
    );
    const invalidMetadata = palette.borderColors.filter(
      (color) => !["rgba(0, 0, 0, 0)", "rgb(232, 222, 195)", "rgb(74, 166, 200)", "rgb(216, 91, 82)"].includes(color),
    );
    assert(invalidMetadata.length === 0, "Wall metadata palette", `unexpected room-cell border colors: ${JSON.stringify(invalidMetadata)}`);
    assert(palette.walls.length > 0, "Rendered wall metadata", "no SVG wall segments are visible");
    const invalidWalls = palette.walls.filter((wall) => wall.stroke !== wall.expectedColor);
    assert(invalidWalls.length === 0, "Rendered wall palette", `wall strokes do not match metadata: ${JSON.stringify(invalidWalls)}`);
    const edgeCounts = new Map();
    for (const wall of palette.walls) edgeCounts.set(wall.edge, (edgeCounts.get(wall.edge) || 0) + 1);
    const duplicateEdges = Array.from(edgeCounts).filter(([, count]) => count > 1);
    assert(duplicateEdges.length === 0, "Shared wall rendering", `duplicate canonical SVG edges: ${JSON.stringify(duplicateEdges)}`);
    const normalWall = palette.walls.find((wall) => wall.type === "indestructible");
    const testedWall = normalWall || palette.walls[0];
    assert(testedWall.strokeWidth === "3px", "Wall stroke geometry", `expected 3px stroke, got ${testedWall.strokeWidth}`);
    if (normalWall) {
      assert(normalWall.stroke === "rgb(232, 222, 195)", "Normal wall color", `got ${normalWall.stroke}`);
    } else {
      assert(palette.legend.normal === "rgb(232, 222, 195)", "Normal wall color", `normal legend is ${palette.legend.normal}`);
    }
    assert(
      (palette.paletteCoverage.blueInMatrix || palette.paletteCoverage.blueInVisibleWalls || palette.paletteCoverage.blueInLegend) &&
        (palette.paletteCoverage.redInMatrix || palette.paletteCoverage.redInVisibleWalls || palette.paletteCoverage.redInLegend),
      "Special wall palette coverage",
      `door/destructible colors missing from matrix, visible walls, and legend: ${JSON.stringify(palette.paletteCoverage)}`,
    );
    mark("Body/floor and inline wall palette verified; SVG walls use 3px strokes with unique shared edges");

    // Exercise native dialog against two real htmx polling swaps.
    const quickSlots = page.locator("#room-state button[data-quick-slot]");
    const firstSlot = quickSlots.first();
    const itemName = await firstSlot.getAttribute("data-item-name");
    const itemDescription = await firstSlot.getAttribute("data-item-description");
    assert(Boolean(itemName && itemDescription), "Inspect room object", "first quick slot lacks real item metadata");
    await page.evaluate(() => {
      window.__paperUITestAfterSwapCount = 0;
      document.addEventListener("htmx:afterSwap", () => {
        window.__paperUITestAfterSwapCount += 1;
      });
    });
    await page.locator("#room-state [data-inspect-trigger]").click();
    const dialog = page.locator("dialog[data-inspect-dialog]");
    await dialog.waitFor({ state: "visible", timeout });
    await page.waitForFunction(() => window.__paperUITestAfterSwapCount >= 2, null, { timeout });
    assert(await dialog.evaluate((node) => node.open), "Inspect dialog polling regression", "dialog closed during htmx swaps");
    assert(
      (await dialog.locator("[data-dialog-name]").textContent())?.trim() === itemName &&
        (await dialog.locator("[data-dialog-description]").textContent())?.trim() === itemDescription,
      "Inspect dialog polling regression",
      "dialog item name or description changed during polling",
    );
    await dialog.getByRole("button", { name: "Close", exact: true }).click();
    await dialog.waitFor({ state: "hidden", timeout });
    if ((await quickSlots.count()) > 1) {
      const secondSlot = quickSlots.nth(1);
      await secondSlot.click();
      assert((await secondSlot.getAttribute("aria-pressed")) === "true", "Quick-slot selection", "click did not select slot 2");
      await page.keyboard.press("1");
      assert(
        (await firstSlot.getAttribute("aria-pressed")) === "true" &&
          (await secondSlot.getAttribute("aria-pressed")) === "false",
        "Quick-slot keyboard selection",
        "keyboard 1 did not restore slot 1 aria-pressed state",
      );
    } else {
      await page.keyboard.press("1");
      assert((await firstSlot.getAttribute("aria-pressed")) === "true", "Quick-slot keyboard selection", "keyboard 1 did not select slot 1");
    }
    mark("Inspect dialog retained real item details across two htmx polls; Close and keyboard slot selection worked");

    // Grow roster to eight through four more independent home-form joins.
    const finalUsers = ["Ada Venn", "Iona Reed", "Kael Dorne", "Tessa Wren"];
    for (let index = 0; index < finalUsers.length; index++) {
      const username = finalUsers[index];
      const guest = await newIndependentPage();
      const joinButton = await fillHomeJoin(guest.page, roomCode, username);
      await submitAndWaitForNavigation(guest.page, joinButton, `Join ${roomCode} as ${username}`);
      await waitForRoom(
        guest.page,
        { code: roomCode, status: "lobby", playerCount: index + 5 },
        `Guest observes ${username} join`,
      );
    }
    await waitForRoom(page, { code: roomCode, status: "lobby", playerCount: 8 }, "Eight-player primary roster poll");
    const crewScrollStart = await page.evaluate(() => {
      window.__paperUITestAfterSwapCount = 0;
      const list = document.querySelector("#room-state .player-list");
      if (!list) return null;
      const target = Math.max(0, list.scrollHeight - list.clientHeight);
      list.scrollTop = target;
      return { target, scrollTop: list.scrollTop, scrollHeight: list.scrollHeight, clientHeight: list.clientHeight };
    });
    assert(Boolean(crewScrollStart), "Eight-player crew scroll", "crew player-list element missing");
    assert(
      crewScrollStart.scrollTop > 0,
      "Eight-player crew scroll",
      `eight rows did not create a scrollable list: ${JSON.stringify(crewScrollStart)}`,
    );
    await page.waitForFunction(() => window.__paperUITestAfterSwapCount >= 2, null, { timeout });
    const crewScroll = await page.locator("#room-state .player-list").evaluate((list) => {
      const listBox = list.getBoundingClientRect();
      const rows = Array.from(list.querySelectorAll(".player-row"));
      return {
        afterPolls: window.__paperUITestAfterSwapCount,
        scrollTop: list.scrollTop,
        rows: rows.slice(-2).map((row) => {
          const box = row.getBoundingClientRect();
          return {
            badge: row.querySelector(".crew-badge")?.textContent?.trim() || "",
            name: row.querySelector(".crew-name")?.textContent?.trim() || "",
            top: box.top,
            bottom: box.bottom,
            reachable: box.top >= listBox.top - 1 && box.bottom <= listBox.bottom + 1,
          };
        }),
      };
    });
    assert(
      crewScroll.afterPolls >= 2 && Math.abs(crewScroll.scrollTop - crewScrollStart.scrollTop) <= 2,
      "Eight-player crew scroll across polling swaps",
      `scroll position changed after polling: start=${crewScrollStart.scrollTop}, result=${JSON.stringify(crewScroll)}`,
    );
    assert(
      crewScroll.rows.length === 2 &&
        crewScroll.rows[0].badge === "P7" &&
        crewScroll.rows[1].badge === "P8" &&
        crewScroll.rows.every((row) => row.reachable),
      "Eight-player roster tail reachability",
      `players P7/P8 are not reachable at scroll end: ${JSON.stringify(crewScroll)}`,
    );
    mark("Eight-player roster scroll stayed at bottom across two polls; P7/P8 rows remain reachable");

    // Start via host UI, verify all peers observe active status, then move through a legal direction if one exists.
    await page.getByRole("button", { name: "Start game", exact: true }).click();
    await waitForRoom(page, { code: roomCode, status: "active", playerCount: 8 }, "Start game");
    for (let index = 0; index < guestContexts.length; index++) {
      await waitForRoom(
        guestPages[index],
        { code: roomCode, status: "active", playerCount: 8 },
        `Guest ${index + 1} observes active status`,
      );
    }
    const directionNames = ["north", "south", "east", "west"];
    const directionRects = await page.locator("#room-state .direction-pad button").evaluateAll((buttons) =>
      buttons.map((button) => {
        const box = button.getBoundingClientRect();
        return {
          name: button.getAttribute("aria-label"),
          x: box.x,
          y: box.y,
          width: box.width,
          height: box.height,
        };
      }),
    );
    assert(directionRects.length === 4, "Movement controls", `expected four movement buttons, got ${directionRects.length}`);
    for (let index = 0; index < directionNames.length; index++) {
      const expectedName = `Move ${directionNames[index]}`;
      assert(directionRects[index].name === expectedName, "Movement controls", `expected accessible name ${expectedName}, got ${directionRects[index].name}`);
      assertNear(directionRects[index].width, 34, "Movement control size", `${expectedName} width`);
      assertNear(directionRects[index].height, 34, "Movement control size", `${expectedName} height`);
      assertNear(directionRects[index].y, directionRects[0].y, "Movement control layout", `${expectedName} row alignment`);
      if (index > 0) {
        assert(directionRects[index].x > directionRects[index - 1].x, "Movement control layout", "N/S/E/W buttons are not laid out horizontally");
      }
    }

    const beforeRoster = await visibleRoster(page);
    const beforeHost = beforeRoster.find((player) => player.id === hostRow);
    assert(Boolean(beforeHost), "Movement state", "host roster row missing after start");
    const roomCoordinateBefore = await page.locator("#room-state").getAttribute("data-current-room");
    const chartedBefore = await page.locator("#room-state .room-cell.charted-room").count();
    const availableDirection = await page.evaluate(() => {
      for (const direction of ["north", "south", "east", "west"]) {
        const button = document.querySelector(`#room-state button[aria-label='Move ${direction}']`);
        if (button && !button.disabled) return direction;
      }
      return "";
    });
    let movement = {
      result: "blocked-start",
      direction: null,
      from: { x: beforeHost.x, y: beforeHost.y },
      to: { x: beforeHost.x, y: beforeHost.y },
      chartedBefore,
      chartedAfter: chartedBefore,
    };
    if (availableDirection) {
      const delta = {
        north: { x: 0, y: 1 },
        south: { x: 0, y: -1 },
        east: { x: 1, y: 0 },
        west: { x: -1, y: 0 },
      }[availableDirection];
      const expected = { x: beforeHost.x + delta.x, y: beforeHost.y + delta.y };
      await page.getByRole("button", { name: `Move ${availableDirection}`, exact: true }).click();
      await page.waitForFunction(
        ({ id, x, y }) => {
          const row = Array.from(document.querySelectorAll("#room-state [data-player-id][data-x][data-y][data-health]"))
            .find((candidate) => candidate.dataset.playerId === id);
          return row && Number(row.dataset.x) === x && Number(row.dataset.y) === y;
        },
        { id: hostRow, ...expected },
        { timeout },
      );
      const afterRoster = await visibleRoster(page);
      const afterHost = afterRoster.find((player) => player.id === hostRow);
      const chartedAfter = await page.locator("#room-state .room-cell.charted-room").count();
      assert(chartedAfter > chartedBefore, "Fog exploration", `charted rooms did not increase (${chartedBefore} to ${chartedAfter})`);
      const readout = await page.locator("#room-state").evaluate((state) => ({
        coordinate: state.dataset.currentRoom,
        heading: state.querySelector("#current-room-heading")?.textContent?.trim() || "",
        eyebrow: state.querySelector(".readout-eyebrow")?.textContent?.trim() || "",
        slots: Array.from(state.querySelectorAll("button[data-quick-slot]"), (slot) => ({
          name: slot.dataset.itemName,
          description: slot.dataset.itemDescription,
          pressed: slot.getAttribute("aria-pressed"),
        })),
        detailName: state.querySelector("[data-detail-name]")?.textContent?.trim() || "",
        detailDescription: state.querySelector("[data-detail-description]")?.textContent?.trim() || "",
      }));
      assert(
        afterHost.x === expected.x && afterHost.y === expected.y,
        "Move host player",
        `expected (${expected.x}, ${expected.y}), got (${afterHost.x}, ${afterHost.y})`,
      );
      assert(
        readout.coordinate !== roomCoordinateBefore &&
          readout.heading === `Sector ${readout.coordinate}` &&
          readout.eyebrow.includes(readout.coordinate),
        "Room coordinate readout",
        `header/readout did not follow new room: ${JSON.stringify(readout)}`,
      );
      assert(
        readout.detailName === (readout.slots[0]?.name || "No room objects") &&
          readout.detailDescription === (readout.slots[0]?.description || "This room has no available objects.") &&
          (!readout.slots.length || readout.slots[0].pressed === "true"),
        "Room item detail reset",
        `item detail does not reflect new room quick slots: ${JSON.stringify(readout)}`,
      );
      movement = {
        result: "moved",
        direction: availableDirection,
        from: { x: beforeHost.x, y: beforeHost.y },
        to: { x: afterHost.x, y: afterHost.y },
        chartedBefore,
        chartedAfter,
        currentRoom: readout.coordinate,
      };
      mark(`Host moved ${availableDirection}; chart, coordinates, item detail, and active peer polls updated`);
    } else {
      const disabledDirections = await page.locator("#room-state .direction-pad button").evaluateAll((buttons) =>
        buttons.filter((button) => !button.disabled).map((button) => button.getAttribute("aria-label")),
      );
      assert(disabledDirections.length === 0, "Blocked starting room", `unexpected enabled moves: ${JSON.stringify(disabledDirections)}`);
      mark("Starting room has no legal moves; all direction controls correctly disabled");
    }

    const responsive = [];
    responsive.push(await checkResponsiveViewport(375, 812));
    responsive.push(await checkResponsiveViewport(1024, 768));
    responsive.push(await checkResponsiveViewport(1280, 900));
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.evaluate(() => window.scrollTo(0, 0));
    mark("375px, 1024px, and 1280px layouts stay within viewport; roster, readout, dock, and controls remain reachable");

    return {
      checks,
      roomCode,
      createdRoomCodes,
      viewer: "Mira Voss",
      movement,
      crewScroll: { start: crewScrollStart, ...crewScroll },
      geometries,
      wallPalette: {
        bodyColor: palette.bodyColor,
        floorColor: palette.floorColor,
        paletteCoverage: palette.paletteCoverage,
        visibleWallCount: palette.walls.length,
        normalWallVisible: Boolean(normalWall),
      },
      responsive,
      finalViewport: { width: 1440, height: 900 },
    };
  } finally {
    await Promise.all(guestContexts.map((context) => context.close().catch(() => {})));
  }
}
