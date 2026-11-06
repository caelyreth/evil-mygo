#ifndef MYGO_EXAMPLE_MEMBRANE_DRAW_H
#define MYGO_EXAMPLE_MEMBRANE_DRAW_H

void mygo_membrane_draw(void *device, void *commandBuffer, void *target,
                        void *source, int width, int height,
                        int sourceWidth, int sourceHeight,
                        float camX, float camY, float camZ, float yaw, float pitch,
                        float waveTime, float amplitude, float viewAspect);

#endif
