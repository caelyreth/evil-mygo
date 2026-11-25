// Dithered soft scroll-edge fade in Metal Shading Language.
// p0 is (r, g, b, maxAlpha) straight color; p1.x > 0.5 fades bottom->top,
// otherwise top->bottom.

float4 effect(float2 p, Effect e, texture2d<float> backdrop) {
	float t = clamp((p.y - e.rect.y) / max(e.rect.w, 0.0001f), 0.0f, 1.0f);
	if (e.p1.x > 0.5f) {
		t = 1.0f - t;
	}
	float a = e.p0.a * (1.0f - t);
	if (a <= 0.0f) {
		return float4(0.0f);
	}
	if (!e.wide && t > 0.0f && t < 1.0f) {
		float2 q = floor(p);
		float f1 = fract(dot(q, float2(0.06711056f, 0.00583715f)));
		float d = (fract(52.9829189f * f1) - 0.5f) / 255.0f;
		a = saturate(a + d);
	}
	return float4(e.p0.rgb * a, a);
}
