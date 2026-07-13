package calendar

import (
	"testing"
	"time"
)

func TestRefreshInterval(t *testing.T) {
	cases := []struct {
		name         string
		syncInterval string
		want         time.Duration
		wantErr      bool
	}{
		{name: "default fifteen minutes", syncInterval: "15m", want: 15 * time.Minute},
		{name: "hours parse", syncInterval: "1h", want: time.Hour},
		{name: "zero disables", syncInterval: "0", want: 0},
		{name: "empty disables", syncInterval: "", want: 0},
		{name: "junk is reported", syncInterval: "often", wantErr: true},
		{name: "negative is refused", syncInterval: "-15m", wantErr: true},
		{name: "sub-minute is refused", syncInterval: "30s", wantErr: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			interval, err := Config{SyncInterval: testCase.syncInterval}.RefreshInterval()

			if testCase.wantErr {
				if err == nil {
					t.Fatal("want a parse error")
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if interval != testCase.want {
				t.Errorf("want %v, got %v", testCase.want, interval)
			}
		})
	}
}
