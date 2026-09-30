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

  async function installSwapTracking() {
    await page.evaluate(() => {
      if (window.__paperUITestSwapTrackingInstalled) return;
      window.__paperUITestSwapTrackingInstalled = true;
      window.__paperUITestAfterSwapCount = 0;
      document.addEventListener("htmx:after:swap", () => {
        window.__paperUITestAfterSwapCount += 1;
      });
    });
  }

  async function afterSwapCount() {
    return page.evaluate(() => window.__paperUITestAfterSwapCount || 0);
  }

  async function pushUnchangedViewerUpdates(code, viewerContext, viewerPage, username, step) {
    const startingRevision = Number(await page.locator("#room-state").getAttribute("data-revision"));
    const browser = page.context().browser();
    assert(Boolean(browser), step, "page has no browser instance");
    const temporaryContext = await browser.newContext();

    async function reconnect(context, label) {
      const response = await context.request.post(`${baseURL}/rooms/${code}/join`, {
        form: { username },
        maxRedirects: 0,
      });
      assert(
        response.status() >= 200 && response.status() < 400,
        step,
        `${label} reconnect returned HTTP ${response.status()}`,
      );
    }

    try {
      let expectedRevision = startingRevision;
      let expectedSwaps = await afterSwapCount();
      await reconnect(temporaryContext, "Temporary takeover");
      await page.waitForFunction(
        ({ revision, swaps }) =>
          Number(document.querySelector("#room-state")?.dataset.revision) > revision &&
          (window.__paperUITestAfterSwapCount || 0) > swaps,
        { revision: expectedRevision, swaps: expectedSwaps },
        { timeout },
      );

      expectedRevision = Number(await page.locator("#room-state").getAttribute("data-revision"));
      expectedSwaps = await afterSwapCount();
      await reconnect(viewerContext, "Original viewer restore");
      await page.waitForFunction(
        ({ revision, swaps }) =>
          Number(document.querySelector("#room-state")?.dataset.revision) > revision &&
          (window.__paperUITestAfterSwapCount || 0) > swaps,
        { revision: expectedRevision, swaps: expectedSwaps },
        { timeout },
      );
    } finally {
      await temporaryContext.close().catch(() => {});
    }

    await viewerPage.goto(`${baseURL}/rooms/${code}`, { waitUntil: "domcontentloaded", timeout });
    await waitForRoom(viewerPage, { code }, `${step}: restore original guest page`);
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
      const cell = document.querySelector(".room-cell, .empty-cell");
      const chartViewport = document.querySelector(".chart-viewport");
      const grid = document.querySelector(".map-grid");
      const style = (selector) => {
        const node = document.querySelector(selector);
        return node ? getComputedStyle(node) : null;
      };
      const cellRect = cell?.getBoundingClientRect();
      const viewportStyle = chartViewport ? getComputedStyle(chartViewport) : null;
      const stateStyle = style("#room-state");
      const bodyStyle = getComputedStyle(document.body);
      return {
        viewport: { width: window.innerWidth, height: window.innerHeight },
        document: {
          width: document.documentElement.scrollWidth,
          height: document.documentElement.scrollHeight,
          zoom: getComputedStyle(document.documentElement).zoom,
          bodyTransform: bodyStyle.transform,
          roomStateTransform: stateStyle?.transform || "none",
        },
        missionHeader: rect(".mission-header"),
        tacticalWorkspace: rect(".tactical-workspace"),
        mapModule: rect(".map-module"),
        intelSidebar: rect(".intel-sidebar"),
        chartViewport: rect(".chart-viewport"),
        sectorChart: rect(".sector-chart"),
        mapChartMatrix: rect(".map-chart-matrix"),
        mapGrid: rect(".map-grid"),
        roomReadout: rect(".room-readout"),
        directiveCard: rect(".directive-card"),
        crewPanel: rect(".crew-panel"),
        vitalsPanel: rect(".vitals-panel"),
        dockOuter: rect(".dock-outer"),
        dockPanel: rect(".dock-panel"),
        moveButton: rect(".direction-pad button"),
        quickSlot: rect(".quickbelt-slots .quick-slot"),
        type: {
          mapHeading: parseFloat(style("#map-heading")?.fontSize || "0"),
          roomDescription: parseFloat(style(".room-description")?.fontSize || "0"),
          moveButton: parseFloat(style(".direction-pad button")?.fontSize || "0"),
        },
        chartViewportOverflowX: viewportStyle?.overflowX || "visible",
        chartViewportScrollWidth: chartViewport?.scrollWidth || 0,
        chartViewportClientWidth: chartViewport?.clientWidth || 0,
        chartViewportContainsGrid: Boolean(chartViewport && grid && chartViewport.contains(grid)),
        mapDimensions: {
          width: Number(document.querySelector(".sector-chart")?.dataset.mapWidth),
          length: Number(document.querySelector(".sector-chart")?.dataset.mapLength),
          cellSize: cell ? parseFloat(getComputedStyle(cell).width) : 0,
          cellHeight: cellRect?.height || 0,
          quickSlotHeight: rect(".quickbelt-slots .quick-slot")?.height || 0,
        },
      };
    });
  }

  function assertContained(inner, outer, step, label, tolerance = 1) {
    assert(
      inner &&
        outer &&
        inner.x >= outer.x - tolerance &&
        inner.y >= outer.y - tolerance &&
        inner.x + inner.width <= outer.x + outer.width + tolerance &&
        inner.y + inner.height <= outer.y + outer.height + tolerance,
      step,
      `${label} is not contained: inner=${JSON.stringify(inner)}, outer=${JSON.stringify(outer)}`,
    );
  }

  function assertNoOverlap(first, second, step, label) {
    assert(
      first &&
        second &&
        (first.x + first.width <= second.x + 1 ||
          second.x + second.width <= first.x + 1 ||
          first.y + first.height <= second.y + 1 ||
          second.y + second.height <= first.y + 1),
      step,
      `${label} overlap: first=${JSON.stringify(first)}, second=${JSON.stringify(second)}`,
    );
  }

  async function waitForLayoutFrame() {
    await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => resolve())));
  }

  async function checkLargeViewport(width, height, baseline, stabilityViewer = null) {
    await page.setViewportSize({ width, height });
    await waitForLayoutFrame();
    const geometry = await measureDesktopGeometry();
    const step = `${width}×${height} desktop layout`;
    const dockBottom = geometry.dockPanel.y + geometry.dockPanel.height;
    const dockBottomGap = height - dockBottom;
    const moduleBottom = geometry.mapModule.y + geometry.mapModule.height;
    const sidebarBottom = geometry.intelSidebar.y + geometry.intelSidebar.height;

    assert(geometry.document.width <= width + 1, step, `document width ${geometry.document.width} exceeds viewport`);
    if (height >= 900) {
      assert(geometry.document.height <= height + 2, step, `document height ${geometry.document.height} exceeds viewport`);
    }
    assertNear(geometry.missionHeader.y, 0, step, "header top", 1);
    assertNear(geometry.mapModule.y, geometry.intelSidebar.y, step, "module/sidebar top alignment", 1);
    assertNear(moduleBottom, sidebarBottom, step, "module/sidebar bottom alignment", 1);
    assertNear(geometry.dockPanel.x, geometry.mapModule.x, step, "dock/workspace left alignment", 1);
    assertNear(
      geometry.dockPanel.x + geometry.dockPanel.width,
      geometry.intelSidebar.x + geometry.intelSidebar.width,
      step,
      "dock/workspace right alignment",
      1,
    );
    assert(
      dockBottomGap > 0 && dockBottomGap <= 64,
      step,
      `dock bottom gap must stay positive and within 64px, got ${dockBottomGap}px`,
    );
    assert(moduleBottom < geometry.dockPanel.y, step, "workspace panels overlap dock");
    assert(
      geometry.chartViewportContainsGrid,
      step,
      "map grid is not contained by chart viewport DOM",
    );
    assertContained(geometry.sectorChart, geometry.chartViewport, step, "sector chart/chart viewport");
    assertContained(geometry.mapChartMatrix, geometry.sectorChart, step, "map matrix/sector chart");
    assertContained(geometry.mapGrid, geometry.chartViewport, step, "map grid/chart viewport");
    assertContained(geometry.roomReadout, geometry.mapModule, step, "room readout/map module");
    assertContained(geometry.moveButton, geometry.roomReadout, step, "movement control/readout");
    assertContained(geometry.quickSlot, geometry.dockPanel, step, "quick slot/dock panel");
    assertNoOverlap(geometry.mapGrid, geometry.roomReadout, step, "map grid and readout");
    assertNear(
      geometry.mapGrid.width,
      geometry.mapDimensions.width * geometry.mapDimensions.cellSize,
      step,
      "grid width from map width × cell size",
      1,
    );
    assertNear(
      geometry.mapGrid.height,
      geometry.mapDimensions.length * geometry.mapDimensions.cellSize,
      step,
      "grid height from map length × cell size",
      1,
    );
    assert(
      Number.isInteger(geometry.mapDimensions.width) &&
        geometry.mapDimensions.width > 0 &&
        Number.isInteger(geometry.mapDimensions.length) &&
        geometry.mapDimensions.length > 0 &&
        geometry.mapDimensions.cellSize > 0,
      step,
      `invalid map chart dimensions: ${JSON.stringify(geometry.mapDimensions)}`,
    );
    assertNear(geometry.mapDimensions.cellSize, geometry.mapDimensions.cellHeight, step, "map cells remain square", 1);
    assert(
      (geometry.document.zoom === "1" || geometry.document.zoom === "normal") &&
        geometry.document.bodyTransform === "none" &&
        geometry.document.roomStateTransform === "none",
      step,
      `page scaling must come from responsive UI sizing: ${JSON.stringify(geometry.document)}`,
    );
    if (width === 1440 && height === 900) {
      assertNear(geometry.missionHeader.height, 68, step, "baseline header height", 3);
      assertNear(geometry.moveButton.width, 34, step, "baseline movement-control width");
      assertNear(geometry.moveButton.height, 34, step, "baseline movement-control height");
      assertNear(geometry.mapDimensions.quickSlotHeight, 84, step, "baseline quick-slot height");
    }

    if (width >= 1920) {
      assert(
        geometry.mapDimensions.cellSize > baseline.mapDimensions.cellSize * 1.08,
        step,
        `map cells did not grow beyond baseline: ${geometry.mapDimensions.cellSize}px vs ${baseline.mapDimensions.cellSize}px`,
      );
      assert(
        geometry.type.mapHeading > baseline.type.mapHeading * 1.08 &&
          geometry.moveButton.width > baseline.moveButton.width * 1.08,
        step,
        `headings or movement controls did not grow beyond baseline: ${JSON.stringify({
          heading: geometry.type.mapHeading,
          baselineHeading: baseline.type.mapHeading,
          moveButton: geometry.moveButton.width,
          baselineMoveButton: baseline.moveButton.width,
        })}`,
      );
    }
    if (width === 1920 && height === 1080) {
      assert(
        geometry.mapDimensions.cellSize >= baseline.mapDimensions.cellSize * 1.1,
        step,
        `map cells should scale with 1920×1080 viewport: ${geometry.mapDimensions.cellSize}px vs ${baseline.mapDimensions.cellSize}px`,
      );
    }
    if (width === 2560 && height === 1440) {
      assert(
        geometry.mapDimensions.cellSize > baseline.mapDimensions.cellSize * 1.3 &&
          geometry.type.mapHeading > baseline.type.mapHeading * 1.4 &&
          geometry.moveButton.width > baseline.moveButton.width * 1.4 &&
          geometry.mapDimensions.quickSlotHeight > baseline.mapDimensions.quickSlotHeight * 1.1,
        step,
        `large-screen chart, headings, controls, and slots did not scale: ${JSON.stringify({
          cellSize: geometry.mapDimensions.cellSize,
          headingSize: geometry.type.mapHeading,
          moveButton: geometry.moveButton.width,
          slotHeight: geometry.mapDimensions.quickSlotHeight,
        })}`,
      );

      assert(Boolean(stabilityViewer), step, "real viewer reconnects were not configured for SSE stability check");
      const beforeSwapCount = await afterSwapCount();
      const beforePushes = await measureDesktopGeometry();
      const beforePushState = await page.evaluate(() => ({
        scrollX: window.scrollX,
        scrollY: window.scrollY,
        rosterScrollTop: document.querySelector(".player-list")?.scrollTop || 0,
        selectedSlot: document.querySelector(".quick-slot[aria-pressed='true']")?.dataset.quickSlot || "",
        currentRoom: document.querySelector("#room-state")?.dataset.currentRoom || "",
        detailName: document.querySelector("[data-detail-name]")?.textContent?.trim() || "",
      }));
      await pushUnchangedViewerUpdates(
        stabilityViewer.code,
        stabilityViewer.context,
        stabilityViewer.page,
        stabilityViewer.username,
        `${step} SSE layout stability`,
      );
      await page.waitForFunction(
        (count) => (window.__paperUITestAfterSwapCount || 0) >= count + 2,
        beforeSwapCount,
        { timeout },
      );
      await waitForLayoutFrame();
      const afterPushes = await measureDesktopGeometry();
      const afterPushState = await page.evaluate(() => ({
        scrollX: window.scrollX,
        scrollY: window.scrollY,
        rosterScrollTop: document.querySelector(".player-list")?.scrollTop || 0,
        selectedSlot: document.querySelector(".quick-slot[aria-pressed='true']")?.dataset.quickSlot || "",
        currentRoom: document.querySelector("#room-state")?.dataset.currentRoom || "",
        detailName: document.querySelector("[data-detail-name]")?.textContent?.trim() || "",
      }));
      const stableBoxes = [
        "missionHeader",
        "mapModule",
        "intelSidebar",
        "chartViewport",
        "mapGrid",
        "roomReadout",
        "dockPanel",
        "moveButton",
        "quickSlot",
      ];
      for (const name of stableBoxes) {
        for (const dimension of ["x", "y", "width", "height"]) {
          assertNear(
            afterPushes[name][dimension],
            beforePushes[name][dimension],
            `${width}×${height} SSE stability`,
            `${name}.${dimension}`,
            1,
          );
        }
      }
      assertNear(afterPushes.type.mapHeading, beforePushes.type.mapHeading, step, "heading font after two SSE pushes", 1);
      assertNear(afterPushes.type.roomDescription, beforePushes.type.roomDescription, step, "readout font after two SSE pushes", 1);
      assertNear(afterPushes.type.moveButton, beforePushes.type.moveButton, step, "movement-control font after two SSE pushes", 1);
      assertNear(afterPushes.mapDimensions.cellSize, beforePushes.mapDimensions.cellSize, step, "map cell after two SSE pushes", 1);
      assertNear(
        afterPushes.mapDimensions.quickSlotHeight,
        beforePushes.mapDimensions.quickSlotHeight,
        step,
        "quick-slot height after two SSE pushes",
        1,
      );
      assert(
        afterPushes.mapDimensions.width === beforePushes.mapDimensions.width &&
          afterPushes.mapDimensions.length === beforePushes.mapDimensions.length &&
          afterPushes.document.width === beforePushes.document.width &&
          afterPushes.document.height === beforePushes.document.height &&
          afterPushState.selectedSlot === beforePushState.selectedSlot &&
          afterPushState.currentRoom === beforePushState.currentRoom &&
          afterPushState.detailName === beforePushState.detailName &&
          Math.abs(afterPushState.rosterScrollTop - beforePushState.rosterScrollTop) <= 1 &&
          afterPushState.scrollX === beforePushState.scrollX &&
          afterPushState.scrollY === beforePushState.scrollY,
        step,
        `SSE pushes changed chart/layout or reset UI state: before=${JSON.stringify({ geometry: beforePushes, state: beforePushState })}, after=${JSON.stringify({ geometry: afterPushes, state: afterPushState })}`,
      );
      geometry.sseStability = {
        afterSwaps: (await afterSwapCount()) - beforeSwapCount,
        stable: true,
      };
    }

    return {
      viewport: { width, height },
      document: geometry.document,
      header: geometry.missionHeader,
      workspace: geometry.tacticalWorkspace,
      mapModule: geometry.mapModule,
      sidebar: geometry.intelSidebar,
      chartViewport: geometry.chartViewport,
      mapGrid: geometry.mapGrid,
      readout: geometry.roomReadout,
      dock: geometry.dockPanel,
      dockBottomGap,
      cellSize: geometry.mapDimensions.cellSize,
      headingFontSize: geometry.type.mapHeading,
      readoutFontSize: geometry.type.roomDescription,
      movementButtonSize: { width: geometry.moveButton.width, height: geometry.moveButton.height },
      quickSlotHeight: geometry.mapDimensions.quickSlotHeight,
      sseStability: geometry.sseStability || null,
    };
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
    await waitForLayoutFrame();
    const horizontal = await page.evaluate(() => ({
      innerWidth: window.innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      chart: (() => {
        const viewport = document.querySelector(".chart-viewport");
        const chart = document.querySelector(".sector-chart");
        const grid = document.querySelector(".map-grid");
        const viewportRect = viewport?.getBoundingClientRect();
        const chartRect = chart?.getBoundingClientRect();
        const gridRect = grid?.getBoundingClientRect();
        return {
          containsGrid: Boolean(viewport && grid && viewport.contains(grid)),
          viewport: viewportRect && { left: viewportRect.left, right: viewportRect.right },
          sectorChart: chartRect && { left: chartRect.left, right: chartRect.right },
          grid: gridRect && { left: gridRect.left, right: gridRect.right },
          overflowX: viewport ? getComputedStyle(viewport).overflowX : "visible",
          scrollWidth: viewport?.scrollWidth || 0,
          clientWidth: viewport?.clientWidth || 0,
          sectorOverflowX: chart ? getComputedStyle(chart).overflowX : "visible",
          sectorScrollWidth: chart?.scrollWidth || 0,
          sectorClientWidth: chart?.clientWidth || 0,
        };
      })(),
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
    assert(horizontal.chart.containsGrid, `${width}px responsive chart`, "map grid is not inside its chart viewport");
    assert(
      horizontal.chart.viewport.left >= -1 && horizontal.chart.viewport.right <= width + 1,
      `${width}px responsive chart`,
      `chart viewport exceeds page width: ${JSON.stringify(horizontal.chart)}`,
    );
    if (
      horizontal.chart.grid.left < horizontal.chart.viewport.left - 1 ||
      horizontal.chart.grid.right > horizontal.chart.viewport.right + 1
    ) {
      assert(
        (horizontal.chart.scrollWidth > horizontal.chart.clientWidth &&
          ["auto", "scroll"].includes(horizontal.chart.overflowX)) ||
          (horizontal.chart.sectorScrollWidth > horizontal.chart.sectorClientWidth &&
            ["auto", "scroll"].includes(horizontal.chart.sectorOverflowX)),
        `${width}px responsive chart scrolling`,
        `map grid outside chart viewport has no internal horizontal scroll: ${JSON.stringify(horizontal.chart)}`,
      );
    }

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

  async function checkShortDesktopViewport() {
    const width = 1440;
    const height = 720;
    await page.setViewportSize({ width, height });
    await waitForLayoutFrame();
    const geometry = await measureDesktopGeometry();
    assert(
      geometry.document.width <= width + 1,
      "1440×720 short desktop width",
      `document width ${geometry.document.width} exceeds viewport`,
    );
    for (const [selector, label] of [
      [".direction-pad button", "movement control"],
      [".quickbelt-slots .quick-slot", "quick slot"],
    ]) {
      const target = page.locator(selector).first();
      await target.scrollIntoViewIfNeeded({ timeout });
      const box = await target.boundingBox();
      assert(
        box && box.x >= -1 && box.x + box.width <= width + 1 && box.y >= -1 && box.y + box.height <= height + 1,
        "1440×720 short desktop reachability",
        `${label} not reachable after vertical scrolling: ${JSON.stringify(box)}`,
      );
    }
    return {
      viewport: { width, height },
      document: geometry.document,
      dock: geometry.dockPanel,
      movementReachable: true,
      quickSlotReachable: true,
    };
  }

  try {
    await page.setViewportSize({ width: 1440, height: 900 });

    // Create disposable rooms through the real home form until generated starting room has an object and a visible wall.
    let roomCode = "";
    for (let attempt = 0; attempt < 16; attempt++) {
      roomCode = await createRoom(page, "Mira Voss");
      const hasInspectableObject = (await page.locator("#room-state button[data-quick-slot]").count()) > 0;
      const hasVisibleWall = (await page.locator("#room-state .map-wall").count()) > 0;
      if (hasInspectableObject && hasVisibleWall) break;
      assert(
        attempt < 15,
        "Find starting-room inspection and wall fixtures",
        "16 generated rooms lacked either a quick-slot object or visible wall metadata",
      );
      await submitAndWaitForNavigation(
        page,
        page.getByRole("button", { name: "Leave game", exact: true }),
        `Leave incomplete candidate room ${roomCode}`,
      );
      roomCode = "";
    }
    assert(Boolean(roomCode), "Find starting-room inspection and wall fixtures", "no candidate room retained");
    await installSwapTracking();
    mark(`Created owned room ${roomCode} through home form with an inspectable starting-room object`);

    // Join three independent browser contexts, preserving this primary page for final review.
    for (const username of ["Eli Rane", "Sana Quill", "Orrin Vale"]) {
      const guest = await newIndependentPage();
      const joinButton = await fillHomeJoin(guest.page, roomCode, username);
      await submitAndWaitForNavigation(guest.page, joinButton, `Join ${roomCode} as ${username}`);
      await waitForRoom(guest.page, { code: roomCode, status: "lobby" }, `Guest observes ${username} join`);
    }
    await waitForRoom(page, { code: roomCode, status: "lobby", playerCount: 4 }, "Primary roster SSE update");
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
    mark("Three independent guests joined; host row, P2 avatar, and username rendered after SSE updates");

    const geometries = await measureDesktopGeometry();
    const desktopBaseline = await checkLargeViewport(1440, 900, geometries);
    mark("Baseline desktop layout fills viewport; map/sidebar align, square chart fits viewport, and dock stays bottom-anchored");
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
    mark("Desktop structure stays aligned; chart scales to map dimensions and crew columns align");

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

    // Keep the native inspector open across two genuine guest-join revisions.
    const quickSlots = page.locator("#room-state button[data-quick-slot]");
    const firstSlot = quickSlots.first();
    const itemName = await firstSlot.getAttribute("data-item-name");
    const itemDescription = await firstSlot.getAttribute("data-item-description");
    assert(Boolean(itemName && itemDescription), "Inspect room object", "first quick slot lacks real item metadata");
    const dialog = page.locator("dialog[data-inspect-dialog]");
    await page.locator("#room-state [data-inspect-trigger]").click();
    await dialog.waitFor({ state: "visible", timeout });
    const dialogSwapBaseline = await afterSwapCount();

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
      await waitForRoom(page, { code: roomCode, status: "lobby", playerCount: index + 5 }, `Host observes ${username} over SSE`);
      if (index === 1) {
        await page.waitForFunction(
          (count) => (window.__paperUITestAfterSwapCount || 0) >= count + 2,
          dialogSwapBaseline,
          { timeout },
        );
        assert(await dialog.evaluate((node) => node.open), "Inspect dialog SSE regression", "dialog closed during real room updates");
        assert(
          (await dialog.locator("[data-dialog-name]").textContent())?.trim() === itemName &&
            (await dialog.locator("[data-dialog-description]").textContent())?.trim() === itemDescription,
          "Inspect dialog SSE regression",
          "dialog item name or description changed during SSE updates",
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
        mark("Inspect dialog retained real item details across two guest-join SSE pushes; Close and keyboard slot selection worked");
      }
    }
    await waitForRoom(page, { code: roomCode, status: "lobby", playerCount: 8 }, "Eight-player primary roster SSE update");
    const rosterSwapBaseline = await afterSwapCount();
    const crewScrollStart = await page.evaluate(() => {
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
    await pushUnchangedViewerUpdates(
      roomCode,
      guestContexts[guestContexts.length - 1],
      guestPages[guestPages.length - 1],
      finalUsers[finalUsers.length - 1],
      "Eight-player roster SSE scroll stability",
    );
    await page.waitForFunction(
      (count) => (window.__paperUITestAfterSwapCount || 0) >= count + 2,
      rosterSwapBaseline,
      { timeout },
    );
    const crewScroll = await page.locator("#room-state .player-list").evaluate((list) => {
      const listBox = list.getBoundingClientRect();
      const rows = Array.from(list.querySelectorAll(".player-row"));
      return {
        afterSwaps: window.__paperUITestAfterSwapCount,
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
      crewScroll.afterSwaps >= rosterSwapBaseline + 2 && Math.abs(crewScroll.scrollTop - crewScrollStart.scrollTop) <= 2,
      "Eight-player crew scroll across SSE pushes",
      `scroll position changed after SSE updates: start=${crewScrollStart.scrollTop}, result=${JSON.stringify(crewScroll)}`,
    );
    assert(
      crewScroll.rows.length === 2 &&
        crewScroll.rows[0].badge === "P7" &&
        crewScroll.rows[1].badge === "P8" &&
        crewScroll.rows.every((row) => row.reachable),
      "Eight-player roster tail reachability",
      `players P7/P8 are not reachable at scroll end: ${JSON.stringify(crewScroll)}`,
    );
    mark("Eight-player roster scroll stayed at bottom across two real SSE pushes; P7/P8 rows remain reachable");

    const largeViewports = [desktopBaseline];
    const stabilityViewer = {
      code: roomCode,
      context: guestContexts[guestContexts.length - 1],
      page: guestPages[guestPages.length - 1],
      username: finalUsers[finalUsers.length - 1],
    };
    for (const [width, height] of [
      [1920, 1080],
      [2560, 1440],
      [3440, 1440],
      [1920, 1800],
      [1440, 1200],
    ]) {
      largeViewports.push(await checkLargeViewport(width, height, geometries, stabilityViewer));
    }
    const tallViewport = largeViewports.find((entry) => entry.viewport.width === 1440 && entry.viewport.height === 1200);
    assert(
      tallViewport.dockBottomGap >= 10 && tallViewport.dockBottomGap <= 32,
      "1440×1200 dock anchoring",
      `dock should remain near viewport bottom padding, gap=${tallViewport.dockBottomGap}px`,
    );
    assert(
      tallViewport.workspace.height > geometries.tacticalWorkspace.height + 200,
      "1440×1200 workspace fill",
      `workspace did not expand with viewport height: baseline=${geometries.tacticalWorkspace.height}px, tall=${tallViewport.workspace.height}px`,
    );
    mark("1920×1080, 2560×1440, ultrawide, and tall layouts fill viewport and scale chart/HUD; two SSE pushes at 2560px do not shift layout");

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
      mark(`Host moved ${availableDirection}; chart, coordinates, item detail, and active peer SSE updates rendered`);
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
    const shortDesktop = await checkShortDesktopViewport();
    await page.setViewportSize({ width: 1440, height: 900 });
    await waitForLayoutFrame();
    await page.evaluate(() => window.scrollTo(0, 0));
    mark("375px, 1024px, 1280px, and short desktop layouts stay horizontally contained; panels and controls remain reachable");

    return {
      checks,
      roomCode,
      createdRoomCodes,
      viewer: "Mira Voss",
      movement,
      crewScroll: { start: crewScrollStart, ...crewScroll },
      geometries,
      desktopBaseline,
      largeViewports,
      shortDesktop,
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
