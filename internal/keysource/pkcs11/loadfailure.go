package pkcs11

// LoadFailureKind is why a module would not load, in terms a person can be
// told in their own language (D-362). The dynamic loader's words are carried
// beside it, untranslated, as data.
type LoadFailureKind string

const (
	// LoadMissingLibrary: the module needs a library this system does not
	// have. Name is its soname.
	LoadMissingLibrary LoadFailureKind = "missing_library"
	// LoadMissingFunction: it needs a function no library here exports.
	// Name is the symbol.
	LoadMissingFunction LoadFailureKind = "missing_function"
	// LoadMissingVersion: it was built against a symbol version its library
	// no longer has. Name is the version node, Library the library.
	LoadMissingVersion LoadFailureKind = "missing_version"
	// LoadNotALibrary: the file is not a shared library at all.
	LoadNotALibrary LoadFailureKind = "not_a_library"
	// LoadNoFile: there is nothing at the path.
	LoadNoFile LoadFailureKind = "no_file"
	// LoadWrongClass: a 32-bit library, and this program is 64-bit.
	LoadWrongClass LoadFailureKind = "wrong_class"
	// LoadLoaderStopped: the dynamic loader ended the probe process while
	// loading the module (exit 127, D-359). There are no loader words: they
	// went with the process.
	LoadLoaderStopped LoadFailureKind = "loader_stopped"
)

// LoadFailureKinds is every kind, for the check that each one reaches a
// person in their language (D-362).
func LoadFailureKinds() []LoadFailureKind {
	return []LoadFailureKind{
		LoadMissingLibrary, LoadMissingFunction, LoadMissingVersion,
		LoadNotALibrary, LoadNoFile, LoadWrongClass, LoadLoaderStopped,
	}
}

// LoadError is a module that would not load, and why.
//
// It crosses from the probe child to the parent as JSON, which is why every
// field is plain data: the child is the process that may be killed by
// somebody else's code, and what it reports is facts, never anything the
// parent acts on (probeResult).
type LoadError struct {
	Kind    LoadFailureKind `json:"kind"`
	Module  string          `json:"module"`
	Name    string          `json:"name,omitempty"`
	Library string          `json:"library,omitempty"`
	// OldOpenSSL is set when the missing piece is an OpenSSL older than any
	// supported distribution ships, which is the one piece of advice the
	// loader's words cannot give: ask the vendor for this system's build.
	OldOpenSSL bool `json:"oldOpenSSL,omitempty"`
	// Loader is dlerror's text, verbatim. It is what a vendor's support desk
	// asks for, and translating it would lose exactly that (SPEC §9.3).
	Loader string `json:"loader,omitempty"`
}

// Error is the English sentence, for logs and `certs --json`, which are data
// (D-092). A person reads the catalogue's sentence instead (D-362).
func (e *LoadError) Error() string {
	var s string
	switch e.Kind {
	case LoadMissingLibrary:
		s = "it needs " + e.Name + ", which is not installed on this system"
	case LoadMissingFunction:
		s = "it needs the function " + e.Name + ", which no library on this system provides; it was probably built for a different system"
	case LoadMissingVersion:
		s = "it was built against " + e.Name + " of " + e.Library + ", and the copy on this system does not provide it"
	case LoadNotALibrary:
		s = "it is not a shared library"
	case LoadNoFile:
		s = "there is no file at this path"
	case LoadWrongClass:
		s = "it is a 32-bit library, and this program is 64-bit"
	case LoadLoaderStopped:
		return "exit 127 is the system loader stopping the process while loading this module or a library it needs — `ldd " + e.Module + "` shows which"
	default:
		s = string(e.Kind)
	}
	if e.OldOpenSSL {
		s += ". That is an older OpenSSL than this distribution ships: the module was built for an older system, and its vendor's build for this one is needed"
	}
	if e.Loader != "" {
		s += " (the system loader said: " + e.Loader + ")"
	}
	return s
}

// loadFailed is a probe child's refusal as the parent sees it: the child's
// own message, unchanged, and the LoadError behind it for errors.As.
type loadFailed struct {
	msg  string
	load *LoadError
}

func (e loadFailed) Error() string { return e.msg }
func (e loadFailed) Unwrap() error { return e.load }

// probeDied is a probe child that did not survive, with the LoadError that
// says why when the loader is known to be the reason. It is errWorkerDied to
// errors.Is either way.
type probeDied struct {
	msg  string
	load *LoadError
}

func (e probeDied) Error() string { return e.msg }
func (e probeDied) Unwrap() []error {
	if e.load == nil {
		return []error{errWorkerDied}
	}
	return []error{errWorkerDied, e.load}
}
