package crawl

import (
	"errors"
	"fmt"
)

var errDisallowed = errors.New("path disallowed by robots.txt")

type httpError struct{ code int }

func (e *httpError) Error() string { return fmt.Sprintf("http status %d", e.code) }
