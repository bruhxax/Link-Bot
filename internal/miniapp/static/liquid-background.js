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

		float hash(vec2 p) {
			p = fract(p * vec2(123.34, 456.21));
			p += dot(p, p + 45.32);
			return fract(p.x * p.y);
		}

		float noise(vec2 p) {
			vec2 i = floor(p);
			vec2 f = fract(p);
			f = f * f * (3.0 - 2.0 * f);
			return mix(mix(hash(i), hash(i + vec2(1.0, 0.0)), f.x),
				mix(hash(i + vec2(0.0, 1.0)), hash(i + vec2(1.0, 1.0)), f.x), f.y);
		}

		float fbm(vec2 p) {
			float value = 0.0;
			float amplitude = 0.52;
			mat2 rotation = mat2(0.80, -0.60, 0.60, 0.80);
			for (int i = 0; i < 5; i++) {
				value += amplitude * noise(p);
				p = rotation * p * 2.02 + 7.13;
				amplitude *= 0.5;
			}
			return value;
		}

		void main() {
			vec2 uv = gl_FragCoord.xy / max(uResolution, vec2(1.0));
			vec2 p = uv - 0.5;
			p.x *= uResolution.x / max(uResolution.y, 1.0);
			float t = uTime;
			float largeFlow = fbm(p * 1.18 + vec2(t * 0.045, -t * 0.030));
			float crossFlow = fbm(p.yx * 1.55 + vec2(-t * 0.026, t * 0.036) + largeFlow * 0.72);
			vec2 warped = p + vec2(largeFlow - 0.5, crossFlow - 0.5) * 0.42;
			float folds = fbm(warped * 1.34 + vec2(crossFlow, largeFlow) * 0.58);

			vec3 color;
			if (uVariant < 1.5) {
				float blueSweep = smoothstep(-0.62, 0.34, warped.x + warped.y * 0.42 + folds * 0.50);
				float purpleFold = smoothstep(-0.28, 0.52, -warped.x * 0.30 - warped.y + crossFlow * 0.78);
				float whiteSheen = smoothstep(0.28, 0.86, warped.x - warped.y * 0.86 + largeFlow * 0.48);
				float darkRibbon = 1.0 - smoothstep(0.12, 0.26, abs(warped.x + warped.y * 0.58 + folds * 0.22));
				color = mix(uColor1, uColor2, blueSweep * 0.94);
				color = mix(color, uColor3, purpleFold * (0.48 + blueSweep * 0.42));
				color = mix(color, uColor1, darkRibbon * 0.48);
				color = mix(color, uColor4, whiteSheen * (0.34 + purpleFold * 0.58));
			} else {
				float narrowLight = smoothstep(-0.05, 0.42, warped.x + folds * 0.28);
				float lowerFlow = smoothstep(0.03, 0.74, -warped.y + crossFlow * 0.55);
				float violetVeil = smoothstep(-0.42, 0.40, warped.x - warped.y * 0.48 + largeFlow * 0.46);
				float whiteEdge = smoothstep(0.54, 1.02, warped.x - warped.y * 1.10 + folds * 0.32);
				color = mix(uColor1, uColor2, narrowLight * 0.72);
				color = mix(color, uColor3, lowerFlow * violetVeil * 0.92);
				color = mix(color, uColor4, whiteEdge * lowerFlow * 0.72);
				color = mix(color, uColor1, smoothstep(-0.24, 0.38, warped.y + warped.x * 0.22) * 0.56);
			}

			float softGlass = (fbm(warped * 3.1 - t * 0.018) - 0.5) * 0.055;
			float grain = hash(gl_FragCoord.xy + floor(t * 8.0)) - 0.5;
			color += softGlass + grain * mix(0.036, 0.012, step(1.5, uVariant));
			color = pow(max(color, 0.0), vec3(0.96));
			gl_FragColor = vec4(color, 1.0);
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
	let colors = ["#000000", "#1646ff", "#7226ff", "#ffffff"];
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
