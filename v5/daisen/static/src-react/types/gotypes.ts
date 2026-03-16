/** Mirrors Go's reflect.Kind enum values. */
export enum VarKind {
  Invalid,
  Bool,
  Int,
  Int8,
  Int16,
  Int32,
  Int64,
  Uint,
  Uint8,
  Uint16,
  Uint32,
  Uint64,
  Uintptr,
  Float32,
  Float64,
  Complex64,
  Complex128,
  Array,
  Chan,
  Func,
  Interface,
  Map,
  Pointer,
  Slice,
  String,
  Struct,
  UnsafePointer,
}

/** Container kinds: Array, Chan, Map, Slice, Struct */
export function isContainerKind(kind: VarKind): boolean {
  return [
    VarKind.Array,
    VarKind.Chan,
    VarKind.Map,
    VarKind.Slice,
    VarKind.Struct,
  ].includes(kind);
}

/** Direct (leaf) kinds: scalars, pointers, strings, etc. */
export function isDirectKind(kind: VarKind): boolean {
  return [
    VarKind.Invalid,
    VarKind.Bool,
    VarKind.Int,
    VarKind.Int8,
    VarKind.Int16,
    VarKind.Int32,
    VarKind.Int64,
    VarKind.Uint,
    VarKind.Uint8,
    VarKind.Uint16,
    VarKind.Uint32,
    VarKind.Uint64,
    VarKind.Float32,
    VarKind.Float64,
    VarKind.Complex64,
    VarKind.String,
    VarKind.UnsafePointer,
  ].includes(kind);
}
