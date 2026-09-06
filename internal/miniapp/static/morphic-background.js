const morphicParticleLayer = document.querySelector(".bg-media__morphic-particles");

function createMorphicBackground(target) {
	if (!target) return null;

	let particles = [];
	let animationFrame = 0;
	let resizeTimer = 0;
	let paused = true;
	let speed = 42;
	let previousTime = 0;
	let lastDrawAt = 0;
	let spawnElapsed = 0;

	function randomBetween(min, max) {
		return min + Math.random() * (max - min);
	}

	function bounds() {
		return {
			width: Math.max(1, target.clientWidth || window.innerWidth),
			height: Math.max(1, target.clientHeight || window.innerHeight),
		};
	}

	function particleLimit() {
		return document.documentElement.dataset.performance === "low" ? 8 : 12;
	}

	function removeParticle(particle) {
		particle.element.remove();
	}

	function clearParticles() {
		particles.forEach(removeParticle);
		particles = [];
	}

	function addParticle(initial = false, options = {}) {
		const area = bounds();
		const size = Number(options.size || randomBetween(14, 50));
		const phase = Number(options.phase ?? randomBetween(0, Math.PI * 2));
		const element = document.createElement("span");
		element.className = "bg-media__morphic-particle";
		element.style.width = `${size.toFixed(2)}px`;
		element.style.height = `${size.toFixed(2)}px`;
		target.appendChild(element);

		const particle = {
			element,
			size,
			x: Number(options.x ?? randomBetween(-size * 0.15, Math.max(1, area.width - size * 0.85))),
			y: Number(options.y ?? (initial ? randomBetween(-size * 0.2, area.height - size * 0.8) : area.height + size * 0.35)),
			rise: Number(options.rise ?? randomBetween(22, 42)),
			sway: Number(options.sway ?? randomBetween(area.width * 0.025, area.width * 0.16)),
			phase,
			phaseSpeed: Number(options.phaseSpeed ?? randomBetween(0.38, 0.78)),
		};
		particles.push(particle);
		placeParticle(particle, area.width);
		return particle;
	}

	function placeParticle(particle, width) {
		const left = Math.max(-particle.size * 0.3, Math.min(width - particle.size * 0.7, particle.x + Math.sin(particle.phase) * particle.sway));
		particle.element.style.transform = `translate3d(${left.toFixed(2)}px, ${particle.y.toFixed(2)}px, 0)`;
	}

	function seed() {
		clearParticles();
		const area = bounds();
		const count = particleLimit();
		for (let index = 0; index < count - 2; index += 1) addParticle(true);

		const mergedY = area.height * 0.68;
		addParticle(true, {
			x: area.width * 0.44,
			y: mergedY,
			size: 42,
			sway: area.width * 0.055,
			phase: 0.7,
			rise: 28,
			phaseSpeed: 0.52,
		});
		addParticle(true, {
			x: area.width * 0.49,
			y: mergedY - 6,
			size: 34,
			sway: area.width * 0.045,
			phase: 0.82,
			rise: 28,
			phaseSpeed: 0.52,
		});
	}

	function frame(now) {
		animationFrame = 0;
		if (paused) return;
		if (document.documentElement.dataset.performance === "low" && now - lastDrawAt < 32) {
			animationFrame = requestAnimationFrame(frame);
			return;
		}

		const delta = Math.min(0.05, Math.max(0, (now - (previousTime || now)) / 1000));
		previousTime = now;
		lastDrawAt = now;
		const area = bounds();
		const speedRatio = (speed - 10) / 90;
		const speedScale = 0.48 + speedRatio * 1.12;
		particles.forEach((particle) => {
			particle.y -= particle.rise * speedScale * delta;
			particle.phase += particle.phaseSpeed * speedScale * delta;
			placeParticle(particle, area.width);
		});
		particles = particles.filter((particle) => {
			if (particle.y >= -particle.size * 1.4) return true;
			removeParticle(particle);
			return false;
		});

		spawnElapsed += delta * 1000;
		const spawnInterval = 2100 - speedRatio * 1200;
		if (spawnElapsed >= spawnInterval && particles.length < particleLimit()) {
			spawnElapsed = 0;
			addParticle(false);
		}
		animationFrame = requestAnimationFrame(frame);
	}

	function setPaused(nextPaused) {
		const shouldPause = Boolean(nextPaused);
		if (paused === shouldPause) return;
		paused = shouldPause;
		previousTime = 0;
		if (paused) {
			if (animationFrame) cancelAnimationFrame(animationFrame);
			animationFrame = 0;
			return;
		}
		if (!particles.length) seed();
		animationFrame = requestAnimationFrame(frame);
	}

	function setConfig(config = {}) {
		speed = Math.max(10, Math.min(100, Number(config.speed ?? speed)));
		const color = String(config.color || "").trim();
		if (color) target.style.setProperty("--morphic-ball", color);
		if (!particles.length) seed();
	}

	window.addEventListener("resize", () => {
		window.clearTimeout(resizeTimer);
		resizeTimer = window.setTimeout(seed, 120);
	}, { passive: true });

	seed();
	return { setConfig, setPaused };
}

const morphicBackground = createMorphicBackground(morphicParticleLayer);
window.__linkBotMorphic = morphicBackground || { setConfig() {}, setPaused() {} };
