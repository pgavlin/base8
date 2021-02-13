# base8

[![PkgGoDev](https://pkg.go.dev/badge/github.com/pgavlin/base8)](https://pkg.go.dev/github.com/pgavlin/base8)
[![codecov](https://codecov.io/gh/pgavlin/base8/branch/master/graph/badge.svg)](https://codecov.io/gh/pgavlin/base8)
[![Go Report Card](https://goreportcard.com/badge/github.com/pgavlin/base8)](https://goreportcard.com/report/github.com/pgavlin/base8)
[![Test](https://github.com/pgavlin/base8/workflows/Test/badge.svg)](https://github.com/pgavlin/base8/actions?query=workflow%3ATest)

Package base8 implements base8 encoding. This encoding uses the UTF-8 digits 0-7 to
represent arbitrary binary data in radix-8. Each non-final digit represets 3 bits of
data. Output is padded to a multiple of 8 digits using the '=' character.

