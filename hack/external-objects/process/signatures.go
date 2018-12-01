package process

import (
	"go/ast"
	"go/types"
	"fmt"

	"sigs.k8s.io/controller-runtime/hack/external-objects/locate"
)

// AllObjects returns all top-level exported objects
// (variables, types, functions) for the given package.
func AllObjects(pkg *locate.PackageInfo) []types.Object {
	var exported []types.Object
	scope := pkg.Parsed.Scope()
	for _, name := range scope.Names() {
		// we don't care about private things
		if !ast.IsExported(name) {
			continue
		}

		exported = append(exported, scope.Lookup(name))
	}

	return exported
}

// FilterMapFunc takes in a Type, and returns either nil to not return any results,
// or some other results.
type FilterMapFunc func(types.Type) interface{}

// filterMapper knows how to filter-map-visit types and all referenced types
// in exported locations (fields, signatures, etc), avoiding cycles from
// self-referential types.
type filterMapper struct {
	typeCache map[types.Type]struct{}
	results []interface{}
}

// FilterMapTypesInObject calls FilterMapType on the type signature of the given object.
func FilterMapTypesInObject(obj types.Object, filterMap FilterMapFunc) []interface{} {
	return FilterMapType(obj.Type(), filterMap)
}

// FilterMapType calls the given function on all types in the given type signature
// and types referenced in exported locations thereof. If the function returns a
// non-nil result, that result is saved for the eventual return value.
// It will automatically visit types within a given signature.  In case type hierarchy or paths
// are desired, it will call filterMap with nil when a given is done being processed.
func FilterMapType(typ types.Type, filterMap FilterMapFunc) []interface{} {
	mapper := &filterMapper{
		typeCache: make(map[types.Type]struct{}),
	}

	mapper.filterMap(typ, filterMap)

	return mapper.results
}

// filterMap calls the given function on all types in the given type signature
// If the function returns a non-nil result, that result is saved for the eventual return value.
// It will automatically visit types within a given signature.  It will automatically detect cycles.
func (m *filterMapper) filterMap(rawType types.Type, filterMap FilterMapFunc) {
	rawTypeDesc := rawType.String()
	rawTypeDesc += "+"
	// check for cycles...
	if _, seen := m.typeCache[rawType]; seen {
		return
	}
	// ...and avoid them
	m.typeCache[rawType] = struct{}{}

	rootRes := filterMap(rawType)
	if rootRes != nil {
		m.results = append(m.results, rootRes)
	}

	switch typ := rawType.(type) {
	case *types.Basic:
		// do nothing, it's a basic type (e.g. int, string, etc)
	case *types.Array:
		m.filterMap(typ.Elem(), filterMap)
	case *types.Slice:
		m.filterMap(typ.Elem(), filterMap)
	case *types.Struct:
		for i := 0; i < typ.NumFields(); i++ {
			field := typ.Field(i)
			if !ast.IsExported(field.Name()) {
				continue
			}
			m.filterMap(field.Type(), filterMap)
		}
	case *types.Pointer:
		m.filterMap(typ.Elem(), filterMap)
	case *types.Tuple:
		for i := 0; i < typ.Len(); i++ {
			m.filterMap(typ.At(i).Type(), filterMap)
		}
	case *types.Signature:
		m.filterMap(typ.Params(), filterMap)
		if typ.Recv() != nil {
			m.filterMap(typ.Recv().Type(), filterMap)
		}
		m.filterMap(typ.Results(), filterMap)
	case *types.Interface:
		for i := 0; i < typ.NumExplicitMethods(); i++ {
			meth := typ.ExplicitMethod(i)
			if !ast.IsExported(meth.Name()) {
				// private methods are package-only anyway,
				// so we don't need to care too much here
				// if we're just looking for breakage.
				continue
			}
			m.filterMap(meth.Type(), filterMap)
		}
		for i := 0; i < typ.NumEmbeddeds(); i++ {
			m.filterMap(typ.EmbeddedType(i), filterMap)
		}
	case *types.Map:
		m.filterMap(typ.Key(), filterMap)
		m.filterMap(typ.Elem(), filterMap)
	case *types.Chan:
		m.filterMap(typ.Elem(), filterMap)
	case *types.Named:
		if ast.IsExported(typ.Obj().Name()) {
			m.filterMap(typ.Underlying(), filterMap)
		}
	default:
		panic(fmt.Sprintf("unknown type %#v", rawType))
	}

	// indicate that we're done
	endRes := filterMap(nil)
	if endRes != nil {
		m.results = append(m.results, endRes)
	}
		
}
