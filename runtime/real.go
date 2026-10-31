package runtime

import "math"

// AbsReal returns the absolute value of x.
func AbsReal(x float64) float64 { return math.Abs(x) }

// Round returns the nearest integer, with ties away from zero.
func Round(x float64) int { return int(math.Round(x)) }

// Sin, Cos, Exp, Ln, Sqrt, and Arctan are the required real functions.
func Sin(x float64) float64    { return math.Sin(x) }
func Cos(x float64) float64    { return math.Cos(x) }
func Exp(x float64) float64    { return math.Exp(x) }
func Arctan(x float64) float64 { return math.Atan(x) }

// Ln panics when x is not positive.
func Ln(x float64) float64 {
	if x <= 0 {
		panic("ln domain")
	}
	return math.Log(x)
}

// Sqrt panics when x is negative.
func Sqrt(x float64) float64 {
	if x < 0 {
		panic("sqrt domain")
	}
	return math.Sqrt(x)
}
