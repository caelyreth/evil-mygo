// Single-pass mip-pyramid progressive blur in HLSL (see scene.Effect
// and blur.go).

float4 pyrAt(int2 p, int2 size, int level) {
	return backdropTex.Load(int3(clamp(p, int2(0, 0), size - 1), level));
}

float4 ditherPyr(float4 c, float2 p) {
	float2 q = floor(p);
	float f1 = frac(dot(q, float2(0.06711056, 0.00583715)));
	float d = (frac(52.9829189 * f1) - 0.5) / 255.0;
	float a = c.a < 1 ? saturate(c.a + d) : c.a;
	return float4(clamp(c.rgb + d, 0, a), a);
}

float4 pyrSampleLevel(Effect e, float2 q, int level) {
	int k = 1 << level;
	int2 size = max((int2(e.area.zw) + k - 1) >> level, int2(1, 1));
	float2 u = (q - e.area.xy) / float(k) - 0.5;
	float2 f = floor(u);
	float2 w = u - f;
	int2 p = int2(f);
	float4 top = pyrAt(p, size, level) * (1 - w.x) + pyrAt(p + int2(1, 0), size, level) * w.x;
	float4 bot = pyrAt(p + int2(0, 1), size, level) * (1 - w.x) + pyrAt(p + int2(1, 1), size, level) * w.x;
	return top * (1 - w.y) + bot * w.y;
}

float pyrLOD(float blur, int maxLevel) {
	if (blur <= 0 || maxLevel <= 0) {
		return 0;
	}
	static const float sig[6] = {0.0, 2.158703, 4.827007, 9.892421, 19.902261, 39.863015};
	[unroll] for (int k = 1; k < 6; k++) {
		if (blur <= sig[k] || k == maxLevel) {
			float f = saturate((blur - sig[k - 1]) / (sig[k] - sig[k - 1]));
			return min(float(k - 1) + f, float(maxLevel));
		}
	}
	return float(maxLevel);
}

float4 effect(float2 p, Effect e) {
	float2 d = e.p0.zw - e.p0.xy;
	float l = d.x * d.x + d.y * d.y;
	float t = 0;
	if (l > 0) {
		t = saturate(((p.x - e.p0.x) * d.x + (p.y - e.p0.y) * d.y) / max(l, 0.0001));
	}
	float blur = e.p1.x * (1 - t) + e.p1.y * t;
	int maxLevel = max(int(e.down) - 1, 0);
	float lod = pyrLOD(blur, maxLevel);
	float2 q = framePoint(e, p);
	int lo = int(floor(lod));
	int hi = min(lo + 1, maxLevel);
	float f = lod - float(lo);
	float4 c = pyrSampleLevel(e, q, lo);
	if (hi != lo && f > 0) {
		c += (pyrSampleLevel(e, q, hi) - c) * f;
	}
	if (lod > 0 && c.a > 0) {
		c = ditherPyr(c, p);
	}
	return c;
}
