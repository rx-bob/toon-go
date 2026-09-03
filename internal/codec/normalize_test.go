package codec

import "testing"

func TestNormalizeNumberStringPreservesLossyFloatConversions(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "large integer", in: "9007199254740993", want: "9007199254740993"},
		{name: "precise decimal", in: "1.0000000000000001", want: "1.0000000000000001"},
		{name: "extreme exponent", in: "1e1000", want: "1e1000"},
		{name: "negative zero", in: "-0", want: "0"},
		{name: "exact decimal", in: "1.0", want: "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeNumberString(tt.in)
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			number, ok := got.(numberValue)
			if !ok || number.literal != tt.want {
				t.Fatalf("normalized = %#v, want number %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeInvalidNumberStringAsString(t *testing.T) {
	got, err := normalizeNumberString("1.")
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if value, ok := got.(string); !ok || value != "1." {
		t.Fatalf("normalized = %#v, want string 1.", got)
	}
}
