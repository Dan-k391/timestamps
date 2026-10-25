// The macOS shell: the menu bar icon and its menu, the app window (a
// WKWebView showing the local UI), notifications, the application menu,
// open panels and alerts.
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <UserNotifications/UserNotifications.h>
#include "mac.h"
#include "_cgo_export.h"

static NSString *str(const char *s) {
	NSString *r = s ? [NSString stringWithUTF8String:s] : nil;
	return r ?: @"";
}

static char *dupstr(NSString *s) {
	if (s == nil) return NULL;
	const char *u = s.UTF8String;
	return u ? strdup(u) : NULL;
}

static void onMain(dispatch_block_t b) {
	if (NSThread.isMainThread) b();
	else dispatch_async(dispatch_get_main_queue(), b);
}

static void onMainSync(dispatch_block_t b) {
	if (NSThread.isMainThread) b();
	else dispatch_sync(dispatch_get_main_queue(), b);
}

static void activateApp(void) {
	if (@available(macOS 14.0, *)) [NSApp activate];
	else [NSApp activateIgnoringOtherApps:YES];
}

static bool bundled(void) { return NSBundle.mainBundle.bundleIdentifier != nil; }

// statusImage draws the menu bar icon: the logo's dial with its activity
// arc, as a template image macOS tints for light and dark menu bars.
static NSImage *statusImage(void) {
	NSImage *img = [NSImage imageWithSize:NSMakeSize(18, 18) flipped:NO drawingHandler:^BOOL(NSRect r) {
		NSPoint c = NSMakePoint(9, 9);
		[NSColor.blackColor set];
		NSBezierPath *ring = [NSBezierPath bezierPathWithOvalInRect:NSInsetRect(r, 1.75, 1.75)];
		ring.lineWidth = 1.5;
		[ring stroke];
		NSBezierPath *active = [NSBezierPath bezierPath];
		[active appendBezierPathWithArcWithCenter:c radius:4.9 startAngle:86 endAngle:-70 clockwise:YES];
		active.lineWidth = 2.2;
		active.lineCapStyle = NSLineCapStyleRound;
		[active stroke];
		[[NSColor colorWithWhite:0 alpha:0.45] set];
		NSBezierPath *away = [NSBezierPath bezierPath];
		[away appendBezierPathWithArcWithCenter:c radius:4.9 startAngle:-115 endAngle:-192 clockwise:YES];
		away.lineWidth = 2.2;
		away.lineCapStyle = NSLineCapStyleRound;
		[away stroke];
		[NSColor.blackColor set];
		NSBezierPath *hands = [NSBezierPath bezierPath];
		[hands moveToPoint:NSMakePoint(9, 12.4)];
		[hands lineToPoint:c];
		[hands lineToPoint:NSMakePoint(9 + 2.6 * sin(128 * M_PI / 180), 9 + 2.6 * cos(128 * M_PI / 180))];
		hands.lineWidth = 1.3;
		hands.lineCapStyle = NSLineCapStyleRound;
		hands.lineJoinStyle = NSLineJoinStyleRound;
		[hands stroke];
		return YES;
	}];
	img.template = YES;
	return img;
}

@interface TSApp : NSObject <NSApplicationDelegate, NSWindowDelegate, NSMenuDelegate, WKNavigationDelegate, WKUIDelegate,
	WKScriptMessageHandler, WKDownloadDelegate, UNUserNotificationCenterDelegate>
@property(strong) NSDictionary *labels;
@property(strong) NSStatusItem *item;
@property(copy) NSString *tip;
@property(strong) NSWindow *window;
@property(strong) WKWebView *web;
@property(assign) BOOL shown;
@property(assign) int appearance; // 0 system, 1 light, 2 dark
@end

static TSApp *app;

@implementation TSApp

- (NSString *)L:(NSString *)key {
	NSString *s = self.labels[key];
	return [s isKindOfClass:NSString.class] ? s : key;
}

// ---- launch, menus ----

- (void)applicationDidFinishLaunching:(NSNotification *)n {
	self.item = [NSStatusBar.systemStatusBar statusItemWithLength:NSSquareStatusItemLength];
	self.item.button.image = statusImage();
	self.item.button.toolTip = self.tip ?: @"Timestamps";
	NSMenu *menu = [[NSMenu alloc] initWithTitle:@"Timestamps"];
	menu.delegate = self;
	menu.autoenablesItems = NO;
	self.item.menu = menu;
	[NSWorkspace.sharedWorkspace.notificationCenter addObserver:self selector:@selector(willSleep:) name:NSWorkspaceWillSleepNotification object:nil];
	if (bundled()) {
		UNUserNotificationCenter *c = UNUserNotificationCenter.currentNotificationCenter;
		c.delegate = self;
		[c requestAuthorizationWithOptions:UNAuthorizationOptionAlert | UNAuthorizationOptionSound completionHandler:^(BOOL granted, NSError *e) {}];
	}
	tsGoLaunched();
}

- (void)menuNeedsUpdate:(NSMenu *)menu {
	[menu removeAllItems];
	char *js = tsGoMenu();
	if (js == NULL) return;
	NSData *data = [NSData dataWithBytesNoCopy:js length:strlen(js) freeWhenDone:YES];
	id items = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
	if ([items isKindOfClass:NSArray.class]) [self fill:menu items:items];
}

- (void)fill:(NSMenu *)menu items:(NSArray *)items {
	for (NSDictionary *it in items) {
		if (![it isKindOfClass:NSDictionary.class]) continue;
		if ([it[@"sep"] boolValue]) {
			[menu addItem:NSMenuItem.separatorItem];
			continue;
		}
		NSString *text = [it[@"text"] isKindOfClass:NSString.class] ? it[@"text"] : @"";
		NSMenuItem *mi = [[NSMenuItem alloc] initWithTitle:text action:@selector(menuCommand:) keyEquivalent:@""];
		mi.target = self;
		mi.tag = [it[@"id"] integerValue];
		mi.enabled = ![it[@"disabled"] boolValue];
		mi.state = [it[@"checked"] boolValue] ? NSControlStateValueOn : NSControlStateValueOff;
		NSArray *sub = it[@"sub"];
		if ([sub isKindOfClass:NSArray.class] && sub.count > 0) {
			NSMenu *sm = [[NSMenu alloc] initWithTitle:text];
			sm.autoenablesItems = NO;
			[self fill:sm items:sub];
			mi.submenu = sm;
			mi.action = NULL;
		}
		[menu addItem:mi];
	}
}

- (void)menuCommand:(NSMenuItem *)mi {
	tsGoCommand((int)mi.tag);
}

- (void)openSettings:(id)sender {
	tsGoOpen("settings");
}

static NSMenuItem *menuItem(NSString *title, SEL action, NSString *key) {
	return [[NSMenuItem alloc] initWithTitle:title action:action keyEquivalent:key];
}

// buildMainMenu makes the application menu: shown while the window is
// open, and what gives the window copy, paste and the other shortcuts.
- (void)buildMainMenu {
	NSMenu *bar = [NSMenu new];

	NSMenu *appMenu = [NSMenu new];
	[appMenu addItem:menuItem([self L:@"about"], @selector(orderFrontStandardAboutPanel:), @"")];
	[appMenu addItem:NSMenuItem.separatorItem];
	NSMenuItem *settings = menuItem([self L:@"settings"], @selector(openSettings:), @",");
	settings.target = self;
	[appMenu addItem:settings];
	[appMenu addItem:NSMenuItem.separatorItem];
	[appMenu addItem:menuItem([self L:@"hide"], @selector(hide:), @"h")];
	NSMenuItem *others = menuItem([self L:@"hideOthers"], @selector(hideOtherApplications:), @"h");
	others.keyEquivalentModifierMask = NSEventModifierFlagOption | NSEventModifierFlagCommand;
	[appMenu addItem:others];
	[appMenu addItem:menuItem([self L:@"showAll"], @selector(unhideAllApplications:), @"")];
	[appMenu addItem:NSMenuItem.separatorItem];
	[appMenu addItem:menuItem([self L:@"quit"], @selector(terminate:), @"q")];
	NSMenuItem *appItem = [NSMenuItem new];
	appItem.submenu = appMenu;
	[bar addItem:appItem];

	NSMenu *edit = [[NSMenu alloc] initWithTitle:[self L:@"edit"]];
	[edit addItem:menuItem([self L:@"undo"], @selector(undo:), @"z")];
	[edit addItem:menuItem([self L:@"redo"], @selector(redo:), @"Z")];
	[edit addItem:NSMenuItem.separatorItem];
	[edit addItem:menuItem([self L:@"cut"], @selector(cut:), @"x")];
	[edit addItem:menuItem([self L:@"copy"], @selector(copy:), @"c")];
	[edit addItem:menuItem([self L:@"paste"], @selector(paste:), @"v")];
	[edit addItem:menuItem([self L:@"selectAll"], @selector(selectAll:), @"a")];
	NSMenuItem *editItem = [NSMenuItem new];
	editItem.submenu = edit;
	[bar addItem:editItem];

	NSMenu *win = [[NSMenu alloc] initWithTitle:[self L:@"window"]];
	[win addItem:menuItem([self L:@"minimize"], @selector(performMiniaturize:), @"m")];
	[win addItem:menuItem([self L:@"zoom"], @selector(performZoom:), @"")];
	[win addItem:NSMenuItem.separatorItem];
	[win addItem:menuItem([self L:@"close"], @selector(performClose:), @"w")];
	NSMenuItem *winItem = [NSMenuItem new];
	winItem.submenu = win;
	[bar addItem:winItem];

	NSApp.mainMenu = bar;
	NSApp.windowsMenu = win;
}

- (void)willSleep:(NSNotification *)n {
	tsGoSleep();
}

- (BOOL)applicationShouldHandleReopen:(NSApplication *)sender hasVisibleWindows:(BOOL)visible {
	tsGoOpen("");
	return NO;
}

- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender {
	return NO;
}

- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)sender {
	tsGoTerminate(); // closes the open segments and the database
	return NSTerminateNow;
}

// ---- the window ----

- (void)showURL:(NSString *)url script:(NSString *)script {
	if (self.window != nil) {
		if (script.length > 0) [self.web evaluateJavaScript:script completionHandler:nil];
		[self present];
		return;
	}
	WKUserContentController *ucc = [WKUserContentController new];
	NSString *bridge = @"window.__desktop = true;"
		"window.tsSetDark = function (d) { window.webkit.messageHandlers.ts.postMessage({ fn: 'dark', v: !!d }); };"
		"window.tsQuit = function () { window.webkit.messageHandlers.ts.postMessage({ fn: 'quit' }); };";
	[ucc addUserScript:[[WKUserScript alloc] initWithSource:bridge injectionTime:WKUserScriptInjectionTimeAtDocumentStart forMainFrameOnly:YES]];
	[ucc addScriptMessageHandler:self name:@"ts"];
	WKWebViewConfiguration *cfg = [WKWebViewConfiguration new];
	cfg.userContentController = ucc;

	NSRect vis = NSScreen.mainScreen.visibleFrame;
	CGFloat w = MAX(MIN(1440, vis.size.width * 0.92), MIN(960, vis.size.width));
	CGFloat h = MAX(MIN(900, vis.size.height * 0.92), MIN(620, vis.size.height));
	NSWindow *win = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, w, h)
		styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable
		backing:NSBackingStoreBuffered defer:NO];
	win.releasedWhenClosed = NO;
	win.title = @"Timestamps";
	win.contentMinSize = NSMakeSize(960, 620);
	win.tabbingMode = NSWindowTabbingModeDisallowed;
	win.delegate = self;
	[win center];
	[win setFrameAutosaveName:@"Timestamps"]; // reopens where it was

	WKWebView *web = [[WKWebView alloc] initWithFrame:win.contentView.bounds configuration:cfg];
	web.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
	web.navigationDelegate = self;
	web.UIDelegate = self;
	win.contentView = web;
	self.window = win;
	self.web = web;
	[self applyAppearance];
	self.shown = NO;
	[web loadRequest:[NSURLRequest requestWithURL:[NSURL URLWithString:url]]];
	// Shown once the page has loaded (no white flash), or after a moment.
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.8 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
		if (self.window == win && !self.shown) [self present];
	});
}

- (void)present {
	[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
	if (self.window.isMiniaturized) [self.window deminiaturize:nil];
	[self.window makeKeyAndOrderFront:nil];
	activateApp();
	self.shown = YES;
}

- (void)applyAppearance {
	if (self.window == nil) return;
	switch (self.appearance) {
	case 1: self.window.appearance = [NSAppearance appearanceNamed:NSAppearanceNameAqua]; break;
	case 2: self.window.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua]; break;
	default: self.window.appearance = nil;
	}
}

- (void)windowWillClose:(NSNotification *)n {
	NSWindow *win = self.window;
	WKWebView *web = self.web;
	if (win == nil || n.object != win) return;
	[web.configuration.userContentController removeScriptMessageHandlerForName:@"ts"];
	web.navigationDelegate = nil;
	web.UIDelegate = nil;
	win.delegate = nil;
	self.window = nil; // the web view goes with it, freeing its memory
	self.web = nil;
	dispatch_async(dispatch_get_main_queue(), ^{
		(void)win; // kept until the window has finished closing
		(void)web;
	});
	[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
	tsGoWindowClosed();
}

// ---- the page ----

- (void)userContentController:(WKUserContentController *)ucc didReceiveScriptMessage:(WKScriptMessage *)m {
	NSDictionary *body = [m.body isKindOfClass:NSDictionary.class] ? m.body : nil;
	NSString *fn = body[@"fn"];
	if ([fn isEqual:@"dark"]) {
		self.appearance = [body[@"v"] boolValue] ? 2 : 1;
		[self applyAppearance];
	} else if ([fn isEqual:@"quit"]) {
		[NSApp terminate:nil];
	}
}

- (BOOL)isLocal:(NSURL *)u {
	NSString *scheme = u.scheme.lowercaseString, *host = u.host.lowercaseString;
	if ([scheme isEqual:@"about"] || [scheme isEqual:@"blob"] || [scheme isEqual:@"data"]) return YES;
	return [scheme isEqual:@"http"] && ([host isEqual:@"127.0.0.1"] || [host isEqual:@"localhost"]);
}

- (void)webView:(WKWebView *)w decidePolicyForNavigationAction:(WKNavigationAction *)a decisionHandler:(void (^)(WKNavigationActionPolicy))decide {
	if (a.shouldPerformDownload) {
		decide(WKNavigationActionPolicyDownload);
		return;
	}
	NSURL *u = a.request.URL;
	if (u == nil || [self isLocal:u]) {
		decide(WKNavigationActionPolicyAllow);
		return;
	}
	[NSWorkspace.sharedWorkspace openURL:u]; // links leave for the browser
	decide(WKNavigationActionPolicyCancel);
}

- (void)webView:(WKWebView *)w decidePolicyForNavigationResponse:(WKNavigationResponse *)r decisionHandler:(void (^)(WKNavigationResponsePolicy))decide {
	decide(r.canShowMIMEType ? WKNavigationResponsePolicyAllow : WKNavigationResponsePolicyDownload);
}

- (void)webView:(WKWebView *)w didFinishNavigation:(WKNavigation *)nav {
	if (!self.shown) [self present];
}

- (void)webView:(WKWebView *)w navigationAction:(WKNavigationAction *)a didBecomeDownload:(WKDownload *)d {
	d.delegate = self;
}

- (void)webView:(WKWebView *)w navigationResponse:(WKNavigationResponse *)r didBecomeDownload:(WKDownload *)d {
	d.delegate = self;
}

// Exports are saved where the user says, starting in Downloads.
- (void)download:(WKDownload *)d decideDestinationUsingResponse:(NSURLResponse *)resp suggestedFilename:(NSString *)name completionHandler:(void (^)(NSURL *))done {
	NSSavePanel *p = [NSSavePanel savePanel];
	p.nameFieldStringValue = name;
	p.directoryURL = [NSFileManager.defaultManager URLsForDirectory:NSDownloadsDirectory inDomains:NSUserDomainMask].firstObject;
	void (^finish)(NSModalResponse) = ^(NSModalResponse r) {
		if (r != NSModalResponseOK || p.URL == nil) {
			done(nil);
			return;
		}
		[NSFileManager.defaultManager removeItemAtURL:p.URL error:nil]; // the panel already asked about replacing it
		done(p.URL);
	};
	if (self.window != nil) [p beginSheetModalForWindow:self.window completionHandler:finish];
	else finish([p runModal]);
}

- (WKWebView *)webView:(WKWebView *)w createWebViewWithConfiguration:(WKWebViewConfiguration *)c forNavigationAction:(WKNavigationAction *)a windowFeatures:(WKWindowFeatures *)f {
	if (a.request.URL != nil) [NSWorkspace.sharedWorkspace openURL:a.request.URL];
	return nil;
}

- (void)webView:(WKWebView *)w runJavaScriptAlertPanelWithMessage:(NSString *)message initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(void))done {
	NSAlert *a = [NSAlert new];
	a.messageText = message;
	[a beginSheetModalForWindow:self.window completionHandler:^(NSModalResponse r) { done(); }];
}

- (void)webView:(WKWebView *)w runJavaScriptConfirmPanelWithMessage:(NSString *)message initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(BOOL))done {
	NSAlert *a = [NSAlert new];
	a.messageText = message;
	[a addButtonWithTitle:[self L:@"ok"]];
	[a addButtonWithTitle:[self L:@"cancel"]];
	[a beginSheetModalForWindow:self.window completionHandler:^(NSModalResponse r) { done(r == NSAlertFirstButtonReturn); }];
}

- (void)webView:(WKWebView *)w runOpenPanelWithParameters:(WKOpenPanelParameters *)params initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(NSArray<NSURL *> *))done {
	NSOpenPanel *p = [NSOpenPanel openPanel];
	p.allowsMultipleSelection = params.allowsMultipleSelection;
	p.canChooseDirectories = params.allowsDirectories;
	[p beginSheetModalForWindow:self.window completionHandler:^(NSModalResponse r) { done(r == NSModalResponseOK ? p.URLs : nil); }];
}

// ---- notifications ----

- (void)userNotificationCenter:(UNUserNotificationCenter *)c willPresentNotification:(UNNotification *)n
	withCompletionHandler:(void (^)(UNNotificationPresentationOptions))done {
	done(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList);
}

- (void)userNotificationCenter:(UNUserNotificationCenter *)c didReceiveNotificationResponse:(UNNotificationResponse *)r
	withCompletionHandler:(void (^)(void))done {
	if ([r.actionIdentifier isEqual:UNNotificationDefaultActionIdentifier]) {
		NSString *route = r.notification.request.content.userInfo[@"route"];
		NSString *rt = [route isKindOfClass:NSString.class] ? route : @"";
		onMain(^{ tsGoOpen((char *)rt.UTF8String); });
	}
	done();
}

@end

// ---- called from Go ----

void tsAppPrepare(void) {
	[NSApplication sharedApplication];
	[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
}

void tsAppInit(const char *labels) {
	tsAppPrepare();
	app = [TSApp new];
	NSData *d = [str(labels) dataUsingEncoding:NSUTF8StringEncoding];
	id l = [NSJSONSerialization JSONObjectWithData:d options:0 error:nil];
	app.labels = [l isKindOfClass:NSDictionary.class] ? l : @{};
	NSApp.delegate = app;
	[app buildMainMenu];
}

void tsAppRun(void) { [NSApp run]; }

void tsSetTip(const char *tip) {
	NSString *t = str(tip);
	onMain(^{
		app.tip = t;
		app.item.button.toolTip = t;
	});
}

void tsNotify(const char *title, const char *text, const char *route) {
	NSString *t = str(title), *b = str(text), *r = str(route);
	onMain(^{
		if (!bundled()) return; // notifications need the app bundle
		UNMutableNotificationContent *c = [UNMutableNotificationContent new];
		c.title = t;
		c.body = b;
		c.userInfo = @{@"route": r};
		UNNotificationRequest *req = [UNNotificationRequest requestWithIdentifier:NSUUID.UUID.UUIDString content:c trigger:nil];
		[UNUserNotificationCenter.currentNotificationCenter addNotificationRequest:req withCompletionHandler:nil];
	});
}

void tsShowWindow(const char *url, const char *script) {
	NSString *u = str(url), *s = str(script);
	onMain(^{ [app showURL:u script:s]; });
}

void tsSetAppearance(int mode) {
	onMain(^{
		app.appearance = mode;
		[app applyAppearance];
	});
}

void tsQuit(void) {
	onMain(^{ [NSApp terminate:nil]; });
}

char *tsPickFile(const char *title, const char *exts, const char *dir) {
	NSString *t = str(title), *e = str(exts), *d = str(dir);
	__block char *res = NULL;
	onMainSync(^{
		NSOpenPanel *p = [NSOpenPanel openPanel];
		p.message = t;
		p.canChooseFiles = YES;
		p.canChooseDirectories = NO;
		p.allowsMultipleSelection = NO;
		if (e.length > 0) p.allowedFileTypes = [e componentsSeparatedByString:@","];
		if (d.length > 0) p.directoryURL = [NSURL fileURLWithPath:d isDirectory:YES];
		activateApp();
		if ([p runModal] == NSModalResponseOK) res = dupstr(p.URL.path);
	});
	return res;
}

char *tsPickFolder(const char *title, const char *dir) {
	NSString *t = str(title), *d = str(dir);
	__block char *res = NULL;
	onMainSync(^{
		NSOpenPanel *p = [NSOpenPanel openPanel];
		p.message = t;
		p.canChooseFiles = NO;
		p.canChooseDirectories = YES;
		p.canCreateDirectories = YES;
		p.allowsMultipleSelection = NO;
		if (d.length > 0) p.directoryURL = [NSURL fileURLWithPath:d isDirectory:YES];
		activateApp();
		if ([p runModal] == NSModalResponseOK) res = dupstr(p.URL.path);
	});
	return res;
}

void tsOpen(const char *target) {
	NSString *t = str(target);
	onMain(^{
		NSString *low = t.lowercaseString;
		NSURL *u = ([low hasPrefix:@"http://"] || [low hasPrefix:@"https://"]) ? [NSURL URLWithString:t] : [NSURL fileURLWithPath:t];
		if (u != nil) [NSWorkspace.sharedWorkspace openURL:u];
	});
}

void tsReveal(const char *path) {
	NSString *p = str(path);
	onMain(^{ [NSWorkspace.sharedWorkspace activateFileViewerSelectingURLs:@[[NSURL fileURLWithPath:p]]]; });
}

void tsAlert(const char *title, const char *text) {
	NSString *t = str(title), *x = str(text);
	onMainSync(^{
		tsAppPrepare();
		activateApp();
		NSAlert *a = [NSAlert new];
		a.messageText = t;
		a.informativeText = x;
		[a runModal];
	});
}

bool tsAsk(const char *title, const char *text, const char *ok, const char *cancel) {
	NSString *t = str(title), *x = str(text), *o = str(ok), *c = str(cancel);
	__block bool yes = false;
	onMainSync(^{
		activateApp();
		NSAlert *a = [NSAlert new];
		a.messageText = t;
		a.informativeText = x;
		[a addButtonWithTitle:o];
		[a addButtonWithTitle:c];
		yes = [a runModal] == NSAlertFirstButtonReturn;
	});
	return yes;
}

char *tsPreferredLanguage(void) {
	@autoreleasepool {
		return dupstr(NSLocale.preferredLanguages.firstObject);
	}
}
