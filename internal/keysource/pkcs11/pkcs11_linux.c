#include <dlfcn.h>
#include <stdlib.h>
#include <string.h>
#include "pkcs11_linux.h"

// The module is reached through its own CK_FUNCTION_LIST, which is the
// only entry point PKCS#11 guarantees. C_GetFunctionList is looked up by
// name; every other call goes through the table it returns, because a
// module may export a symbol that is not the function the table names.
typedef CK_RV (*get_function_list_t)(CK_FUNCTION_LIST_PTR_PTR);

CK_RV liro_get_function_list(void *handle, CK_FUNCTION_LIST_PTR *out) {
	get_function_list_t f = (get_function_list_t)dlsym(handle, "C_GetFunctionList");
	if (f == NULL) return CKR_FUNCTION_NOT_SUPPORTED;
	return f(out);
}

// Every call below is a one-line wrapper rather than a cgo call through a
// Go function pointer, because a Go pointer may not be stored in C memory
// and a C function pointer may not be called from Go directly.
CK_RV liro_initialize(CK_FUNCTION_LIST_PTR l, CK_BBOOL os_locking) {
	CK_C_INITIALIZE_ARGS args;
	memset(&args, 0, sizeof args);
	if (!os_locking) return l->C_Initialize(NULL_PTR);
	args.flags = CKF_OS_LOCKING_OK;
	return l->C_Initialize(&args);
}
CK_RV liro_finalize(CK_FUNCTION_LIST_PTR l)            { return l->C_Finalize(NULL_PTR); }
CK_RV liro_get_info(CK_FUNCTION_LIST_PTR l, CK_INFO *i){ return l->C_GetInfo(i); }
CK_RV liro_get_slot_list(CK_FUNCTION_LIST_PTR l, CK_BBOOL present, CK_SLOT_ID *ids, CK_ULONG *n) {
	return l->C_GetSlotList(present, ids, n);
}
// Only the flags leave C: the two strings in CK_SLOT_INFO are a reader's
// name, which nothing above this layer needs and the report must not carry.
CK_RV liro_get_slot_flags(CK_FUNCTION_LIST_PTR l, CK_SLOT_ID s, CK_FLAGS *flags) {
	CK_SLOT_INFO si;
	memset(&si, 0, sizeof si);
	CK_RV rv = l->C_GetSlotInfo(s, &si);
	*flags = si.flags;
	return rv;
}
CK_RV liro_get_token_info(CK_FUNCTION_LIST_PTR l, CK_SLOT_ID s, CK_TOKEN_INFO *t) {
	return l->C_GetTokenInfo(s, t);
}
CK_RV liro_get_mechanism_list(CK_FUNCTION_LIST_PTR l, CK_SLOT_ID s, CK_MECHANISM_TYPE *m, CK_ULONG *n) {
	return l->C_GetMechanismList(s, m, n);
}
CK_RV liro_open_session(CK_FUNCTION_LIST_PTR l, CK_SLOT_ID s, CK_SESSION_HANDLE *h) {
	return l->C_OpenSession(s, CKF_SERIAL_SESSION, NULL_PTR, NULL_PTR, h);
}
CK_RV liro_close_session(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h) {
	return l->C_CloseSession(h);
}
CK_RV liro_login(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h, CK_UTF8CHAR *pin, CK_ULONG n) {
	return l->C_Login(h, CKU_USER, pin, n);
}
CK_RV liro_logout(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h) { return l->C_Logout(h); }

CK_RV liro_find_init(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h, CK_ATTRIBUTE *t, CK_ULONG n) {
	return l->C_FindObjectsInit(h, t, n);
}
CK_RV liro_find(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h, CK_OBJECT_HANDLE *o, CK_ULONG max, CK_ULONG *n) {
	return l->C_FindObjects(h, o, max, n);
}
CK_RV liro_find_final(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h) {
	return l->C_FindObjectsFinal(h);
}
CK_RV liro_get_attribute(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h, CK_OBJECT_HANDLE o,
                                CK_ATTRIBUTE_TYPE typ, void *buf, CK_ULONG *len) {
	CK_ATTRIBUTE a;
	a.type = typ;
	a.pValue = buf;
	a.ulValueLen = *len;
	CK_RV rv = l->C_GetAttributeValue(h, o, &a, 1);
	*len = a.ulValueLen;
	return rv;
}
CK_RV liro_sign_init(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h, CK_OBJECT_HANDLE key) {
	CK_MECHANISM m;
	m.mechanism = CKM_RSA_PKCS;
	m.pParameter = NULL_PTR;
	m.ulParameterLen = 0;
	return l->C_SignInit(h, &m, key);
}
CK_RV liro_sign(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h,
                       CK_BYTE *in, CK_ULONG inlen, CK_BYTE *out, CK_ULONG *outlen) {
	return l->C_Sign(h, in, inlen, out, outlen);
}

// The sizes this layer would otherwise have to assume. They are read off
// the compiler rather than written down: F11 measured three of four
// hand-written candidate layouts returning CKR_OK with a zero length, so
// a wrong layout does not announce itself (F12 §1).
size_t liro_sizeof_ck_ulong(void)     { return sizeof(CK_ULONG); }
size_t liro_sizeof_ck_attribute(void) { return sizeof(CK_ATTRIBUTE); }
