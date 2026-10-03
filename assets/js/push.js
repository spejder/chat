// The switch for push notifications in the person menu.
//
// The switch subscribes this browser with the push service of the browser and
// hands the subscription to the server, which posts to it when a message
// arrives. The service worker /sw.js shows the notification.
//
// The conversion between base64url and bytes is written out by hand, like in
// auth.js, because there is no function for it in the Baseline target.
(() => {
	"use strict";

	const box = document.querySelector("[data-push]");
	const toggle = document.querySelector("[data-push-switch]");
	const note = document.querySelector("[data-push-note]");

	const supported = "serviceWorker" in navigator && "PushManager" in window && "Notification" in window;

	// A browser without push never sees the switch.
	if (!supported || !box || !toggle || !note) {
		return;
	}

	box.hidden = false;

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

	toggle.addEventListener("change", async () => {
		const wanted = toggle.checked;

		try {
			if (wanted) {
				// The browser shows its question only in answer to a click,
				// so this call comes before anything else.
				const permission = await Notification.requestPermission();

				if (permission !== "granted") {
					throw new Error(`the permission is ${permission}`);
				}

				toggle.disabled = true;
				await subscribe();
			} else {
				toggle.disabled = true;
				await unsubscribe();
			}

			show(wanted);
		} catch (error) {
			console.warn("notifications: the switch failed", error);
			show(!wanted);
		}
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

	start().catch((error) => {
		console.warn("notifications: could not read the state of this browser", error);
		show(false);
	});
})();
