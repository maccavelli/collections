package cmd

import "sync"

// InternalUIState governs the strict concurrent boundaries required
// for navigating transient arrays that continuously shift via ring buffer roll-overs.
type InternalUIState struct {
	mu           sync.Mutex
	ActiveTab    int32
	SpansFocus   bool            // false = Sidebar, true = MainContent
	SpansExpand  map[string]bool // which TraceIDs are manually expanded
	SpansSelect  string          // exactly which TraceID GUID is actively focused
	currentNodes []string        // linearized active frame output tracking coordinates
}

// NewUIState initializes the interactive dashboard components.
func NewUIState() *InternalUIState {
	return &InternalUIState{
		ActiveTab:   1,
		SpansExpand: make(map[string]bool),
	}
}

// UpdateNodes is called rigidly during the `renderSpans` traversal to physically
// export the 1-dimensional projection of the Spans tree. It dynamically resets
// the cursor to origin if a trace violently expires from the rolling ring buffer buffer.
func (s *InternalUIState) UpdateNodes(nodes []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentNodes = nodes

	found := false
	for _, n := range nodes {
		if n == s.SpansSelect {
			found = true
			break
		}
	}
	if !found && len(nodes) > 0 {
		s.SpansSelect = nodes[0]
	}
}

// MoveSpansCursor linearly navigates the TUI bounds securely.
func (s *InternalUIState) MoveSpansCursor(delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.currentNodes) == 0 {
		return
	}
	idx := 0
	for i, id := range s.currentNodes {
		if id == s.SpansSelect {
			idx = i
			break
		}
	}
	idx += delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(s.currentNodes) {
		idx = len(s.currentNodes) - 1
	}
	s.SpansSelect = s.currentNodes[idx]
}

// ToggleSpanExpand handles Enter/Space execution.
func (s *InternalUIState) ToggleSpanExpand() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SpansSelect != "" {
		s.SpansExpand[s.SpansSelect] = !s.SpansExpand[s.SpansSelect]
	}
}

// GetSnapshot retrieves deep copies to prevent lock interference during render blocking.
func (s *InternalUIState) GetSnapshot() (string, map[string]bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cpy := make(map[string]bool)
	for k, v := range s.SpansExpand {
		cpy[k] = v
	}
	return s.SpansSelect, cpy, s.SpansFocus
}

func (s *InternalUIState) SetFocus(f bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SpansFocus = f
}

func (s *InternalUIState) SetActiveTab(t int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ActiveTab = t
	if t != 9 {
		s.SpansFocus = false
	}
}

func (s *InternalUIState) GetActiveTab() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ActiveTab
}
