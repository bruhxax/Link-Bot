import test from 'node:test';
import assert from 'node:assert/strict';
import { legacySourceCrop, cropMediaGeometry, zoomCrop, resizeCropCorner, resizeBannerProportionally } from './static/banner-crop.mjs';

const near = (actual, expected) => assert.ok(Math.abs(actual - expected) < 1e-8, `${actual} != ${expected}`);
const crop = { left: 12.5, top: 20, width: 75, height: 60 };

test('legacy focal points including edges preserve the previous cover composition', () => {
	for (const x of [0, 25, 50, 100]) {
		for (const y of [0, 70, 100]) {
			const converted = legacySourceCrop({bannerCropX: x, bannerCropY: y, bannerZoom: 175}, 1920, 1080, 3);
			const g = cropMediaGeometry(converted, 1920, 1080, {left: 0, top: 0, width: 360, height: 120});
			const scale = Math.max(360 / 1920, 120 / 1080) * 1.75;
			near(g.width, 1920 * scale);
			near(g.height, 1080 * scale);
			near(g.left, (360 - 1920 * scale) * x / 100);
			near(g.top, (120 - 1080 * scale) * y / 100);
		}
	}
});

test('layout resizing preserves source crop and intrinsic aspect at any box ratio', () => {
	for (const [width, height] of [[360, 132], [180, 300], [800, 90], [2000, 1500]]) {
		const g = cropMediaGeometry(crop, 3840, 2160, { left: 0, top: 0, width, height });
		near(g.width / g.height, 3840 / 2160);
		near((g.viewport.left - g.left) / g.width * 100, crop.left);
		near((g.viewport.top - g.top) / g.height * 100, crop.top);
		near(g.viewport.width / g.width * 100, crop.width);
		near(g.viewport.height / g.height * 100, crop.height);
		assert.ok(g.viewport.width <= width + 1e-8 && g.viewport.height <= height + 1e-8);
	}
});

test('each corner keeps its opposite corner anchored and stays in source bounds', () => {
	for (const corner of ['tl', 'tr', 'bl', 'br']) {
		for (const delta of [-1000, -4, 8, 1000]) {
			const next = resizeCropCorner(crop, corner, delta, delta);
			assert.ok(next.left >= 0 && next.top >= 0 && next.width >= 1 && next.height >= 1);
			assert.ok(next.left + next.width <= 100 && next.top + next.height <= 100);
			near(corner.endsWith('l') ? next.left + next.width : next.left, corner.endsWith('l') ? crop.left + crop.width : crop.left);
			near(corner.startsWith('t') ? next.top + next.height : next.top, corner.startsWith('t') ? crop.top + crop.height : crop.top);
		}
	}
});

test('zoom retains aspect, respects bounds and reverses around the crop center', () => {
	assert.deepEqual(zoomCrop(zoomCrop(crop, 2), .5), crop);
	for (const rect of [crop, { left: 0, top: 10, width: 1, height: 80 }]) {
		for (const factor of [.01, .8, 1.2, 1000]) {
			const next = zoomCrop(rect, factor);
			near(next.width / next.height, rect.width / rect.height);
			assert.ok(next.width >= 1 && next.height >= 1 && next.left >= 0 && next.top >= 0);
			assert.ok(next.left + next.width <= 100 && next.top + next.height <= 100);
		}
	}
});

test('constructor resize follows original aspect and all size constraints', () => {
	for (const [dx, dy] of [[80, 0], [0, 100], [-1000, -900], [1000, 300]]) {
		const size = resizeBannerProportionally(300, 120, dx, dy, 32, 20, 400, 720);
		near(size.width / size.height, 2.5);
		assert.ok(size.width >= 32 && size.width <= 400 && size.height >= 20 && size.height <= 720);
	}
});
