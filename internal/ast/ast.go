package ast

// Program is a heading, then a block. The block is labels, constants, type
// definitions, variables, routines, and a compound statement, in that order.
// Files is the program-parameter list; it is nil when the heading has no
// parameter list. Body is the sequence of commands. Empty statements,
// including one introduced by a trailing semicolon, are omitted.
type Program struct {
	Name       string
	Files      []string
	FileAt     []*Name // source position of each name in Files
	Labels     []string
	LabelAt    []*Name // source position of each label in Labels
	Consts     []*ConstDef
	Types      []*TypeDef
	Vars       []*Decl
	Routines   []Routine
	Body       []Stmt
	MissingEnd Stmt // begin, when the final end was omitted
}

// Routine is a procedure or a function.
type Routine interface{ isRoutine() }

// Proc is a procedure. Its body is a block. Forward is a forward declaration:
// the block is supplied by a later declaration of the same name.
type Proc struct {
	Name     string
	Params   []Formal
	Labels   []string
	LabelAt  []*Name // source position of each label in Labels
	Consts   []*ConstDef
	Types    []*TypeDef
	Vars     []*Decl
	Routines []Routine
	Body     []Stmt
	Forward  bool
}

func (*Proc) isRoutine() {}

// Func is a function. Result is the type identifier of its result; it is empty
// on the completing declaration of a forward function. The result is assigned
// to the function name in the body. Forward is a forward declaration.
type Func struct {
	Name     string
	Params   []Formal
	Result   string
	Labels   []string
	LabelAt  []*Name // source position of each label in Labels
	Consts   []*ConstDef
	Types    []*TypeDef
	Vars     []*Decl
	Routines []Routine
	Body     []Stmt
	Forward  bool
}

func (*Func) isRoutine() {}

// Formal is one formal parameter section: a value or var section, a
// conformant-array section, or a procedure or function heading.
type Formal interface{ isFormal() }

// Param is a value or var parameter section. ByRef is true for a var parameter.
// Type is a type identifier.
type Param struct {
	Names []string
	ByRef bool
	Type  string
}

func (*Param) isFormal() {}

// ProcParam is a procedure heading used as a parameter.
type ProcParam struct {
	Name   string
	Params []Formal
}

func (*ProcParam) isFormal() {}

// FuncParam is a function heading used as a parameter.
type FuncParam struct {
	Name   string
	Params []Formal
	Result string
}

func (*FuncParam) isFormal() {}

// ConfParam is a conformant-array parameter section. ByRef is true when the
// section begins with var. Schema gives the index bounds and the component.
type ConfParam struct {
	Names  []string
	ByRef  bool
	Schema *ConfSchema
}

func (*ConfParam) isFormal() {}

// ConfSchema is one conformant-array-schema. Packed is true only for
// packed array [low..high : ordinal] of type-identifier. Indexes holds each
// index-type-specification; an unpacked schema may have more than one, which
// means an array of arrays. Nested is set when the component is itself a
// schema. Elem is the component type identifier otherwise.
type ConfSchema struct {
	Packed  bool
	Indexes []ConfIndex
	Elem    string
	Nested  *ConfSchema
}

// ConfIndex is low..high : ordinal-type. Low and High are the bound identifiers.
type ConfIndex struct {
	Low, High, Type string
}

// ConstDef gives a name to a constant.
type ConstDef struct {
	Name  string
	Value Expr
}

// TypeDef gives a name to a type.
type TypeDef struct {
	Name string
	Type Type
}

// Type is a type identifier, an enumeration, a subrange, an array, or a set.
type Type interface{ isType() }

// TypeName is a named type, such as integer or a name from a type definition.
// TypeName is a type identifier. Skip is set when text after the name was
// discarded. The name is then an error even when it denotes a type.
type TypeName struct {
	Name string
	Skip bool
}

func (*TypeName) isType() {}

// Enum is a scalar type written as a list of identifiers.
type Enum struct{ Names []string }

func (*Enum) isType() {}

// Subrange is a subrange of constants, written low..high.
type Subrange struct {
	Low  Expr
	High Expr
}

func (*Subrange) isType() {}

// Array is array [index, …] of element. Packed asks for a compact representation.
type Array struct {
	Packed bool
	Index  []Type
	Elem   Type
}

func (*Array) isType() {}

// Set is set of base type. The base type is a simple type. Packed asks for a
// compact representation.
type Set struct {
	Packed bool
	Base   Type
}

func (*Set) isType() {}

// File is a file type. Elem is the type of each component.
type File struct {
	Packed bool
	Elem   Type
}

func (*File) isType() {}

// Pointer is a pointer type. To is the domain type identifier.
type Pointer struct {
	To string
}

func (*Pointer) isType() {}

// Record is a record type. Fields is nil for an empty record.
type Record struct {
	Packed bool
	Fields *FieldList
}

func (*Record) isType() {}

// FieldList is the fixed fields of a record and its optional variant part.
type FieldList struct {
	Fixed   []*Field
	Variant *Variant
}

// Field is one record section. Several names may share a type.
type Field struct {
	Names []string
	Type  Type
}

// Variant is the case part of a record. Tag is empty when the case has no tag field.
type Variant struct {
	Tag  string
	Type string
	Arms []*VariantArm
}

// VariantArm is one variant. Fields is nil for an empty field list ().
type VariantArm struct {
	Labels []Expr
	Fields *FieldList
}

// Decl is one variable declaration.
type Decl struct {
	Names []string
	Type  Type
}

// Stmt is a statement in the compound statement.
type Stmt interface{ isStmt() }

// Call is a procedure statement. Args is empty when the call has no parameter
// list. An empty pair of parentheses is not a call.
type Call struct {
	Name string
	Args []Expr
}

func (*Call) isStmt() {}

// Loose is an identifier followed by text that is not a call, an assignment,
// or a factor. The checker reports the name.
type Loose struct{ Name string }

func (*Loose) isStmt() {}
func (*Loose) isExpr() {}

// Read is a call of read or readln. Vars are the variables read into.
// Ln selects readln. A file variable, if present, is the first variable.
type Read struct {
	Ln   bool
	Vars []Expr
}

func (*Read) isStmt() {}

// Write is a call of write or writeln. Ln selects writeln.
// A file variable, if present, is the first argument.
type Write struct {
	Ln   bool
	Args []WriteArg
}

func (*Write) isStmt() {}

// WriteArg is one write argument. Width and Frac are nil when omitted.
type WriteArg struct {
	Value Expr
	Width Expr
	Frac  Expr
}

// Goto transfers control to a label in the same block.
type Goto struct {
	Label string
}

func (*Goto) isStmt() {}

// Labeled prefixes a statement with a label. Stmt is nil when the statement is empty.
type Labeled struct {
	Label string
	Stmt  Stmt
}

func (*Labeled) isStmt() {}

// Assign stores a value in a variable. Left may be a name or an indexed variable.
type Assign struct {
	Left  Expr
	Value Expr
}

func (*Assign) isStmt() {}

// Compound is a begin/end statement. Open is set when end is missing and the
// program period follows the body.
type Compound struct {
	Body []Stmt
	Open bool
}

func (*Compound) isStmt() {}

// If is a conditional. Else is nil when there is no else branch.
// An empty branch is also nil.
type If struct {
	Cond Expr
	Then Stmt
	Else Stmt
}

func (*If) isStmt() {}

// While is a while-do statement.
type While struct {
	Cond Expr
	Body Stmt
}

func (*While) isStmt() {}

// Repeat is a repeat-until statement.
type Repeat struct {
	Body []Stmt
	Cond Expr
}

func (*Repeat) isStmt() {}

// For is a for-to or for-downto statement. Down is true for downto.
type For struct {
	Name string
	From Expr
	Down bool
	To   Expr
	Body Stmt
}

func (*For) isStmt() {}

// Case is a case statement.
type Case struct {
	Expr Expr
	Arms []*Arm
}

func (*Case) isStmt() {}

// With opens the fields of the listed record variables for its body.
type With struct {
	Recs []Expr
	Body Stmt
}

func (*With) isStmt() {}

// Arm is one case alternative. Stmt is nil when the branch is empty.
type Arm struct {
	Labels []Expr
	Stmt   Stmt
}

// Expr is the right-hand side of an assignment, a condition, or a procedure argument.
type Expr interface{ isExpr() }

// Int is an unsigned integer literal.
type Int struct{ Text string }

func (*Int) isExpr() {}

// Real is an unsigned real literal.
type Real struct{ Text string }

func (*Real) isExpr() {}

// Str is a character or string literal, with quotes removed.
type Str struct{ Value string }

func (*Str) isExpr() {}

// Name is an identifier used as a value.
type Name struct{ Ident string }

func (*Name) isExpr() {}

// CallExpr is a function designator with at least one argument.
// A function with no arguments is written as a name.
type CallExpr struct {
	Name string
	Args []Expr
}

func (*CallExpr) isExpr() {}

// Indexed is a variable followed by one bracketed index list, as in a[i, j].
// A second pair of brackets is another Indexed value whose base is the first.
type Indexed struct {
	Base    Expr
	Indexes []Expr
}

func (*Indexed) isExpr() {}

// Select is a field of a record variable, as in d.year.
type Select struct {
	Base  Expr
	Field string
}

func (*Select) isExpr() {}

// Deref is the variable identified by a pointer, or a file buffer: p^ or f^.
type Deref struct {
	Base Expr
}

func (*Deref) isExpr() {}

// SetLit is a set constructor. Elems is nil for the empty set [].
type SetLit struct {
	Elems []*SetElem
}

func (*SetLit) isExpr() {}

// SetElem is one member of a set constructor. High is nil when the member
// is a single expression rather than a range.
type SetElem struct {
	Low  Expr
	High Expr
}

// Nil is the nil value.
type Nil struct{}

func (*Nil) isExpr() {}

// Paren is a parenthesized expression. It has the value of the inner
// expression and is not a variable-access.
type Paren struct {
	X Expr
}

func (*Paren) isExpr() {}

// Unary is a sign or not applied to its operand.
type Unary struct {
	Op string
	X  Expr
}

func (*Unary) isExpr() {}

// Binary is an operator with two operands.
type Binary struct {
	Op    string
	Left  Expr
	Right Expr
}

func (*Binary) isExpr() {}
