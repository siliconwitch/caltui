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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			interval, err := Config{SyncInterval: c.syncInterval}.RefreshInterval()

			if c.wantErr {
				if err == nil {
					t.Fatal("want a parse error")
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if interval != c.want {
				t.Errorf("want %v, got %v", c.want, interval)
			}
		})
	}
}
