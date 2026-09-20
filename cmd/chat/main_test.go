package main

import "testing"

// TestDefaultAddr covers the two ways a host names the port.
func TestDefaultAddr(t *testing.T) {
	tests := []struct {
		name string
		port string
		want string
	}{
		{name: "no environment variable", port: "", want: ":8080"},
		{name: "the host sets PORT", port: "3000", want: ":3000"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("PORT", test.port)

			if got := defaultAddr(); got != test.want {
				t.Errorf("defaultAddr() = %q, want %q", got, test.want)
			}
		})
	}
}
