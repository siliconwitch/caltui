package errorpopup

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/siliconwitch/caltui/msgs"
)

func TestErrorPopup(t *testing.T) {
	cases := []struct {
		name         string
		messages     []tea.Msg
		yank         bool
		wantPending  int
		wantInView   []string
		wantClosed   bool
		dismissTimes int
	}{
		{
			name:        "sync failure is queued with its account name",
			messages:    []tea.Msg{msgs.SyncedMsg{Account: "zoho", Err: errors.New("finding principal: 501 Not Implemented")}},
			wantPending: 1,
			wantInView:  []string{"zoho", "501 Not Implemented", "y yank", "dismiss"},
		},
		{
			name:        "successful sync queues nothing",
			messages:    []tea.Msg{msgs.SyncedMsg{Account: "zoho"}},
			wantPending: 0,
		},
		{
			name: "multiple errors show a counter",
			messages: []tea.Msg{
				msgs.SyncedMsg{Account: "zoho", Err: errors.New("first")},
				msgs.StoreErrorMsg{Err: errors.New("second")},
			},
			wantPending: 2,
			wantInView:  []string{"Error (1 of 2)", "first"},
		},
		{
			name: "dismissing advances to the next error",
			messages: []tea.Msg{
				msgs.SyncedMsg{Account: "zoho", Err: errors.New("first")},
				msgs.StoreErrorMsg{Err: errors.New("second")},
			},
			dismissTimes: 1,
			wantPending:  1,
			wantInView:   []string{"second"},
		},
		{
			name:         "dismissing the last error closes the popup",
			messages:     []tea.Msg{msgs.StoreErrorMsg{Err: errors.New("only")}},
			dismissTimes: 1,
			wantPending:  0,
			wantClosed:   true,
		},
		{
			name:        "yank keeps the error and confirms the copy",
			messages:    []tea.Msg{msgs.StoreErrorMsg{Err: errors.New("long error")}},
			yank:        true,
			wantPending: 1,
			wantInView:  []string{"long error", "copied to clipboard"},
		},
		{
			name: "dismissing after a yank clears the confirmation",
			messages: []tea.Msg{
				msgs.StoreErrorMsg{Err: errors.New("first")},
				msgs.StoreErrorMsg{Err: errors.New("second")},
			},
			yank:         true,
			dismissTimes: 1,
			wantPending:  1,
			wantInView:   []string{"second", "y yank"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var model tea.Model = Model{}

			model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

			for _, message := range testCase.messages {
				model, _ = model.Update(message)
			}

			if testCase.yank {
				model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
			}

			closed := false
			for range testCase.dismissTimes {
				var cmd tea.Cmd

				model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

				if cmd != nil {
					if _, ok := cmd().(msgs.ClosePopupMsg); ok {
						closed = true
					}
				}
			}

			if closed != testCase.wantClosed {
				t.Fatalf("want closed %v, got %v", testCase.wantClosed, closed)
			}

			pending := model.(Model).Pending()
			if pending != testCase.wantPending {
				t.Fatalf("want %d pending, got %d", testCase.wantPending, pending)
			}

			view := model.(Model).View()
			for _, want := range testCase.wantInView {
				if !strings.Contains(view, want) {
					t.Errorf("view does not contain %q:\n%s", want, view)
				}
			}
		})
	}
}
