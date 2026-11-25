// Single-pass mip-pyramid progressive blur in Metal Shading Language (see
// scene.Effect and blur.go). p0 is the mask's line from start to end in
// pixels; p1.xy is the blur radius in pixels at start and end; e.area is
// level 0's (x, y, w, h); e.down is the number of pyramid levels.

float4 pyrAt(texture2d<float> t, int2 p, int2 size, uint level) {
	return t.read(uint2(clamp(p, int2(0), size - 1)), level);
}

float4 ditherPyr(float4 c, float2 p) {
	float2 q = floor(p);
	float f1 = fract(dot(q, float2(0.06711056f, 0.00583715f)));
	float d = (fract(52.9829189f * f1) - 0.5f) / 255.0f;
	float a = c.a < 1.0f ? saturate(c.a + d) : c.a;
	return float4(clamp(c.rgb + d, 0.0f, a), a);
}

float4 pyrSampleLevel(texture2d<float> t, Effect e, float2 q, int level) {
	int k = 1 << level;
	int2 size = max((int2(e.area.zw) + k - 1) >> level, int2(1));
	float2 u = (q - e.area.xy) / float(k) - 0.5f;
	float2 f = floor(u);
	float2 w = u - f;
	int2 p = int2(f);
	float4 top = pyrAt(t, p, size, uint(level)) * (1.0f - w.x) + pyrAt(t, p + int2(1, 0), size, uint(level)) * w.x;
	float4 bot = pyrAt(t, p + int2(0, 1), size, uint(level)) * (1.0f - w.x) + pyrAt(t, p + int2(1, 1), size, uint(level)) * w.x;
	return top * (1.0f - w.y) + bot * w.y;
}

float pyrLOD(float blur, int maxLevel) {
	if (blur <= 0.0f || maxLevel <= 0) {
		return 0.0f;
	}
	float sig[6] = {0.0f, 2.158703f, 4.827007f, 9.892421f, 19.902261f, 39.863015f};
	for (int k = 1; k < 6; k++) {
		if (blur <= sig[k] || k == maxLevel) {
			float f = saturate((blur - sig[k - 1]) / (sig[k] - sig[k - 1]));
			return min(float(k - 1) + f, float(maxLevel));
		}
	}
	return float(maxLevel);
}

float4 effect(float2 p, Effect e, texture2d<float> backdrop) {
	float2 d = e.p0.zw - e.p0.xy;
	float l = d.x * d.x + d.y * d.y;
	float t = 0.0f;
	if (l > 0.0f) {
		t = clamp(((p.x - e.p0.x) * d.x + (p.y - e.p0.y) * d.y) / max(l, 0.0001f), 0.0f, 1.0f);
	}
	float blur = e.p1.x * (1.0f - t) + e.p1.y * t;
	int maxLevel = max(int(e.down) - 1, 0);
	float lod = pyrLOD(blur, maxLevel);
	float2 q = framePoint(e, p);
	int lo = int(floor(lod));
	int hi = min(lo + 1, maxLevel);
	float f = lod - float(lo);
	float4 c = pyrSampleLevel(backdrop, e, q, lo);
	if (hi != lo && f > 0.0f) {
		c += (pyrSampleLevel(backdrop, e, q, hi) - c) * f;
	}
	if (!e.wide && lod > 0.0f && c.a > 0.0f) {
		c = ditherPyr(c, p);
	}
	return c;
}
