//go:build js && wasm

package main

import (
	"fmt"
	"net/http"
	"syscall/js"
	"time"

	"github.com/jiacai2050/peekd/internal/preview"
)

const maxPreviewSize = 512 << 10

type previewRequest struct {
	Name         string
	ContentType  string
	Size         int64
	LastModified int64
	Data         []byte
}

func main() {
	js.Global().Set("peekdPreview", js.FuncOf(previewFile))
	js.Global().Set("peekdVersion", "wasm")
	if ready := js.Global().Get("peekdReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}
	js.Global().Call("postMessage", map[string]any{"type": "ready"})
	select {}
}

func previewFile(_ js.Value, args []js.Value) any {
	if len(args) != 1 {
		return map[string]any{"error": "preview expects one file object"}
	}

	request, err := decodeRequest(args[0])
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return makePreview(request)
}

func decodeRequest(value js.Value) (previewRequest, error) {
	name := value.Get("name").String()
	if name == "" {
		return previewRequest{}, fmt.Errorf("file name is required")
	}

	request := previewRequest{
		Name:         name,
		ContentType:  value.Get("type").String(),
		Size:         valueInt64(value, "size"),
		LastModified: valueInt64(value, "lastModified"),
	}

	kind := preview.PreviewTypeByExtension(name)
	switch kind {
	case preview.PreviewTypeImage, preview.PreviewTypeAudio, preview.PreviewTypeVideo, preview.PreviewTypePDF:
		// Media files do not need content bytes in WASM; the browser renders them directly.
		return request, nil
	}

	dataValue := value.Get("data")
	if !dataValue.Truthy() {
		return previewRequest{}, fmt.Errorf("file data is required")
	}
	data := make([]byte, dataValue.Get("byteLength").Int())
	js.CopyBytesToGo(data, dataValue)
	request.Data = data

	if request.Size == 0 {
		request.Size = int64(len(data))
	}
	return request, nil
}

func valueInt64(value js.Value, property string) int64 {
	field := value.Get(property)
	if !field.Truthy() {
		return 0
	}
	return int64(field.Int())
}

func makePreview(request previewRequest) map[string]any {
	response := map[string]any{
		"name": request.Name,
		"size": preview.FormatFileSize(request.Size),
	}
	if request.LastModified > 0 {
		response["modified"] = time.UnixMilli(request.LastModified).Format("2006-01-02 15:04:05 -07:00")
	}

	kind := preview.PreviewTypeByExtension(request.Name)
	if kind == preview.PreviewTypeNone {
		contentType := request.ContentType
		if contentType == "" || contentType == "application/octet-stream" {
			contentType = http.DetectContentType(request.Data)
		}
		kind = preview.PreviewTypeByContent(contentType)
	}
	response["kind"] = string(kind)

	switch kind {
	case preview.PreviewTypeImage, preview.PreviewTypeAudio, preview.PreviewTypeVideo, preview.PreviewTypePDF:
		return response
	case preview.PreviewTypeZIP, preview.PreviewTypeTAR, preview.PreviewTypeTARGZ:
		entries, err := preview.ReadArchivePreviewBytes(request.Name, request.Data)
		if err != nil {
			response["kind"] = ""
			response["error"] = "unable to read archive: " + err.Error()
			return response
		}
		jsEntries := js.Global().Get("Array").New(len(entries))
		for i, e := range entries {
			obj := js.Global().Get("Object").New()
			obj.Set("name", e.Name)
			obj.Set("compressedSize", e.CompressedSize)
			obj.Set("uncompressedSize", e.UncompressedSize)
			obj.Set("ratio", e.Ratio)
			obj.Set("method", e.Method)
			obj.Set("permissions", e.Permissions)
			obj.Set("modified", e.Modified)
			obj.Set("isDir", e.IsDir)
			jsEntries.SetIndex(i, obj)
		}
		response["entries"] = jsEntries
		return response
	}

	prepared, preparedKind, err := preview.PrepareBytesPreview(kind, request.Data, maxPreviewSize)
	if err != nil {
		response["error"] = "unable to prepare preview: " + err.Error()
		return response
	}
	if preparedKind == preview.PreviewTypeNone {
		response["kind"] = ""
		return response
	}
	response["kind"] = string(preparedKind)

	switch preparedKind {
	case preview.PreviewTypeHTML, preview.PreviewTypeMarkdown, preview.PreviewTypeOrg:
		response["html"] = prepared.Formatted
	case preview.PreviewTypeCSV, preview.PreviewTypeTSV:
		if prepared.Rows != nil {
			jsRows := js.Global().Get("Array").New(len(prepared.Rows))
			for i, row := range prepared.Rows {
				jsRow := js.Global().Get("Array").New(len(row))
				for j, cell := range row {
					jsRow.SetIndex(j, cell)
				}
				jsRows.SetIndex(i, jsRow)
			}
			response["rows"] = jsRows
		}
	default:
		response["content"] = prepared.Formatted
	}
	return response
}
