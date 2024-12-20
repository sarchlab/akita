package stateful

import (
	"encoding/json"
	"io"
)

// Codec determines how states is encoded.
type Codec interface {
	Encode(w io.Writer, data map[string]any) error
	Decode(r io.Reader) (map[string]any, error)
}

type JSONCodec struct{}

// Encode writes the data map as JSON to the provided writer
func (c JSONCodec) Encode(w io.Writer, data map[string]any) error {
	encoder := json.NewEncoder(w)
	return encoder.Encode(data)
}

// Decode reads JSON data from the reader and returns it as a map
func (c JSONCodec) Decode(r io.Reader) (map[string]any, error) {
	decoder := json.NewDecoder(r)

	var data map[string]any

	err := decoder.Decode(&data)
	if err != nil {
		return nil, err
	}

	return data, nil
}
