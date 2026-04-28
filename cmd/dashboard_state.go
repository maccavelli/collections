package cmd

import "sync"

type InternalUIState struct {
	mu        sync.Mutex
	ActiveTab int32
}

func NewUIState() *InternalUIState {
	return &InternalUIState{
		ActiveTab: 1,
	}
}

func (s *InternalUIState) SetActiveTab(t int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ActiveTab = t
}

func (s *InternalUIState) GetActiveTab() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ActiveTab
}
