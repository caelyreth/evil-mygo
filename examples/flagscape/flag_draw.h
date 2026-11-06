#ifndef MYGO_EXAMPLE_FLAG_DRAW_H
#define MYGO_EXAMPLE_FLAG_DRAW_H

void mygo_panel_draw(void *device, void *commandBuffer, void *target,
                     void *source, int width, int height,
                     int sourceWidth, int sourceHeight,
                     float camX, float camY, float camZ, float yaw, float pitch);

#endif
