#import "wallpaper.h"
#import <QuartzCore/QuartzCore.h>

#include "bridge.h"

static NSString *const kOriginalsKey = @"originalWallpapers";

static NSString *snapshotDir(void) {
	static NSString *dir;
	if (!dir) {
		NSURL *base = [NSFileManager.defaultManager URLsForDirectory:NSApplicationSupportDirectory
		                                                   inDomains:NSUserDomainMask]
		                  .firstObject;
		dir = [base URLByAppendingPathComponent:@"ascii-wallpaper/snapshots" isDirectory:YES].path;
		[NSFileManager.defaultManager createDirectoryAtPath:dir withIntermediateDirectories:YES attributes:nil error:nil];
	}
	return dir;
}

static uint32_t screenIDOf(NSScreen *s) {
	return [s.deviceDescription[@"NSScreenNumber"] unsignedIntValue];
}

static NSString *keyOf(NSScreen *s) {
	return [NSString stringWithFormat:@"%u", screenIDOf(s)];
}

static BOOL isSnapshot(NSURL *url) {
	return url.isFileURL && [url.path.stringByStandardizingPath hasPrefix:snapshotDir().stringByStandardizingPath];
}

// Remember the screen's current picture as the one to restore, unless it is
// one of our snapshots (left behind by a crash). Called again before every
// snapshot so a picture the user picks while we run wins.
static void rememberOriginal(NSScreen *s) {
	NSURL *cur = [NSWorkspace.sharedWorkspace desktopImageURLForScreen:s];
	if (!cur || isSnapshot(cur)) {
		return;
	}
	NSUserDefaults *d = NSUserDefaults.standardUserDefaults;
	NSMutableDictionary *orig = [[d dictionaryForKey:kOriginalsKey] mutableCopy] ?: [NSMutableDictionary dictionary];
	if ([orig[keyOf(s)] isEqual:cur.absoluteString]) {
		return;
	}
	orig[keyOf(s)] = cur.absoluteString;
	[d setObject:orig forKey:kOriginalsKey];
}

void awRememberOriginals(void) {
	for (NSScreen *s in NSScreen.screens) {
		rememberOriginal(s);
	}
}

static NSScreen *screenWithID(uint32_t screenID) {
	for (NSScreen *s in NSScreen.screens) {
		if (screenIDOf(s) == screenID) {
			return s;
		}
	}
	return nil;
}

static void applySnapshot(uint32_t screenID, NSString *path) {
	NSScreen *s = screenWithID(screenID);
	if (!s) {
		return;
	}
	rememberOriginal(s);
	NSError *err = nil;
	NSDictionary *opts = @{
		NSWorkspaceDesktopImageScalingKey : @(NSImageScaleProportionallyUpOrDown),
		NSWorkspaceDesktopImageAllowClippingKey : @YES,
	};
	if (![NSWorkspace.sharedWorkspace setDesktopImageURL:[NSURL fileURLWithPath:path]
	                                           forScreen:s
	                                             options:opts
	                                               error:&err]) {
		NSLog(@"ascii-wallpaper: set desktop picture: %@", err);
	}
}

void awSnapshot(NSView *view, uint32_t screenID, CFTimeInterval minInterval) {
	static NSMutableDictionary<NSNumber *, NSNumber *> *last;
	if (!last) {
		last = [NSMutableDictionary dictionary];
	}
	CFTimeInterval now = CACurrentMediaTime();
	NSNumber *prev = last[@(screenID)];
	if (prev && now - prev.doubleValue < minInterval) {
		return;
	}
	last[@(screenID)] = @(now);

	NSBitmapImageRep *rep = [view bitmapImageRepForCachingDisplayInRect:view.bounds];
	if (!rep) {
		return;
	}
	[view cacheDisplayInRect:view.bounds toBitmapImageRep:rep];
	// Alternate between two files per screen. The system caches desktop
	// pictures by URL, so each update needs a URL different from the current
	// one; and Mission Control and other Spaces keep pointing at whichever file
	// was current when they last looked, so no file may ever disappear while
	// we run — a missing file makes them fall back to the original picture.
	static NSMutableDictionary<NSNumber *, NSNumber *> *flip;
	if (!flip) {
		flip = [NSMutableDictionary dictionary];
	}
	BOOL b = !flip[@(screenID)].boolValue;
	flip[@(screenID)] = @(b);
	NSString *path = [snapshotDir()
	    stringByAppendingPathComponent:[NSString stringWithFormat:@"snapshot-%u-%c.png", screenID, b ? 'b' : 'a']];
	dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^{
		NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
		if (![png writeToFile:path atomically:YES]) {
			return;
		}
		dispatch_async(dispatch_get_main_queue(), ^{
			applySnapshot(screenID, path);
		});
	});
}

void awRestoreWallpaper(void) {
	@autoreleasepool {
		NSUserDefaults *d = NSUserDefaults.standardUserDefaults;
		NSDictionary *orig = [d dictionaryForKey:kOriginalsKey];
		NSString *fallback = orig.allValues.firstObject;
		for (NSScreen *s in NSScreen.screens) {
			// Leave the screen alone if the user already picked something else.
			if (!isSnapshot([NSWorkspace.sharedWorkspace desktopImageURLForScreen:s])) {
				continue;
			}
			NSString *str = orig[keyOf(s)] ?: fallback;
			if (!str) {
				continue;
			}
			NSError *err = nil;
			if (![NSWorkspace.sharedWorkspace setDesktopImageURL:[NSURL URLWithString:str]
			                                           forScreen:s
			                                             options:@{}
			                                               error:&err]) {
				NSLog(@"ascii-wallpaper: restore desktop picture: %@", err);
			}
		}
		// The snapshot files stay: other Spaces may still reference them, and
		// deleting them would leave those Spaces on a blank fallback.
		[d synchronize];
	}
}
