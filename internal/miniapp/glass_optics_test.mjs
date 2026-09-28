import assert from "node:assert/strict";
import test from "node:test";
import { glassSample, buildGlassMaps } from "./static/glass-optics.mjs";

test("the four rims stretch the backdrop outwards using inverse sampling", () => {
  const sample = (x, y) => glassSample(x, y, 240, 120, 24, 12);
  const top = sample(120, 2);
  const bottom = sample(120, 118);
  const left = sample(2, 60);
  const right = sample(238, 60);
  assert.ok(top.y > 0 && bottom.y < 0);
  assert.ok(left.x > 0 && right.x < 0);
  assert.equal(top.x, 0);
  assert.equal(left.y, 0);
  assert.equal(top.y, -bottom.y);
  assert.equal(left.x, -right.x);
});

test("rounded corners refract diagonally and the center stays undistorted", () => {
  const corner = glassSample(8, 8, 240, 120, 24, 12);
  assert.ok(corner.x > 0 && corner.y > 0 && corner.edge > 0);
  assert.equal(corner.x, corner.y);
  assert.deepEqual(glassSample(120, 60, 240, 120, 24, 12), { x: 0, y: 0, edge: 0 });
  assert.deepEqual(glassSample(0, 0, 240, 120, 24, 12), { x: 0, y: 0, edge: 0 });
  assert.ok(glassSample(120, 2, 240, 120, 24, 12).y > glassSample(120, 8, 240, 120, 24, 12).y);
});

test("all four corners point outwards and capsule maps follow the actual radius", () => {
  for (const [x, y, signX, signY] of [[8, 8, 1, 1], [232, 8, -1, 1], [8, 112, 1, -1], [232, 112, -1, -1]]) {
    const field = glassSample(x, y, 240, 120, 24, 12);
    assert.equal(Math.sign(field.x), signX);
    assert.equal(Math.sign(field.y), signY);
    assert.ok(field.edge > 0);
  }
  const capsule = buildGlassMaps(160, 40, 999);
  const sameRadius = buildGlassMaps(160, 40, 20);
  assert.deepEqual(capsule.displacement, sameRadius.displacement);
  assert.deepEqual(capsule.mask, sameRadius.mask);
});

test("maps preserve geometry within a bounded pixel budget", () => {
  const maps = buildGlassMaps(2000, 1000, 24, 100000);
  assert.ok(maps.width * maps.height <= 101000);
  assert.ok(Math.abs(maps.width / maps.height - 2) < 0.01);
  assert.equal(maps.displacement.length, maps.width * maps.height * 4);
  const middle = (Math.floor(maps.height / 2) * maps.width + Math.floor(maps.width / 2)) * 4;
  assert.equal(maps.displacement[middle], 128);
  assert.equal(maps.displacement[middle + 1], 128);
  assert.equal(maps.mask[middle + 3], 0);
});
