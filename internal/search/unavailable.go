package search

import (
	"context"
	"fmt"
)

type Unavailable struct{ Reason string }

func NewUnavailable(reason string) *Unavailable { return &Unavailable{Reason: reason} }
func (u *Unavailable) ProviderName() string     { return "search_unavailable" }
func (u *Unavailable) Synthetic() bool          { return false }
func (u *Unavailable) Search(context.Context, Query) ([]Result, error) {
	reason := u.Reason
	if reason == "" {
		reason = "search provider unavailable"
	}
	return nil, fmt.Errorf("%s", reason)
}
