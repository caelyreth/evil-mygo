#ifndef MYGO_LAYER_SURFACE_PRESENT_H
#define MYGO_LAYER_SURFACE_PRESENT_H

void mygo_layer_present(void *device, void *commandBuffer, void *target,
                        void *source, int width, int height,
                        int sourceWidth, int sourceHeight);

#endif
