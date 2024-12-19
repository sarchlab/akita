package serialization

import (
	"encoding/json"
	"io"
)

type JSONCodec struct {
}

func NewJSONCodec() *JSONCodec {
	return &JSONCodec{}
}

func (c JSONCodec) Encode(v map[string]*Value, writer io.Writer) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)

	return encoder.Encode(v)
}

func (c JSONCodec) Decode(reader io.Reader) (map[string]*Value, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	v := map[string]*Value{}

	err := decoder.Decode(&v)
	if err != nil {
		return nil, err
	}

	return v, nil
}
