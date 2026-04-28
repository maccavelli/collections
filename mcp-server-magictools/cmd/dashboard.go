package cmd

import (
	"atomicgo.dev/keyboard"
	"atomicgo.dev/keyboard/keys"
	"context"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"os"
	"time"
)

var dashCmd = &cobra.Command{
	Use:   "dash",
	Short: "Launch the observability dashboard",
	Run: func(cmd *cobra.Command, args []string) {
		findQuery, _ := cmd.Flags().GetString("find")
		if findQuery != "" {
			runDashboardSearch(findQuery)
			return
		}
		runInteractiveDashboard()
	},
}

func init() {
	dashCmd.Flags().String("find", "", "Search historical telemetry using BadgerDB/Bleve")
	rootCmd.AddCommand(dashCmd)
}

func runDashboardSearch(query string) {
	pterm.Info.Printf("Searching historical logs for: %s\n", query)
	pterm.Success.Println("Search capability initialized.")
}

func runInteractiveDashboard() {
	pterm.Info.Println("Initializing MagicTools Dashboard Navigation...")
	time.Sleep(1 * time.Second)

	enableVirtualTerminalProcessing()
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

			_, _, spansFocus := uiState.GetSnapshot()

			if spansFocus {
				switch key.Code {
				case keys.Left:
					uiState.SetFocus(false)
					triggerRender()
				case keys.Up:
					uiState.MoveSpansCursor(-1)
					triggerRender()
				case keys.Down:
					uiState.MoveSpansCursor(1)
					triggerRender()
				case keys.Enter, keys.Space:
					uiState.ToggleSpanExpand()
					triggerRender()
				}
			} else {
				switch key.Code {
				case keys.Right, keys.Enter:
					if uiState.GetActiveTab() == 4 {
						uiState.SetFocus(true)
						triggerRender()
					} else if key.Code == keys.Enter && uiState.GetActiveTab() == int32(len(DashboardTabs)) {
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
			}
			return false, nil
		})
	}()

	ticker := time.NewTicker(10 * time.Second) // Real-time 10s refresh
	defer ticker.Stop()

	// Keep updating the area
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
			} else if len(snapshot) == 0 {
				area.Update(pterm.Warning.Sprint("Waiting for orchestrator telemetry data...\n"))
			} else {
				area.Update(renderPtermDashboard(snapshot, logs, uiState))
			}
		case <-forceRender:
			snapshot, logs, err := ReadDashboardSnapshot()
			if err != nil {
				area.Update(pterm.Error.Sprintf("Failed to read observability state: %v\n", err))
			} else if len(snapshot) == 0 {
				area.Update(pterm.Warning.Sprint("Waiting for orchestrator telemetry data...\n"))
			} else {
				area.Update(renderPtermDashboard(snapshot, logs, uiState))
			}
		}
	}
}
