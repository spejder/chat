// The parts that belong to every page of a signed in person.
//
// The tab says how many messages wait, and the polls slow down while nobody
// looks at the page.
(() => {
	"use strict";

	// The title that the server wrote, without a count in front of it.
	const plainTitle = document.title;

	// How often the polls ask while the tab is hidden.
	const sleeping = "every 30s";

	const pollingElements = () =>
		[document.getElementById("conversation-list"), document.getElementById("message-list")].filter(
			(element) => element !== null,
		);

	// awakeTrigger remembers what the server asked for, so the page can go
	// back to it. The slow value never counts as the remembered one, or the
	// page would stay slow after a swap while the tab was hidden.
	const awakeTrigger = (element) => {
		const current = element.getAttribute("data-hx-trigger") || "";

		if (!element.dataset.awake && current && current !== sleeping) {
			element.dataset.awake = current;
		}

		return element.dataset.awake || current;
	};

	const setPace = () => {
		const hidden = document.visibilityState === "hidden";

		for (const element of pollingElements()) {
			const awake = awakeTrigger(element);
			const wanted = hidden ? sleeping : awake;

			if (element.getAttribute("data-hx-trigger") === wanted) {
				continue;
			}

			element.setAttribute("data-hx-trigger", wanted);

			if (window.htmx) {
				window.htmx.process(element);
			}
		}
	};

	// askNow brings the page up to date the moment somebody looks at it
	// again, instead of waiting for the next turn of the poll.
	const askNow = () => {
		if (!window.htmx) {
			return;
		}

		for (const element of pollingElements()) {
			const address = element.getAttribute("data-hx-get");

			if (address) {
				window.htmx.ajax("GET", address, { target: "#" + element.id, swap: "outerHTML" });
			}
		}
	};

	const writeTitle = () => {
		const list = document.getElementById("conversation-list");
		const unread = list ? Number(list.dataset.unread || "0") : 0;

		document.title = unread > 0 ? "(" + unread + ") " + plainTitle : plainTitle;
	};

	const update = () => {
		writeTitle();
		setPace();
	};

	document.addEventListener("DOMContentLoaded", update);
	document.addEventListener("htmx:after:swap", update);

	document.addEventListener("visibilitychange", () => {
		setPace();

		if (document.visibilityState === "visible") {
			askNow();
		}
	});

	if (document.readyState !== "loading") {
		update();
	}
})();
