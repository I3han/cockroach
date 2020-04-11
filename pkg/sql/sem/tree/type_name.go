// Copyright 2020 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package tree

import (
	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgcode"
	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgerror"
	"github.com/cockroachdb/cockroach/pkg/sql/types"
	"github.com/cockroachdb/errors"
)

// TypeName corresponds to the name of a type in a CREATE TYPE statement,
// in an expression, or column type etc.
//
// Users of this struct should not construct it directly, and
// instead use the constructors below.
type TypeName struct {
	objName
}

// Satisfy the linter.
var _ = (*TypeName).Type
var _ = (*TypeName).FQString
var _ = NewUnqualifiedTypeName

// Type returns the unqualified name of this TypeName.
func (t *TypeName) Type() string {
	return string(t.ObjectName)
}

// Format implements the NodeFormatter interface.
func (t *TypeName) Format(ctx *FmtCtx) {
	t.ObjectNamePrefix.Format(ctx)
	if t.ExplicitSchema || ctx.alwaysFormatTablePrefix() {
		ctx.WriteByte('.')
	}
	ctx.FormatNode(&t.ObjectName)
}

// String implements the Stringer interface.
func (t *TypeName) String() string {
	return AsString(t)
}

// FQString renders the type name in full, not omitting the prefix
// schema and catalog names. Suitable for logging, etc.
func (t *TypeName) FQString() string {
	ctx := NewFmtCtx(FmtSimple)
	ctx.FormatNode(&t.CatalogName)
	ctx.WriteByte('.')
	ctx.FormatNode(&t.SchemaName)
	ctx.WriteByte('.')
	ctx.FormatNode(&t.ObjectName)
	return ctx.CloseAndGetString()
}

// NewUnqualifiedTypeName returns a new base type name.
func NewUnqualifiedTypeName(typ Name) *TypeName {
	return &TypeName{objName{
		ObjectName: typ,
	}}
}

// TypeReferenceResolver is the interface that will provide the ability
// to actually look up type metadata and transform references into
// *types.T's. In practice, this will probably be implemented by
// the planner, but for now it is a dummy interface.
type TypeReferenceResolver interface {
	// In the future this will take a context.
	ResolveType(name *UnresolvedObjectName) (*types.T, error)
}

// ResolvableTypeReference represents a type that is possibly unknown
// until type-checking/type name resolution is performed.
type ResolvableTypeReference interface {
	SQLString() string
}

var _ ResolvableTypeReference = &UnresolvedObjectName{}
var _ ResolvableTypeReference = &ArrayTypeReference{}
var _ ResolvableTypeReference = &types.T{}

// ResolveType converts a ResolvableTypeReference into a *types.T.
func ResolveType(ref ResolvableTypeReference, resolver TypeReferenceResolver) (*types.T, error) {
	switch t := ref.(type) {
	case *types.T:
		return t, nil
	case *ArrayTypeReference:
		typ, err := ResolveType(t.ElementType, resolver)
		if err != nil {
			return nil, err
		}
		return types.MakeArray(typ), nil
	case *UnresolvedObjectName:
		if resolver == nil {
			// If we don't have a resolver, we can't actually resolve this
			// name into a type.
			return nil, pgerror.Newf(pgcode.UndefinedObject, "type %q does not exist", t)
		}
		return resolver.ResolveType(t)
	default:
		return nil, errors.AssertionFailedf("unknown resolvable type reference type %s", t)
	}
}

// GetStaticallyKnownType possibly promotes a ResolvableTypeReference into a
// *types.T if the reference is a statically known type. It is only safe to
// access the returned type if ok is true.
func GetStaticallyKnownType(ref ResolvableTypeReference) (typ *types.T, ok bool) {
	typ, ok = ref.(*types.T)
	return typ, ok
}

// MustBeStaticallyKnownType does the same thing as GetStaticallyKnownType but panics
// in the case that the reference is not statically known. This function
// is intended to be used in tests or in cases where it is not possible
// to have any unresolved type references.
func MustBeStaticallyKnownType(ref ResolvableTypeReference) *types.T {
	if typ, ok := ref.(*types.T); ok {
		return typ
	}
	panic(errors.AssertionFailedf("type reference was not a statically known type"))
}

// ArrayTypeReference represents an array of possibly unknown type references.
type ArrayTypeReference struct {
	ElementType ResolvableTypeReference
}

// SQLString implements the ResolvableTypeReference interface.
func (node *ArrayTypeReference) SQLString() string {
	var ctx FmtCtx
	if typ, ok := GetStaticallyKnownType(node.ElementType); ok {
		ctx.WriteString(types.MakeArray(typ).SQLString())
	} else {
		ctx.WriteString(node.ElementType.SQLString())
		ctx.WriteString("[]")
	}
	return ctx.String()
}

// SQLString implements the ResolvableTypeReference interface.
func (name *UnresolvedObjectName) SQLString() string {
	return name.String()
}

// IsReferenceSerialType returns whether the input reference is a known
// serial type. It should only be used during parsing.
func IsReferenceSerialType(ref ResolvableTypeReference) bool {
	if typ, ok := GetStaticallyKnownType(ref); ok {
		return types.IsSerialType(typ)
	}
	return false
}

// TestingMapTypeResolver is a fake type resolver for testing purposes.
type TestingMapTypeResolver struct {
	typeMap map[string]*types.T
}

// ResolveType implements the TypeReferenceResolver interface.
func (dtr *TestingMapTypeResolver) ResolveType(name *UnresolvedObjectName) (*types.T, error) {
	typ, ok := dtr.typeMap[name.String()]
	if !ok {
		return nil, errors.Newf("type %q does not exist", name)
	}
	return typ, nil
}

// MakeTestingMapTypeResolver creates a TestingMapTypeResolver from a map.
func MakeTestingMapTypeResolver(typeMap map[string]*types.T) TypeReferenceResolver {
	return &TestingMapTypeResolver{typeMap: typeMap}
}
