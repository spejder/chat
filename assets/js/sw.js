// The service worker of Chat. It exists for the push notifications alone, so
// it has no fetch handler and keeps no cache.
//
// The server sends a small JSON message: title, body, url, tag, the unread
// count and a mark for the night. The worker shows it, unless a visible
// window already shows that conversation.
//
// A conversation sounds once per unread stretch, like the SMS reminders:
// the first message alerts, and the next ones update the same notification
// without a sound and count up. It sounds again only after realertMs
// without an alert. Opening the notification, or the conversation in a page
// (chat.js), ends the stretch.
"use strict";

// realertMs is the silence after which a waiting conversation may sound
// again. It equals the wait of the SMS reminders.
const realertMs = 15 * 60 * 1000;

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
			// The count of unread messages goes on the icon of the
			// installed app, also when no notification shows. Only some
			// browsers have the Badging API, and a refusal changes nothing
			// else.
			if (message && Number.isInteger(message.unread) && "setAppBadge" in navigator) {
				const badge = message.unread > 0 ? navigator.setAppBadge(message.unread) : navigator.clearAppBadge();

				await badge.catch(() => {});
			}

			// The reader already looks at the conversation, and the poll of
			// the page brings the message in. A browser accepts a push
			// without a notification while a window of the site is visible.
			const open = await windows();

			if (open.some((client) => client.visibilityState === "visible" && pathOf(client.url) === url)) {
				return;
			}

			const tag = (message && message.tag) || "chat";
			const line = (message && message.body) || "A new message arrived.";

			// A notification of this conversation that still shows means
			// the stretch goes on.
			const [showing] = await self.registration.getNotifications({ tag });
			const before = (showing && showing.data) || {};
			const count = (Number(before.count) || 0) + 1;
			const now = Date.now();
			const due = !showing || now - (Number(before.alertedAt) || 0) >= realertMs;

			// In the night the notification arrives without sound or
			// vibration. The specification refuses silent together with
			// renotify, so a quiet one never renotifies.
			const silent = Boolean(message && message.quiet);
			const alert = due && !silent;

			await self.registration.showNotification(title, {
				body: count > 1 ? `${count} new messages\n${line}` : line,
				// A newer message of the same conversation replaces the older
				// notification. renotify makes the replacement sound.
				tag,
				renotify: alert,
				silent,
				icon: "/assets/img/icon-192.png",
				data: { url, count, alertedAt: alert || !showing ? now : before.alertedAt },
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
