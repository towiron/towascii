#import <Cocoa/Cocoa.h>
#import <CoreText/CoreText.h>
#import <QuartzCore/QuartzCore.h>

#include "bridge.h"
#include "_cgo_export.h"
#import "wallpaper.h"

// Everything snaps to ~13x19pt cells, so a fish changes cell only a few times a
// second; 20 fps looks the same as 30 at two thirds of the cost.
static const double kFPS = 20.0;
static const NSTimeInterval kSnapshotEvery = 60;

static BOOL awWhite, awPaused, awSuspended;
static NSTimer *awTimer;
static CFTimeInterval awLast;
static NSMutableDictionary<NSNumber *, NSWindow *> *awWindows;

static void setFill(CGContextRef ctx, uint32_t rgba) {
	CGContextSetRGBFillColor(ctx, (rgba >> 24 & 0xFF) / 255.0, (rgba >> 16 & 0xFF) / 255.0,
	                         (rgba >> 8 & 0xFF) / 255.0, 1.0);
}

@interface AWView : NSView
@property(nonatomic) uint32_t screenID;
@end

@implementation AWView {
	AWLayout _layout;
	AWItem *_items;
	int _capacity;
	int _count;
	CTFontRef _font;
	CGGlyph _glyphs[128];
	CGFloat _advance, _baseline;
	CGRect *_rects;
	CGGlyph *_runGlyphs;
	CGPoint *_runPositions;
}

- (BOOL)isOpaque {
	return YES;
}

- (void)dealloc {
	free(_items);
	free(_rects);
	free(_runGlyphs);
	free(_runPositions);
	if (_font) {
		CFRelease(_font);
	}
}

- (void)relayout {
	NSSize size = self.bounds.size;
	AWLayout l = {0};
	goResize(self.screenID, size.width, size.height, &l);
	BOOL sameGrid = l.cols == _layout.cols && l.rows == _layout.rows;
	BOOL sameCell = l.cellW == _layout.cellW && l.cellH == _layout.cellH;
	_layout = l;
	if (!sameGrid) {
		_capacity = 3 * l.cols * l.rows;
		_count = 0;
		free(_items);
		free(_rects);
		free(_runGlyphs);
		free(_runPositions);
		_items = calloc(_capacity, sizeof(AWItem));
		_rects = calloc(_capacity, sizeof(CGRect));
		_runGlyphs = calloc(_capacity, sizeof(CGGlyph));
		_runPositions = calloc(_capacity, sizeof(CGPoint));
	}
	if (!sameCell || !_font) {
		[self loadFont];
	}
	[self refresh:0];
}

- (void)loadFont {
	if (_font) {
		CFRelease(_font);
	}
	NSFont *f = [NSFont monospacedSystemFontOfSize:round(_layout.cellH * 0.74) weight:NSFontWeightSemibold];
	_font = (CTFontRef)CFBridgingRetain(f);
	UniChar chars[128];
	for (int i = 0; i < 128; i++) {
		chars[i] = i < 32 ? ' ' : (UniChar)i;
	}
	CTFontGetGlyphsForCharacters(_font, chars, _glyphs, 128);
	CGSize adv;
	CGGlyph m = _glyphs['M'];
	CTFontGetAdvancesForGlyphs(_font, kCTFontOrientationHorizontal, &m, &adv, 1);
	_advance = adv.width;
	// Centre the em box on the cell's midline, like canvas textBaseline "middle".
	_baseline = (CTFontGetAscent(_font) - CTFontGetDescent(_font)) / 2;
}

- (void)refresh:(double)dt {
	if (!_items) {
		return;
	}
	int n = goTick(self.screenID, _items, _capacity, dt);
	if (n >= 0) {
		_count = n;
		[self setNeedsDisplay:YES];
	}
}

- (void)drawGridIn:(CGContextRef)ctx height:(CGFloat)H {
	CGFloat scale = self.window.backingScaleFactor ?: 1;
	CGFloat c = awWhite ? 0 : 1;
	CGContextSetRGBStrokeColor(ctx, c, c, c, 0.055);
	CGContextSetLineWidth(ctx, 1 / scale);
	CGContextBeginPath(ctx);
	CGFloat W = self.bounds.size.width;
	for (int i = 0; i <= _layout.cols; i++) {
		CGFloat x = round(i * _layout.cellW * scale) / scale + 0.5 / scale;
		CGContextMoveToPoint(ctx, x, 0);
		CGContextAddLineToPoint(ctx, x, H);
	}
	for (int i = 0; i <= _layout.rows; i++) {
		CGFloat y = H - (round(i * _layout.cellH * scale) / scale + 0.5 / scale);
		CGContextMoveToPoint(ctx, 0, y);
		CGContextAddLineToPoint(ctx, W, y);
	}
	CGContextStrokePath(ctx);
}

- (void)drawRect:(NSRect)dirty {
	CGContextRef ctx = NSGraphicsContext.currentContext.CGContext;
	CGFloat H = self.bounds.size.height;
	CGFloat bg = awWhite ? 1 : 0;
	CGContextSetRGBFillColor(ctx, bg, bg, bg, 1);
	CGContextFillRect(ctx, self.bounds);
	if (!_items) {
		return;
	}
	[self drawGridIn:ctx height:H];
	CGContextSetTextMatrix(ctx, CGAffineTransformIdentity);

	const CGFloat cw = _layout.cellW, ch = _layout.cellH, dot = _layout.dotSize;
	// Items arrive sorted so that equal (kind, colour) neighbours form runs;
	// each run is one CoreGraphics call.
	int run = 0;
	for (int i = 0; i <= _count; i++) {
		BOOL flush = i == _count || (run > 0 && (_items[i].kind != _items[i - 1].kind ||
		                                         _items[i].rgba != _items[i - 1].rgba));
		if (flush && run > 0) {
			AWItem *last = &_items[i - 1];
			setFill(ctx, last->rgba);
			if (last->kind == AW_KIND_GLYPH) {
				CTFontDrawGlyphs(_font, _runGlyphs, _runPositions, run, ctx);
			} else {
				CGContextFillRects(ctx, _rects, run);
			}
			run = 0;
		}
		if (i == _count) {
			break;
		}
		AWItem *it = &_items[i];
		CGFloat x = it->col * cw, y = it->row * ch;
		switch (it->kind) {
		case AW_KIND_TILE:
			_rects[run] = CGRectMake(x + 1, H - (y + ch), cw - 1, ch - 1);
			break;
		case AW_KIND_DOT: {
			// Sit the dot low-right in its cell, as in the reference.
			CGFloat dx = round(x + cw * 0.66 - dot / 2), dy = round(y + ch * 0.64 - dot / 2);
			_rects[run] = CGRectMake(dx, H - dy - dot, dot, dot);
			break;
		}
		default:
			_runGlyphs[run] = _glyphs[it->glyph & 0x7F];
			_runPositions[run] = CGPointMake(x + cw / 2 - _advance / 2, H - (y + ch / 2 + 1) - _baseline);
			break;
		}
		run++;
	}
}

@end

static uint32_t screenIDOf(NSScreen *s) {
	return [s.deviceDescription[@"NSScreenNumber"] unsignedIntValue];
}

static NSWindow *makeWindow(NSScreen *s) {
	NSWindow *w = [[NSWindow alloc] initWithContentRect:s.frame
	                                          styleMask:NSWindowStyleMaskBorderless
	                                            backing:NSBackingStoreBuffered
	                                              defer:NO];
	w.releasedWhenClosed = NO;
	// Two levels above the desktop: WindowManager draws a "Fullscreen Backdrop"
	// copy of the system wallpaper at desktop+1 during Mission Control and
	// full-screen spaces, which would otherwise cover the aquarium. Finder's
	// icons stay on top at kCGDesktopIconWindowLevel (desktop+20).
	w.level = CGWindowLevelForKey(kCGDesktopWindowLevelKey) + 2;
	w.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorStationary |
	                       NSWindowCollectionBehaviorIgnoresCycle;
	w.ignoresMouseEvents = YES;
	w.hasShadow = NO;
	w.opaque = YES;
	w.backgroundColor = awWhite ? NSColor.whiteColor : NSColor.blackColor;

	AWView *v = [[AWView alloc] initWithFrame:NSMakeRect(0, 0, s.frame.size.width, s.frame.size.height)];
	v.screenID = screenIDOf(s);
	w.contentView = v;
	// Covered means animation stops here, so freeze this exact frame into the
	// system wallpaper that Space transitions will show.
	__weak NSWindow *weak = w;
	[NSNotificationCenter.defaultCenter addObserverForName:NSWindowDidChangeOcclusionStateNotification
	                                                object:w
	                                                 queue:NSOperationQueue.mainQueue
	                                            usingBlock:^(NSNotification *n) {
		                                            NSWindow *win = weak;
		                                            if (win && !(win.occlusionState & NSWindowOcclusionStateVisible)) {
			                                            awSnapshot(win.contentView, screenIDOf(s), 2);
		                                            }
	                                            }];
	[w setFrame:s.frame display:NO];
	[w orderFrontRegardless];
	return w;
}

static void syncScreens(void) {
	awRememberOriginals();
	NSMutableSet<NSNumber *> *seen = [NSMutableSet set];
	for (NSScreen *s in NSScreen.screens) {
		NSNumber *key = @(screenIDOf(s));
		[seen addObject:key];
		NSWindow *w = awWindows[key];
		if (!w) {
			w = makeWindow(s);
			awWindows[key] = w;
		} else {
			[w setFrame:s.frame display:NO];
		}
		[(AWView *)w.contentView relayout];
	}
	for (NSNumber *key in awWindows.allKeys) {
		if (![seen containsObject:key]) {
			[awWindows[key] close];
			[awWindows removeObjectForKey:key];
			goScreenRemoved(key.unsignedIntValue);
		}
	}
}

static void tick(void) {
	CFTimeInterval now = CACurrentMediaTime();
	double dt = now - awLast;
	awLast = now;
	for (NSWindow *w in awWindows.allValues) {
		if (w.occlusionState & NSWindowOcclusionStateVisible) {
			[(AWView *)w.contentView refresh:dt];
		}
	}
}

static void updateTimer(void) {
	BOOL run = !awPaused && !awSuspended;
	if (run && !awTimer) {
		awLast = CACurrentMediaTime();
		awTimer = [NSTimer timerWithTimeInterval:1.0 / kFPS
		                                 repeats:YES
		                                   block:^(NSTimer *t) {
			                                   tick();
		                                   }];
		awTimer.tolerance = 0.1 / kFPS;
		// Common modes keep the animation running while the status menu is open.
		[NSRunLoop.mainRunLoop addTimer:awTimer forMode:NSRunLoopCommonModes];
	} else if (!run && awTimer) {
		[awTimer invalidate];
		awTimer = nil;
	}
}

static void setSuspended(BOOL s) {
	awSuspended = s;
	updateTimer();
}

@interface AWController : NSObject <NSApplicationDelegate>
@property(nonatomic, strong) NSStatusItem *item;
@property(nonatomic, strong) NSMenuItem *blackItem, *whiteItem, *pauseItem;
@end

@implementation AWController

- (NSMenuItem *)addItem:(NSString *)title action:(SEL)sel to:(NSMenu *)menu {
	NSMenuItem *it = [[NSMenuItem alloc] initWithTitle:title action:sel keyEquivalent:@""];
	it.target = self;
	[menu addItem:it];
	return it;
}

- (void)setupMenu {
	self.item = [NSStatusBar.systemStatusBar statusItemWithLength:NSVariableStatusItemLength];
	self.item.button.title = @"><>";
	self.item.button.font = [NSFont monospacedSystemFontOfSize:12 weight:NSFontWeightRegular];

	NSMenu *menu = [[NSMenu alloc] init];
	self.blackItem = [self addItem:@"Чёрный фон" action:@selector(pickBlack:) to:menu];
	self.whiteItem = [self addItem:@"Белый фон" action:@selector(pickWhite:) to:menu];
	[menu addItem:NSMenuItem.separatorItem];
	self.pauseItem = [self addItem:@"Пауза" action:@selector(togglePause:) to:menu];
	[menu addItem:NSMenuItem.separatorItem];
	[self addItem:@"Выход" action:@selector(quit:) to:menu];
	self.item.menu = menu;
}

- (void)applicationWillTerminate:(NSNotification *)n {
	awRestoreWallpaper();
}

- (void)pickBlack:(id)sender {
	goMenuAction(AW_ACTION_BLACK);
}

- (void)pickWhite:(id)sender {
	goMenuAction(AW_ACTION_WHITE);
}

- (void)togglePause:(id)sender {
	goMenuAction(AW_ACTION_PAUSE);
}

- (void)quit:(id)sender {
	goMenuAction(AW_ACTION_QUIT);
}

@end

static AWController *awController;

void awApply(int white, int paused) {
	awWhite = white != 0;
	awPaused = paused != 0;
	awController.blackItem.state = awWhite ? NSControlStateValueOff : NSControlStateValueOn;
	awController.whiteItem.state = awWhite ? NSControlStateValueOn : NSControlStateValueOff;
	awController.pauseItem.state = awPaused ? NSControlStateValueOn : NSControlStateValueOff;
	for (NSWindow *w in awWindows.allValues) {
		w.backgroundColor = awWhite ? NSColor.whiteColor : NSColor.blackColor;
		// Colours are baked into the draw list, so re-render even while paused.
		AWView *v = (AWView *)w.contentView;
		[v refresh:0];
		awSnapshot(v, v.screenID, 0);
	}
	updateTimer();
}

void awQuit(void) {
	[NSApp terminate:nil];
}

void awQuitAsync(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp terminate:nil];
	});
}

static void observe(NSNotificationCenter *nc, NSNotificationName name, void (^block)(void)) {
	[nc addObserverForName:name
	                object:nil
	                 queue:NSOperationQueue.mainQueue
	            usingBlock:^(NSNotification *n) {
		            block();
	            }];
}

void awRun(int white, int paused) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		NSApp.activationPolicy = NSApplicationActivationPolicyAccessory;
		awController = [[AWController alloc] init];
		NSApp.delegate = awController;
		awWindows = [NSMutableDictionary dictionary];
		awWhite = white != 0;

		[awController setupMenu];
		syncScreens();
		awApply(white, paused);

		observe(NSNotificationCenter.defaultCenter, NSApplicationDidChangeScreenParametersNotification, ^{
			syncScreens();
		});

		NSNotificationCenter *ws = NSWorkspace.sharedWorkspace.notificationCenter;
		observe(ws, NSWorkspaceScreensDidSleepNotification, ^{
			setSuspended(YES);
		});
		observe(ws, NSWorkspaceScreensDidWakeNotification, ^{
			setSuspended(NO);
		});
		observe(ws, NSWorkspaceSessionDidResignActiveNotification, ^{
			setSuspended(YES);
		});
		observe(ws, NSWorkspaceSessionDidBecomeActiveNotification, ^{
			setSuspended(NO);
		});

		NSNotificationCenter *dc = NSDistributedNotificationCenter.defaultCenter;
		observe(dc, @"com.apple.screenIsLocked", ^{
			setSuspended(YES);
		});
		observe(dc, @"com.apple.screenIsUnlocked", ^{
			setSuspended(NO);
		});

		[NSTimer scheduledTimerWithTimeInterval:kSnapshotEvery
		                                repeats:YES
		                                  block:^(NSTimer *t) {
			                                  if (!awTimer) {
				                                  return;
			                                  }
			                                  for (NSWindow *w in awWindows.allValues) {
				                                  if (w.occlusionState & NSWindowOcclusionStateVisible) {
					                                  AWView *v = (AWView *)w.contentView;
					                                  awSnapshot(v, v.screenID, kSnapshotEvery / 2);
				                                  }
			                                  }
		                                  }];

		[NSApp run];
	}
}
