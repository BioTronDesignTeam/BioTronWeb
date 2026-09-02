package server

import "testing"

func TestRequiresRequestHeader(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{method: "GET", want: false},
		{method: "HEAD", want: false},
		{method: "OPTIONS", want: false},
		{method: "POST", want: true},
		{method: "PATCH", want: true},
		{method: "PUT", want: true},
		{method: "DELETE", want: true},
	}
	for _, test := range tests {
		if got := requiresRequestHeader(test.method); got != test.want {
			t.Fatalf("requiresRequestHeader(%q) = %v, want %v", test.method, got, test.want)
		}
	}
}
