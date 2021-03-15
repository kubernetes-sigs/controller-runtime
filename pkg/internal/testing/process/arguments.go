package process

import (
	"bytes"
	"html/template"
)

// RenderTemplates returns an []string to render the templates
//
// Deprecated: this should be removed when we remove APIServer.Args.
func RenderTemplates(argTemplates []string, data interface{}) (args []string, err error) {
	var t *template.Template

	for _, arg := range argTemplates {
		t, err = template.New(arg).Parse(arg)
		if err != nil {
			args = nil
			return
		}

		buf := &bytes.Buffer{}
		err = t.Execute(buf, data)
		if err != nil {
			args = nil
			return
		}
		args = append(args, buf.String())
	}

	return
}

// EmptyArguments constructs an empty set of flags with no defaults.
func EmptyArguments() *Arguments {
	return &Arguments{
		values: make(map[string]Arg),
	}
}

// Arguments are structured, overridable arguments.
// Each Arguments object contains some set of default arguments, which may
// be appended to, or overridden.
//
// When ready, you can serialize them to pass to exec.Command and friends using
// AsStrings.
//
// All flag-setting methods return the *same* instance of Arguments so that you
// can chain calls.
type Arguments struct {
	// values contains the user-set values for the arguments.
	// `values[key] = dontPass` means "don't pass this flag"
	// `values[key] = passAsName` means "pass this flag without args like --key`
	// `values[key] = []string{a, b, c}` means "--key=a --key=b --key=c`
	// any values not explicitly set here will be copied from defaults on final rendering.
	values map[string]Arg
}

// Arg is an argument that has one or more values,
// and optionally falls back to default values.
type Arg interface {
	// Append adds new values to this argument, returning
	// a new instance contain the new value.  The intermediate
	// argument should generally be assumed to be consumed.
	Append(vals ...string) Arg
	// Get returns the full set of values, optionally including
	// the passed in defaults.  If it returns nil, this will be
	// skipped.  If it returns a non-nil empty slice, it'll be
	// assumed that the argument should be passed as name-only.
	Get(defaults []string) []string
}

type userArg []string

func (a userArg) Append(vals ...string) Arg {
	return userArg(append(a, vals...))
}
func (a userArg) Get(_ []string) []string {
	return []string(a)
}

type defaultedArg []string

func (a defaultedArg) Append(vals ...string) Arg {
	return defaultedArg(append(a, vals...))
}
func (a defaultedArg) Get(defaults []string) []string {
	res := append([]string(nil), defaults...)
	return append(res, a...)
}

type dontPassArg struct{}

func (a dontPassArg) Append(vals ...string) Arg {
	return userArg(vals)
}
func (dontPassArg) Get(_ []string) []string {
	return nil
}

type passAsNameArg struct{}

func (a passAsNameArg) Append(_ ...string) Arg {
	return passAsNameArg{}
}
func (passAsNameArg) Get(_ []string) []string {
	return []string{}
}

var (
	// DontPass indicates that the given argument will not actually be
	// rendered.
	DontPass Arg = dontPassArg{}
	// PassAsName indicates that the given flag will be passed as `--key`
	// without any value.
	PassAsName Arg = passAsNameArg{}
)

// AsStrings serializes this set of arguments to a slice of strings appropriate
// for passing to exec.Command and friends, making use of the given defaults
// as indicated for each particular argument.
//
// - Any flag in defaults that's not in Arguments will be present in the output
// - Any flag that's present in Arguments will be passed the corresponding
//   defaults to do with as it will (ignore, append-to, suppress, etc).
func (a *Arguments) AsStrings(defaults map[string][]string) []string {
	// sort for deterministic ordering
	keysInOrder := make([]string, 0, len(defaults)+len(a.values))
	for key := range defaults {
		if _, userSet := a.values[key]; userSet {
			continue
		}
		keysInOrder = append(keysInOrder, key)
	}
	for key := range a.values {
		keysInOrder = append(keysInOrder, key)
	}
	sort.Strings(keysInOrder)

	var res []string
	for _, key := range keysInOrder {
		vals := a.Get(key).Get(defaults[key])
		switch {
		case vals == nil: // don't pass
			continue
		case len(vals) == 0: // pass as name
			res = append(res, "--"+key)
		default:
			for _, val := range vals {
				res = append(res, "--"+key+"="+val)
			}
		}
	}

	return res
}

// Get returns the value of the given flag.  If nil,
// it will not be passed in AsString, otherwise:
//
// len == 0 --> `--key`, len > 0 --> `--key=val1 --key=val2 ...`
func (a *Arguments) Get(key string) Arg {
	if vals, ok := a.values[key]; ok {
		return vals
	}
	return defaultedArg(nil)
}

// Enable configures the given key to be passed as a "name-only" flag,
// like, `--key`.
func (a *Arguments) Enable(key string) *Arguments {
	a.values[key] = PassAsName
	return a
}

// Disable prevents this flag from be passed.
func (a *Arguments) Disable(key string) *Arguments {
	a.values[key] = DontPass
	return a
}

// Append adds additional values to this flag.  If this flag has
// yet to be set, initial values will include defaults.  If you want
// to intentionally ignore defaults/start from scratch, call AppendNoDefaults.
//
// Multiple values will look like `--key=value1 --key=value2 ...`.
func (a *Arguments) Append(key string, values ...string) *Arguments {
	vals, present := a.values[key]
	if !present {
		vals = defaultedArg{}
	}
	a.values[key] = vals.Append(values...)
	return a
}

// AppendNoDefaults adds additional values to this flag.  However,
// unlike Append, it will *not* copy values from defaults.
func (a *Arguments) AppendNoDefaults(key string, values ...string) *Arguments {
	vals, present := a.values[key]
	if !present {
		vals = userArg{}
	}
	a.values[key] = vals.Append(values...)
	return a
}

// Set resets the given flag to the specified values, ignoring any existing
// values or defaults.
func (a *Arguments) Set(key string, values ...string) *Arguments {
	a.values[key] = userArg(values)
	return a
}

// SetRaw sets the given flag to the given Arg value directly.  Use this if
// you need to do some complicated deferred logic or something.
//
// Otherwise behaves like Set.
func (a *Arguments) SetRaw(key string, val Arg) *Arguments {
	a.values[key] = val
	return a
}

// FuncArg is a basic implementation of Arg that can be used for custom argument logic,
// like pulling values out of APIServer, or dynamically calculating values just before
// launch.
//
// The given function will be mapped directly to Arg#Get, and will generally be
// used in conjunction with SetRaw.  For example, to set `--some-flag` to the
// API server's CertDir, you could do:
//
//     server.Configure().SetRaw("--some-flag", FuncArg(func(defaults []string) []string {
//         return []string{server.CertDir}
//     }))
//
// FuncArg ignores Appends; if you need to support appending values too, consider implementing
// Arg directly.
type FuncArg func([]string) []string

// Append is a no-op for FuncArg, and just returns itself.
func (a FuncArg) Append(vals ...string) Arg { return a }

// Get delegates functionality to the FuncArg function itself.
func (a FuncArg) Get(defaults []string) []string {
	return a(defaults)
}
