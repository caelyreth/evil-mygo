#ifndef MYGO_EXAMPLE_LAYER_DRAW_H
#define MYGO_EXAMPLE_LAYER_DRAW_H

void mygo_draw_layer(void *device, void *commandBuffer, void *target,
                     void *source, int width, int height,
                     int sourceWidth, int sourceHeight,
                     float time, float energy);

#endif
