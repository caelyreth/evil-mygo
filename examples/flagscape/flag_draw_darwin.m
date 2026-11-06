#import <Foundation/Foundation.h>
#import <Metal/Metal.h>

#include "flag_draw.h"

static id<MTLComputePipelineState> mygo_panel_pipeline;
static dispatch_once_t mygo_panel_once;

static NSString *mygo_panel_source =
    @"#include <metal_stdlib>\n"
     "using namespace metal;\n"
     "struct U { float width, height, sourceWidth, sourceHeight; float camX, camY, camZ, yaw, pitch; };\n"
     "constant float panelY = 0.28;\n"
     "constant float thickness = 0.025;\n"
     "float intersectPlane(float3 cam, float3 rd, float z) {\n"
     "  return abs(rd.z) > 0.02 ? (z - cam.z) / rd.z : -1.0;\n"
     "}\n"
     "kernel void panel(texture2d<float> src [[texture(0)]],\n"
     "                  texture2d<float, access::write> dst [[texture(1)]],\n"
     "                  constant U &u [[buffer(0)]],\n"
     "                  uint2 gid [[thread_position_in_grid]]) {\n"
     "  if (gid.x >= uint(u.width) || gid.y >= uint(u.height)) return;\n"
     "  constexpr sampler texSampler(coord::normalized, filter::linear, address::clamp_to_edge);\n"
     "  float2 uv = (float2(gid) + 0.5) / float2(u.width, u.height);\n"
     "  float2 ndc = uv * 2.0 - 1.0; ndc.y = -ndc.y;\n"
     "  float3 cam = float3(u.camX, u.camY, u.camZ);\n"
     "  float3 fwd = normalize(float3(sin(u.yaw) * cos(u.pitch), sin(u.pitch), -cos(u.yaw) * cos(u.pitch)));\n"
     "  float3 right = normalize(cross(fwd, float3(0.0, 1.0, 0.0)));\n"
     "  float3 up = normalize(cross(right, fwd));\n"
     "  float aspect = float(u.width) / float(u.height);\n"
     "  float3 rd = normalize(fwd + right * ndc.x * 0.74 * aspect + up * ndc.y * 0.74);\n"
     "  float h = smoothstep(-0.22, 0.68, rd.y);\n"
     "  float3 col = mix(float3(0.70), float3(0.95), h);\n"
     "  float sun = pow(max(dot(rd, normalize(float3(0.42, 0.34, 0.84))), 0.0), 80.0);\n"
     "  col += float3(0.12) * sun;\n"
     "  const float groundY = -0.72;\n"
     "  float best = 1e9;\n"
     "  if (rd.y < -0.001) {\n"
     "    float tg = (groundY - cam.y) / rd.y;\n"
     "    if (tg > 0.0 && tg < best) {\n"
     "      float3 p = cam + rd * tg; best = tg;\n"
     "      float2 g = abs(fract(p.xz * 4.0) - 0.5);\n"
     "      float grid = 1.0 - smoothstep(0.0, 0.025, min(g.x, g.y));\n"
     "      col = mix(float3(0.43), float3(0.73), 1.0 - smoothstep(0.0, 3.5, length(p.xz)));\n"
     "      col += float3(0.12) * grid * exp(-abs(p.x) * 0.6);\n"
     "      float shadow = exp(-pow(length(p.xz - float2(0.0, 0.0)) / 1.25, 2.0));\n"
     "      col *= 1.0 - shadow * 0.10;\n"
     "    }\n"
     "  }\n"
     "  float sourceAspect = float(u.sourceWidth) / max(float(u.sourceHeight), 1.0);\n"
     "  float halfH = 0.68, halfW = halfH * sourceAspect;\n"
     "  float tf = intersectPlane(cam, rd, thickness);\n"
     "  float tb = intersectPlane(cam, rd, -thickness);\n"
     "  float tp = 1e9; bool back = false;\n"
     "  if (tf > 0.01) { tp = tf; }\n"
     "  if (tb > 0.01 && tb < tp) { tp = tb; back = true; }\n"
     "  float ts = 1e9; float3 sn = float3(0.0);\n"
     "  if (abs(rd.x) > 0.001) {\n"
     "    float x = (rd.x > 0.0) ? halfW : -halfW;\n"
     "    float t = (x - cam.x) / rd.x; float3 p = cam + rd * t;\n"
     "    if (t > 0.01 && t < ts && abs(p.y - panelY) <= halfH && abs(p.z) <= thickness) { ts = t; sn = float3(sign(x), 0.0, 0.0); }\n"
     "  }\n"
     "  if (abs(rd.y) > 0.001) {\n"
     "    float y = (rd.y > 0.0) ? panelY + halfH : panelY - halfH;\n"
     "    float t = (y - cam.y) / rd.y; float3 p = cam + rd * t;\n"
     "    if (t > 0.01 && t < ts && abs(p.x) <= halfW && abs(p.z) <= thickness) { ts = t; sn = float3(0.0, sign(y - panelY), 0.0); }\n"
     "  }\n"
     "  bool side = false;\n"
     "  if (ts < tp) { tp = ts; back = false; side = true; }\n"
     "  if (tp > 0.01 && tp < best) {\n"
     "    float3 p = cam + rd * tp;\n"
     "    if (side) {\n"
     "      float light = 0.28 + 0.26 * max(dot(sn, normalize(float3(-0.55, 0.7, 0.45))), 0.0);\n"
     "      col = mix(col, float3(light), smoothstep(0.0, 0.001, min(thickness - abs(p.z), 0.01)));\n"
     "    } else {\n"
     "      float2 fuv = float2((p.x + halfW) / (2.0 * halfW), (halfH - (p.y - panelY)) / (2.0 * halfH));\n"
     "      if (back) fuv.x = 1.0 - fuv.x;\n"
     "      float4 tex = src.sample(texSampler, clamp(fuv, float2(0.0), float2(1.0)));\n"
     "      tex = float4(tex.b, tex.g, tex.r, tex.a);\n"
     "      float3 n = back ? float3(0.0, 0.0, -1.0) : float3(0.0, 0.0, 1.0);\n"
     "      float3 L = normalize(float3(-0.55, 0.70, 0.45));\n"
     "      float3 V = -rd;\n"
     "      float diffuse = max(dot(n, L), 0.0);\n"
     "      float3 Hv = normalize(L + V);\n"
     "      float spec = pow(max(dot(n, Hv), 0.0), 36.0) * (back ? 0.025 : 0.08);\n"
     "      float edge = smoothstep(halfW + 0.0015, halfW - 0.0005, abs(p.x)) * smoothstep(halfH + 0.0015, halfH - 0.0005, abs(p.y - panelY));\n"
     "      float albedo = dot(tex.rgb, float3(0.299, 0.587, 0.114));\n"
     "      float shade = albedo * (0.48 + 0.42 * diffuse * (back ? 0.65 : 1.0)) + spec;\n"
     "      col = mix(col, float3(shade), tex.a * edge);\n"
     "    }\n"
     "    best = tp;\n"
     "  }\n"
     "  float2 q = uv - 0.5;\n"
     "  col *= 1.0 - dot(q, q) * 0.28;\n"
     "  col = float3(pow(max(dot(col, float3(0.299, 0.587, 0.114)), 0.0), 0.88));\n"
     "  dst.write(float4(col, 1.0), gid);\n"
     "}\n";

void mygo_panel_draw(void *devicePtr, void *commandBufferPtr, void *targetPtr,
                     void *sourcePtr, int width, int height,
                     int sourceWidth, int sourceHeight,
                     float camX, float camY, float camZ, float yaw, float pitch) {
  @autoreleasepool {
    id<MTLDevice> device = (__bridge id<MTLDevice>)devicePtr;
    dispatch_once(&mygo_panel_once, ^{
      NSError *error = nil;
      id<MTLLibrary> library = [device newLibraryWithSource:mygo_panel_source
                                                   options:nil
                                                     error:&error];
      if (!library) {
        NSLog(@"mygo flagscape: %@", error);
        return;
      }
      id<MTLFunction> function = [library newFunctionWithName:@"panel"];
      NSError *pipelineError = nil;
      mygo_panel_pipeline = [device newComputePipelineStateWithFunction:function error:&pipelineError];
      if (!mygo_panel_pipeline) NSLog(@"mygo flagscape: %@", pipelineError);
    });
    if (!mygo_panel_pipeline || !commandBufferPtr || !targetPtr || !sourcePtr ||
        width <= 0 || height <= 0 || sourceWidth <= 0 || sourceHeight <= 0) return;
    id<MTLCommandBuffer> commandBuffer = (__bridge id<MTLCommandBuffer>)commandBufferPtr;
    id<MTLTexture> target = (__bridge id<MTLTexture>)targetPtr;
    id<MTLTexture> source = (__bridge id<MTLTexture>)sourcePtr;
    id<MTLComputeCommandEncoder> encoder = [commandBuffer computeCommandEncoder];
    struct {
      float width, height, sourceWidth, sourceHeight;
      float camX, camY, camZ, yaw, pitch;
    } uniforms = {(float)width, (float)height, (float)sourceWidth, (float)sourceHeight,
                  camX, camY, camZ, yaw, pitch};
    [encoder setComputePipelineState:mygo_panel_pipeline];
    [encoder setTexture:source atIndex:0];
    [encoder setTexture:target atIndex:1];
    [encoder setBytes:&uniforms length:sizeof(uniforms) atIndex:0];
    NSUInteger threadWidth = mygo_panel_pipeline.threadExecutionWidth;
    NSUInteger threadHeight = MAX((NSUInteger)1, MIN((NSUInteger)8, mygo_panel_pipeline.maxTotalThreadsPerThreadgroup / threadWidth));
    [encoder dispatchThreads:MTLSizeMake((NSUInteger)width, (NSUInteger)height, 1)
        threadsPerThreadgroup:MTLSizeMake(threadWidth, threadHeight, 1)];
    [encoder endEncoding];
  }
}
