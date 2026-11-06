#import <Foundation/Foundation.h>
#import <Metal/Metal.h>

#include "membrane_draw.h"

static id<MTLComputePipelineState> mygo_membrane_pipeline;
static dispatch_once_t mygo_membrane_once;

static NSString *mygo_membrane_source =
    @"#include <metal_stdlib>\n"
     "using namespace metal;\n"
     "struct U { float width, height, sourceWidth, sourceHeight;\n"
     "           float camX, camY, camZ, yaw, pitch;\n"
     "           float waveTime, amplitude, viewAspect; };\n"
     "constant float panelLift = 0.28;\n"
     "constant float panelHalfH = 0.70;\n"
     "constant float groundY = -0.74;\n"
     "constant float fieldOfView = 0.74;\n"
     "float membraneWave(float u, float v, float t, float amp) {\n"
     "  float a = sin(6.1 * u + 0.82 * t) * sin(3.2 * v - 0.46 * t);\n"
     "  float b = sin(12.4 * u - 1.27 * t) * sin(6.3 * v + 0.31 * t);\n"
     "  return amp * (0.105 * a + 0.034 * b);\n"
     "}\n"
     "float3 membraneNormal(float u, float v, float t, float amp, float aspect) {\n"
     "  const float e = 0.0015;\n"
     "  float halfW = panelHalfH * aspect;\n"
     "  float zu = (membraneWave(u + e, v, t, amp) - membraneWave(u - e, v, t, amp)) / (2.0 * e);\n"
     "  float zv = (membraneWave(u, v + e, t, amp) - membraneWave(u, v - e, t, amp)) / (2.0 * e);\n"
     "  return normalize(float3(-zu * 2.0 * panelHalfH, 2.0 * halfW * zv, 4.0 * halfW * panelHalfH));\n"
     "}\n"
     "struct Hit { float t; float2 uv; float3 p; float3 n; bool ok; bool back; };\n"
     "Hit intersectMembrane(float3 cam, float3 rd, constant U &u) {\n"
     "  Hit h; h.t = 0.0; h.uv = float2(0.0); h.p = float3(0.0); h.n = float3(0.0, 0.0, 1.0);\n"
     "  h.ok = false; h.back = false;\n"
     "  if (fabs(rd.z) < 0.015) return h;\n"
     "  float halfW = panelHalfH * u.viewAspect;\n"
     "  float t = -cam.z / rd.z;\n"
     "  float3 p = cam;\n"
     "  float2 uv = float2(0.0);\n"
     "  for (int i = 0; i < 9; ++i) {\n"
     "    p = cam + rd * t;\n"
     "    uv = float2((p.x + halfW) / (2.0 * halfW),\n"
     "                (panelLift + panelHalfH - p.y) / (2.0 * panelHalfH));\n"
     "    float z = membraneWave(clamp(uv.x, 0.0, 1.0), clamp(uv.y, 0.0, 1.0), u.waveTime, u.amplitude);\n"
     "    t = (z - cam.z) / rd.z;\n"
     "  }\n"
     "  p = cam + rd * t;\n"
     "  uv = float2((p.x + halfW) / (2.0 * halfW),\n"
     "              (panelLift + panelHalfH - p.y) / (2.0 * panelHalfH));\n"
     "  if (t <= 0.01 || uv.x < 0.0 || uv.x > 1.0 || uv.y < 0.0 || uv.y > 1.0) return h;\n"
     "  h.t = t; h.uv = uv; h.p = p; h.n = membraneNormal(uv.x, uv.y, u.waveTime, u.amplitude, u.viewAspect);\n"
     "  h.ok = true; h.back = dot(rd, h.n) >= 0.0;\n"
     "  return h;\n"
     "}\n"
     "float floorDepth(float3 cam, float3 rd) {\n"
     "  return rd.y < -0.001 ? (groundY - cam.y) / rd.y : -1.0;\n"
     "}\n"
     "float3 rayDirection(float2 uv, float3 fwd, float3 right, float3 up, float aspect) {\n"
     "  float2 ndc = uv * 2.0 - 1.0; ndc.y = -ndc.y;\n"
     "  return normalize(fwd + right * ndc.x * fieldOfView * aspect + up * ndc.y * fieldOfView);\n"
     "}\n"
     "float3 environment(float3 rd) {\n"
     "  float h = smoothstep(-0.25, 0.72, rd.y);\n"
     "  float3 sky = mix(float3(0.73), float3(0.97), h);\n"
     "  float sun = pow(max(dot(rd, normalize(float3(0.38, 0.42, 0.82))), 0.0), 64.0);\n"
     "  return sky + float3(0.07) * sun;\n"
     "}\n"
     "float3 floorColor(float3 p) {\n"
     "  float2 g = abs(fract(p.xz * 4.0) - 0.5);\n"
     "  float grid = 1.0 - smoothstep(0.0, 0.025, min(g.x, g.y));\n"
     "  float fog = 1.0 - smoothstep(0.0, 3.2, length(p.xz));\n"
     "  float3 col = mix(float3(0.46), float3(0.77), fog);\n"
     "  return col + float3(0.10) * grid * fog;\n"
     "}\n"
     "float3 shadeSurface(float3 base, float3 n, float3 rd, bool back) {\n"
     "  float3 L = normalize(float3(-0.55, 0.70, 0.45));\n"
     "  float3 V = -rd;\n"
     "  float diffuse = max(dot(n, L), 0.0);\n"
     "  float spec = pow(max(dot(n, normalize(L + V)), 0.0), 42.0);\n"
     "  float edge = mix(0.58, 1.0, smoothstep(0.0, 0.22, diffuse));\n"
     "  float3 lit = base * edge + float3(spec * (back ? 0.025 : 0.15));\n"
     "  return lit;\n"
     "}\n"
     "kernel void membrane(texture2d<float> src [[texture(0)]],\n"
     "                     texture2d<float, access::write> dst [[texture(1)]],\n"
     "                     constant U &u [[buffer(0)]],\n"
     "                     uint2 gid [[thread_position_in_grid]]) {\n"
     "  if (gid.x >= uint(u.width) || gid.y >= uint(u.height)) return;\n"
     "  float3 cam = float3(u.camX, u.camY, u.camZ);\n"
     "  float3 fwd = normalize(float3(sin(u.yaw) * cos(u.pitch), sin(u.pitch), -cos(u.yaw) * cos(u.pitch)));\n"
     "  float3 right = normalize(cross(fwd, float3(0.0, 1.0, 0.0)));\n"
     "  float3 up = normalize(cross(right, fwd));\n"
     "  constexpr sampler texSampler(coord::normalized, filter::linear, address::clamp_to_edge);\n"
     "  float3 total = float3(0.0);\n"
     "  for (int sy = 0; sy < 2; ++sy) {\n"
     "    for (int sx = 0; sx < 2; ++sx) {\n"
     "      float2 jitter = (float2(sx, sy) + 0.5) * 0.5;\n"
     "      float2 screen = (float2(gid) + jitter) / float2(u.width, u.height);\n"
     "      float3 rd = rayDirection(screen, fwd, right, up, u.viewAspect);\n"
     "      float3 col = environment(rd);\n"
     "      float tf = floorDepth(cam, rd);\n"
     "      Hit h = intersectMembrane(cam, rd, u);\n"
     "      bool floorVisible = tf > 0.01 && (!h.ok || tf < h.t);\n"
     "      if (floorVisible) col = floorColor(cam + rd * tf);\n"
     "      if (h.ok && (!floorVisible || h.t < tf)) {\n"
     "        float3 base = float3(0.15);\n"
     "        if (!h.back) {\n"
     "          float4 tex = src.sample(texSampler, clamp(h.uv, float2(0.0), float2(1.0)));\n"
     "          float3 rgb = float3(tex.b, tex.g, tex.r);\n"
     "          base = float3(dot(rgb, float3(0.299, 0.587, 0.114)));\n"
     "        }\n"
     "        col = shadeSurface(base, h.n, rd, h.back);\n"
     "      }\n"
     "      float2 q = screen - 0.5;\n"
     "      col *= 1.0 - dot(q, q) * 0.12;\n"
     "      total += col;\n"
     "    }\n"
     "  }\n"
     "  total *= 0.25;\n"
     "  dst.write(float4(total, 1.0), gid);\n"
     "}\n";

void mygo_membrane_draw(void *devicePtr, void *commandBufferPtr, void *targetPtr,
                        void *sourcePtr, int width, int height,
                        int sourceWidth, int sourceHeight,
                        float camX, float camY, float camZ, float yaw, float pitch,
                        float waveTime, float amplitude, float viewAspect) {
  @autoreleasepool {
    id<MTLDevice> device = (__bridge id<MTLDevice>)devicePtr;
    dispatch_once(&mygo_membrane_once, ^{
      NSError *error = nil;
      id<MTLLibrary> library = [device newLibraryWithSource:mygo_membrane_source
                                                   options:nil
                                                     error:&error];
      if (!library) {
        NSLog(@"mygo layer-membrane: %@", error);
        return;
      }
      id<MTLFunction> function = [library newFunctionWithName:@"membrane"];
      NSError *pipelineError = nil;
      mygo_membrane_pipeline = [device newComputePipelineStateWithFunction:function
                                                                    error:&pipelineError];
      if (!mygo_membrane_pipeline) NSLog(@"mygo layer-membrane: %@", pipelineError);
    });
    if (!mygo_membrane_pipeline || !commandBufferPtr || !targetPtr || !sourcePtr ||
        width <= 0 || height <= 0 || sourceWidth <= 0 || sourceHeight <= 0) return;

    id<MTLCommandBuffer> commandBuffer = (__bridge id<MTLCommandBuffer>)commandBufferPtr;
    id<MTLTexture> target = (__bridge id<MTLTexture>)targetPtr;
    id<MTLTexture> source = (__bridge id<MTLTexture>)sourcePtr;
    id<MTLComputeCommandEncoder> encoder = [commandBuffer computeCommandEncoder];
    struct {
      float width, height, sourceWidth, sourceHeight;
      float camX, camY, camZ, yaw, pitch;
      float waveTime, amplitude, viewAspect;
    } uniforms = {
      (float)width, (float)height, (float)sourceWidth, (float)sourceHeight,
      camX, camY, camZ, yaw, pitch,
      waveTime, amplitude, MAX(viewAspect, 0.01f),
    };
    [encoder setComputePipelineState:mygo_membrane_pipeline];
    [encoder setTexture:source atIndex:0];
    [encoder setTexture:target atIndex:1];
    [encoder setBytes:&uniforms length:sizeof(uniforms) atIndex:0];
    NSUInteger tw = mygo_membrane_pipeline.threadExecutionWidth;
    NSUInteger th = MAX((NSUInteger)1,
        MIN((NSUInteger)8, mygo_membrane_pipeline.maxTotalThreadsPerThreadgroup / tw));
    [encoder dispatchThreads:MTLSizeMake((NSUInteger)width, (NSUInteger)height, 1)
        threadsPerThreadgroup:MTLSizeMake(tw, th, 1)];
    [encoder endEncoding];
  }
}
