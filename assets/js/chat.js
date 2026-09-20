// The small parts of a conversation page that need a script.
//
// htmx draws the messages. This file only keeps the newest message in view
// and empties the field after a message goes out. Both live here and not in
// an attribute, because the Content Security Policy allows no script that
// comes from a string.
(() => {
	"use strict";

	const messages = () => document.getElementById("messages");

	const toNewest = () => {
		const list = messages();

		if (list) {
			list.scrollTop = list.scrollHeight;
		}
	};

	document.addEventListener("DOMContentLoaded", toNewest);

	document.addEventListener("htmx:after:swap", (event) => {
		if (event.target && event.target.id === "messages") {
			toNewest();
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
