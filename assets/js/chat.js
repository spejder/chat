// The parts of a conversation page that need a script.
//
// htmx draws the messages. This file keeps the reader in place, tells them
// when a message arrives below the fold, and makes the write field behave the
// way a messaging application does.
(() => {
	"use strict";

	// A reader this close to the bottom wants to follow the conversation. A
	// reader further up wants to stay where they are.
	const nearBottomPixels = 100;

	// The write field grows to this height and then scrolls.
	const maxFieldPixels = 192;

	const messages = () => document.getElementById("messages");
	const jump = () => document.getElementById("jump");
	const countMessages = () => document.querySelectorAll("#messages [data-message]").length;

	let follow = true;
	let keepTop = 0;
	let counted = 0;
	let waiting = 0;

	const atBottom = (list) =>
		list.scrollHeight - list.scrollTop - list.clientHeight < nearBottomPixels;

	const toNewest = () => {
		const list = messages();

		if (list) {
			list.scrollTop = list.scrollHeight;
		}
	};

	const hideJump = () => {
		waiting = 0;

		const button = jump();

		if (button) {
			button.hidden = true;
		}
	};

	const showJump = () => {
		const button = jump();

		if (!button) {
			return;
		}

		button.textContent = waiting === 1 ? "1 new message" : waiting + " new messages";
		button.hidden = false;
	};

	// grow lets the field follow the text instead of scrolling from the first
	// line. The height is an inline style, which the policy allows.
	const grow = (field) => {
		field.style.height = "auto";
		field.style.height = Math.min(field.scrollHeight, maxFieldPixels) + "px";
	};

	const resetField = (field) => {
		field.style.height = "";
	};

	const wireField = () => {
		const field = document.querySelector("[data-grow]");

		if (!field || field.dataset.wired === "true") {
			return;
		}

		field.dataset.wired = "true";

		field.addEventListener("input", () => grow(field));

		field.addEventListener("keydown", (event) => {
			// Shift and Enter writes a new line, and a keyboard that builds a
			// character from several keys must finish first.
			if (event.key !== "Enter" || event.shiftKey || event.isComposing) {
				return;
			}

			const form = field.closest("form");

			if (!form) {
				return;
			}

			event.preventDefault();
			form.requestSubmit();
		});
	};

	document.addEventListener("DOMContentLoaded", () => {
		counted = countMessages();
		wireField();
		toNewest();
	});

	document.addEventListener("click", (event) => {
		if (event.target && event.target.id === "jump") {
			hideJump();
			toNewest();
		}
	});

	// A swap empties the list for a moment, so the position must be read
	// before it and written after it.
	document.addEventListener("htmx:before:swap", () => {
		const list = messages();

		if (list) {
			follow = atBottom(list);
			keepTop = list.scrollTop;
		}
	});

	document.addEventListener("htmx:after:swap", () => {
		const list = messages();

		if (!list) {
			return;
		}

		wireField();

		const now = countMessages();
		const arrived = Math.max(now - counted, 0);
		counted = now;

		if (follow) {
			hideJump();
			toNewest();

			return;
		}

		list.scrollTop = keepTop;

		if (arrived > 0) {
			waiting += arrived;
			showJump();
		}
	});

	// A message that this reader sends always brings the newest into view.
	document.addEventListener("htmx:before:request", (event) => {
		if (event.target && event.target.id === "write") {
			follow = true;
		}
	});

	document.addEventListener("htmx:after:request", (event) => {
		const form = event.target;

		if (!form || form.id !== "write") {
			return;
		}

		const answer = event.detail && event.detail.xhr ? event.detail.xhr : event.detail.response;

		if (!answer || answer.status === undefined || answer.status < 400) {
			form.reset();

			const field = form.querySelector("[data-grow]");

			if (field) {
				resetField(field);
			}
		}
	});

	document.addEventListener(
		"scroll",
		(event) => {
			if (event.target && event.target.id === "messages" && atBottom(event.target)) {
				hideJump();
			}
		},
		true,
	);

	if (document.readyState !== "loading") {
		counted = countMessages();
		wireField();
		toNewest();
	}
})();
