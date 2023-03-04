//
// copyright 2023 Scott Means Consulting, LLC DBA CloudTerm Partners
//

package diffstream

import (
	"errors"

	"golang.org/x/exp/constraints"
)

func assert(f bool) {
	if !f {
		panic(errors.New("assertion failed"))
	}
}

func max[T constraints.Ordered](a T, b T) T {
	if a > b {
		return a
	} else {
		return b
	}
}

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	} else {
		return b
	}
}

func nonZero[T comparable](s []T) bool {
	var z T

	for _, v := range s {
		if v != z {
			return true
		}
	}

	return false
}

func isZero[T comparable](s []T) bool {
	return !nonZero(s)
}

func findElement[T comparable](s []T, v T) int {
	for i, sv := range s {
		if sv == v {
			return i
		}
	}

	return -1
}

func filterSlice[T any](s []T, ffn func(i int, v T) bool) []T {
	sfi := []T{}

	for i, v := range s {
		if ffn(i, v) {
			sfi = append(sfi, v)
		}
	}

	return sfi
}

func visitSlice[T any](s []T, vfn func(v T) bool) {
	for _, v := range s {
		if !vfn(v) {
			break
		}
	}
}

func insertElement[T any](s []T, i int, v T) []T {
	if len(s) == i {
		return append(s, v)
	}

	s = append(s[:i+1], s[i:]...)
	s[i] = v

	return s
}

func insertSlice[T any](s []T, i int, v []T) []T {
	for vi, vv := range v {
		s = insertElement(s, i+vi, vv)
	}

	return s
}
