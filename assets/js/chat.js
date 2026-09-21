// The small parts of a conversation page that need a script.
//
// htmx draws the messages. This file only keeps the newest message in view
// and empties the field after a message goes out. Both live here and not in
// an attribute, because the Content Security Policy allows no script that
// comes from a string.
(() => {
	"use strict";

	const messages = () => document.getElementById("messages");

	// A reader who sits this close to the bottom wants to follow the
	// conversation. A reader who scrolled up wants to stay where they are.
	const nearBottomPixels = 100;

	let follow = true;
	let keepTop = 0;

	const atBottom = (list) =>
		list.scrollHeight - list.scrollTop - list.clientHeight < nearBottomPixels;

	const toNewest = () => {
		const list = messages();

		if (list) {
			list.scrollTop = list.scrollHeight;
		}
	};

	document.addEventListener("DOMContentLoaded", toNewest);

	// The answer of the poll replaces the whole list, so the decision must be
	// made before the swap.
	document.addEventListener("htmx:before:swap", (event) => {
		if (event.target && event.target.id === "messages") {
			follow = atBottom(event.target);
			keepTop = event.target.scrollTop;
		}
	});

	// A swap empties the list for a moment, and the browser forgets where the
	// reader was. Follow the newest message, or put them back.
	document.addEventListener("htmx:after:swap", (event) => {
		if (!event.target || event.target.id !== "messages") {
			return;
		}

		if (follow) {
			toNewest();

			return;
		}

		event.target.scrollTop = keepTop;
	});

	// A message that this reader sends always brings the newest into view.
	document.addEventListener("htmx:before:request", (event) => {
		if (event.target && event.target.id === "write") {
			follow = true;
		}
	});

	// A message that went out leaves an empty field behind.
	document.addEventListener("htmx:after:request", (event) => {
		const form = event.target;

		if (!form || form.id !== "write") {
			return;
		}

		const answer = event.detail && event.detail.xhr ? event.detail.xhr : event.detail.response;

		if (!answer || answer.status === undefined || answer.status < 400) {
			form.reset();
		}
	});

	if (document.readyState !== "loading") {
		toNewest();
	}
})();
