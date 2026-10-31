package check

import (
	"fmt"
	"strings"
)

// Code is the number printed with a diagnostic.
// Numbers shared with Pascal-P mean the same fault. The wording is ours.
type Code int

const (
	ErrBadStmt          Code = 442
	ErrParamList        Code = 7
	ErrBadType          Code = 10
	ErrConfSchema       Code = 10
	ErrNotConst         Code = 50
	ErrBadExpr          Code = 58
	ErrDupIdent         Code = 101
	ErrEmptySubrange    Code = 102
	ErrIsFunc           Code = 103
	ErrNotDeclared      Code = 104
	ErrSign             Code = 105
	ErrSubrange         Code = 107
	ErrTagOrdinal       Code = 110
	ErrNewTag           Code = 111
	ErrIndexOrdinal     Code = 113
	ErrSetBase          Code = 115
	ErrPackComp         Code = 116
	ErrForward          Code = 117
	ErrPtrOpen          Code = 118
	ErrResultType       Code = 120
	ErrFileParam        Code = 121
	ErrNoResult         Code = 123
	ErrWriteFrac        Code = 124
	ErrFuncInt          Code = 125
	ErrArgCount         Code = 126
	ErrConfBounds       Code = 127
	ErrOpType           Code = 129
	ErrIn               Code = 130
	ErrBadOp            Code = 134
	ErrCondBool         Code = 135
	ErrSetMember        Code = 136
	ErrSetRange         Code = 137
	ErrSetTypes         Code = 137
	ErrNeedArray        Code = 138
	ErrIndexType        Code = 139
	ErrNotRecord        Code = 140
	ErrDeref            Code = 141
	ErrForOrdinal       Code = 143
	ErrCaseOrdinal      Code = 144
	ErrForBounds        Code = 145
	ErrResultMismatch   Code = 145
	ErrTypeConflict     Code = 145
	ErrCaseLabel        Code = 147
	ErrNoField          Code = 152
	ErrReadMismatch     Code = 153
	ErrNeedVar          Code = 154
	ErrThreaten         Code = 155
	ErrDupCase          Code = 156
	ErrNeedVariant      Code = 158
	ErrDupLabel         Code = 165
	ErrUndefLabel       Code = 167
	ErrLabelNoStmt      Code = 168
	ErrProgramParamFile Code = 172
	ErrBadPredef        Code = 174
	ErrInput            Code = 175
	ErrNeedOutput       Code = 176
	ErrDupVariant       Code = 178
	ErrForVar           Code = 183
	ErrGoto             Code = 185
	ErrProcMismatch     Code = 189
	ErrFileOfFile       Code = 190
	ErrNeedPacked       Code = 197
	ErrVarType          Code = 199
	ErrVariantCover     Code = 200
	ErrCharRange        Code = 203
	ErrLabelRange       Code = 205
	ErrWriteKind        Code = 236
	ErrWriteType        Code = 236
	ErrDupProgramParam  Code = 301
	ErrSetSize          Code = 398
	ErrConfRange        Code = 401
	ErrConfType         Code = 402
	ErrDivType          Code = 403
	ErrFuncArgs         Code = 404
	ErrFuncCall         Code = 405
	ErrFuncNum          Code = 406
	ErrIndexRange       Code = 407
	ErrIsProc           Code = 408
	ErrNeedFile         Code = 409
	ErrNeedInput        Code = 410
	ErrNeedList         Code = 411
	ErrNeedOrdinal      Code = 412
	ErrNeedPtr          Code = 413
	ErrNeedText         Code = 414
	ErrNeedUnpacked     Code = 415
	ErrNoVariant        Code = 416
	ErrNotArray         Code = 417
	ErrNotBool          Code = 418
	ErrNotConstExpr     Code = 419
	ErrNotConstOp       Code = 420
	ErrNotProc          Code = 421
	ErrNotRoutine       Code = 422
	ErrNotType          Code = 423
	ErrNotValue         Code = 424
	ErrNotVar           Code = 425
	ErrOneArg           Code = 426
	ErrOpBool           Code = 427
	ErrOpInt            Code = 428
	ErrOrdArgs          Code = 429
	ErrOrdType          Code = 430
	ErrPackIndex        Code = 431
	ErrPackRange        Code = 432
	ErrPackSpan         Code = 467
	ErrPage             Code = 433
	ErrProcArg          Code = 434
	ErrRange            Code = 435
	ErrReadInput        Code = 436
	ErrReadList         Code = 437
	ErrReadText         Code = 438
	ErrReadType         Code = 439
	ErrReadVar          Code = 440
	ErrSignType         Code = 441
	ErrSyntax           Code = 6
	ErrTagArgs          Code = 443
	ErrTagLabel         Code = 444
	ErrTagRange         Code = 445
	ErrThree            Code = 446
	ErrValType          Code = 447
	ErrVarParam         Code = 448
	ErrWith             Code = 449
	ErrWriteFracInt     Code = 450
	ErrWriteList        Code = 451
	ErrWriteOutput      Code = 452
	ErrWriteText        Code = 453
	ErrWriteValue       Code = 454
	ErrWriteWidth       Code = 455
	ErrWriteWidthFile   Code = 456
	ErrEmptyString      Code = 457
	ErrFuncResult       Code = 458
	ErrIncomplete       Code = 459
	ErrBeforeDef        Code = 460
	ErrArraySize        Code = 461
	ErrIntRange         Code = 462
	ErrExponent         Code = 463
	ErrPackedVar        Code = 464
	ErrTagVar           Code = 465
	ErrJoined           Code = 466
	ErrIndexVar         Code = 468
	ErrNeedReal         Code = 469
	ErrConfSame         Code = 470
	ErrConfValue        Code = 471
	ErrPtrVar           Code = 472
)

var textOf = map[Code]string{
	ErrBadStmt:          "illegal symbol",
	ErrParamList:        "error in parameter list",
	ErrBadType:          "error in type",
	ErrNotConst:         "'%s' is not a constant",
	ErrBadExpr:          "error in expression",
	ErrDupIdent:         "identifier '%s' declared twice",
	ErrEmptySubrange:    "low bound exceeds high bound",
	ErrIsFunc:           "'%s' is a function",
	ErrNotDeclared:      "identifier '%s' not declared",
	ErrSign:             "sign not allowed",
	ErrSubrange:         "subrange bounds are not the same ordinal type",
	ErrTagOrdinal:       "tag type must be ordinal",
	ErrNewTag:           "'%s' tag is not a constant of the tag type",
	ErrIndexOrdinal:     "index type must be ordinal",
	ErrSetBase:          "set base must be ordinal",
	ErrPackComp:         "'%s' component types differ",
	ErrForward:          "forward declaration '%s' not completed",
	ErrPtrOpen:          "unresolved pointer",
	ErrResultType:       "function result is not a simple or pointer type",
	ErrFileParam:        "value parameter has a file type",
	ErrNoResult:         "function '%s' has no result type",
	ErrWriteFrac:        "fraction is allowed only for a real",
	ErrFuncInt:          "function requires an integer",
	ErrArgCount:         "number of parameters does not agree",
	ErrConfBounds:       "conformant array bounds differ",
	ErrOpType:           "type conflict of operands",
	ErrIn:               "type conflict in 'in'",
	ErrBadOp:            "unknown operator '%s'",
	ErrCondBool:         "condition must be boolean",
	ErrSetMember:        "set element must be ordinal",
	ErrSetRange:         "set element types differ",
	ErrNeedArray:        "'%s' requires an array",
	ErrIndexType:        "index type incompatible",
	ErrNotRecord:        "field '%s' of a non-record",
	ErrDeref:            "'^' applied to a non-pointer",
	ErrForOrdinal:       "control variable must be ordinal",
	ErrCaseOrdinal:      "case selector must be ordinal",
	ErrForBounds:        "type conflict",
	ErrCaseLabel:        "case label type incompatible",
	ErrNoField:          "no such field '%s'",
	ErrReadMismatch:     "type error in read",
	ErrNeedVar:          "'%s' requires a variable",
	ErrThreaten:         "control variable '%s' is threatened",
	ErrDupCase:          "case label declared twice",
	ErrNeedVariant:      "'%s' requires a variant record",
	ErrDupLabel:         "label '%s' declared twice",
	ErrUndefLabel:       "label '%s' not declared",
	ErrLabelNoStmt:      "label '%s' labels no statement",
	ErrProgramParamFile: "program parameter '%s' is not a file",
	ErrBadPredef:        "unknown procedure '%s'",
	ErrInput:            "requires input",
	ErrNeedOutput:       "'%s' requires output",
	ErrDupVariant:       "variant label declared twice",
	ErrForVar:           "control variable '%s' is not local",
	ErrGoto:             "goto '%s' enters a structured statement",
	ErrProcMismatch:     "procedural parameter mismatch",
	ErrFileOfFile:       "file component contains a file",
	ErrNeedPacked:       "'%s' requires a packed array",
	ErrVarType:          "variable parameter type mismatch",
	ErrVariantCover:     "variant does not cover '%s'",
	ErrCharRange:        "character outside the set",
	ErrLabelRange:       "label out of range",
	ErrWriteKind:        "type error in write",
	ErrDupProgramParam:  "program parameter '%s' declared twice",
	ErrSetSize:          "set base exceeds 256 values",
	ErrConfRange:        "conformant bound out of range",
	ErrConfType:         "conformant array type mismatch",
	ErrDivType:          "'/' requires reals",
	ErrFuncArgs:         "function requires one argument",
	ErrFuncCall:         "function '%s' requires arguments",
	ErrFuncNum:          "function requires an integer or real",
	ErrIndexRange:       "index out of range",
	ErrIsProc:           "'%s' is a procedure",
	ErrNeedFile:         "requires a file variable",
	ErrNeedInput:        "'%s' requires input",
	ErrNeedList:         "'%s' requires its parameter list",
	ErrNeedOrdinal:      "'%s' requires an ordinal",
	ErrNeedPtr:          "'%s' requires a pointer variable",
	ErrNeedText:         "requires a textfile",
	ErrNeedUnpacked:     "'%s' requires an unpacked array",
	ErrNoVariant:        "'%s' tag selects no variant",
	ErrNotArray:         "type of variable is not array",
	ErrNotBool:          "'not' requires a boolean",
	ErrNotConstExpr:     "expression is not a constant",
	ErrNotConstOp:       "'not' is not a constant operator",
	ErrNotProc:          "'%s' is not a procedure",
	ErrNotRoutine:       "'%s' is not a procedure or function",
	ErrNotType:          "'%s' is not a type",
	ErrNotValue:         "'%s' is not a value",
	ErrNotVar:           "assignment requires a variable",
	ErrOneArg:           "'%s' requires one argument",
	ErrOpBool:           "operator '%s' requires booleans",
	ErrOpInt:            "operator '%s' requires integers",
	ErrOrdArgs:          "ord requires one argument",
	ErrOrdType:          "ord requires an ordinal",
	ErrPackIndex:        "'%s' index type mismatch",
	ErrPackRange:        "'%s' index out of range",
	ErrPackSpan:         "'%s' copies past the end of the array",
	ErrPage:             "page requires output",
	ErrProcArg:          "procedural parameter requires a routine",
	ErrRange:            "constant out of range",
	ErrReadInput:        "readln requires input",
	ErrReadList:         "readln requires its parameter list",
	ErrReadText:         "readln requires a textfile",
	ErrReadType:         "read requires an integer, real, or char",
	ErrReadVar:          "read requires a variable",
	ErrSignType:         "sign requires an integer or real",
	ErrSyntax:           "expected %s",
	ErrTagArgs:          "'%s' has too many tag arguments",
	ErrTagLabel:         "variant label is not a constant of the tag type",
	ErrTagRange:         "'%s' tag out of range",
	ErrThree:            "'%s' requires three arguments",
	ErrValType:          "parameter type mismatch",
	ErrVarParam:         "actual parameter must be a variable",
	ErrWith:             "with requires a record variable",
	ErrWriteFracInt:     "write fraction is not an integer",
	ErrWriteList:        "writeln requires its parameter list",
	ErrWriteOutput:      "writeln requires output",
	ErrWriteText:        "writeln requires a textfile",
	ErrWriteValue:       "write requires a value",
	ErrWriteWidth:       "write width is not an integer",
	ErrWriteWidthFile:   "fraction or width on a non-text file",
	ErrEmptyString:      "empty string not allowed",
	ErrFuncResult:       "function '%s' does not assign its result",
	ErrIncomplete:       "type '%s' is incomplete",
	ErrBeforeDef:        "identifier '%s' is used before its definition",
	ErrArraySize:        "array is too large",
	ErrIntRange:         "integer constant exceeds maxint",
	ErrExponent:         "real exponent is out of range",
	ErrPackedVar:        "variable parameter is a component of a packed variable",
	ErrTagVar:           "variable parameter is a tag field",
	ErrJoined:           "number and word are not separated",
	ErrIndexVar:         "index requires a variable",
	ErrNeedReal:         "'%s' requires a real",
	ErrConfSame:         "conformant section requires the same type",
	ErrConfValue:        "value conformant array requires an array type",
	ErrPtrVar:           "'^' requires a variable",
}

func (c Code) format(args ...any) string {
	t, ok := textOf[c]
	if !ok {
		return fmt.Sprintf("error %d", int(c))
	}
	if strings.Contains(t, "%") {
		return fmt.Sprintf(t, args...)
	}
	return t
}

// Syntax classifies a parser failure. detail is the pigeon message after the
// rule name. A mismatch lists every token that could have occurred there,
// including alternatives, and leaves out spacing and comment delimiters.
func Syntax(detail string) (Code, string) {
	switch detail {
	case "label out of range":
		return ErrLabelRange, ErrLabelRange.format()
	case "invalid parameter list":
		return ErrParamList, ErrParamList.format()
	case "number and word are not separated":
		return ErrJoined, ErrJoined.format()
	}
	const key = "expected:"
	i := strings.Index(detail, key)
	if i < 0 {
		return ErrSyntax, "syntax error"
	}
	list := expectedList(detail[i+len(key):])
	if list == "" {
		return ErrSyntax, "syntax error"
	}
	return ErrSyntax, ErrSyntax.format(list)
}

func expectedList(raw string) string {
	raw = strings.ReplaceAll(raw, " or ", ", ")
	var keep []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" || part == "..." || skipExpected(part) {
			continue
		}
		keep = append(keep, displayToken(part))
	}
	keep = unique(keep)
	if len(keep) == 0 {
		return ""
	}
	if len(keep) == 1 {
		return keep[0]
	}
	return strings.Join(keep[:len(keep)-1], ", ") + " or " + keep[len(keep)-1]
}

func skipExpected(tok string) bool {
	switch tok {
	case `"(*"`, `"{"`, `"\n"`, ".":
		return true
	}
	// A "not newline" predicate is how a string stops at the end of the line.
	if tok == `!"\n"` || tok == `!"\r"` {
		return true
	}
	if strings.HasPrefix(tok, "[") && strings.HasSuffix(tok, "]") {
		body := tok[1 : len(tok)-1]
		body = strings.ReplaceAll(body, `\t`, "")
		body = strings.ReplaceAll(body, `\r`, "")
		body = strings.ReplaceAll(body, `\n`, "")
		body = strings.ReplaceAll(body, " ", "")
		return body == ""
	}
	return false
}

func displayToken(tok string) string {
	switch tok {
	case "[0-9]":
		return "digit"
	case "[A-Za-z]":
		return "letter"
	case "[A-Za-z0-9]":
		return "letter or digit"
	}
	if strings.HasPrefix(tok, `"`) {
		s := strings.TrimSuffix(tok, "i")
		s = strings.Trim(s, `"`)
		if s != "" && strings.Trim(s, "'") == "" {
			return "'''"
		}
		return "'" + s + "'"
	}
	return tok
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
