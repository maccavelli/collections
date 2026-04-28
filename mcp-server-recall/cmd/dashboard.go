package cmd

import (
	"context"
	"os"
	"time"

	"atomicgo.dev/keyboard"
	"atomicgo.dev/keyboard/keys"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var dashCmd = &cobra.Command{
	Use:   "dash",
	Short: "Launch the observability dashboard",
	Run: func(cmd *cobra.Command, args []string) {
		runInteractiveDashboard()
	},
}

func init() {
	RootCmd.AddCommand(dashCmd)
}

func runInteractiveDashboard() {
	pterm.Info.Println("Initializing Recall Dashboard Navigation...")
	time.Sleep(1 * time.Second)

	// In pterm we just use DefaultArea to take over rendering
	area, _ := pterm.DefaultArea.WithFullscreen().Start()
	defer area.Stop()

	uiState := NewUIState()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	forceRender := make(chan struct{}, 1)
	triggerRender := func() {
		go func() {
			select {
			case forceRender <- struct{}{}:
			default:
			}
		}()
	}

	go func() {
		_ = keyboard.Listen(func(key keys.Key) (stop bool, err error) {
			if key.Code == keys.CtrlC || key.Code == keys.Escape || key.String() == "q" {
				cancel()
				return true, nil
			}

			switch key.Code {
			case keys.Enter:
				if uiState.GetActiveTab() == int32(len(DashboardTabs)) {
					cancel()
					return true, nil
				}
			case keys.Up:
				current := uiState.GetActiveTab()
				if current > 1 {
					uiState.SetActiveTab(current - 1)
					triggerRender()
				}
			case keys.Down:
				current := uiState.GetActiveTab()
				if current < int32(len(DashboardTabs)) {
					uiState.SetActiveTab(current + 1)
					triggerRender()
				}
			}
			return false, nil
		})
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			pterm.Success.Println("\nDashboard exited gracefully.")
			os.Exit(0)
			return
		case <-ticker.C:
			snapshot, logs, err := ReadDashboardSnapshot()
			if err != nil {
				area.Update(pterm.Error.Sprintf("Failed to read observability state: %v\n", err))
			} else {
				area.Update(renderPtermDashboard(snapshot, logs, uiState))
			}
		case <-forceRender:
			snapshot, logs, err := ReadDashboardSnapshot()
			if err != nil {
				area.Update(pterm.Error.Sprintf("Failed to read observability state: %v\n", err))
			} else {
				area.Update(renderPtermDashboard(snapshot, logs, uiState))
			}
		}
	}
}
