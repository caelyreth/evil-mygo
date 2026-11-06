#import <Foundation/Foundation.h>
#import <Metal/Metal.h>

#include "layer_draw.h"

static id<MTLComputePipelineState> mygo_layer_surface_pipeline;
static dispatch_once_t mygo_layer_surface_once;

static NSString *mygo_layer_surface_source =
    @"#include <metal_stdlib>\n"
     "using namespace metal;\n"
     "struct U { float width, height, sourceWidth, sourceHeight; };\n"
     "kernel void present(texture2d<float> src [[texture(0)]],\n"
     "                    texture2d<float, access::write> dst [[texture(1)]],\n"
     "                    constant U &u [[buffer(0)]],\n"
     "                    uint2 gid [[thread_position_in_grid]]) {\n"
     "  if (gid.x >= uint(u.width) || gid.y >= uint(u.height)) return;\n"
     "  constexpr sampler s(coord::normalized, filter::linear, address::clamp_to_edge);\n"
     "  float2 uv = (float2(gid) + 0.5) / float2(u.width, u.height);\n"
     "  float4 c = src.sample(s, uv);\n"
     "  c = float4(c.b, c.g, c.r, c.a);\n"
     "  float vignette = 1.0 - 0.10 * dot(uv - 0.5, uv - 0.5);\n"
     "  dst.write(float4(c.rgb * vignette, c.a), gid);\n"
     "}\n";

void mygo_layer_present(void *devicePtr, void *commandBufferPtr, void *targetPtr,
                        void *sourcePtr, int width, int height,
                        int sourceWidth, int sourceHeight) {
  @autoreleasepool {
    id<MTLDevice> device = (__bridge id<MTLDevice>)devicePtr;
    dispatch_once(&mygo_layer_surface_once, ^{
      NSError *error = nil;
      id<MTLLibrary> library = [device newLibraryWithSource:mygo_layer_surface_source
                                                   options:nil
                                                     error:&error];
      if (!library) {
        NSLog(@"mygo layer-surface: %@", error);
        return;
      }
      id<MTLFunction> function = [library newFunctionWithName:@"present"];
      NSError *pipelineError = nil;
      mygo_layer_surface_pipeline = [device newComputePipelineStateWithFunction:function error:&pipelineError];
      if (!mygo_layer_surface_pipeline) NSLog(@"mygo layer-surface: %@", pipelineError);
    });
    if (!mygo_layer_surface_pipeline || !commandBufferPtr || !targetPtr || !sourcePtr ||
        width <= 0 || height <= 0 || sourceWidth <= 0 || sourceHeight <= 0) return;
    id<MTLCommandBuffer> commandBuffer = (__bridge id<MTLCommandBuffer>)commandBufferPtr;
    id<MTLTexture> target = (__bridge id<MTLTexture>)targetPtr;
    id<MTLTexture> source = (__bridge id<MTLTexture>)sourcePtr;
    id<MTLComputeCommandEncoder> encoder = [commandBuffer computeCommandEncoder];
    struct { float width, height, sourceWidth, sourceHeight; } u =
        {(float)width, (float)height, (float)sourceWidth, (float)sourceHeight};
    [encoder setComputePipelineState:mygo_layer_surface_pipeline];
    [encoder setTexture:source atIndex:0];
    [encoder setTexture:target atIndex:1];
    [encoder setBytes:&u length:sizeof(u) atIndex:0];
    NSUInteger tw = mygo_layer_surface_pipeline.threadExecutionWidth;
    NSUInteger th = MAX((NSUInteger)1, MIN((NSUInteger)8, mygo_layer_surface_pipeline.maxTotalThreadsPerThreadgroup / tw));
    [encoder dispatchThreads:MTLSizeMake((NSUInteger)width, (NSUInteger)height, 1)
        threadsPerThreadgroup:MTLSizeMake(tw, th, 1)];
    [encoder endEncoding];
  }
}
