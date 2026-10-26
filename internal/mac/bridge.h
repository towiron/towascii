#ifndef AW_BRIDGE_H
#define AW_BRIDGE_H

#include <stdint.h>

enum {
	AW_KIND_TILE = 0,
	AW_KIND_DOT = 1,
	AW_KIND_GLYPH = 2,
};

typedef struct {
	uint32_t rgba;
	uint16_t col;
	uint16_t row;
	uint8_t glyph;
	uint8_t kind;
} AWItem;

typedef struct {
	double cellW;
	double cellH;
	double dotSize;
	int cols;
	int rows;
} AWLayout;

enum {
	AW_ACTION_BLACK = 0,
	AW_ACTION_WHITE = 1,
	AW_ACTION_PAUSE = 2,
	AW_ACTION_QUIT = 3,
};

// awRun never returns: it owns the main thread until the app terminates.
void awRun(int white, int paused);
void awApply(int white, int paused);
void awQuit(void);
// awQuitAsync may be called from any thread.
void awQuitAsync(void);
// awRestoreWallpaper puts back the desktop picture that was set before the
// aquarium replaced it with snapshots; it is a no-op if there is nothing to undo.
void awRestoreWallpaper(void);

#endif
