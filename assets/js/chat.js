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

	// The line counts as down when no answer has arrived for this long.
	const silenceMs = 10000;

	const messages = () => document.getElementById("messages");
	const jump = () => document.getElementById("jump");
	const offline = () => document.getElementById("offline");
	const announcer = () => document.getElementById("announce");
	const list = () => document.getElementById("message-list");
	const errorLine = () => document.querySelector("#write [data-error]");
	const countMessages = () => document.querySelectorAll("#messages [data-message]").length;

	let follow = true;
	let keepTop = 0;
	let counted = 0;
	let waiting = 0;
	let lastAnswer = Date.now();

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

	const showOffline = (down) => {
		const notice = offline();

		if (notice) {
			notice.hidden = !down;
		}
	};

	// announce says one sentence to a screen reader. A live region on the
	// list itself would read the whole conversation after every swap.
	const announce = () => {
		const line = announcer();
		const newest = [...document.querySelectorAll("#messages [data-message]")].pop();

		if (!line || !newest || newest.hasAttribute("data-mine")) {
			return;
		}

		const body = newest.querySelector("[data-body]");

		line.textContent = (newest.dataset.author || "") + ": " + (body ? body.textContent : "");
	};

	// askAgain takes over when htmx gives up after a failed request.
	const askAgain = () => {
		const element = list();

		if (!element || !window.htmx) {
			return;
		}

		const address = element.getAttribute("data-hx-get");

		if (!address) {
			return;
		}

		window.htmx.ajax("GET", address, { target: "#message-list", swap: "outerHTML" });
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

		if (arrived > 0) {
			announce();
		}

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

	// Any answer at all means the line is up again.
	document.addEventListener("htmx:after:request", () => {
		lastAnswer = Date.now();
		showOffline(false);
	});

	// The server says with a header that the message went out, and the field
	// empties. A refused message never swaps, so the text stays.
	document.addEventListener("chat:sent", () => {
		const form = document.getElementById("write");

		if (!form) {
			return;
		}

		const line = errorLine();

		if (line) {
			line.textContent = "";
		}

		form.reset();

		const field = form.querySelector("[data-grow]");

		if (field) {
			resetField(field);
		}
	});

	// The server says with a header why it refused a message.
	document.addEventListener("chat:error", (event) => {
		const line = errorLine();

		if (!line) {
			return;
		}

		const detail = event.detail;
		const message = detail && typeof detail === "object" ? detail.value : detail;

		line.textContent = message || "The message did not go out.";
	});

	// htmx stops asking after a failed request, so the page says so and takes
	// over the asking itself.
	document.addEventListener("htmx:error", () => {
		showOffline(true);
	});

	setInterval(() => {
		if (!list()) {
			return;
		}

		if (Date.now() - lastAnswer < silenceMs) {
			return;
		}

		showOffline(true);
		askAgain();
	}, 5000);

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
