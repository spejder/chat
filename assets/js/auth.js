// Passkeys and the code from the SMS.
//
// htmx does the page work. This file only does the two things that need a
// browser API: the passkey ceremonies and, where the browser has it, reading
// the code out of the message.
//
// The options come from data attributes, so the page needs no inline script
// and the Content Security Policy can stay strict.
//
// The conversion between base64url and bytes is written out by hand, because
// PublicKeyCredential.parseCreationOptionsFromJSON is newer than the Baseline
// target of this project.
(() => {
	"use strict";

	const bytesFromBase64Url = (value) => {
		const base64 = value.replace(/-/g, "+").replace(/_/g, "/");
		const binary = atob(base64);
		const bytes = new Uint8Array(binary.length);

		for (let i = 0; i < binary.length; i += 1) {
			bytes[i] = binary.charCodeAt(i);
		}

		return bytes;
	};

	const base64UrlFromBytes = (buffer) => {
		const bytes = new Uint8Array(buffer);
		let binary = "";

		for (const byte of bytes) {
			binary += String.fromCharCode(byte);
		}

		return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
	};

	const withBytes = (list) =>
		(list || []).map((item) => ({ ...item, id: bytesFromBase64Url(item.id) }));

	const requestOptions = (options) => ({
		...options,
		challenge: bytesFromBase64Url(options.challenge),
		allowCredentials: withBytes(options.allowCredentials),
	});

	const creationOptions = (options) => ({
		...options,
		challenge: bytesFromBase64Url(options.challenge),
		user: { ...options.user, id: bytesFromBase64Url(options.user.id) },
		excludeCredentials: withBytes(options.excludeCredentials),
	});

	const assertionToJSON = (credential) => ({
		id: credential.id,
		rawId: base64UrlFromBytes(credential.rawId),
		type: credential.type,
		response: {
			clientDataJSON: base64UrlFromBytes(credential.response.clientDataJSON),
			authenticatorData: base64UrlFromBytes(credential.response.authenticatorData),
			signature: base64UrlFromBytes(credential.response.signature),
			userHandle: credential.response.userHandle
				? base64UrlFromBytes(credential.response.userHandle)
				: null,
		},
	});

	const attestationToJSON = (credential) => ({
		id: credential.id,
		rawId: base64UrlFromBytes(credential.rawId),
		type: credential.type,
		response: {
			clientDataJSON: base64UrlFromBytes(credential.response.clientDataJSON),
			attestationObject: base64UrlFromBytes(credential.response.attestationObject),
		},
	});

	const showError = (panel, message) => {
		const field = panel.querySelector("[data-error]");

		if (field) {
			field.textContent = message;
		}
	};

	const send = async (url, body) => {
		const response = await fetch(url, {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(body),
		});

		const answer = await response.json().catch(() => ({}));

		if (!response.ok) {
			throw new Error(answer.error || "The sign in did not work. Try again.");
		}

		return answer;
	};

	const runPasskey = async (panel) => {
		const options = JSON.parse(panel.dataset.options).publicKey;
		const challenge = panel.dataset.challenge;
		const creating = panel.hasAttribute("data-passkey-create");

		showError(panel, "");

		const credential = creating
			? await navigator.credentials.create({ publicKey: creationOptions(options) })
			: await navigator.credentials.get({ publicKey: requestOptions(options) });

		const url = creating
			? `/login/passkey/register?challenge=${encodeURIComponent(challenge)}`
			: `/login/passkey?challenge=${encodeURIComponent(challenge)}`;

		const answer = await send(url, creating ? attestationToJSON(credential) : assertionToJSON(credential));

		window.location.assign(answer.redirect || "/");
	};

	const wirePasskey = (panel) => {
		if (panel.dataset.wired === "true") {
			return;
		}

		panel.dataset.wired = "true";

		const start = async () => {
			try {
				await runPasskey(panel);
			} catch (error) {
				showError(panel, error.message || String(error));
			}
		};

		const button = panel.querySelector("[data-passkey-start]");

		if (button) {
			button.addEventListener("click", start);
		}

		// A sign in with a passkey starts by itself. The creation of a new
		// passkey waits for a click, because the person may want to skip it.
		if (panel.hasAttribute("data-passkey-login")) {
			start();
		}
	};

	// wireCode asks the browser to read the code out of the message. Only
	// some browsers can do this, so the field also carries
	// autocomplete="one-time-code", which works on its own.
	const wireCode = (field) => {
		if (field.dataset.wired === "true" || !("OTPCredential" in window)) {
			return;
		}

		field.dataset.wired = "true";

		const form = field.closest("form");
		const controller = new AbortController();

		if (form) {
			form.addEventListener("submit", () => controller.abort(), { once: true });
		}

		navigator.credentials
			.get({ otp: { transport: ["sms"] }, signal: controller.signal })
			.then((otp) => {
				if (!otp || !otp.code) {
					return;
				}

				field.value = otp.code;

				if (form) {
					form.requestSubmit();
				}
			})
			.catch(() => {
				// The person types the code instead.
			});
	};

	const wire = () => {
		for (const panel of document.querySelectorAll("[data-passkey-login], [data-passkey-create]")) {
			wirePasskey(panel);
		}

		for (const field of document.querySelectorAll("[data-otp]")) {
			wireCode(field);
		}
	};

	document.addEventListener("DOMContentLoaded", wire);
	document.addEventListener("htmx:after:swap", wire);

	if (document.readyState !== "loading") {
		wire();
	}
})();
