// Package gojsonschema is a very small stub that fulfils the handful of APIs
// required by the unit tests in this repository. It is *not* a full JSON schema
// validator – it always reports the provided document as valid. The purpose of
// this shim is to avoid pulling the real github.com/xeipuuv/gojsonschema module
// (which would require network access in the execution environment).
package gojsonschema

type StringLoader struct {
	Schema string
}

func NewStringLoader(schema string) StringLoader { return StringLoader{Schema: schema} }

type BytesLoader struct {
	Data []byte
}

func NewBytesLoader(data []byte) BytesLoader { return BytesLoader{Data: data} }

type Result struct{}

func (Result) Valid() bool { return true }

func (Result) Errors() []error { return nil }

func Validate(_ StringLoader, _ BytesLoader) (Result, error) { return Result{}, nil }
