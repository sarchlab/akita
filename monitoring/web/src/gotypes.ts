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

export function isContainerKind(kind: VarKind) {
	const list = [17, 18, 21, 23];
	return list.includes(kind)
}

export function isDirectKind(kind: VarKind) {
	const list = [0, 1, 2, 3, 4, 5, 6, 7, 8, 8, 10, 11, 13, 14, 15, 24];
	return list.includes(kind)
}