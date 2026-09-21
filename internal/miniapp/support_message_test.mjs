import test from "node:test";
import assert from "node:assert/strict";
import { tokenizeSupportMessage } from "./static/support-message.mjs";

test("standalone Telegram usernames become username tokens", () => {
	assert.deepEqual(tokenizeSupportMessage("Напишите @bruhvpnsupport или (@Support_User)."), [
		{ type: "text", value: "Напишите " },
		{ type: "username", value: "@bruhvpnsupport" },
		{ type: "text", value: " или (" },
		{ type: "username", value: "@Support_User" },
		{ type: "text", value: ")." },
	]);
});

test("email addresses and invalid usernames remain plain text", () => {
	const source = `mail@example.com @abcd @1wrongname @${"a".repeat(33)} слово@bruhvpnsupport`;
	assert.deepEqual(tokenizeSupportMessage(source), [{ type: "text", value: source }]);
});

test("links and usernames can coexist without changing either value", () => {
	assert.deepEqual(tokenizeSupportMessage("https://example.com/u/@inside и @outside_user"), [
		{ type: "link", value: "https://example.com/u/@inside" },
		{ type: "text", value: " и " },
		{ type: "username", value: "@outside_user" },
	]);
});
