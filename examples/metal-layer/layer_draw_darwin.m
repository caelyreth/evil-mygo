#import <Foundation/Foundation.h>
#import <Metal/Metal.h>

#include "layer_draw.h"

static id<MTLComputePipelineState> mygo_layer_pipeline;
static dispatch_once_t mygo_layer_once;

static NSString *mygo_layer_source =
    @"#include <metal_stdlib>\n"
     "using namespace metal;\n"
     "struct U { float2 size; float2 srcSize; float time; float energy; };\n"
     "kernel void warp(texture2d<float, access::read> src [[texture(0)]],\n"
     "                 texture2d<float, access::write> dst [[texture(1)]],\n"
     "                 constant U &u [[buffer(0)]],\n"
     "                 uint2 gid [[thread_position_in_grid]]) {\n"
     "  if (gid.x >= uint(u.size.x) || gid.y >= uint(u.size.y)) return;\n"
     "  float2 uv = (float2(gid) + 0.5) / max(u.size, float2(1));\n"
     "  float2 p = uv - 0.5;\n"
     "  float wave = sin(p.y * 18.0 + u.time * 0.9) * 0.013 * u.energy;\n"
     "  float cross = sin(p.x * 13.0 - u.time * 0.7) * 0.009 * u.energy;\n"
     "  float2 q = uv + float2(wave, cross);\n"
     "  int2 ip = clamp(int2(q * u.srcSize), int2(0), max(int2(u.srcSize) - 1, int2(0)));\n"
     "  float4 c = src.read(uint2(ip));\n"
     "  c = float4(c.b, c.g, c.r, c.a); // Canvas textures are BGRA8.\n"
     "  float vignette = smoothstep(0.18, 0.72, length(p));\n"
     "  c.rgb *= 1.0 - vignette * 0.22 * u.energy;\n"
     "  float scan = smoothstep(0.78, 1.0, sin((uv.y + uv.x) * 90.0 + u.time)) * 0.035;\n"
     "  c.rgb += float3(0.35, 0.55, 0.42) * scan * u.energy * (1.0 - vignette);\n"
     "  dst.write(c, gid);\n"
     "}\n";

void mygo_draw_layer(void *devicePtr, void *commandBufferPtr, void *targetPtr,
                     void *sourcePtr, int width, int height,
                     int sourceWidth, int sourceHeight, float time, float energy) {
  @autoreleasepool {
    id<MTLDevice> device = (__bridge id<MTLDevice>)devicePtr;
    dispatch_once(&mygo_layer_once, ^{
      NSError *error = nil;
      id<MTLLibrary> library = [device newLibraryWithSource:mygo_layer_source
                                                    options:nil
                                                      error:&error];
      if (!library) {
        NSLog(@"mygo metal layer example: %@", error);
        return;
      }
      id<MTLFunction> function = [library newFunctionWithName:@"warp"];
      NSError *pipelineError = nil;
      mygo_layer_pipeline = [device newComputePipelineStateWithFunction:function
                                                                  error:&pipelineError];
      if (!mygo_layer_pipeline) {
        NSLog(@"mygo metal layer example: %@", pipelineError);
      }
    });
    if (!mygo_layer_pipeline || !commandBufferPtr || !targetPtr || !sourcePtr || width <= 0 || height <= 0 || sourceWidth <= 0 || sourceHeight <= 0) {
      return;
    }
    id<MTLCommandBuffer> commandBuffer = (__bridge id<MTLCommandBuffer>)commandBufferPtr;
    id<MTLTexture> target = (__bridge id<MTLTexture>)targetPtr;
    id<MTLTexture> source = (__bridge id<MTLTexture>)sourcePtr;
    id<MTLComputeCommandEncoder> encoder = [commandBuffer computeCommandEncoder];
    struct {
      float width;
      float height;
      float sourceWidth;
      float sourceHeight;
      float time;
      float energy;
    } uniforms = {(float)width, (float)height, (float)sourceWidth, (float)sourceHeight, time, energy};
    [encoder setComputePipelineState:mygo_layer_pipeline];
    [encoder setTexture:source atIndex:0];
    [encoder setTexture:target atIndex:1];
    [encoder setBytes:&uniforms length:sizeof(uniforms) atIndex:0];
    NSUInteger threadWidth = mygo_layer_pipeline.threadExecutionWidth;
    NSUInteger threadHeight = MAX((NSUInteger)1, mygo_layer_pipeline.maxTotalThreadsPerThreadgroup / threadWidth);
    [encoder dispatchThreads:MTLSizeMake((NSUInteger)width, (NSUInteger)height, 1)
        threadsPerThreadgroup:MTLSizeMake(threadWidth, threadHeight, 1)];
    [encoder endEncoding];
  }
}
