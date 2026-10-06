// The parts that belong to every page of a signed in person.
//
// The server tells the page through a stream of events when a conversation
// changes, and the page then asks for the lists it shows. The tab title and
// the icon of the app say how many messages wait.
(() => {
	"use strict";

	// The title that the server wrote, without a count in front of it.
	const plainTitle = document.title;

	const sidebarList = () => document.getElementById("conversation-list");
	const messageList = () => document.getElementById("message-list");

	// The conversation that this page shows, if any.
	const openConversation = () => {
		const form = document.getElementById("write");

		return form ? form.dataset.conversation : "";
	};

	// refresh asks for a list with the version it holds. htmx sends the
	// request that the element describes, and the server answers 204 when
	// nothing changed, so nothing moves on the page.
	const refresh = (element) => {
		if (element) {
			element.dispatchEvent(new CustomEvent("chat:refresh"));
		}
	};

	// resync asks for a list without the version, so the answer always
	// replaces it. After a broken line the page cannot know what it missed,
	// and a failed request may have stopped the slow poll of htmx, which a
	// fresh element starts again.
	const resync = (element) => {
		if (!element || !window.htmx) {
			return;
		}

		const address = element.getAttribute("data-hx-get");

		if (!address) {
			return;
		}

		const fresh = new URL(address, window.location.href);
		fresh.searchParams.delete("v");

		window.htmx.ajax("GET", fresh.pathname + fresh.search, { target: "#" + element.id, swap: "outerHTML" });
	};

	const askNow = () => {
		refresh(sidebarList());
		refresh(messageList());
	};

	// listen opens the stream of changes. Every event names a conversation:
	// the sidebar always asks, and the message list asks when it shows that
	// conversation. The browser connects again by itself after a break, and
	// chat.js shows the notice about the line while it is down.
	const listen = () => {
		if (!("EventSource" in window)) {
			return;
		}

		const source = new EventSource("/events");
		let broken = false;

		source.addEventListener("open", () => {
			document.dispatchEvent(new CustomEvent("chat:online"));

			if (broken) {
				broken = false;
				resync(sidebarList());
				resync(messageList());
			}
		});

		source.addEventListener("error", () => {
			broken = true;
			document.dispatchEvent(new CustomEvent("chat:offline"));
		});

		// Somebody writes in the open conversation right now. chat.js shows
		// it for a few seconds.
		source.addEventListener("typing", (event) => {
			let detail = null;

			try {
				detail = JSON.parse(event.data);
			} catch {
				return;
			}

			if (detail && detail.conversation === openConversation()) {
				document.dispatchEvent(new CustomEvent("chat:typing", { detail }));
			}
		});

		source.addEventListener("changed", (event) => {
			refresh(sidebarList());

			if (event.data && event.data === openConversation()) {
				refresh(messageList());
			}
		});
	};

	// writeBadge puts the count on the icon of the installed app. Only some
	// browsers have the Badging API, and none of them needs it to work, so
	// the call sits behind a test and a refusal only warns.
	const writeBadge = (unread) => {
		if (!("setAppBadge" in navigator)) {
			return;
		}

		const done = unread > 0 ? navigator.setAppBadge(unread) : navigator.clearAppBadge();

		done.catch((error) => console.warn("the badge of the app was refused", error));
	};

	const writeTitle = () => {
		const list = document.getElementById("conversation-list");

		if (!list) {
			return;
		}

		const unread = Number(list.dataset.unread || "0");

		document.title = unread > 0 ? "(" + unread + ") " + plainTitle : plainTitle;
		writeBadge(unread);
	};

	// chat.js keeps an unsent message under this key and the identifier of
	// its conversation.
	const draftPrefix = "chat:draft:";

	const storedDraft = (conversation) => {
		try {
			return (window.localStorage.getItem(draftPrefix + conversation) || "").trim();
		} catch {
			// A private window refuses the store, and then there is no draft.
			return "";
		}
	};

	// showDrafts puts an unsent message in place of the preview, so a half
	// written answer is not forgotten. The open conversation shows its draft
	// in the write field already, so its line stays as it is. The server
	// never sees a draft, which is why the browser writes the line.
	const showDrafts = () => {
		for (const line of document.querySelectorAll("#conversation-list [data-conversation]")) {
			const preview = line.querySelector("[data-preview]");

			if (!preview || line.getAttribute("aria-current") === "page") {
				continue;
			}

			const draft = storedDraft(line.dataset.conversation);

			if (!draft) {
				continue;
			}

			const label = document.createElement("span");
			label.className = "font-medium text-foreground";
			label.textContent = "Draft: ";

			preview.replaceChildren(label, draft.replace(/\s+/g, " "));
		}
	};

	const update = () => {
		writeTitle();
		showDrafts();
	};

	// The list page holds nothing to read in its room, and on a phone the
	// list hides behind the trigger. The page asks for the sheet to open,
	// and the sidebar script of the registry does nothing on a wide screen.
	const openOnPhone = () => {
		const sidebar = window.tui && window.tui.sidebar;

		if (sidebar && document.querySelector("[data-open-on-phone]") && sidebar.isMobile()) {
			sidebar.setOpenMobile(true);
		}
	};

	// A swipe opens and closes the sidebar sheet on a phone. The trigger in
	// the top bar stays, so the swipe is an extra way and never the only one.
	//
	// The swipe to open starts in the left third of the screen, not at the
	// edge: Android and Safari go back on a swipe in from the very edge, and
	// that gesture must keep working.
	const swipe = {
		minPixels: 60,
		maxMs: 500,
		// The share of the screen width, from the left, where a swipe to open
		// may start.
		openZone: 1 / 3,
	};

	let touchStart = null;

	// Another dialog or the person menu is open, and a swipe would fight it.
	const busy = () =>
		document.querySelector(
			'[data-tui-dialog-content][data-open]:not([id$="-mobile"]), [data-tui-dropdownmenu-content][data-open]',
		) !== null;

	const watchSwipes = () => {
		document.addEventListener(
			"touchstart",
			(event) => {
				const touch = event.touches.length === 1 ? event.touches[0] : null;
				const inField = event.target instanceof Element && event.target.closest("textarea, input");

				touchStart = touch && !inField ? { x: touch.clientX, y: touch.clientY, at: Date.now() } : null;
			},
			{ passive: true },
		);

		document.addEventListener(
			"touchend",
			(event) => {
				const begin = touchStart;
				const sidebar = window.tui && window.tui.sidebar;

				touchStart = null;

				if (!begin || !sidebar || !sidebar.isMobile() || busy() || event.changedTouches.length !== 1) {
					return;
				}

				const touch = event.changedTouches[0];
				const across = touch.clientX - begin.x;
				const down = Math.abs(touch.clientY - begin.y);

				const isSwipe =
					Date.now() - begin.at <= swipe.maxMs &&
					Math.abs(across) >= swipe.minPixels &&
					down <= Math.abs(across) / 2;

				if (!isSwipe) {
					return;
				}

				const open = sidebar.openMobile();

				if (across > 0 && !open && begin.x <= window.innerWidth * swipe.openZone) {
					sidebar.setOpenMobile(true);
				} else if (across < 0 && open) {
					sidebar.setOpenMobile(false);
				}
			},
			{ passive: true },
		);
	};

	// The switch for the SMS reminders stores itself on every change. The
	// person menu moves its content into <body>, where htmx no longer
	// listens, so a listener on the document does the work. A checkbox sends
	// its value only while it is on, and a failed request puts the switch
	// back.
	const watchSmsSwitch = () => {
		document.addEventListener("change", async (event) => {
			const toggle = event.target instanceof Element ? event.target.closest("[data-sms-switch]") : null;

			if (!toggle) {
				return;
			}

			const body = new URLSearchParams();

			if (toggle.checked) {
				body.set(toggle.name, toggle.value);
			}

			try {
				const answer = await fetch("/sms-reminders", { method: "PUT", body });

				if (!answer.ok) {
					throw new Error(`the server answered ${answer.status}`);
				}
			} catch (error) {
				console.warn("sms reminders: the switch failed", error);
				toggle.checked = !toggle.checked;
			}
		});
	};

	const start = () => {
		update();
		openOnPhone();
		listen();
		watchSwipes();
		watchSmsSwitch();
	};

	document.addEventListener("htmx:after:swap", update);

	// A phone may have frozen the page in the background. It asks once when
	// somebody looks at it again.
	document.addEventListener("visibilitychange", () => {
		if (document.visibilityState === "visible") {
			askNow();
		}
	});

	if (document.readyState === "loading") {
		document.addEventListener("DOMContentLoaded", start);
	} else {
		start();
	}
})();
