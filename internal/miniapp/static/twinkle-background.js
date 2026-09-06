const twinkleCanvas = document.querySelector(".bg-media__twinkle-canvas");

function createTwinkleBackground(canvas) {
	if (!canvas) return null;
	const context = canvas.getContext("2d", { alpha: true });
	if (!context) return null;

	let stars = [];
	let color = [255, 255, 255];
	let speed = 38;
	let width = 1;
	let height = 1;
	let paused = true;
	let animationFrame = 0;
	let resizeTimer = 0;
	let startedAt = performance.now();
	let elapsedBeforePause = 0;
	let lastDrawAt = 0;

	function randomBetween(min, max) {
		return min + Math.random() * (max - min);
	}

	function parseColor(value) {
		const normalized = String(value || "").replace(/^#/, "");
		if (!/^[0-9a-f]{6}$/i.test(normalized)) return [255, 255, 255];
		return [0, 2, 4].map((offset) => Number.parseInt(normalized.slice(offset, offset + 2), 16));
	}

	function targetStarCount() {
		const lowPerformance = document.documentElement.dataset.performance === "low";
		const area = width * height;
		return Math.round(Math.max(lowPerformance ? 22 : 32, Math.min(lowPerformance ? 36 : 68, area / (lowPerformance ? 11200 : 6800))));
	}

	function seed() {
		const count = targetStarCount();
		stars = Array.from({ length: count }, (_, index) => {
			const bright = index % 11 === 0;
			return {
				x: Math.random() * width,
				y: Math.random() * height,
				radius: bright ? randomBetween(1.45, 2.15) : randomBetween(0.55, 1.5),
				baseAlpha: bright ? randomBetween(0.2, 0.34) : randomBetween(0.08, 0.24),
				amplitude: bright ? randomBetween(0.5, 0.64) : randomBetween(0.24, 0.5),
				phase: randomBetween(0, Math.PI * 2),
				rate: randomBetween(0.62, 1.34),
				glow: bright ? randomBetween(6, 9) : randomBetween(3, 6),
			};
		});
	}

	function resize() {
		if (animationFrame) cancelAnimationFrame(animationFrame);
		animationFrame = 0;
		const parent = canvas.parentElement;
		width = Math.max(1, parent?.clientWidth || window.innerWidth);
		height = Math.max(1, parent?.clientHeight || window.innerHeight);
		const performanceMode = document.documentElement.dataset.performance;
		const ratio = performanceMode === "low" || performanceMode === "reduced" ? 1 : Math.min(window.devicePixelRatio || 1, 1.5);
		const pixelWidth = Math.max(1, Math.round(width * ratio));
		const pixelHeight = Math.max(1, Math.round(height * ratio));
		if (canvas.width !== pixelWidth || canvas.height !== pixelHeight) {
			canvas.width = pixelWidth;
			canvas.height = pixelHeight;
			canvas.style.width = `${width}px`;
			canvas.style.height = `${height}px`;
			context.setTransform(ratio, 0, 0, ratio, 0, 0);
		}
		seed();
		draw(performance.now());
	}

	function draw(now) {
		animationFrame = 0;
		if (!paused && document.documentElement.dataset.performance === "low" && now - lastDrawAt < 42) {
			animationFrame = requestAnimationFrame(draw);
			return;
		}
		lastDrawAt = now;
		context.clearRect(0, 0, width, height);
		const elapsed = (elapsedBeforePause + (paused ? 0 : now - startedAt)) / 1000;
		const speedScale = 0.38 + ((speed - 10) / 90) * 1.32;
		const lowPerformance = document.documentElement.dataset.performance === "low";
		for (const star of stars) {
			const pulse = 0.5 + Math.sin(star.phase + elapsed * star.rate * speedScale) * 0.5;
			const alpha = Math.min(0.94, star.baseAlpha + star.amplitude * pulse);
			context.beginPath();
			context.arc(star.x, star.y, star.radius, 0, Math.PI * 2);
			context.fillStyle = `rgba(${color[0]}, ${color[1]}, ${color[2]}, ${alpha.toFixed(3)})`;
			context.shadowColor = `rgba(${color[0]}, ${color[1]}, ${color[2]}, ${(alpha * 0.7).toFixed(3)})`;
			context.shadowBlur = lowPerformance ? 2.5 : star.glow;
			context.fill();
		}
		context.shadowBlur = 0;
		if (!paused) animationFrame = requestAnimationFrame(draw);
	}

	function setPaused(nextPaused) {
		const shouldPause = Boolean(nextPaused);
		if (paused === shouldPause) return;
		const now = performance.now();
		if (shouldPause) {
			elapsedBeforePause += now - startedAt;
			paused = true;
			if (animationFrame) cancelAnimationFrame(animationFrame);
			animationFrame = 0;
			draw(now);
			return;
		}
		paused = false;
		startedAt = now;
		animationFrame = requestAnimationFrame(draw);
	}

	function setConfig(config = {}) {
		color = parseColor(config.color);
		speed = Math.max(10, Math.min(100, Number(config.speed ?? speed)));
		if (stars.length !== targetStarCount()) seed();
		if (paused) draw(performance.now());
	}

	window.addEventListener("resize", () => {
		window.clearTimeout(resizeTimer);
		resizeTimer = window.setTimeout(resize, 120);
	}, { passive: true });

	resize();
	return { setConfig, setPaused };
}

const twinkleBackground = createTwinkleBackground(twinkleCanvas);
window.__linkBotTwinkle = twinkleBackground || { setConfig() {}, setPaused() {} };
