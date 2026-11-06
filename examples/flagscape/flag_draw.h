#ifndef MYGO_EXAMPLE_FLAG_DRAW_H
#define MYGO_EXAMPLE_FLAG_DRAW_H

void mygo_flag_draw(void *device, void *commandBuffer, void *target,
                    void *source, int width, int height,
                    int sourceWidth, int sourceHeight,
                    float time, float camX, float camY, float camZ,
                    float yaw, float pitch, float wind, float light);

#endif
