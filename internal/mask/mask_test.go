package mask

import "testing"

func TestDocument(t *testing.T) {
	cases := map[string]string{
		"52998224725":    "***.***.***-25",
		"529.982.247-25": "***.***.***-25",
		"11222333000181": "**.***.***/****-81",
		"1":              "***",
		"":               "***",
	}
	for in, want := range cases {
		if got := Document(in); got != want {
			t.Errorf("Document(%q) = %q, esperava %q", in, got, want)
		}
	}
}

func TestEmail(t *testing.T) {
	cases := map[string]string{
		"ana.souza@example.com": "a***@example.com",
		"x@y.io":                "x***@y.io",
		"semarroba":             "***",
		"@dominio.com":          "***",
	}
	for in, want := range cases {
		if got := Email(in); got != want {
			t.Errorf("Email(%q) = %q, esperava %q", in, got, want)
		}
	}
}
