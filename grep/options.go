package grep

import (
	"net/http"
	"strconv"
	"strings"
)

type GrepOption interface {
	SkipFile(string) bool
	SkipFileContent([]byte) bool
	SetData(interface{})
}

type ExtensionFilterOption struct {
	extensions []string
	inverse    bool
}

func (f *ExtensionFilterOption) fileHasAnyExtension(path string) bool {
	for _, e := range f.extensions {
		if strings.HasSuffix(path, e) {
			return true
		}
	}

	return false
}

func (f *ExtensionFilterOption) SkipFile(path string) bool {
	/*
		hasExtension		inverse		result
		f					f			f
		f					t			t
		t					f			t
		t					t			f
	*/

	// go does not have a XOR operation for booleans, so resort to "values being different"
	return f.fileHasAnyExtension(path) != f.inverse
}

func (f *ExtensionFilterOption) SkipFileContent([]byte) bool {
	return false
}

func (f *ExtensionFilterOption) SetData(interface{}) {}

func WithFileExtensions(extensions ...string) GrepOption {
	return &ExtensionFilterOption{
		extensions: extensions,
		inverse:    false,
	}
}

func WithExcludedFileExtensions(extensions ...string) GrepOption {
	return &ExtensionFilterOption{
		extensions: extensions,
		inverse:    true,
	}
}

type FileContentFilterOption struct {
	cb func([]byte) bool
}

func (f *FileContentFilterOption) SkipFile(string) bool {
	return false
}

func (f *FileContentFilterOption) SkipFileContent(data []byte) bool {
	return f.cb(data)
}

func (f *FileContentFilterOption) SetData(interface{}) {}

func WithPrintableContent() GrepOption {
	return &FileContentFilterOption{
		cb: func(data []byte) bool {
			const checkSize = 64
			const neededPrintable = int(0.75 * checkSize)

			maxPos := MinInt(len(data), checkSize)
			nReadable := 0

			for i := 0; i < maxPos; i++ {
				if strconv.IsPrint(rune(data[i])) {
					nReadable++
				}
			}

			return nReadable >= neededPrintable
		},
	}
}

func ContentFilterType(fileTypes []string) GrepOption {
	return &FileContentFilterOption{
		cb: func(data []byte) bool {
			contentType := http.DetectContentType(data)
			for i := range fileTypes {
				if strings.Contains(contentType, fileTypes[i]) {
					return false
				}
			}
			return true
		},
	}
}

func SettingData(interface{}) GrepOption {
	return nil
}
