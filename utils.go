//
// copyright 2023 Scott Means Consulting, LLC DBA CloudTerm Partners
//

package diffstream

import "errors"

func assert(f bool, m string) {
	if !f {
		panic(errors.New(m))
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
