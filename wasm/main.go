//go:build js && wasm

package main

import (
	"encoding/json"
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

type previewResponse struct {
	Kind     preview.PreviewType    `json:"kind"`
	Name     string                 `json:"name"`
	Size     string                 `json:"size"`
	Modified string                 `json:"modified,omitempty"`
	Content  string                 `json:"content,omitempty"`
	HTML     string                 `json:"html,omitempty"`
	Rows     [][]string             `json:"rows,omitempty"`
	Entries  []preview.ArchiveEntry `json:"entries,omitempty"`
	Error    string                 `json:"error,omitempty"`
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
		return responseJSON(previewResponse{Error: "preview expects one file object"})
	}

	request, err := decodeRequest(args[0])
	if err != nil {
		return responseJSON(previewResponse{Error: err.Error()})
	}
	response := makePreview(request)
	return responseJSON(response)
}

func decodeRequest(value js.Value) (previewRequest, error) {
	name := value.Get("name").String()
	if name == "" {
		return previewRequest{}, fmt.Errorf("file name is required")
	}

	dataValue := value.Get("data")
	if !dataValue.Truthy() {
		return previewRequest{}, fmt.Errorf("file data is required")
	}
	data := make([]byte, dataValue.Get("byteLength").Int())
	js.CopyBytesToGo(data, dataValue)

	request := previewRequest{
		Name:         name,
		ContentType:  value.Get("type").String(),
		Size:         valueInt64(value, "size"),
		LastModified: valueInt64(value, "lastModified"),
		Data:         data,
	}
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

func makePreview(request previewRequest) previewResponse {
	response := previewResponse{
		Name: request.Name,
		Size: preview.FormatFileSize(request.Size),
	}
	if request.LastModified > 0 {
		response.Modified = time.UnixMilli(request.LastModified).Format("2006-01-02 15:04:05")
	}

	kind := preview.PreviewTypeByExtension(request.Name)
	if kind == preview.PreviewTypeNone {
		contentType := request.ContentType
		if contentType == "" || contentType == "application/octet-stream" {
			contentType = http.DetectContentType(request.Data)
		}
		kind = preview.PreviewTypeByContent(contentType)
	}
	response.Kind = kind

	switch kind {
	case preview.PreviewTypeImage, preview.PreviewTypeAudio, preview.PreviewTypeVideo, preview.PreviewTypePDF:
		return response
	case preview.PreviewTypeZIP, preview.PreviewTypeTAR, preview.PreviewTypeTARGZ:
		entries, err := preview.ReadArchivePreviewBytes(request.Name, request.Data)
		if err != nil {
			response.Kind = preview.PreviewTypeNone
			response.Error = "unable to read archive: " + err.Error()
			return response
		}
		response.Entries = entries
		return response
	}

	prepared, preparedKind, err := preview.PrepareBytesPreview(kind, request.Data, maxPreviewSize)
	if err != nil {
		response.Error = "unable to prepare preview: " + err.Error()
		return response
	}
	if preparedKind == preview.PreviewTypeNone {
		response.Kind = preview.PreviewTypeNone
		return response
	}
	response.Kind = preparedKind
	response.Content = prepared.Formatted
	response.Rows = prepared.Rows
	if preparedKind == preview.PreviewTypeHTML {
		response.HTML = prepared.Formatted
		response.Content = ""
	}
	if preparedKind == preview.PreviewTypeMarkdown || preparedKind == preview.PreviewTypeOrg {
		response.HTML = prepared.Formatted
		response.Content = ""
	}
	return response
}

func responseJSON(response previewResponse) string {
	data, err := json.Marshal(response)
	if err != nil {
		return `{"error":"unable to encode preview response"}`
	}
	return string(data)
}
