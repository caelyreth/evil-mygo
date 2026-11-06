#ifndef MYGO_EXAMPLE_METAL_DRAW_H
#define MYGO_EXAMPLE_METAL_DRAW_H

void mygo_draw_metal(void *device, void *commandBuffer, void *texture,
                     int width, int height, float time, float energy, float tint);

#endif
