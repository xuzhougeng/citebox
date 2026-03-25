//go:build cgo && !nocgo

package fitz

/*
#cgo CFLAGS: -Iinclude

#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/libs -lmupdf_linux_amd64 -lmupdfthird_linux_amd64 -lm
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/libs -lmupdf_linux_arm64 -lmupdfthird_linux_arm64 -lm
#cgo windows,amd64 LDFLAGS: -L${SRCDIR}/libs -lmupdf_windows_amd64 -lmupdfthird_windows_amd64 -lm -lcomdlg32 -lgdi32
#cgo darwin,amd64 LDFLAGS: -L${SRCDIR}/libs -lmupdf_darwin_amd64 -lmupdfthird_darwin_amd64 -lm
#cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/libs -lmupdf_darwin_arm64 -lmupdfthird_darwin_arm64 -lm
*/
import "C"
