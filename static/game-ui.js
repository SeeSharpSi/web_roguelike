(function () {
	let currentRoomCode = "";
	let currentSector = "";
	let selectedSlot = 0;
	let rosterScrollRoomCode = "";
	let storedCrewsScrollTop = 0;

	function roomState() {
		return document.getElementById("room-state");
	}

	function availableSlots(state) {
		return Array.from(state.querySelectorAll("button[data-quick-slot]"));
	}

	function isRoomStateRoot(target) {
		return target instanceof Element &&
			target.id === "room-state" &&
			target.getAttribute("hx-swap") === "outerHTML";
	}

	document.addEventListener("htmx:beforeSwap", function (event) {
		const state = roomState();
		const target = event.detail && event.detail.target;
		if (!state || target !== state || !isRoomStateRoot(target)) {
			return;
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

	document.addEventListener("htmx:afterSwap", function (event) {
		renderSelection();
		restoreRosterScroll(event.detail && event.detail.target);
	});
	document.addEventListener("htmx:afterSettle", renderSelection);
	if (document.readyState === "loading") {
		document.addEventListener("DOMContentLoaded", renderSelection, { once: true });
	} else {
		renderSelection();
	}
})();
