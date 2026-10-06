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

	// Room above the line for the unread messages when the page opens there,
	// so the date line that sticks to the top does not cover it.
	const roomAbovePixels = 56;

	// The page tells the others at most this often that the reader writes.
	const typingEveryMs = 3000;

	// A name leaves the typing line when no word arrives for this long. It is
	// two rounds of typingEveryMs, so one lost event does not make it blink.
	const typingForMs = 7000;

	// The write field grows to this height and then scrolls.
	const maxFieldPixels = 192;

	const messages = () => document.getElementById("messages");
	const jump = () => document.getElementById("jump");
	const offline = () => document.getElementById("offline");
	const announcer = () => document.getElementById("announce");
	const typingLine = () => document.getElementById("typing");
	const errorLine = () => document.querySelector("#write [data-error]");
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

	// openAtStart opens the conversation where the reader left off: at the
	// line for the unread messages when the newest ones would push it out of
	// view, and at the newest message otherwise. A phone does the same.
	const openAtStart = () => {
		const list = messages();
		const line = document.querySelector("#messages [data-unread-line]");

		if (!list || !line) {
			toNewest();

			return;
		}

		const lineTop = line.offsetTop - roomAbovePixels;
		const bottomTop = list.scrollHeight - list.clientHeight;

		if (lineTop >= bottomTop) {
			toNewest();

			return;
		}

		list.scrollTop = Math.max(lineTop, 0);
		follow = false;
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

	// The people who write right now, by full name, each with the timer that
	// takes them off the line again.
	const typists = new Map();

	const firstName = (name) => name.split(/\s+/)[0] || name;

	const showTypists = () => {
		const line = typingLine();

		if (!line) {
			return;
		}

		const names = [...typists.keys()].map(firstName);
		const list = messages();
		const stay = list ? atBottom(list) : false;

		if (names.length === 0) {
			line.hidden = true;
			line.textContent = "";

			return;
		}

		if (names.length === 1) {
			line.textContent = names[0] + " is writing…";
		} else if (names.length === 2) {
			line.textContent = names[0] + " and " + names[1] + " are writing…";
		} else {
			line.textContent = "Several people are writing…";
		}

		line.hidden = false;

		// A reader at the bottom sees the line without scrolling.
		if (stay) {
			toNewest();
		}
	};

	const stopTyping = (name) => {
		const timer = typists.get(name);

		if (timer === undefined) {
			return;
		}

		window.clearTimeout(timer);
		typists.delete(name);
		showTypists();
	};

	document.addEventListener("chat:typing", (event) => {
		const name = event.detail && event.detail.name;

		if (!name) {
			return;
		}

		window.clearTimeout(typists.get(name));
		typists.set(name, window.setTimeout(() => stopTyping(name), typingForMs));
		showTypists();
	});

	// tellTyping lets the others know that the reader writes, at most once
	// every few seconds. The answer does not matter: a lost word only makes
	// the line on the other side go a little sooner.
	let lastTyping = 0;

	const tellTyping = (field) => {
		const form = field.closest("form");
		const now = Date.now();

		if (!form || !form.dataset.typing || !field.value.trim() || now - lastTyping < typingEveryMs) {
			return;
		}

		lastTyping = now;

		fetch(form.dataset.typing, { method: "POST" }).catch(() => {});
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

	// The draft of an unsent message lives in this browser only, under one
	// key per conversation.
	const draftKey = () => {
		const form = document.getElementById("write");

		return form && form.dataset.conversation ? "chat:draft:" + form.dataset.conversation : "";
	};

	const readDraft = () => {
		const key = draftKey();

		if (!key) {
			return "";
		}

		try {
			return window.localStorage.getItem(key) || "";
		} catch {
			// A private window refuses the store, and a draft is not worth a
			// broken page.
			return "";
		}
	};

	const writeDraft = (text) => {
		const key = draftKey();

		if (!key) {
			return;
		}

		try {
			if (text) {
				window.localStorage.setItem(key, text);
			} else {
				window.localStorage.removeItem(key);
			}
		} catch {
			// See readDraft.
		}
	};

	let draftTimer = 0;

	const wireField = () => {
		const field = document.querySelector("[data-grow]");

		if (!field || field.dataset.wired === "true") {
			return;
		}

		field.dataset.wired = "true";

		const draft = readDraft();

		if (draft && !field.value) {
			field.value = draft;
			grow(field);
		}

		field.addEventListener("input", () => {
			grow(field);
			tellTyping(field);

			window.clearTimeout(draftTimer);
			draftTimer = window.setTimeout(() => writeDraft(field.value), 300);
		});

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
		openAtStart();
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

			// A person whose message arrived has stopped writing.
			for (const message of [...document.querySelectorAll("#messages [data-message]")].slice(-arrived)) {
				stopTyping(message.dataset.author || "");
			}
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

	// app.js watches the stream of changes. While its line is down, the
	// page hears about nothing, so it says so. The browser connects again by
	// itself, and app.js catches up when it does.
	document.addEventListener("chat:offline", () => showOffline(true));
	document.addEventListener("chat:online", () => showOffline(false));

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

		window.clearTimeout(draftTimer);
		writeDraft("");
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

	// A failed request also means the line is down. The stream decides when
	// it is up again.
	document.addEventListener("htmx:error", () => {
		showOffline(true);
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
		openAtStart();
	}
})();
