// The switch for push notifications in the person menu.
//
// The switch subscribes this browser with the push service of the browser and
// hands the subscription to the server, which posts to it when a message
// arrives. The service worker /sw.js shows the notification.
//
// The card at the bottom of the sidebar suggests the same thing once, and
// on an iPhone or a phone with Chrome it first suggests the home screen,
// because an iPhone delivers a push only to the app there.
//
// The conversion between base64url and bytes is written out by hand, like in
// auth.js, because there is no function for it in the Baseline target.
(() => {
	"use strict";

	const box = document.querySelector("[data-push]");
	const toggle = document.querySelector("[data-push-switch]");
	const note = document.querySelector("[data-push-note]");

	// The server renders the switch only when it can send a push.
	if (!box || !toggle || !note) {
		return;
	}

	const supported = "serviceWorker" in navigator && "PushManager" in window && "Notification" in window;

	const key = toggle.dataset.pushKey;
	const person = toggle.dataset.pushUser;

	// The browser remembers who turned the notifications on. Somebody else
	// who signs in on the same browser starts with them off.
	const ownerKey = "chat:push:owner";

	const remember = (value) => {
		try {
			if (value) {
				localStorage.setItem(ownerKey, value);
			} else {
				localStorage.removeItem(ownerKey);
			}
		} catch {
			// A private window refuses the store. The switch then asks again
			// after the next sign in, which is the safe side.
		}
	};

	const owner = () => {
		try {
			return localStorage.getItem(ownerKey);
		} catch {
			return null;
		}
	};

	const bytesFromBase64Url = (value) => {
		const binary = atob(value.replace(/-/g, "+").replace(/_/g, "/"));
		const bytes = new Uint8Array(binary.length);

		for (let i = 0; i < binary.length; i += 1) {
			bytes[i] = binary.charCodeAt(i);
		}

		return bytes;
	};

	const base64UrlFromBytes = (buffer) => {
		let binary = "";

		for (const byte of new Uint8Array(buffer)) {
			binary += String.fromCharCode(byte);
		}

		return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
	};

	const tell = async (method, body) => {
		const answer = await fetch("/push/subscriptions", {
			method,
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(body),
		});

		if (!answer.ok) {
			throw new Error(`the server answered ${answer.status}`);
		}
	};

	const current = async () => {
		const registration = await navigator.serviceWorker.getRegistration("/");

		return registration ? registration.pushManager.getSubscription() : null;
	};

	// A subscription made with another key is useless, because the push
	// service checks the signature of the server against that key. That
	// happens when the database lost its key pair.
	const madeWithThisKey = (subscription) => {
		const own = subscription.options && subscription.options.applicationServerKey;

		return !own || base64UrlFromBytes(own) === key;
	};

	const subscribe = async () => {
		await navigator.serviceWorker.register("/sw.js", { scope: "/" });

		const registration = await navigator.serviceWorker.ready;
		let subscription = await registration.pushManager.getSubscription();

		if (subscription && !madeWithThisKey(subscription)) {
			await subscription.unsubscribe();
			subscription = null;
		}

		if (!subscription) {
			subscription = await registration.pushManager.subscribe({
				userVisibleOnly: true,
				applicationServerKey: bytesFromBase64Url(key),
			});
		}

		await tell("POST", subscription.toJSON());
		remember(person);
	};

	const unsubscribe = async () => {
		const subscription = await current();

		remember(null);

		if (!subscription) {
			return;
		}

		// The server forgets the browser first, while the endpoint is still
		// known. The browser then drops the subscription itself.
		await tell("DELETE", { endpoint: subscription.endpoint }).catch((error) => {
			console.warn("notifications: the server could not forget this browser", error);
		});

		await subscription.unsubscribe();
	};

	const show = (on) => {
		const denied = Notification.permission === "denied";

		toggle.checked = on && !denied;
		toggle.disabled = denied;
		note.hidden = !denied;
	};

	// The browser shows its question only in answer to a click, so this
	// call comes before anything else in a click handler.
	const turnOn = async () => {
		const permission = await Notification.requestPermission();

		if (permission !== "granted") {
			throw new Error(`the permission is ${permission}`);
		}

		toggle.disabled = true;
		await subscribe();
	};

	toggle.addEventListener("change", async () => {
		const wanted = toggle.checked;

		try {
			if (wanted) {
				await turnOn();
			} else {
				toggle.disabled = true;
				await unsubscribe();
			}

			show(wanted);
		} catch (error) {
			console.warn("notifications: the switch failed", error);
			show(!wanted);
		}

		// The switch is an answer too, so the card leaves.
		suggest();
	});

	// The page opens with the switch in the state of this browser. A browser
	// that is subscribed sends its subscription again, because the server
	// drops it with an old session. A browser that somebody else subscribed
	// stops, so the new person starts with the notifications off.
	const start = async () => {
		const subscription = Notification.permission === "granted" ? await current() : null;

		if (!subscription) {
			show(false);

			return;
		}

		if (owner() !== person) {
			await subscription.unsubscribe();
			remember(null);
			show(false);

			return;
		}

		await subscribe();
		show(true);
	};

	// The card in the sidebar suggests the next step towards notifications
	// on this device, and never to somebody who said no. A permission that
	// is denied is a no. A permission that is granted without a
	// subscription means that the person turned the switch off, which is a
	// no too. Only a browser that never asked hears about notifications.
	const card = document.querySelector("[data-nudge]");

	// "Not now" rests the card for 30 days. After the second time it
	// never comes back on this device.
	const laterKey = "chat:nudge";
	const laterMs = 30 * 24 * 60 * 60 * 1000;
	const laterMax = 2;

	// Chrome and Edge fire this event only while the site is not installed.
	// The event is the only way to show their install question from a
	// button. Safari and Firefox never fire it.
	let installEvent = null;

	const later = () => {
		try {
			const value = JSON.parse(localStorage.getItem(laterKey));

			return value && typeof value === "object" ? value : {};
		} catch {
			return {};
		}
	};

	const rest = () => {
		const count = (Number(later().count) || 0) + 1;

		try {
			localStorage.setItem(laterKey, JSON.stringify({ count, until: Date.now() + laterMs }));
		} catch {
			// A private window refuses the store. The card then comes back
			// with the next page, which is a small price.
		}
	};

	const resting = () => {
		const { count = 0, until = 0 } = later();

		return count >= laterMax || until > Date.now();
	};

	const installed = () => matchMedia("(display-mode: standalone)").matches || navigator.standalone === true;

	const phone = () => matchMedia("(pointer: coarse)").matches;

	// Safari on an iPhone or an iPad is the one browser that has
	// navigator.standalone. It is false in a tab, and such a tab can never
	// receive a push, only the app on the home screen can.
	const appleTab = () => "standalone" in navigator && !navigator.standalone && phone();

	const step = () => {
		if (resting()) {
			return null;
		}

		if (!installed() && appleTab()) {
			return "home";
		}

		if (!installed() && installEvent && phone()) {
			return "install";
		}

		if (supported && Notification.permission === "default") {
			return "notify";
		}

		return null;
	};

	function suggest() {
		if (!card) {
			return;
		}

		const chosen = step();

		for (const part of card.querySelectorAll("[data-nudge-step]")) {
			part.hidden = part.dataset.nudgeStep !== chosen;
		}

		card.querySelector("[data-nudge-install]").hidden = chosen !== "install";
		card.querySelector("[data-nudge-notify]").hidden = chosen !== "notify";
		card.hidden = chosen === null;
	}

	window.addEventListener("beforeinstallprompt", (event) => {
		// The browser keeps its own question for the button in the card,
		// and the install item in its menu stays.
		event.preventDefault();
		installEvent = event;
		suggest();
	});

	window.addEventListener("appinstalled", () => {
		installEvent = null;
		suggest();
	});

	if (card) {
		card.querySelector("[data-nudge-later]").addEventListener("click", () => {
			rest();
			suggest();
		});

		card.querySelector("[data-nudge-install]").addEventListener("click", async () => {
			const event = installEvent;

			if (!event) {
				return;
			}

			installEvent = null;
			event.prompt();

			// A no to the question of the browser counts as "Not now".
			const { outcome } = await event.userChoice;

			if (outcome !== "accepted") {
				rest();
			}

			suggest();
		});

		card.querySelector("[data-nudge-notify]").addEventListener("click", async () => {
			try {
				await turnOn();
				show(true);
			} catch (error) {
				console.warn("notifications: the card failed", error);
				show(false);

				// A question that the person closed without an answer leaves
				// the permission at default. That counts as "Not now".
				if (Notification.permission === "default") {
					rest();
				}
			}

			suggest();
		});
	}

	if (!supported) {
		suggest();

		return;
	}

	box.hidden = false;

	start()
		.catch((error) => {
			console.warn("notifications: could not read the state of this browser", error);
			show(false);
		})
		.finally(suggest);
})();
