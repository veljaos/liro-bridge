//go:build darwin && cgo

package platform

// SPEC §6.4's macOS row: the Keychain (F13, [[D-463]]).
//
// **Written without a Mac.** CI's macOS runner stores, reads and deletes
// one item through it (secretstore_darwin_test.go); nothing else has run
// it. The device secrets go into the person's login Keychain as generic
// passwords — service "liro-bridge", account the secret's name — through
// Security.framework's SecItem calls, not the `security` command: an item
// a program creates is, by the Keychain's own rule, readable without a
// prompt by that program and asks the person before any other program
// reads it, which is §6.4's "item bound to the application" — and an item
// created by /usr/bin/security would be bound to /usr/bin/security.
//
// What binds it is the program's code signature. **This program is not
// signed for macOS yet** (§15.1, and F13 §6), so the Keychain identifies
// it by its path and contents, and an update — a new binary — will be
// asked about the old items once. That is the Keychain's answer to an
// unsigned program, not a defect in this file; signing is what makes it
// go away, and it is on the owner's list.
//
// The data-protection keychain (kSecUseDataProtectionKeychain) is not
// used: it needs a keychain-access-groups entitlement, which an unsigned
// program cannot have, and every call would fail with
// errSecMissingEntitlement.

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <stdlib.h>
#include <string.h>
#include <Security/Security.h>

static CFMutableDictionaryRef liro_query(const char *service, const char *account) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFStringRef s = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
	CFStringRef a = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(q, kSecAttrService, s);
	CFDictionarySetValue(q, kSecAttrAccount, a);
	CFRelease(s);
	CFRelease(a);
	return q;
}

static OSStatus liro_kc_delete(const char *service, const char *account) {
	CFMutableDictionaryRef q = liro_query(service, account);
	OSStatus st = SecItemDelete(q);
	CFRelease(q);
	return st;
}

static OSStatus liro_kc_set(const char *service, const char *account, const void *data, size_t len) {
	OSStatus st = liro_kc_delete(service, account);
	if (st != errSecSuccess && st != errSecItemNotFound) {
		return st;
	}
	CFMutableDictionaryRef q = liro_query(service, account);
	CFDataRef d = CFDataCreate(NULL, (const UInt8 *)data, (CFIndex)len);
	CFDictionarySetValue(q, kSecValueData, d);
	CFDictionarySetValue(q, kSecAttrAccessible, kSecAttrAccessibleWhenUnlocked);
	st = SecItemAdd(q, NULL);
	CFRelease(d);
	CFRelease(q);
	return st;
}

// liro_kc_get copies the item's data into a malloc'd buffer the caller
// wipes and frees.
static OSStatus liro_kc_get(const char *service, const char *account, void **out, size_t *outLen) {
	CFMutableDictionaryRef q = liro_query(service, account);
	CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef result = NULL;
	OSStatus st = SecItemCopyMatching(q, &result);
	CFRelease(q);
	if (st != errSecSuccess) {
		return st;
	}
	CFDataRef d = (CFDataRef)result;
	CFIndex n = CFDataGetLength(d);
	*out = malloc(n > 0 ? (size_t)n : 1);
	if (*out == NULL) {
		CFRelease(result);
		return errSecAllocate;
	}
	memcpy(*out, CFDataGetBytePtr(d), (size_t)n);
	*outLen = (size_t)n;
	CFRelease(result);
	return errSecSuccess;
}

static void liro_wipe_free(void *p, size_t n) {
	if (p == NULL) return;
	volatile unsigned char *b = (volatile unsigned char *)p;
	for (size_t i = 0; i < n; i++) b[i] = 0;
	free(p);
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// OSStatus values, as Go constants: cgo may give Security's enum
// constants a C type other than OSStatus, and a comparison between the
// two would not compile. Values from SecBase.h.
const (
	osSuccess      = 0
	osItemNotFound = -25300
)

// keychainService is the Keychain item's service attribute — the
// application, as the Linux store's attribute names it.
const keychainService = "liro-bridge"

type keychainStore struct{ service string }

func newSecretStore(dir string) (SecretStore, error) {
	_ = dir // the Keychain is the person's, not a file beside the pairings
	return keychainStore{service: keychainService}, nil
}

func keychainError(op string, st C.OSStatus) error {
	return fmt.Errorf("platform: Keychain %s failed (OSStatus %d)", op, int32(st))
}

func (k keychainStore) Get(name string) ([]byte, error) {
	cs, cn := C.CString(k.service), C.CString(name)
	defer C.free(unsafe.Pointer(cs))
	defer C.free(unsafe.Pointer(cn))
	var out unsafe.Pointer
	var n C.size_t
	st := C.liro_kc_get(cs, cn, &out, &n)
	if int32(st) == osItemNotFound {
		return nil, ErrSecretNotFound
	}
	if int32(st) != osSuccess {
		return nil, keychainError("read", st)
	}
	defer C.liro_wipe_free(out, n)
	return C.GoBytes(out, C.int(n)), nil
}

func (k keychainStore) Set(name string, value []byte) error {
	if len(value) == 0 {
		return errEmptySecret
	}
	cs, cn := C.CString(k.service), C.CString(name)
	defer C.free(unsafe.Pointer(cs))
	defer C.free(unsafe.Pointer(cn))
	// The value is copied into C memory for the call and wiped after, so
	// that the only copies are the caller's and the Keychain's.
	buf := C.CBytes(value)
	defer C.liro_wipe_free(buf, C.size_t(len(value)))
	if st := C.liro_kc_set(cs, cn, buf, C.size_t(len(value))); int32(st) != osSuccess {
		return keychainError("write", st)
	}
	return nil
}

func (k keychainStore) Delete(name string) error {
	cs, cn := C.CString(k.service), C.CString(name)
	defer C.free(unsafe.Pointer(cs))
	defer C.free(unsafe.Pointer(cn))
	if st := C.liro_kc_delete(cs, cn); int32(st) != osSuccess && int32(st) != osItemNotFound {
		return keychainError("delete", st)
	}
	return nil
}

func (k keychainStore) Describe() SecretStoreDescription {
	return SecretStoreDescription{Mechanism: MechanismKeychain}
}
