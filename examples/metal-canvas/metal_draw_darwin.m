#import <Foundation/Foundation.h>
#import <Metal/Metal.h>

#include "metal_draw.h"

static id<MTLComputePipelineState> mygo_pipeline;
static dispatch_once_t mygo_once;

static NSString *mygo_source =
    @"#include <metal_stdlib>\n"
     "using namespace metal;\n"
     "struct U { float2 size; float time; float energy; float tint; };\n"
     "kernel void fill(texture2d<float, access::write> out [[texture(0)]],\n"
     "                 constant U &u [[buffer(0)]],\n"
     "                 uint2 gid [[thread_position_in_grid]]) {\n"
     "  if (gid.x >= uint(u.size.x) || gid.y >= uint(u.size.y)) return;\n"
     "  float2 p = (float2(gid) + 0.5) / max(u.size, float2(1.0));\n"
     "  float2 c = p - 0.5;\n"
     "  float r = length(c);\n"
     "  float a = atan2(c.y, c.x);\n"
     "  float wave = sin(r * 42.0 - u.time * 2.1 + sin(a * 5.0) * 2.0);\n"
     "  float glow = exp(-r * 5.5) * (0.55 + 0.45 * wave);\n"
     "  float ring = smoothstep(0.035, 0.0, abs(r - 0.22 - 0.025 * sin(u.time * 0.7)));\n"
     "  float3 bg = mix(float3(0.015, 0.055, 0.065), float3(0.025, 0.15, 0.14), p.y);\n"
     "  float3 color = bg + float3(0.12, 0.65, 0.50) * glow * u.energy\n"
     "               + float3(0.95, 0.42, 0.16) * ring * u.tint;\n"
     "  float spark = smoothstep(0.78, 1.0, sin((p.x + p.y) * 90.0 + u.time * 1.7));\n"
     "  color += spark * float3(0.7, 0.8, 0.55) * (1.0 - smoothstep(0.0, 0.75, r)) * 0.08;\n"
     "  out.write(float4(color, 1.0), gid);\n"
     "}\n";

void mygo_draw_metal(void *devicePtr, void *commandBufferPtr, void *texturePtr,
                     int width, int height, float time, float energy, float tint) {
  @autoreleasepool {
    id<MTLDevice> device = (__bridge id<MTLDevice>)devicePtr;
    dispatch_once(&mygo_once, ^{
      NSError *error = nil;
      id<MTLLibrary> library = [device newLibraryWithSource:mygo_source
                                                    options:nil
                                                      error:&error];
      if (!library) {
        NSLog(@"mygo metal example: %@", error);
        return;
      }
      id<MTLFunction> function = [library newFunctionWithName:@"fill"];
      NSError *pipelineError = nil;
      mygo_pipeline = [device newComputePipelineStateWithFunction:function
                                                            error:&pipelineError];
      if (!mygo_pipeline) {
        NSLog(@"mygo metal example: %@", pipelineError);
      }
    });
    if (!mygo_pipeline || !devicePtr || !commandBufferPtr || !texturePtr || width <= 0 || height <= 0) {
      return;
    }
    id<MTLCommandBuffer> commandBuffer = (__bridge id<MTLCommandBuffer>)commandBufferPtr;
    id<MTLTexture> texture = (__bridge id<MTLTexture>)texturePtr;
    id<MTLComputeCommandEncoder> encoder = [commandBuffer computeCommandEncoder];
    struct {
      float width;
      float height;
      float time;
      float energy;
      float tint;
    } uniforms = {(float)width, (float)height, time, energy, tint};
    [encoder setComputePipelineState:mygo_pipeline];
    [encoder setTexture:texture atIndex:0];
    [encoder setBytes:&uniforms length:sizeof(uniforms) atIndex:0];
    NSUInteger threadWidth = mygo_pipeline.threadExecutionWidth;
    NSUInteger threadHeight = MAX((NSUInteger)1, mygo_pipeline.maxTotalThreadsPerThreadgroup / threadWidth);
    [encoder dispatchThreads:MTLSizeMake((NSUInteger)width, (NSUInteger)height, 1)
        threadsPerThreadgroup:MTLSizeMake(threadWidth, threadHeight, 1)];
    [encoder endEncoding];
  }
}
