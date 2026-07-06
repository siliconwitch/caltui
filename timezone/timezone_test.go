package timezone

import (
	"testing"
	"time"
)

func TestSearch(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		wantFirst string
	}{
		{name: "city name", query: "stockholm", wantFirst: "Europe/Stockholm"},
		{name: "city with space", query: "los angeles", wantFirst: "America/Los_Angeles"},
		{name: "partial city", query: "kolk", wantFirst: "Asia/Kolkata"},
		{name: "region substring", query: "europe/st", wantFirst: "Europe/Stockholm"},
		{name: "country", query: "sweden", wantFirst: "Europe/Stockholm"},
		{name: "country without city zone", query: "japan", wantFirst: "Asia/Tokyo"},
		{name: "partial country", query: "vietn", wantFirst: "Asia/Ho_Chi_Minh"},
		{name: "utc", query: "utc", wantFirst: "UTC"},
		{name: "fuzzy city", query: "stkhlm", wantFirst: "Europe/Stockholm"},
		{name: "fuzzy country", query: "grmany", wantFirst: "Europe/Berlin"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			matches := Search(testCase.query)

			if len(matches) == 0 || matches[0].name != testCase.wantFirst {
				got := "none"
				if len(matches) > 0 {
					got = matches[0].name
				}

				t.Fatalf("first match for %q: got %s, want %s", testCase.query, got, testCase.wantFirst)
			}
		})
	}

	if len(Search("")) != len(zones) {
		t.Fatalf("empty query must return every zone")
	}
}

func TestMarker(t *testing.T) {
	stockholm, err := time.LoadLocation("Europe/Stockholm")

	if err != nil {
		t.Fatal(err)
	}

	losAngeles, err := time.LoadLocation("America/Los_Angeles")

	if err != nil {
		t.Fatal(err)
	}

	berlin, err := time.LoadLocation("Europe/Berlin")

	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		at   time.Time
		base *time.Location
		want string
	}{
		{
			name: "summer pacific against stockholm",
			at:   time.Date(2026, time.July, 6, 12, 41, 0, 0, losAngeles),
			base: stockholm,
			want: "(PDT)",
		},
		{
			name: "winter pacific against stockholm",
			at:   time.Date(2026, time.January, 6, 12, 41, 0, 0, losAngeles),
			base: stockholm,
			want: "(PST)",
		},
		{
			name: "same zone hides marker",
			at:   time.Date(2026, time.July, 6, 12, 41, 0, 0, stockholm),
			base: stockholm,
			want: "",
		},
		{
			name: "equal offset hides marker",
			at:   time.Date(2026, time.July, 6, 12, 41, 0, 0, berlin),
			base: stockholm,
			want: "",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := Marker(testCase.at, testCase.base); got != testCase.want {
				t.Fatalf("got %q, want %q", got, testCase.want)
			}
		})
	}
}
