//go:build darwin

package wakelock

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <IOKit/pwr_mgt/IOPMLib.h>
#include <CoreFoundation/CoreFoundation.h>

static IOPMAssertionID assertionID = 0;

int darwinEnable() {
    CFStringRef reason = CFStringCreateWithCString(NULL, "assistente em foco", kCFStringEncodingUTF8);
    IOReturn ret = IOPMAssertionCreateWithName(kIOPMAssertionTypeNoDisplaySleep,
        kIOPMAssertionLevelOn, reason, &assertionID);
    CFRelease(reason);
    return ret == kIOReturnSuccess ? 0 : 1;
}

int darwinDisable() {
    if (assertionID == 0) return 0;
    IOReturn ret = IOPMAssertionRelease(assertionID);
    if (ret == kIOReturnSuccess) assertionID = 0;
    return ret == kIOReturnSuccess ? 0 : 1;
}
*/
import "C"

func enable() {
	_, _, _ = C.darwinEnable(), 0, 0
}

func disable() {
	_, _, _ = C.darwinDisable(), 0, 0
}
