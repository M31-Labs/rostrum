package present

import "testing"

func TestAllowedResourceEmbed(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "privacy enhanced youtube", value: "https://www.youtube-nocookie.com/embed/example", want: true},
		{name: "vimeo player", value: "https://player.vimeo.com/video/123", want: true},
		{name: "plain http", value: "http://www.youtube-nocookie.com/embed/example", want: false},
		{name: "lookalike host", value: "https://www.youtube-nocookie.com.example.test/embed/example", want: false},
		{name: "javascript", value: "javascript:alert(1)", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := allowedResourceEmbed(test.value); got != test.want {
				t.Fatalf("allowedResourceEmbed(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}
