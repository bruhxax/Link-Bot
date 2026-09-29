const canvas = document.querySelector(".bg-media__liquid-canvas");

function createLiquidBackground(target) {
	if (!target) return null;
	const gl = target.getContext("webgl", {
		alpha: false,
		antialias: false,
		depth: false,
		stencil: false,
		powerPreference: "low-power",
	});
	if (!gl) return null;

	const vertexSource = `
		attribute vec2 aPosition;
		void main() {
			gl_Position = vec4(aPosition, 0.0, 1.0);
		}
	`;
	const fragmentSource = `
		precision highp float;
		uniform vec2 uResolution;
		uniform float uTime;
		uniform float uVariant;
		uniform vec3 uColor1;
		uniform vec3 uColor2;
		uniform vec3 uColor3;
		uniform vec3 uColor4;

		float glow(float distance, float width) {
			float normalized = distance / width;
			return exp(-normalized * normalized);
		}

		float haze(vec2 point, vec2 center, vec2 scale) {
			vec2 offset = (point - center) * scale;
			return exp(-dot(offset, offset));
		}

		void main() {
			vec2 point = (gl_FragCoord.xy / max(uResolution, vec2(1.0))) * 2.0 - 1.0;
			point.x *= uResolution.x / max(uResolution.y, 1.0);
			float time = uTime;
			point += vec2(0.035 * sin(time * 0.31), 0.035 * cos(time * 0.25));
			point.x += 0.068 * sin(point.y * 2.7 + time * 0.24);
			point.y += 0.025 * sin(point.x * 2.2 - time * 0.18);

			float topLight = haze(point, vec2(-0.34, 0.34), vec2(1.12, 1.22));
			float bottomLight = haze(point, vec2(0.35, -0.52), vec2(1.18, 1.42));
			float shimmer = 0.5 + 0.5 * sin(point.y * 2.8 + time * 0.43);
			vec3 color = uColor1;
			if (uVariant < 1.5) {
				float sheet = point.x + point.y * 0.43 - 0.28 * sin(point.y * 2.15 + time * 0.27) + 0.10;
				float secondSheet = point.x - point.y * 0.42 + 0.19 * sin(point.y * 1.55 - time * 0.21) - 0.30;
				float glassBody = smoothstep(-0.43, -0.12, sheet) * (1.0 - smoothstep(0.16, 0.48, sheet));
				float edge = glow(sheet + 0.19, 0.062) * (0.55 + 0.45 * topLight);
				float edgeReturn = glow(sheet - 0.23, 0.092) * (0.35 + 0.65 * bottomLight);
				float distantEdge = glow(secondSheet, 0.10) * (0.20 + 0.45 * bottomLight);
				color = mix(uColor1, uColor2, 0.07 + topLight * 0.19);
				color = mix(color, uColor3, bottomLight * 0.18 + glassBody * 0.18);
				color += uColor2 * topLight * 0.08 + uColor3 * bottomLight * 0.045;
				color *= 1.0 - glow(sheet + 0.015, 0.10) * 0.16;
				color += mix(uColor2, uColor3, shimmer) * (edge * 0.19 + distantEdge * 0.14);
				color += uColor4 * (edge * 0.26 + edgeReturn * 0.12 + distantEdge * 0.08);
			} else {
				float sheet = point.x - point.y * 0.42 + 0.27 * sin(point.y * 1.8 - time * 0.23) - 0.06;
				float shadow = glow(sheet - 0.22, 0.32);
				float paleEdge = glow(sheet + 0.16, 0.072) * (0.35 + 0.50 * topLight);
				float fineEdge = glow(sheet - 0.13, 0.037) * (0.28 + 0.62 * bottomLight);
				float secondary = glow(point.x + point.y * 0.45 + 0.31 + 0.10 * sin(time * 0.19), 0.18) * bottomLight;
				color = mix(uColor1, uColor2, 0.04 + topLight * 0.11);
				color = mix(color, uColor3, bottomLight * 0.16 + secondary * 0.14);
				color *= 1.0 - shadow * 0.14;
				color += mix(uColor2, uColor3, shimmer) * (paleEdge * 0.10 + secondary * 0.09);
				color += uColor4 * (paleEdge * 0.27 + fineEdge * 0.20);
			}
			float vignette = 1.0 - 0.16 * smoothstep(0.18, 1.55, length(point));
			gl_FragColor = vec4(clamp(color * vignette, 0.0, 1.0), 1.0);
		}
	`;

	function compile(type, source) {
		const shader = gl.createShader(type);
		gl.shaderSource(shader, source);
		gl.compileShader(shader);
		if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
			gl.deleteShader(shader);
			return null;
		}
		return shader;
	}

	const vertex = compile(gl.VERTEX_SHADER, vertexSource);
	const fragment = compile(gl.FRAGMENT_SHADER, fragmentSource);
	if (!vertex || !fragment) return null;
	const program = gl.createProgram();
	gl.attachShader(program, vertex);
	gl.attachShader(program, fragment);
	gl.linkProgram(program);
	if (!gl.getProgramParameter(program, gl.LINK_STATUS)) return null;
	gl.useProgram(program);

	const position = gl.createBuffer();
	gl.bindBuffer(gl.ARRAY_BUFFER, position);
	gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
	const positionLocation = gl.getAttribLocation(program, "aPosition");
	gl.enableVertexAttribArray(positionLocation);
	gl.vertexAttribPointer(positionLocation, 2, gl.FLOAT, false, 0, 0);

	const uniforms = {
		resolution: gl.getUniformLocation(program, "uResolution"),
		time: gl.getUniformLocation(program, "uTime"),
		variant: gl.getUniformLocation(program, "uVariant"),
		colors: [1, 2, 3, 4].map((index) => gl.getUniformLocation(program, `uColor${index}`)),
	};
	let colors = ["#07111d", "#407f8d", "#88799e", "#e9eeea"];
	let variant = 1;
	let speed = 35;
	let paused = true;
	let frame = 0;
	let startedAt = performance.now();
	let elapsedBeforePause = 0;
	let lastDrawAt = 0;

	function parseColor(value) {
		const normalized = String(value || "").replace(/^#/, "");
		if (!/^[0-9a-f]{6}$/i.test(normalized)) return [0, 0, 0];
		return [0, 2, 4].map((offset) => Number.parseInt(normalized.slice(offset, offset + 2), 16) / 255);
	}

	function resize() {
		const performanceMode = document.documentElement.dataset.performance;
		const ratio = performanceMode === "low" || performanceMode === "reduced" ? 1 : Math.min(window.devicePixelRatio || 1, 1.5);
		const width = Math.max(1, Math.round(target.clientWidth * ratio));
		const height = Math.max(1, Math.round(target.clientHeight * ratio));
		if (target.width !== width || target.height !== height) {
			target.width = width;
			target.height = height;
			gl.viewport(0, 0, width, height);
		}
	}

	function draw(now = performance.now()) {
		if (!paused && document.documentElement.dataset.performance === "low" && now - lastDrawAt < 30) {
			frame = requestAnimationFrame(draw);
			return;
		}
		lastDrawAt = now;
		resize();
		const speedScale = 0.22 + ((speed - 10) / 90) * 0.78;
		const elapsed = elapsedBeforePause + (paused ? 0 : now - startedAt);
		gl.uniform2f(uniforms.resolution, target.width, target.height);
		gl.uniform1f(uniforms.time, elapsed * 0.001 * speedScale);
		gl.uniform1f(uniforms.variant, variant);
		colors.forEach((color, index) => gl.uniform3fv(uniforms.colors[index], parseColor(color)));
		gl.drawArrays(gl.TRIANGLES, 0, 3);
		if (!paused) frame = requestAnimationFrame(draw);
	}

	function setPaused(nextPaused) {
		const next = Boolean(nextPaused);
		if (next === paused) return;
		if (next) {
			elapsedBeforePause += performance.now() - startedAt;
			paused = true;
			cancelAnimationFrame(frame);
			draw();
			return;
		}
		paused = false;
		startedAt = performance.now();
		frame = requestAnimationFrame(draw);
	}

	function setConfig(config = {}) {
		variant = config.variant === 2 || config.variant === "liquid2" ? 2 : 1;
		colors = Array.from({ length: 4 }, (_, index) => String(config.colors?.[index] || colors[index]));
		speed = Math.max(10, Math.min(100, Number(config.speed ?? speed)));
		if (paused) draw();
		else {
			cancelAnimationFrame(frame);
			frame = requestAnimationFrame(draw);
		}
	}

	window.addEventListener("resize", resize, { passive: true });
	target.addEventListener("webglcontextlost", (event) => {
		event.preventDefault();
		setPaused(true);
	});
	resize();
	draw();
	return { setConfig, setPaused, draw };
}

const liquidBackground = createLiquidBackground(canvas);
if (!liquidBackground) document.documentElement.dataset.liquidFallback = "true";
window.__linkBotLiquid = liquidBackground || { setConfig() {}, setPaused() {}, draw() {} };
