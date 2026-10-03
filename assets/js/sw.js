// The service worker of Chat. It exists for the push notifications alone, so
// it has no fetch handler and keeps no cache.
//
// The server sends a small JSON message: title, body, url and tag. The
// worker shows it, unless a visible window already shows that conversation.
"use strict";

// A new version takes over at once. The worker holds no state, so nothing is
// lost when an old one stops.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));

const windows = () => self.clients.matchAll({ type: "window", includeUncontrolled: true });

const pathOf = (address) => new URL(address, self.location.origin).pathname;

self.addEventListener("push", (event) => {
	let message = null;

	try {
		message = event.data ? event.data.json() : null;
	} catch {
		message = null;
	}

	const title = (message && message.title) || "Chat";
	const url = (message && message.url) || "/conversations";

	event.waitUntil(
		(async () => {
			// The reader already looks at the conversation, and the poll of
			// the page brings the message in. A browser accepts a push
			// without a notification while a window of the site is visible.
			const open = await windows();

			if (open.some((client) => client.visibilityState === "visible" && pathOf(client.url) === url)) {
				return;
			}

			await self.registration.showNotification(title, {
				body: (message && message.body) || "A new message arrived.",
				// A newer message of the same conversation replaces the older
				// notification, and renotify makes it sound again.
				tag: (message && message.tag) || "chat",
				renotify: true,
				icon: "/assets/img/icon-192.png",
				data: { url },
			});
		})(),
	);
});

self.addEventListener("notificationclick", (event) => {
	event.notification.close();

	const url = new URL((event.notification.data && event.notification.data.url) || "/", self.location.origin).href;

	event.waitUntil(
		(async () => {
			const open = await windows();

			// A window that shows the conversation comes to the front.
			const same = open.find((client) => client.url === url);
			if (same) {
				return same.focus();
			}

			// Another window of the site goes there. A window that the worker
			// does not control refuses to navigate, so a new one opens then.
			for (const client of open) {
				try {
					const moved = await client.navigate(url);

					if (moved) {
						return moved.focus();
					}
				} catch {
					// Try the next window, and open a new one at the end.
				}
			}

			return self.clients.openWindow(url);
		})(),
	);
});
