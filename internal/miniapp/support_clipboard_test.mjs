import assert from "node:assert/strict";
import test from "node:test";
import { supportMediaFromClipboard } from "./static/support-clipboard.mjs";

test("a pasted screenshot becomes a support attachment", () => {
	const screenshot = { name: "image.png", type: "image/png", size: 1234 };
	assert.equal(supportMediaFromClipboard({ files: [screenshot], items: [] }), screenshot);
});

test("a pasted video from clipboard items becomes a support attachment", () => {
	const video = { name: "recording.mp4", type: "video/mp4", size: 1234 };
	assert.equal(supportMediaFromClipboard({ files: [], items: [{ kind: "file", getAsFile: () => video }] }), video);
});

test("text paste and unsupported files keep the normal paste path", () => {
	assert.equal(supportMediaFromClipboard({ files: [], items: [{ kind: "string" }] }), null);
	assert.equal(supportMediaFromClipboard({ files: [{ name: "archive.zip", type: "application/zip" }] }), null);
});
