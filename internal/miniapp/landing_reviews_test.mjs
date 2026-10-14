import test from 'node:test';
import assert from 'node:assert/strict';
import {reviewTickerItem} from './static/landing-reviews.mjs';

test('review ticker preserves rating, author, full text and their order', () => {
  for (let rating = 1; rating <= 5; rating++) {
    const text = 'Отзыв без обрезания '.repeat(80).trim();
    const html = reviewTickerItem({rating, username: '@test_user', comment: text});
    assert.equal((html.match(/<svg /g) || []).length, rating);
    assert.ok(html.includes(text));
    assert.ok(html.indexOf('__stars') < html.indexOf('@test_user'));
    assert.ok(html.indexOf('@test_user') < html.indexOf(text));
  }
});

test('review text cannot inject HTML and missing usernames never expose account IDs', () => {
  const html = reviewTickerItem({rating: 5, username: '<img onerror=x>', telegramId: 123456789, comment: '<script>alert("x")</script> &\nработает'});
  assert.ok(!html.includes('<script>'));
  assert.ok(html.includes('&lt;script&gt;alert(&quot;x&quot;)&lt;/script&gt; &amp; работает'));
  assert.ok(html.includes('Пользователь'));
  assert.ok(!html.includes('123456789'));
});

test('invalid ratings are excluded and textless ratings still appear', () => {
  for (const rating of [0, 6, 1.5, null, 'oops']) assert.equal(reviewTickerItem({rating}), '');
  assert.ok(reviewTickerItem({rating: 4, username: 'valid_user', comment: ''}).includes('@valid_user'));
});
