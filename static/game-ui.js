(function () {
	let currentRoomCode = "";
	let currentSector = "";
	let selectedSlot = 0;
	let rosterScrollRoomCode = "";
	let storedCrewsScrollTop = 0;
	let leavingRoom = false;
	const movementFormIDs = new Set(["move-north", "move-south", "move-east", "move-west"]);
	let movementAvailability = null;

	function roomState() {
		return document.getElementById("room-state");
	}

	function availableSlots(state) {
		return Array.from(state.querySelectorAll("button[data-quick-slot]"));
	}

	function movementAvailabilityFrom(state) {
		const availability = new Map();
		for (const form of state.querySelectorAll(".direction-pad form")) {
			if (!movementFormIDs.has(form.id)) {
				continue;
			}
			const button = form.querySelector("button");
			if (button) {
				availability.set(form.id, button.disabled);
			}
		}
		return availability;
	}

	function isRoomStateRoot(target) {
		return target instanceof Element &&
			target.id === "room-state" &&
			target.getAttribute("hx-swap") === "outerMorph";
	}

	function revisionValue(value) {
		if (typeof value !== "string" || !/^\d+$/.test(value)) {
			return null;
		}
		try {
			return BigInt(value);
		} catch {
			return null;
		}
	}

	function incomingRoomState(text) {
		if (typeof text !== "string") {
			return null;
		}
		const fragment = new DOMParser().parseFromString(text, "text/html");
		return fragment.getElementById("room-state");
	}

	function roomPath(code) {
		if (typeof code !== "string" || !/^[A-HJ-NP-Z2-9]{6}$/.test(code)) {
			return null;
		}
		return "/rooms/" + encodeURIComponent(code);
	}

	function navigateToRoom(code) {
		if (leavingRoom) {
			return;
		}
		const path = roomPath(code);
		if (path) {
			window.location.assign(path);
		}
	}

	function isCurrentRoomLeaveSubmission(event) {
		const form = event.target;
		const state = roomState();
		if (!(form instanceof HTMLFormElement) || !state?.contains(form)) {
			return false;
		}

		const path = roomPath(state.dataset.roomCode || "");
		const submitter = event.submitter;
		const method = submitter?.hasAttribute("formmethod") ? submitter.formMethod : form.method;
		const action = submitter?.hasAttribute("formaction") ? submitter.formAction : form.action;
		if (!path || method.toUpperCase() !== "POST") {
			return false;
		}

		try {
			const url = new URL(action, document.baseURI);
			return url.origin === window.location.origin &&
				url.pathname === path + "/leave" &&
				url.search === "" &&
				url.hash === "";
		} catch {
			return false;
		}
	}

	document.addEventListener("submit", function (event) {
		if (!event.defaultPrevented && isCurrentRoomLeaveSubmission(event)) {
			leavingRoom = true;
		}
	});

	document.addEventListener("htmx:before:response", function (event) {
		const ctx = event.detail && event.detail.ctx;
		if (!ctx || ctx.sourceElement?.id !== "room-events") {
			return;
		}

		const status = ctx.response && (ctx.response.status || ctx.response.raw?.status);
		if ([401, 404, 410].includes(status)) {
			navigateToRoom(roomState()?.dataset.roomCode || ctx.sourceElement.dataset.roomCode);
			event.preventDefault();
		}
	});

	document.addEventListener("htmx:sse:error", function (event) {
		const source = document.getElementById("room-events");
		const status = event.detail && event.detail.status;
		if (!source || event.target !== source || ![401, 404, 410].includes(status)) {
			return;
		}
		navigateToRoom(roomState()?.dataset.roomCode || source.dataset.roomCode);
	});

	document.addEventListener("room-redirect", function (event) {
		const source = document.getElementById("room-events");
		if (!source || event.target !== source) {
			return;
		}
		navigateToRoom(roomState()?.dataset.roomCode || source.dataset.roomCode);
	});

	document.addEventListener("htmx:sse:before:message", function (event) {
		const message = event.detail && event.detail.message;
		if (!message || message.event) {
			return;
		}

		const state = roomState();
		const incomingRevision = revisionValue(message.id);
		const currentRevision = revisionValue(state?.dataset.revision);
		if (incomingRevision !== null && currentRevision !== null && incomingRevision <= currentRevision) {
			event.preventDefault();
		}
	});

	document.addEventListener("htmx:before:swap", function (event) {
		const state = roomState();
		const detail = event.detail;
		const ctx = detail && detail.ctx;
		const target = ctx && ctx.target;
		const mainTask = detail?.tasks?.find((task) => task.type === "main");
		if (!state || target !== state || !isRoomStateRoot(target) || mainTask?.target !== state) {
			return;
		}

		const incoming = incomingRoomState(ctx.text);
		const incomingRevision = revisionValue(incoming?.dataset.revision);
		const currentRevision = revisionValue(state.dataset.revision);
		if (incomingRevision !== null && currentRevision !== null && incomingRevision < currentRevision) {
			event.preventDefault();
			return;
		}
		if (incoming) {
			movementAvailability = movementAvailabilityFrom(incoming);
		}

		const roomCode = state.dataset.roomCode || "";
		if (roomCode !== rosterScrollRoomCode) {
			rosterScrollRoomCode = roomCode;
			storedCrewsScrollTop = 0;
		}
		const roster = state.querySelector(".player-list");
		if (roster) {
			storedCrewsScrollTop = roster.scrollTop;
		}
	});

	function restoreRosterScroll(target) {
		if (!isRoomStateRoot(target)) {
			return;
		}
		const state = roomState();
		if (!state) {
			return;
		}

		const roomCode = state.dataset.roomCode || "";
		if (roomCode !== rosterScrollRoomCode) {
			rosterScrollRoomCode = roomCode;
			storedCrewsScrollTop = 0;
		}
		const roster = state.querySelector(".player-list");
		if (roster) {
			roster.scrollTop = storedCrewsScrollTop;
		}
	}

	function renderSelection() {
		const state = roomState();
		if (!state) {
			return;
		}

		const roomCode = state.dataset.roomCode || "";
		const sector = state.dataset.currentRoom || "";
		if (roomCode !== currentRoomCode || sector !== currentSector) {
			currentRoomCode = roomCode;
			currentSector = sector;
			selectedSlot = 0;
		}

		const slots = availableSlots(state);
		if (!slots.some((slot) => Number(slot.dataset.quickSlot) === selectedSlot)) {
			selectedSlot = slots.length > 0 ? Number(slots[0].dataset.quickSlot) : 0;
		}

		let selected = null;
		for (const slot of slots) {
			const isSelected = Number(slot.dataset.quickSlot) === selectedSlot;
			slot.classList.toggle("quick-slot-selected", isSelected);
			slot.setAttribute("aria-pressed", String(isSelected));
			if (isSelected) {
				selected = slot;
			}
		}

		const detail = state.querySelector("[data-item-detail]");
		if (!detail) {
			return;
		}

		const name = detail.querySelector("[data-detail-name]");
		const description = detail.querySelector("[data-detail-description]");
		const glyphWrapper = detail.querySelector(".item-detail-glyph");
		const glyph = detail.querySelector("[data-detail-glyph]");
		const inspect = detail.querySelector("[data-inspect-trigger]");
		if (!selected) {
			if (name) name.textContent = "No room objects";
			if (description) description.textContent = "This room has no available objects.";
			if (glyphWrapper) glyphWrapper.className = "item-detail-glyph item-detail-empty";
			if (glyph) glyph.textContent = "—";
			if (inspect) inspect.disabled = true;
			return;
		}

		if (name) name.textContent = selected.dataset.itemName || "Room object";
		if (description) description.textContent = selected.dataset.itemDescription || "No further details available.";
		if (glyphWrapper) {
			glyphWrapper.className = "item-detail-glyph";
			const sourceGlyph = selected.querySelector(".item-glyph");
			for (const className of sourceGlyph ? sourceGlyph.classList : []) {
				if (className.startsWith("item-glyph-")) {
					glyphWrapper.classList.add(className);
				}
			}
		}
		if (glyph) {
			const sourceGlyph = selected.querySelector(".item-glyph");
			glyph.textContent = sourceGlyph ? sourceGlyph.textContent : "";
		}
		if (inspect) inspect.disabled = false;
	}

	document.addEventListener("click", function (event) {
		const slot = event.target.closest("button[data-quick-slot]");
		if (slot && !slot.disabled) {
			selectedSlot = Number(slot.dataset.quickSlot);
			renderSelection();
			return;
		}

		const inspect = event.target.closest("[data-inspect-trigger]");
		if (!inspect || inspect.disabled) {
			return;
		}

		const state = roomState();
		const dialog = document.querySelector("[data-inspect-dialog]");
		if (!dialog) {
			return;
		}
		const name = state.querySelector("[data-detail-name]");
		const description = state.querySelector("[data-detail-description]");
		const dialogName = dialog.querySelector("[data-dialog-name]");
		const dialogDescription = dialog.querySelector("[data-dialog-description]");
		if (dialogName) dialogName.textContent = name ? name.textContent : "Room object";
		if (dialogDescription) dialogDescription.textContent = description ? description.textContent : "No further details available.";
		if (typeof dialog.showModal === "function") {
			dialog.showModal();
		}
	});

	document.addEventListener("keydown", function (event) {
		if (!/^[1-6]$/.test(event.key) || event.altKey || event.ctrlKey || event.metaKey) {
			return;
		}
		if (event.target instanceof Element && event.target.closest("input, textarea, select, [contenteditable='true']")) {
			return;
		}
		const state = roomState();
		if (!state || document.querySelector("dialog[open]")) {
			return;
		}
		const slotNumber = Number(event.key) - 1;
		if (availableSlots(state).some((slot) => Number(slot.dataset.quickSlot) === slotNumber)) {
			selectedSlot = slotNumber;
			renderSelection();
		}
	});

	document.addEventListener("htmx:after:swap", function (event) {
		const ctx = event.detail && event.detail.ctx;
		const target = ctx && ctx.target;
		if (!isRoomStateRoot(target)) {
			return;
		}
		renderSelection();
		restoreRosterScroll(target);
	});
	document.addEventListener("htmx:finally:request", function (event) {
		const ctx = event.detail && event.detail.ctx;
		const state = roomState();
		const sourceID = ctx?.sourceElement?.id;
		if (!state || !isRoomStateRoot(state) || ctx?.target !== state || !movementFormIDs.has(sourceID) ||
			!state.querySelector(`.direction-pad form#${sourceID}`)) {
			return;
		}

		// HTMX emits finally:request before reenabling request-disabled controls.
		queueMicrotask(function () {
			const current = roomState();
			if (!current || !isRoomStateRoot(current) || !movementAvailability) {
				return;
			}

			// Reassert only directions blocked by latest server snapshot; queued requests may disable allowed ones.
			for (const [id, disabled] of movementAvailability) {
				if (!disabled) {
					continue;
				}
				const button = current.querySelector(`.direction-pad form#${id} button`);
				if (button) {
					button.disabled = true;
				}
			}
		});
	});
	function initializeRoomUI() {
		const state = roomState();
		if (state && movementAvailability === null) {
			movementAvailability = movementAvailabilityFrom(state);
		}
		renderSelection();
	}
	if (document.readyState === "loading") {
		document.addEventListener("DOMContentLoaded", initializeRoomUI, { once: true });
	} else {
		initializeRoomUI();
	}
})();
