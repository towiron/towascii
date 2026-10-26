#ifndef AW_WALLPAPER_H
#define AW_WALLPAPER_H

#import <Cocoa/Cocoa.h>

// Mission Control and Space transitions draw the system desktop picture, not
// desktop-level windows, so the system wallpaper is kept in sync with a still
// of the aquarium and the user's own picture is restored on exit.
void awRememberOriginals(void);
void awSnapshot(NSView *view, uint32_t screenID, CFTimeInterval minInterval);

#endif
