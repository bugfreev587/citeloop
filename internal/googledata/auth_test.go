package googledata

import (
	"context"
	"testing"
)

func TestNewServiceAccountClientRejectsEmptyCredentials(t *testing.T) {
	_, err := NewServiceAccountClient(context.Background(), "")
	if err == nil {
		t.Fatal("expected empty credentials to fail")
	}
}
