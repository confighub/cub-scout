package agent

import (
	"testing"
)

// TestParseViewRef covers the council-required URL-as-positional contract.
// Each case maps to an input shape downstream commands will need to
// handle: bare UUIDs, View Explorer URLs (with and without extras),
// invalid inputs that should return clear errors.
func TestParseViewRef(t *testing.T) {
	const (
		validUUID    = "806aac53-236c-446d-8ad6-91d6daf6810e"
		validURL     = "https://hub.confighub.com/x/view-explorer?view=806aac53-236c-446d-8ad6-91d6daf6810e"
		validURLWithGroup = "https://hub.confighub.com/x/view-explorer?view=806aac53-236c-446d-8ad6-91d6daf6810e&group=prod"
	)

	t.Run("bare UUID", func(t *testing.T) {
		ref, err := ParseViewRef(validUUID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ref.UUID != validUUID {
			t.Errorf("UUID = %q, want %q", ref.UUID, validUUID)
		}
		if ref.SourceForm != ViewRefUUID {
			t.Errorf("SourceForm = %q, want %q", ref.SourceForm, ViewRefUUID)
		}
		if ref.OriginalURL != "" {
			t.Errorf("OriginalURL = %q, want empty for UUID input", ref.OriginalURL)
		}
		if len(ref.Extras) != 0 {
			t.Errorf("Extras non-empty for UUID input: %v", ref.Extras)
		}
	})

	t.Run("View Explorer URL", func(t *testing.T) {
		ref, err := ParseViewRef(validURL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ref.UUID != validUUID {
			t.Errorf("UUID = %q, want %q", ref.UUID, validUUID)
		}
		if ref.SourceForm != ViewRefURL {
			t.Errorf("SourceForm = %q, want %q", ref.SourceForm, ViewRefURL)
		}
		if ref.OriginalURL != validURL {
			t.Errorf("OriginalURL = %q, want %q", ref.OriginalURL, validURL)
		}
	})

	t.Run("View Explorer URL with extra params", func(t *testing.T) {
		ref, err := ParseViewRef(validURLWithGroup)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := ref.Extras.Get("group"); got != "prod" {
			t.Errorf("Extras[group] = %q, want %q", got, "prod")
		}
		if ref.Extras.Get("view") != "" {
			t.Errorf("Extras retained the view param: %v", ref.Extras)
		}
	})

	t.Run("uppercase UUID is normalized to lowercase", func(t *testing.T) {
		upper := "806AAC53-236C-446D-8AD6-91D6DAF6810E"
		ref, err := ParseViewRef(upper)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ref.UUID != "806aac53-236c-446d-8ad6-91d6daf6810e" {
			t.Errorf("UUID = %q, want lowercase normalized", ref.UUID)
		}
	})

	t.Run("on-prem ConfigHub URL accepted (no host pin)", func(t *testing.T) {
		// Council framing: the parser does not pin hub.confighub.com so
		// users running against on-prem ConfigHub aren't excluded.
		onprem := "https://confighub.example.internal/x/view-explorer?view=" + validUUID
		ref, err := ParseViewRef(onprem)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ref.UUID != validUUID {
			t.Errorf("UUID = %q, want %q", ref.UUID, validUUID)
		}
	})

	errorCases := []struct {
		name  string
		input string
		want  string // substring of the error message
	}{
		{"empty input", "", "empty"},
		{"random non-URL non-UUID", "not a uuid or url", "non-URL"},
		{"URL without view-explorer path", "https://hub.confighub.com/units/" + validUUID, "view-explorer"},
		{"URL missing ?view= param", "https://hub.confighub.com/x/view-explorer", "view=<uuid>"},
		{"URL with malformed UUID", "https://hub.confighub.com/x/view-explorer?view=not-a-uuid", "valid UUID"},
		{"unparseable URL", "https://[::1:invalid/x/view-explorer?view=" + validUUID, "parseable URL"},
	}

	for _, tc := range errorCases {
		t.Run("error: "+tc.name, func(t *testing.T) {
			_, err := ParseViewRef(tc.input)
			if err == nil {
				t.Fatalf("expected error for input %q, got nil", tc.input)
			}
			if tc.want != "" && !viewRefContainsSubstring(err.Error(), tc.want) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

// viewRefContainsSubstring is a local helper. Named with the file's
// prefix to avoid colliding with sibling test files that already define
// `contains` and `containsSubstring`.
func viewRefContainsSubstring(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
