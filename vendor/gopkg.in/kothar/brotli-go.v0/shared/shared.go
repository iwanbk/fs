// Package shared contains the common dictionary used by the enc and dec packages
package shared

/*
#include "dictionary.h"
*/
import "C"

import "unsafe"

// GetDictionary retrieves a pointer to the dictionary data structure
func GetDictionary() unsafe.Pointer {
	return unsafe.Pointer(&C.sharedBrotliDictionary)
}
