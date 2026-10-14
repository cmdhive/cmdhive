#import <Cocoa/Cocoa.h>

int aitActive(void) { return [NSApp isActive] ? 1 : 0; }

void aitBounce(void) {
	dispatch_async(dispatch_get_main_queue(), ^{ [NSApp requestUserAttention:NSInformationalRequest]; });
}
