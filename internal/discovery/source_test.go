package discovery

import "testing"

func TestNormalizeSource(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "canonical refresh", input: "refresh", want: SourceRefresh},
		{name: "legacy refresh alias", input: "refresh-peer", want: SourceRefresh},
		{name: "legacy import alias", input: "known-import", want: SourceImport},
		{name: "legacy cache alias", input: "stale-cache", want: SourceCache},
		{name: "empty", input: "", want: SourceUnknown},
		{name: "none", input: "none", want: SourceUnknown},
		{name: "unknown source", input: "experimental-source", want: SourceUnknown},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeSource(tc.input); got != tc.want {
				t.Fatalf("NormalizeSource(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestIsSupportedSourceFilter(t *testing.T) {
	t.Parallel()

	if !IsSupportedSourceFilter("refresh-peer") {
		t.Fatal("IsSupportedSourceFilter(refresh-peer) = false, want true")
	}
	if !IsSupportedSourceFilter("cache") {
		t.Fatal("IsSupportedSourceFilter(cache) = false, want true")
	}
	if IsSupportedSourceFilter("future-source") {
		t.Fatal("IsSupportedSourceFilter(future-source) = true, want false")
	}
}
