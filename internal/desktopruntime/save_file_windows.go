//go:build windows

package desktopruntime

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	ofnOverwritePrompt = 0x00000002
	ofnHideReadOnly    = 0x00000004
	ofnPathMustExist   = 0x00000800
	ofnNoChangeDir     = 0x00000008
	ofnExplorer        = 0x00080000
)

var (
	windowsFilenameSanitizer = strings.NewReplacer(
		"<", "_",
		">", "_",
		":", "_",
		"\"", "_",
		"/", "_",
		"\\", "_",
		"|", "_",
		"?", "_",
		"*", "_",
	)

	comdlg32DLL              = syscall.NewLazyDLL("comdlg32.dll")
	getSaveFileNameWProc     = comdlg32DLL.NewProc("GetSaveFileNameW")
	commDlgExtendedErrorProc = comdlg32DLL.NewProc("CommDlgExtendedError")
)

type openFilename struct {
	structSize       uint32
	owner            uintptr
	instance         uintptr
	filter           *uint16
	customFilter     *uint16
	maxCustomFilter  uint32
	filterIndex      uint32
	file             *uint16
	maxFile          uint32
	fileTitle        *uint16
	maxFileTitle     uint32
	initialDir       *uint16
	title            *uint16
	flags            uint32
	fileOffset       uint16
	fileExtension    uint16
	defaultExtension *uint16
	customData       uintptr
	hook             uintptr
	templateName     *uint16
	reserved         unsafe.Pointer
	reservedUint32   uint32
	flagsEx          uint32
}

func saveFile(filename string, dataBase64 string) (bool, error) {
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return false, fmt.Errorf("decode file data: %w", err)
	}

	targetPath, saved, err := chooseSavePath(filename)
	if err != nil || !saved {
		return false, err
	}

	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		return false, fmt.Errorf("save file: %w", err)
	}

	return true, nil
}

func chooseSavePath(filename string) (string, bool, error) {
	defaultName := sanitizeWindowsFilename(filename)
	fileBuffer := make([]uint16, 4096)
	defaultNameUTF16 := syscall.StringToUTF16(defaultName)
	if len(defaultNameUTF16) > len(fileBuffer) {
		defaultNameUTF16 = append([]uint16{}, defaultNameUTF16[:len(fileBuffer)]...)
		defaultNameUTF16[len(defaultNameUTF16)-1] = 0
	}
	copy(fileBuffer, defaultNameUTF16)

	var defExt []uint16
	var defExtPtr *uint16
	if ext := strings.TrimPrefix(filepath.Ext(defaultName), "."); ext != "" {
		defExt = syscall.StringToUTF16(ext)
		defExtPtr = &defExt[0]
	}

	dialogTitle := syscall.StringToUTF16("Save File")
	allFilesFilter := utf16WithTrailingNull("All Files\x00*.*\x00\x00")

	dialog := openFilename{
		structSize:       uint32(unsafe.Sizeof(openFilename{})),
		filter:           &allFilesFilter[0],
		filterIndex:      1,
		file:             &fileBuffer[0],
		maxFile:          uint32(len(fileBuffer)),
		title:            &dialogTitle[0],
		flags:            ofnExplorer | ofnNoChangeDir | ofnOverwritePrompt | ofnPathMustExist | ofnHideReadOnly,
		defaultExtension: defExtPtr,
	}

	result, _, _ := getSaveFileNameWProc.Call(uintptr(unsafe.Pointer(&dialog)))
	if result == 0 {
		extendedErr, _, _ := commDlgExtendedErrorProc.Call()
		if extendedErr == 0 {
			return "", false, nil
		}
		return "", false, fmt.Errorf("show save dialog: 0x%x", uint32(extendedErr))
	}

	return syscall.UTF16ToString(fileBuffer), true, nil
}

func utf16WithTrailingNull(value string) []uint16 {
	encoded := utf16.Encode([]rune(value))
	if len(encoded) == 0 || encoded[len(encoded)-1] != 0 {
		encoded = append(encoded, 0)
	}
	return encoded
}

func sanitizeWindowsFilename(filename string) string {
	name := strings.TrimSpace(filename)
	if name == "" {
		return "download.bin"
	}

	name = filepath.Base(name)
	name = windowsFilenameSanitizer.Replace(name)
	name = strings.Trim(name, ". ")
	if name == "" {
		return "download.bin"
	}

	stem := strings.TrimSuffix(name, filepath.Ext(name))
	switch strings.ToUpper(stem) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return "_" + name
	default:
		return name
	}
}
