# gopas

gopas translates a Standard Pascal program into Go and runs it. It conforms to ISO 7185:1990, Level 0 and Level 1 conformant-array parameters, and it passes the Pascal Validation Suite, version 5.7, as a whole.

## Build

Go 1.18 or later.

```
go build -o gopas ./cmd/gopas
```

`make gopas` does the same.

## Usage

```
gopas file.pas
gopas -o main.go file.pas
gopas -b hello file.pas
```

The first form translates the program and runs it. `-o` writes the Go source instead, and `-o -` writes that source to standard output. `-b` writes an executable to the given path and does not run it. `-o` and `-b` cannot be used together.

The defaults are short-circuit `and` and `or`, no overflow check, no range check, and `dispose` of a pointer variable.

- `-complete` evaluates both operands of `and` and `or`.
- `-overflow` stops the program when integer arithmetic overflows.
- `-range` stops when a value is outside its range, or a `case` has no matching label.
- `-disposeexpr` allows `dispose` of a pointer expression.

## Translation

`integer` is a Go `int`, `real` is `float64`, `char` is a byte, and `boolean` is `bool`. An array is a Go array. A set is a bit vector of the ordinals 0 through 255. A nested routine is a Go function, or a closure when it uses an enclosing variable. The runtime is copied into the generated program. `input` and `output` are standard input and standard output.

## What the translation does not carry out

`packed` does not change the layout. A packed array, record, set, or file uses the same storage as the unpacked form. The checker still treats them as different where the standard does, and `pack` and `unpack` still copy elements.

A variant record stores every field of every variant at once. The variants do not overlap. `new(p, c1, …, cn)` and `dispose(p, c1, …, cn)` check that the constants select a variant, then allocate or clear the pointer as `new(p)` and `dispose(p)` do. The constants are not written into the tag.

`dispose` does not free the record. It sets the pointer variable to nil, and the Go garbage collector reclaims the storage. Disposing a pointer that a `with` statement is using stops the program.

A subrange is stored as its host type. Without `-range`, a value outside the subrange is kept. An index outside a Go array still panics, because the array has a fixed length.

## Benchmarks

These figures are from one run of about a second on an AMD Ryzen 7 5700U. Another machine will produce different numbers. Both sides used the sources in `testdata/bench`. Free Pascal 3.2.2 was built with `-Miso -O3`.

| Program | gopas | Free Pascal |
| --- | --- | --- |
| Whetstone | 2080 MWIPS (1.01 s) | 1736 MWIPS (1.21 s) |
| Dhrystone | 15.5 million/s (1.06 s) | 5.56 million/s (2.97 s) |

`go test -count=1 -v -run 'TestWhetstone|TestDhrystone' ./cmd/gopas` repeats the gopas measurement.

## Tests

```
make test
```

This runs `go test ./...`.

`testdata/pvs` is the Pascal Validation Suite, version 5.7, copyright the British Standards Institution. It is included so the translator can be checked against ISO 7185. Running it here is not a validation by BSI or by anyone else, and the result is for the whole suite.

A conformance program must run and report PASS. The empty program is the exception and prints nothing. A deviance or extension program must be rejected before it finishes. An error-handling pretest must run; the error itself may be left undetected. An implementation-defined or implementation-dependent program must run.

`testdata/p5` is the P5 Pascal-P test suite. A test of a designated error that the standard allows an implementation to leave unreported is skipped.

## Limits

gopas implements the language of ISO 7185:1990 and rejects a program that depends on an extension from another compiler.

`integer` is Go's `int`. `maxint` is the largest value of that type, and the smallest integer is `-maxint - 1`. On a 64-bit machine `maxint` is 9223372036854775807. An integer operation that overflows wraps within that range. With `-overflow`, the program stops instead.

`div` truncates toward zero, so `(-7) div 2` is `-3`. `mod` with a positive divisor returns a remainder in `0` through `divisor - 1`, so `(-7) mod 2` is `1`. Division by zero stops the program, as does `mod` with a zero or negative divisor. Real division also stops on a zero divisor.

The standard leaves the value of a fresh variable undefined. gopas sets it to zero: a number is 0, a boolean is false, a character is `chr(0)`, a pointer is nil, and every component of an array, record, or set is zero in the same way.

`eoln(f)` is true when the text file `f` is at the end of a line, and also when it is at the end of the file. A loop that reads until end of line therefore stops at the end of the file as well.

`write` of a real with no fraction width uses scientific notation, such as `1.2500000000000000E+01`. A fraction width selects fixed point with that many digits after the decimal point. When the total width is omitted, an integer uses 11 columns, a real in scientific form uses 29, a boolean uses 5 (`TRUE` or `FALSE`), a character uses 1, and a string uses its own length. A value longer than the requested width is written in full, except a boolean or a string, which is cut to fit.

A violation of a language rule is reported and the program is not run. A designated error is a situation the standard lists as an error and allows an implementation to leave undetected. gopas may let such a program continue. Storing a value outside a subrange is one of these: the value is kept, unless `-range` is set, in which case the program stops. An index outside an array still stops the program, because a Go array has a fixed length.
