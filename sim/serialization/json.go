package serialization

import (
	"encoding/json"
	"io"
)

type JSONCodec struct {
	writer io.Writer
	reader io.Reader
}

func NewJSONCodec(writer io.Writer, reader io.Reader) *JSONCodec {
	return &JSONCodec{
		writer: writer,
		reader: reader,
	}
}

func (c JSONCodec) Encode(v map[string]any) error {
	encoder := json.NewEncoder(c.writer)
	encoder.SetEscapeHTML(false)

	return encoder.Encode(v)
}

func (c JSONCodec) Decode() (map[string]any, error) {
	decoder := json.NewDecoder(c.reader)
	decoder.DisallowUnknownFields()

	v := map[string]any{}

	err := decoder.Decode(&v)
	if err != nil {
		return nil, err
	}

	return v, nil
}
