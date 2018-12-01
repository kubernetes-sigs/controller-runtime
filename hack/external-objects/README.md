External Imports Checker
========================

This tool knows how to check for external imports, to make sure that we
don't accidentally break ourselves because of Kubernetes.  It's also
a general framework for doing this kind of thing, so it could be adapted
to general type signature checking, etc.

Usage
-----

### Printing all external references

This will print out all external references, plus their type signatures
(non-exported parts excluded on structs).  You can diff this against the
output from a previous commit to check for differences.

```bash
$ go run ./*.go --root ../.. find
```

### Investigating a new reference

This will print out the "top-level" objects (functions, interfaces,
variables, etc) that eventually reference the given external type, as well
as the intermediate types to get there.

For instance, if controller-runtime method `Foo` references external
struct `Bar` that includes the target type `Baz`, it'll show the chain
from `Foo` to `Bar` to `Baz`.

```bash
$ go run ./*.go --root ../.. what-refs "k8s.io/some/other/package#SomeType"
```
