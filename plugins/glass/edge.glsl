// Dithered soft scroll-edge fade in GLSL.
// p0 is (r, g, b, maxAlpha) straight color; p1.x > 0.5 fades bottom->top,
// otherwise top->bottom.

vec4 effect(vec2 p, Effect e) {
	float t = clamp((p.y - e.rect.y) / max(e.rect.w, 0.0001), 0.0, 1.0);
	if (e.p1.x > 0.5) {
		t = 1.0 - t;
	}
	float a = e.p0.a * (1.0 - t);
	if (a <= 0.0) {
		return vec4(0.0);
	}
	if (t > 0.0 && t < 1.0) {
		vec2 q = floor(p);
		float f1 = fract(dot(q, vec2(0.06711056, 0.00583715)));
		float d = (fract(52.9829189 * f1) - 0.5) / 255.0;
		a = clamp(a + d, 0.0, 1.0);
	}
	return vec4(e.p0.rgb * a, a);
}
