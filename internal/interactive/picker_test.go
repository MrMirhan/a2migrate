package interactive

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModel_Navigation(t *testing.T) {
	m := New([]Item{{ID: "a"}, {ID: "b"}, {ID: "c"}})
	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d", m.cursor)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.cursor != 1 {
		t.Fatalf("after down cursor = %d", m.cursor)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.cursor != 0 {
		t.Fatalf("after up cursor = %d", m.cursor)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.cursor != 2 {
		t.Fatalf("cursor = %d want 2", m.cursor)
	}
}

func TestModel_Toggle(t *testing.T) {
	m := New([]Item{{ID: "a"}, {ID: "b"}})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	if !m.selected[0] {
		t.Fatal("first toggle should select")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	if m.selected[0] {
		t.Fatal("second toggle should deselect")
	}
}

func TestModel_Confirm(t *testing.T) {
	m := New([]Item{{ID: "a"}, {ID: "b"}, {ID: "c"}})
	m.selected[0] = true
	m.selected[2] = true
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if len(m.choice) != 2 {
		t.Fatalf("choice = %d want 2", len(m.choice))
	}
	if m.choice[0].ID != "a" || m.choice[1].ID != "c" {
		t.Fatalf("choice ids = %v %v", m.choice[0].ID, m.choice[1].ID)
	}
}

func TestModel_SelectAllNone(t *testing.T) {
	m := New([]Item{{ID: "a"}, {ID: "b"}})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	if !m.selected[0] || !m.selected[1] {
		t.Fatal("a should select all")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(Model)
	if m.selected[0] || m.selected[1] {
		t.Fatal("n should deselect all")
	}
}

func TestModel_View(t *testing.T) {
	m := New([]Item{{Title: "alpha"}, {Title: "beta"}})
	v := m.View()
	if v == "" {
		t.Fatal("view empty")
	}
}