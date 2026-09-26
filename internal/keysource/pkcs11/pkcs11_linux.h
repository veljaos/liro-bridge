/* The PKCS#11 calls this package makes, as C functions.
 *
 * They are a .h/.c pair rather than a cgo preamble because two Go files
 * need them — module_linux.go and login_linux.go — and a cgo preamble
 * belongs to the one file it is written in. login_linux.go is a
 * separate file because SPEC §6.5.1 clause 2 requires the PIN to live
 * in one function and `pin_test.go` enforces it, which is why the
 * login sequence could not simply be folded in here (D-350).
 *
 * Every call goes through the module's own CK_FUNCTION_LIST, which is
 * the only entry point PKCS#11 guarantees: a module may export a symbol
 * that is not the function its table names.
 */
#ifndef LIRO_PKCS11_LINUX_H
#define LIRO_PKCS11_LINUX_H

#include <stddef.h>
#include <p11-kit/pkcs11.h>

CK_RV liro_get_function_list(void *handle, CK_FUNCTION_LIST_PTR *out);
CK_RV liro_initialize(CK_FUNCTION_LIST_PTR l, CK_BBOOL os_locking);
CK_RV liro_finalize(CK_FUNCTION_LIST_PTR l);
CK_RV liro_get_info(CK_FUNCTION_LIST_PTR l, CK_INFO *i);
CK_RV liro_get_slot_list(CK_FUNCTION_LIST_PTR l, CK_BBOOL present, CK_SLOT_ID *ids, CK_ULONG *n);
CK_RV liro_get_slot_flags(CK_FUNCTION_LIST_PTR l, CK_SLOT_ID s, CK_FLAGS *flags);
CK_RV liro_get_token_info(CK_FUNCTION_LIST_PTR l, CK_SLOT_ID s, CK_TOKEN_INFO *t);
CK_RV liro_get_mechanism_list(CK_FUNCTION_LIST_PTR l, CK_SLOT_ID s, CK_MECHANISM_TYPE *m, CK_ULONG *n);
CK_RV liro_open_session(CK_FUNCTION_LIST_PTR l, CK_SLOT_ID s, CK_SESSION_HANDLE *h);
CK_RV liro_close_session(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h);
CK_RV liro_login(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h, CK_UTF8CHAR *pin, CK_ULONG n);
CK_RV liro_logout(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h);
CK_RV liro_find_init(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h, CK_ATTRIBUTE *t, CK_ULONG n);
CK_RV liro_find(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h, CK_OBJECT_HANDLE *o, CK_ULONG max, CK_ULONG *n);
CK_RV liro_find_final(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h);
CK_RV liro_get_attribute(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h, CK_OBJECT_HANDLE o,
                         CK_ATTRIBUTE_TYPE typ, void *buf, CK_ULONG *len);
CK_RV liro_sign_init(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h, CK_OBJECT_HANDLE key);
CK_RV liro_sign(CK_FUNCTION_LIST_PTR l, CK_SESSION_HANDLE h,
                CK_BYTE *in, CK_ULONG inlen, CK_BYTE *out, CK_ULONG *outlen);

size_t liro_sizeof_ck_ulong(void);
size_t liro_sizeof_ck_attribute(void);

#endif
