package engine

import (
	"context"
	"net/http"
	"net/url"
)

type Request struct {
	Method, Path, Token, Namespace string
	Query                          url.Values
	Body                           []byte
}
type Response struct {
	Status    int
	Header    http.Header
	Body      []byte
	Attempts  int
	RequestID string
}

func (r *Response) Zero() {
	if r != nil {
		clear(r.Body)
		r.Body = nil
	}
}

type Sender interface {
	Send(context.Context, Request) (*Response, error)
	CloseIdleConnections()
}
