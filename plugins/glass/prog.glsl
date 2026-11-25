// Single-pass mip-pyramid progressive blur in GLSL (see scene.Effect
// and blur.go).

vec4 pyrAt(ivec2 p, ivec2 size, int level) {
	return texelFetch(uBackdrop, clamp(p, ivec2(0), size - 1), level);
}

vec4 ditherPyr(vec4 c, vec2 p) {
	vec2 q = floor(p);
	float f1 = fract(dot(q, vec2(0.06711056, 0.00583715)));
	float d = (fract(52.9829189 * f1) - 0.5) / 255.0;
	float a = c.a < 1.0 ? clamp(c.a + d, 0.0, 1.0) : c.a;
	return vec4(clamp(c.rgb + d, vec3(0.0), vec3(a)), a);
}

vec4 pyrSampleLevel(Effect e, vec2 q, int level) {
	int k = 1 << level;
	ivec2 size = max((ivec2(e.area.zw) + k - 1) >> level, ivec2(1));
	vec2 u = (q - e.area.xy) / float(k) - 0.5;
	vec2 f = floor(u);
	vec2 w = u - f;
	ivec2 p = ivec2(f);
	vec4 top = pyrAt(p, size, level) * (1.0 - w.x) + pyrAt(p + ivec2(1, 0), size, level) * w.x;
	vec4 bot = pyrAt(p + ivec2(0, 1), size, level) * (1.0 - w.x) + pyrAt(p + ivec2(1, 1), size, level) * w.x;
	return top * (1.0 - w.y) + bot * w.y;
}

float pyrLOD(float blur, int maxLevel) {
	if (blur <= 0.0 || maxLevel <= 0) {
		return 0.0;
	}
	float sig[6];
	sig[0] = 0.0;
	sig[1] = 2.158703;
	sig[2] = 4.827007;
	sig[3] = 9.892421;
	sig[4] = 19.902261;
	sig[5] = 39.863015;
	for (int k = 1; k < 6; k++) {
		if (blur <= sig[k] || k == maxLevel) {
			float f = clamp((blur - sig[k - 1]) / (sig[k] - sig[k - 1]), 0.0, 1.0);
			return min(float(k - 1) + f, float(maxLevel));
		}
	}
	return float(maxLevel);
}

vec4 effect(vec2 p, Effect e) {
	vec2 d = e.p0.zw - e.p0.xy;
	float l = d.x * d.x + d.y * d.y;
	float t = 0.0;
	if (l > 0.0) {
		t = clamp(((p.x - e.p0.x) * d.x + (p.y - e.p0.y) * d.y) / max(l, 0.0001), 0.0, 1.0);
	}
	float blur = e.p1.x * (1.0 - t) + e.p1.y * t;
	int maxLevel = max(int(e.down) - 1, 0);
	float lod = pyrLOD(blur, maxLevel);
	vec2 q = framePoint(e, p);
	int lo = int(floor(lod));
	int hi = min(lo + 1, maxLevel);
	float f = lod - float(lo);
	vec4 c = pyrSampleLevel(e, q, lo);
	if (hi != lo && f > 0.0) {
		c += (pyrSampleLevel(e, q, hi) - c) * f;
	}
	if (lod > 0.0 && c.a > 0.0) {
		c = ditherPyr(c, p);
	}
	return c;
}
