package codec

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/viant/datly"
)

// DatlyComponentCodec adapts datly.Service to the ComponentCodec interface.
type DatlyComponentCodec struct {
	dao *datly.Service
}

// NewDatly constructs a new Datly-backed codec.
func NewDatly(dao *datly.Service) (*DatlyComponentCodec, error) {
	if dao == nil {
		return nil, fmt.Errorf("datly service is required")
	}
	return &DatlyComponentCodec{dao: dao}, nil
}

func (c *DatlyComponentCodec) Marshal(w http.ResponseWriter, r *http.Request, method, uri string, data interface{}) error {
	if c == nil || c.dao == nil {
		return fmt.Errorf("codec not initialized")
	}
	marshaler, contentType, _, err := c.dao.GetMarshaller(r, method+":"+uri)
	if err != nil {
		return err
	}
	payload, err := marshaler(data)
	if err != nil {
		return err
	}
	if w != nil {
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(payload)
	}
	return nil
}

func (c *DatlyComponentCodec) Unmarshal(r *http.Request, method, uri string, dst interface{}) error {
	if c == nil || c.dao == nil {
		return fmt.Errorf("codec not initialized")
	}
	unmarshal, _, err := c.dao.GetUnmarshaller(r, method+":"+uri)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	return unmarshal(body, dst)
}

func (c *DatlyComponentCodec) Operate(ctx context.Context, method, uri string, input interface{}, output interface{}, session ...SessionOption) error {
	if c == nil || c.dao == nil {
		return fmt.Errorf("codec not initialized")
	}
	opts := []datly.OperateOption{datly.WithURI(method + ":" + uri)}
	if input != nil {
		opts = append(opts, datly.WithInput(input))
	}
	if output != nil {
		opts = append(opts, datly.WithOutput(output))
	}
	if len(session) > 0 {
		opts = append(opts, datly.WithSessionOptions(session...))
	}
	_, err := c.dao.Operate(ctx, opts...)
	return err
}
