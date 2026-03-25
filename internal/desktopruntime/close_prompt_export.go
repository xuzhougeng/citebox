//go:build darwin || windows

package desktopruntime

/*
#include <stdint.h>
*/
import "C"

//export citeboxRequestClosePrompt
func citeboxRequestClosePrompt(windowToken C.uintptr_t) C.int {
	if dispatchClosePrompt(uintptr(windowToken)) {
		return 1
	}
	return 0
}
