// Dithered soft scroll-edge fade in HLSL.
// p0 is (r, g, b, maxAlpha) straight color; p1.x > 0.5 fades bottom->top,
// otherwise top->bottom.

float4 effect(float2 p, Effect e) {
	float t = saturate((p.y - e.rect.y) / max(e.rect.w, 0.0001));
	if (e.p1.x > 0.5) {
		t = 1.0 - t;
	}
	float a = e.p0.a * (1.0 - t);
	if (a <= 0) {
		return float4(0, 0, 0, 0);
	}
	if (t > 0 && t < 1) {
		float2 q = floor(p);
		float f1 = frac(dot(q, float2(0.06711056, 0.00583715)));
		float d = (frac(52.9829189 * f1) - 0.5) / 255.0;
		a = saturate(a + d);
	}
	return float4(e.p0.rgb * a, a);
}
