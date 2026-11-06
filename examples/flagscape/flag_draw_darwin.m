#import <Foundation/Foundation.h>
#import <Metal/Metal.h>

#include "flag_draw.h"

static id<MTLComputePipelineState> mygo_flag_pipeline;
static dispatch_once_t mygo_flag_once;

static NSString *mygo_flag_source =
    @"#include <metal_stdlib>\n"
     "using namespace metal;\n"
     "struct U { float width, height, sourceWidth, sourceHeight; float time; float camX, camY, camZ; float yaw, pitch, wind, light; };\n"
     "float hash21(float2 p) {\n"
     "  p = fract(p * float2(123.34, 456.21));\n"
     "  p += dot(p, p + 45.32);\n"
     "  return fract(p.x * p.y);\n"
     "}\n"
     "float3 sky(float3 rd, float2 uv, float light) {\n"
     "  float h = smoothstep(-0.22, 0.62, rd.y);\n"
     "  float3 c = mix(float3(0.008, 0.026, 0.032), float3(0.025, 0.12, 0.13), h);\n"
     "  float sun = pow(max(dot(rd, normalize(float3(-0.48, 0.34, 0.81))), 0.0), 90.0);\n"
     "  c += float3(0.95, 0.38, 0.12) * sun * (0.45 + 0.55 * light);\n"
     "  float star = step(0.9975, hash21(floor(uv * float2(310.0, 190.0)) + floor(rd.xy * 18.0)));\n"
     "  c += star * float3(0.4, 0.72, 0.68) * (1.0 - h) * 0.55;\n"
     "  return c;\n"
     "}\n"
     "kernel void flag(texture2d<float> src [[texture(0)]],\n"
     "                 texture2d<float, access::write> dst [[texture(1)]],\n"
     "                 constant U &u [[buffer(0)]],\n"
     "                 uint2 gid [[thread_position_in_grid]]) {\n"
     "  if (gid.x >= uint(u.width) || gid.y >= uint(u.height)) return;\n"
     "  constexpr sampler textureSampler(coord::normalized, filter::linear, address::clamp_to_edge);\n"
     "  float2 uv = (float2(gid) + 0.5) / float2(u.width, u.height);\n"
     "  float2 ndc = uv * 2.0 - 1.0; ndc.y = -ndc.y;\n"
     "  float3 cam = float3(u.camX, u.camY, u.camZ);\n"
     "  float3 fwd = normalize(float3(sin(u.yaw) * cos(u.pitch), sin(u.pitch), -cos(u.yaw) * cos(u.pitch)));\n"
     "  float3 right = normalize(cross(fwd, float3(0.0, 1.0, 0.0)));\n"
     "  float3 up = normalize(cross(right, fwd));\n"
     "  float aspect = float(u.width) / float(u.height);\n"
     "  float3 rd = normalize(fwd + right * ndc.x * 0.74 * aspect + up * ndc.y * 0.74);\n"
     "  float3 col = sky(rd, uv, u.light);\n"
     "  float best = 1e9;\n"
     "  const float groundY = -0.72;\n"
     "  if (rd.y < -0.001) {\n"
     "    float t = (groundY - cam.y) / rd.y;\n"
     "    if (t > 0.0 && t < best) {\n"
     "      float3 p = cam + rd * t; best = t;\n"
     "      float2 g = abs(fract(p.xz * float2(4.0, 4.0)) - 0.5);\n"
     "      float grid = 1.0 - smoothstep(0.0, 0.025, min(g.x, g.y));\n"
     "      float horizon = exp(-abs(p.x) * 0.8);\n"
     "      col = mix(float3(0.012, 0.04, 0.045), float3(0.10, 0.18, 0.16), 1.0 - smoothstep(0.0, 3.0, length(p.xz)));\n"
     "      col += float3(0.12, 0.32, 0.27) * grid * (0.35 + 0.65 * u.light) * horizon;\n"
     "    }\n"
     "  }\n"
     "  float tFlag = 1e9;\n"
     "  if (abs(rd.z) > 0.0001) tFlag = -cam.z / rd.z;\n"
     "  if (tFlag > 0.0 && tFlag < best) {\n"
     "    float3 p = cam + rd * tFlag;\n"
     "    float sourceAspect = float(u.sourceWidth) / max(float(u.sourceHeight), 1.0);\n"
     "    float halfH = 0.68, halfW = halfH * sourceAspect;\n"
     "    float poleX = -halfW - 0.045;\n"
     "    if (abs(p.x) < halfW + 0.012 && abs(p.y) < halfH + 0.012) {\n"
     "      float2 fuv = float2((p.x + halfW) / (2.0 * halfW), (halfH - p.y) / (2.0 * halfH));\n"
     "      float phase = u.time * 1.55 + p.x * 5.2 + p.y * 1.7;\n"
     "      float wave = sin(phase) * 0.18 * u.wind;\n"
     "      float2 q = fuv + float2(wave * 0.25, sin(u.time * 0.7 + p.x * 3.0) * 0.018 * u.wind);\n"
     "      float4 tex = src.sample(textureSampler, clamp(q, float2(0.0), float2(1.0)));\n"
     "      tex = float4(tex.b, tex.g, tex.r, tex.a);\n"
     "      float3 n = normalize(float3(-cos(phase) * 5.2 * 0.18 * u.wind, -cos(phase + 1.7) * 1.7 * 0.18 * u.wind, 1.0));\n"
     "      float lit = clamp(dot(n, normalize(float3(-0.55, 0.68, 0.48))), 0.0, 1.0);\n"
     "      float3 shade = tex.rgb * (0.46 + 0.64 * lit * (0.55 + 0.45 * u.light));\n"
     "      shade += float3(0.06, 0.24, 0.18) * pow(1.0 - abs(dot(n, float3(0.0, 0.0, 1.0))), 3.0) * 0.5;\n"
     "      float edgeAA = smoothstep(halfW + 0.008, halfW - 0.002, abs(p.x)) * smoothstep(halfH + 0.008, halfH - 0.002, abs(p.y));\n"
     "      float clothEdge = 1.0 - smoothstep(0.975, 1.0, fuv.x + 0.02 * sin(u.time * 1.4 + fuv.y * 9.0));\n"
     "      col = mix(col, shade, tex.a * edgeAA * clothEdge);\n"
     "      best = tFlag;\n"
     "    }\n"
     "  }\n"
     "  float2 o = cam.xz, d = rd.xz;\n"
     "  float sourceAspect = float(u.sourceWidth) / max(float(u.sourceHeight), 1.0);\n"
     "  float poleX = -0.68 * sourceAspect - 0.045;\n"
     "  float2 ring = float2(poleX, 0.0);\n"
     "  float a = dot(d, d), b = 2.0 * dot(o - ring, d), c = dot(o - ring, o - ring) - 0.022 * 0.022;\n"
     "  float disc = b * b - 4.0 * a * c;\n"
     "  if (a > 0.0001 && disc > 0.0) {\n"
     "    float t = (-b - sqrt(disc)) / (2.0 * a);\n"
     "    float3 p = cam + rd * t;\n"
     "    if (t > 0.0 && t < best && p.y > groundY && p.y < 0.84) {\n"
     "      float3 metal = float3(0.38, 0.47, 0.43) + float3(0.42, 0.24, 0.08) * smoothstep(0.0, 0.8, p.y);\n"
     "      col = mix(metal, float3(1.0, 0.68, 0.34), 0.25 * u.light); best = t;\n"
     "    }\n"
     "  }\n"
     "  float2 sphere = float2(poleX, 0.80);\n"
     "  float3 oc = cam - float3(sphere.x, sphere.y, 0.0);\n"
     "  float bb = dot(oc, rd), cc = dot(oc, oc) - 0.052 * 0.052;\n"
     "  float sd = bb * bb - cc;\n"
     "  if (sd > 0.0) {\n"
     "    float t = -bb - sqrt(sd);\n"
     "    if (t > 0.0 && t < best) { col = float3(1.0, 0.78, 0.42) * (0.52 + 0.48 * u.light); best = t; }\n"
     "  }\n"
     "  float2 q = uv - 0.5;\n"
     "  col *= 1.0 - dot(q, q) * 0.34;\n"
     "  col = pow(max(col, 0.0), float3(0.86));\n"
     "  float gray = dot(col, float3(0.299, 0.587, 0.114));\n"
     "  col = float3(gray);\n"
     "  dst.write(float4(col, 1.0), gid);\n"
     "}\n";

void mygo_flag_draw(void *devicePtr, void *commandBufferPtr, void *targetPtr,
                    void *sourcePtr, int width, int height,
                    int sourceWidth, int sourceHeight,
                    float time, float camX, float camY, float camZ,
                    float yaw, float pitch, float wind, float light) {
  @autoreleasepool {
    id<MTLDevice> device = (__bridge id<MTLDevice>)devicePtr;
    dispatch_once(&mygo_flag_once, ^{
      NSError *error = nil;
      id<MTLLibrary> library = [device newLibraryWithSource:mygo_flag_source
                                                   options:nil
                                                     error:&error];
      if (!library) {
        NSLog(@"mygo flagscape: %@", error);
        return;
      }
      id<MTLFunction> function = [library newFunctionWithName:@"flag"];
      NSError *pipelineError = nil;
      mygo_flag_pipeline = [device newComputePipelineStateWithFunction:function error:&pipelineError];
      if (!mygo_flag_pipeline) NSLog(@"mygo flagscape: %@", pipelineError);
    });
    if (!mygo_flag_pipeline || !commandBufferPtr || !targetPtr || !sourcePtr ||
        width <= 0 || height <= 0 || sourceWidth <= 0 || sourceHeight <= 0) {
      return;
    }
    id<MTLCommandBuffer> commandBuffer = (__bridge id<MTLCommandBuffer>)commandBufferPtr;
    id<MTLTexture> target = (__bridge id<MTLTexture>)targetPtr;
    id<MTLTexture> source = (__bridge id<MTLTexture>)sourcePtr;
    id<MTLComputeCommandEncoder> encoder = [commandBuffer computeCommandEncoder];
    struct {
      float width, height, sourceWidth, sourceHeight;
      float time, camX, camY, camZ, yaw, pitch, wind, light;
    } uniforms = {(float)width, (float)height, (float)sourceWidth, (float)sourceHeight,
                  time, camX, camY, camZ, yaw, pitch, wind, light};
    [encoder setComputePipelineState:mygo_flag_pipeline];
    [encoder setTexture:source atIndex:0];
    [encoder setTexture:target atIndex:1];
    [encoder setBytes:&uniforms length:sizeof(uniforms) atIndex:0];
    NSUInteger threadWidth = mygo_flag_pipeline.threadExecutionWidth;
    NSUInteger threadHeight = MAX((NSUInteger)1, MIN((NSUInteger)8, mygo_flag_pipeline.maxTotalThreadsPerThreadgroup / threadWidth));
    [encoder dispatchThreads:MTLSizeMake((NSUInteger)width, (NSUInteger)height, 1)
        threadsPerThreadgroup:MTLSizeMake(threadWidth, threadHeight, 1)];
    [encoder endEncoding];
  }
}
